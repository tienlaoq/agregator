package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"github.com/tienlao/agregator/services/api-gateway/internal/realtime/chatws"
	"google.golang.org/grpc"
)

type ChatHandler struct {
	client       chatGatewayClient
	booking      bookingGatewayClient
	venue        venueGatewayClient
	master       masterGatewayClient
	users        userGatewayClient
	limiter      chatMessageLimiter
	ticketRedis  *redis.Client
	natsConn     *nats.Conn
	upgrader     websocket.Upgrader
	hub          *chatws.Hub
}

type userGatewayClient interface {
	GetUser(ctx context.Context, in *userv1.GetUserRequest, opts ...grpc.CallOption) (*userv1.UserResponse, error)
}

type chatGatewayClient interface {
	EnsureThread(ctx context.Context, in *chatv1.EnsureThreadRequest, opts ...grpc.CallOption) (*chatv1.ThreadResponse, error)
	ListThreads(ctx context.Context, in *chatv1.ListThreadsRequest, opts ...grpc.CallOption) (*chatv1.ListThreadsResponse, error)
	ListMessages(ctx context.Context, in *chatv1.ListMessagesRequest, opts ...grpc.CallOption) (*chatv1.ListMessagesResponse, error)
	SendMessage(ctx context.Context, in *chatv1.SendMessageRequest, opts ...grpc.CallOption) (*chatv1.MessageResponse, error)
	MarkRead(ctx context.Context, in *chatv1.MarkReadRequest, opts ...grpc.CallOption) (*chatv1.ThreadResponse, error)
}

type bookingGatewayClient interface {
	GetBooking(ctx context.Context, in *bookingv1.GetBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error)
}

type venueGatewayClient interface {
	GetVenue(ctx context.Context, in *venuev1.GetVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	GetVenueManagementAccess(ctx context.Context, in *venuev1.GetVenueManagementAccessRequest, opts ...grpc.CallOption) (*venuev1.GetVenueManagementAccessResponse, error)
	ListVenueStaff(ctx context.Context, in *venuev1.ListVenueStaffRequest, opts ...grpc.CallOption) (*venuev1.ListVenueStaffResponse, error)
}

type masterGatewayClient interface {
	GetMasterBooking(ctx context.Context, in *masterv1.GetMasterBookingRequest, opts ...grpc.CallOption) (*masterv1.MasterBookingResponse, error)
}

type chatMessageLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

type inMemoryChatLimiter struct{ byK sync.Map }

func (l *inMemoryChatLimiter) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, error) {
	now := time.Now()
	cutoff := now.Add(-window)
	var prev []time.Time
	if v, ok := l.byK.Load(key); ok {
		prev, _ = v.([]time.Time)
	}
	kept := prev[:0]
	for _, ts := range prev {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= limit {
		l.byK.Store(key, kept)
		return false, nil
	}
	kept = append(kept, now)
	l.byK.Store(key, kept)
	return true, nil
}

const (
	sendRateWindow = time.Minute
	sendRateMax    = 20
)

const wsTicketTTL = 90 * time.Second

func NewChatHandler(
	client chatGatewayClient,
	booking bookingGatewayClient,
	venue venueGatewayClient,
	master masterGatewayClient,
	users userGatewayClient,
	limiter chatMessageLimiter,
	ticketRedis *redis.Client,
	natsConn *nats.Conn,
) *ChatHandler {
	if limiter == nil {
		limiter = &inMemoryChatLimiter{}
	}
	return &ChatHandler{
		client:      client,
		booking:     booking,
		venue:       venue,
		master:      master,
		users:       users,
		limiter:     limiter,
		ticketRedis: ticketRedis,
		natsConn:    natsConn,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := strings.TrimSpace(r.Header.Get("Origin"))
				if origin == "" {
					return true
				}
				originURL, err := url.Parse(origin)
				if err != nil || originURL.Host == "" {
					return false
				}
				allowed := middleware.CORSAllowedOrigins()
				for _, a := range allowed {
					u, err := url.Parse(a)
					if err == nil && strings.EqualFold(u.Host, originURL.Host) {
						return true
					}
				}
				// Локальная разработка: любой порт localhost / 127.0.0.1 (::1 включён — см. middleware).
				if middleware.IsLocalLoopbackOrigin(origin) {
					return true
				}
				return false
			},
		},
		hub: chatws.NewHub(),
	}
}

func chatThreadToJSON(t *chatv1.ChatThread) map[string]any {
	if t == nil {
		return nil
	}
	out := map[string]any{
		"id":                   t.GetId(),
		"kind":                 t.GetKind(),
		"ref_id":               t.GetRefId(),
		"participant_user_ids": t.GetParticipantUserIds(),
		"last_message_id":      t.GetLastMessageId(),
		"unread_count":         t.GetUnreadCount(),
	}
	if t.GetLastMessageAt() != nil {
		out["last_message_at"] = t.GetLastMessageAt().AsTime().Format(time.RFC3339)
	}
	reads := make([]map[string]any, 0, len(t.GetParticipantReads()))
	for _, pr := range t.GetParticipantReads() {
		item := map[string]any{"user_id": strings.TrimSpace(pr.GetUserId())}
		if id := strings.TrimSpace(pr.GetLastReadMessageId()); id != "" {
			item["last_read_message_id"] = strings.ToLower(id)
		}
		reads = append(reads, item)
	}
	out["participant_reads"] = reads
	return out
}

func chatMessageToJSON(m *chatv1.ChatMessage) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{
		"id":             m.GetId(),
		"thread_id":      m.GetThreadId(),
		"author_user_id": m.GetAuthorUserId(),
		"text":           m.GetText(),
	}
	if m.GetCreatedAt() != nil {
		out["created_at"] = m.GetCreatedAt().AsTime().Format(time.RFC3339)
	}
	return out
}

func uniqueNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		s = strings.ToLower(s)
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func sameUserID(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func (h *ChatHandler) allowSend(ctx context.Context, userID, threadID string) bool {
	key := userID + "|" + threadID
	ok, err := h.limiter.Allow(ctx, "chat:send:"+key, sendRateMax, sendRateWindow)
	if err != nil {
		return true
	}
	return ok
}

func (h *ChatHandler) emitToUsers(userIDs []string, payload map[string]any) {
	if h.natsConn != nil && len(userIDs) > 0 {
		b, err := json.Marshal(struct {
			UserIDs []string       `json:"user_ids"`
			Payload map[string]any `json:"payload"`
		}{UserIDs: userIDs, Payload: payload})
		if err == nil {
			_ = h.natsConn.Publish("chat.fanout", b)
			return
		}
	}
	h.hub.Broadcast(userIDs, payload)
}

// HandleFanoutMessage applies a NATS fan-out payload to the local WebSocket hub (all gateway replicas).
func (h *ChatHandler) HandleFanoutMessage(data []byte) {
	var envelope struct {
		UserIDs []string       `json:"user_ids"`
		Payload map[string]any `json:"payload"`
	}
	if json.Unmarshal(data, &envelope) != nil || len(envelope.UserIDs) == 0 || envelope.Payload == nil {
		return
	}
	h.hub.Broadcast(envelope.UserIDs, envelope.Payload)
}

func (h *ChatHandler) IssueWSTicket(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	if h.ticketRedis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "ws_ticket_unavailable",
		})
		return
	}
	id := uuid.NewString()
	payload, err := json.Marshal(map[string]string{
		"user_id": userID,
		"role":    middleware.RoleFromCtx(r.Context()),
		"email":   middleware.EmailFromCtx(r.Context()),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "marshal"})
		return
	}
	key := "chat:wst:" + id
	if err := h.ticketRedis.Set(r.Context(), key, payload, wsTicketTTL).Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "redis"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":          id,
		"expires_in_sec": int(wsTicketTTL.Seconds()),
	})
}

func parsePositiveInt(raw string, def, max int32) int32 {
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 32)
	if err != nil || v <= 0 {
		return def
	}
	if int32(v) > max {
		return max
	}
	return int32(v)
}

func (h *ChatHandler) ensureVenueBookingThread(r *http.Request, userID, refID string) (*chatv1.ChatThread, error) {
	b, err := h.booking.GetBooking(r.Context(), &bookingv1.GetBookingRequest{Id: refID})
	if err != nil {
		return nil, err
	}
	venueID := b.GetVenueId()
	clientID := b.GetUserId()
	allowed := sameUserID(clientID, userID)
	participants := []string{clientID}
	var ownerID string
	if venueID != "" {
		v, vErr := h.venue.GetVenue(r.Context(), &venuev1.GetVenueRequest{Id: venueID})
		if vErr == nil {
			ownerID = v.GetOwnerId()
			if sameUserID(ownerID, userID) {
				allowed = true
			}
		}
		acc, aErr := h.venue.GetVenueManagementAccess(r.Context(), &venuev1.GetVenueManagementAccessRequest{
			VenueId: venueID,
			UserId:  userID,
		})
		if aErr == nil && strings.TrimSpace(acc.GetAccess()) != "" {
			allowed = true
		}
		staffResp, sErr := h.venue.ListVenueStaff(r.Context(), &venuev1.ListVenueStaffRequest{
			VenueId: venueID,
			ActorId: userID,
		})
		if sErr == nil {
			for _, m := range staffResp.GetMembers() {
				uid := strings.TrimSpace(m.GetUserId())
				if uid == "" || sameUserID(uid, clientID) || sameUserID(uid, ownerID) {
					continue
				}
				participants = append(participants, uid)
			}
		}
	}
	if !allowed {
		return nil, pkgerrors.PermissionDenied("chat access denied")
	}
	if ownerID != "" && !sameUserID(ownerID, clientID) {
		participants = append(participants, ownerID)
	}
	participants = uniqueNonEmpty(participants)
	if len(participants) < 2 {
		participants = append(participants, userID)
	}
	resp, err := h.client.EnsureThread(r.Context(), &chatv1.EnsureThreadRequest{
		Kind:               "venue_booking",
		RefId:              refID,
		ParticipantUserIds: participants,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetThread(), nil
}

func (h *ChatHandler) ensureMasterBookingThread(r *http.Request, userID, refID string) (*chatv1.ChatThread, error) {
	b, err := h.master.GetMasterBooking(r.Context(), &masterv1.GetMasterBookingRequest{
		BookingId:   refID,
		ActorUserId: userID,
	})
	if err != nil {
		return nil, err
	}
	bk := b.GetBooking()
	if bk == nil {
		return nil, pkgerrors.PermissionDenied("chat access denied")
	}
	clientID := strings.TrimSpace(bk.GetClientUserId())
	masterUserID := strings.TrimSpace(bk.GetMasterUserId())
	if clientID == "" || masterUserID == "" {
		return nil, pkgerrors.PermissionDenied("chat access denied")
	}
	participants := uniqueNonEmpty([]string{clientID, masterUserID})
	if len(participants) < 2 {
		return nil, pkgerrors.PermissionDenied("chat access denied")
	}
	resp, err := h.client.EnsureThread(r.Context(), &chatv1.EnsureThreadRequest{
		Kind:               "master_booking",
		RefId:              refID,
		ParticipantUserIds: participants,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetThread(), nil
}

func (h *ChatHandler) EnsureThread(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	var body struct {
		Kind  string `json:"kind"`
		RefID string `json:"ref_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeCatalog(w, apicatalog.GatewayRequestInvalidJson)
		return
	}
	var (
		t   *chatv1.ChatThread
		err error
	)
	switch strings.TrimSpace(body.Kind) {
	case "venue_booking":
		t, err = h.ensureVenueBookingThread(r, userID, strings.TrimSpace(body.RefID))
	case "master_booking":
		t, err = h.ensureMasterBookingThread(r, userID, strings.TrimSpace(body.RefID))
	default:
		writeCatalog(w, apicatalog.GatewayRequestInvalidBody.WithMessage("unsupported thread kind"))
		return
	}
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	item := chatThreadToJSON(t)
	attachPeerDisplayToThreadJSON(item, t.GetId(), h.peerDisplayNamesBatch(r.Context(), userID, []*chatv1.ChatThread{t}))
	writeJSON(w, http.StatusOK, map[string]any{"thread": item})
}

func (h *ChatHandler) ListThreads(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 100, 200)
	offset := parsePositiveInt(r.URL.Query().Get("offset"), 0, 1000000)
	resp, err := h.client.ListThreads(r.Context(), &chatv1.ListThreadsRequest{
		UserId: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	rawThreads := resp.GetThreads()
	peerByThread := h.peerDisplayNamesBatch(r.Context(), userID, rawThreads)
	threads := make([]map[string]any, 0, len(rawThreads))
	for _, t := range rawThreads {
		item := chatThreadToJSON(t)
		attachPeerDisplayToThreadJSON(item, t.GetId(), peerByThread)
		threads = append(threads, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"threads": threads, "total": resp.GetTotal()})
}

func (h *ChatHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 200, 500)
	offset := parsePositiveInt(r.URL.Query().Get("offset"), 0, 1000000)
	threadID := chi.URLParam(r, "threadId")
	resp, err := h.client.ListMessages(r.Context(), &chatv1.ListMessagesRequest{
		ThreadId: threadID,
		UserId:   userID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	messages := make([]map[string]any, 0, len(resp.GetMessages()))
	for _, m := range resp.GetMessages() {
		messages = append(messages, chatMessageToJSON(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": messages, "total": resp.GetTotal()})
}

func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	threadID := chi.URLParam(r, "threadId")
	if !h.allowSend(r.Context(), userID, threadID) {
		writeCatalog(w, apicatalog.GatewayRequestRateLimited.WithMessage("chat send rate limit exceeded"))
		return
	}
	var body struct {
		Text        string `json:"text"`
		ClientMsgID string `json:"client_msg_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeCatalog(w, apicatalog.GatewayRequestInvalidJson)
		return
	}
	resp, err := h.client.SendMessage(r.Context(), &chatv1.SendMessageRequest{
		ThreadId:    threadID,
		UserId:      userID,
		Text:        body.Text,
		ClientMsgId: strings.TrimSpace(body.ClientMsgID),
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	payload := map[string]any{
		"type":    "message_new",          // v1
		"event":   "chat.message.created", // v2
		"thread":  chatThreadToJSON(resp.GetThread()),
		"message": chatMessageToJSON(resp.GetMessage()),
	}
	h.emitToUsers(resp.GetThread().GetParticipantUserIds(), payload)
	writeJSON(w, http.StatusCreated, map[string]any{
		"thread":  chatThreadToJSON(resp.GetThread()),
		"message": chatMessageToJSON(resp.GetMessage()),
	})
}

func (h *ChatHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	threadID := chi.URLParam(r, "threadId")
	resp, err := h.client.MarkRead(r.Context(), &chatv1.MarkReadRequest{
		ThreadId: threadID,
		UserId:   userID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	payload := map[string]any{
		"type":   "read_updated",             // v1
		"event":  "chat.thread.read_updated", // v2
		"thread": chatThreadToJSON(resp.GetThread()),
	}
	h.emitToUsers(resp.GetThread().GetParticipantUserIds(), payload)
	writeJSON(w, http.StatusOK, map[string]any{"thread": chatThreadToJSON(resp.GetThread())})
}

func (h *ChatHandler) WS(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	rawConn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer rawConn.Close()
	c := h.hub.Add(userID, rawConn)
	defer h.hub.Remove(userID, c)

	_ = c.SendJSON(map[string]any{"type": "connected", "event": "chat.connected"})

	for {
		var in struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
			Text     string `json:"text"`
		}
		if err := rawConn.ReadJSON(&in); err != nil {
			return
		}
		switch in.Type {
		case "ping":
			_ = c.SendJSON(map[string]any{"type": "pong", "event": "chat.pong"})
		case "send_message":
			if !h.allowSend(r.Context(), userID, in.ThreadID) {
				_ = c.SendJSON(map[string]any{"type": "error", "event": "chat.error", "error": "rate_limit"})
				continue
			}
			resp, err := h.client.SendMessage(r.Context(), &chatv1.SendMessageRequest{
				ThreadId: in.ThreadID,
				UserId:   userID,
				Text:     in.Text,
			})
			if err != nil {
				_ = c.SendJSON(map[string]any{"type": "error", "event": "chat.error", "error": "send_failed"})
				continue
			}
			h.emitToUsers(resp.GetThread().GetParticipantUserIds(), map[string]any{
				"type":    "message_new",
				"event":   "chat.message.created",
				"thread":  chatThreadToJSON(resp.GetThread()),
				"message": chatMessageToJSON(resp.GetMessage()),
			})
		case "mark_read":
			resp, err := h.client.MarkRead(r.Context(), &chatv1.MarkReadRequest{
				ThreadId: in.ThreadID,
				UserId:   userID,
			})
			if err != nil {
				_ = c.SendJSON(map[string]any{"type": "error", "event": "chat.error", "error": "mark_read_failed"})
				continue
			}
			h.emitToUsers(resp.GetThread().GetParticipantUserIds(), map[string]any{
				"type":   "read_updated",
				"event":  "chat.thread.read_updated",
				"thread": chatThreadToJSON(resp.GetThread()),
			})
		default:
			_ = c.SendJSON(map[string]any{"type": "error", "event": "chat.error", "error": "unknown_command"})
		}
	}
}
