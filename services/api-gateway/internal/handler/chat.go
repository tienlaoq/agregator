package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/limits"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"github.com/tienlao/agregator/services/api-gateway/internal/ratelimit"
	"github.com/tienlao/agregator/services/api-gateway/internal/realtime/chatws"
	"google.golang.org/grpc"
)

type ChatHandler struct {
	log         zerolog.Logger
	client      chatGatewayClient
	users       userGatewayClient
	limiter     chatMessageLimiter
	ticketRedis *redis.Client
	natsConn    *nats.Conn
	upgrader    websocket.Upgrader
	hub         *chatws.Hub
	// resolver owns the "who participates in this thread?" business logic
	// (P1#7): isolated here so ChatHandler stays a thin routing/auth layer.
	resolver *chatThreadResolver
}

type userGatewayClient interface {
	GetUser(ctx context.Context, in *userv1.GetUserRequest, opts ...grpc.CallOption) (*userv1.UserResponse, error)
	GetUsersBatch(ctx context.Context, in *userv1.GetUsersBatchRequest, opts ...grpc.CallOption) (*userv1.GetUsersBatchResponse, error)
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
	GetBookingsBatch(ctx context.Context, in *bookingv1.GetBookingsBatchRequest, opts ...grpc.CallOption) (*bookingv1.GetBookingsBatchResponse, error)
}

type venueGatewayClient interface {
	GetVenue(ctx context.Context, in *venuev1.GetVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	GetVenuesBatch(ctx context.Context, in *venuev1.GetVenuesBatchRequest, opts ...grpc.CallOption) (*venuev1.GetVenuesBatchResponse, error)
}

// crmGatewayClient narrows the CRM proto client to only the methods the chat
// thread resolver needs. Defined at the consumer site so test fakes can
// implement just these two without touching the rest of the CRM surface.
type crmGatewayClient interface {
	GetManagementAccess(ctx context.Context, in *crmv1.GetManagementAccessRequest, opts ...grpc.CallOption) (*crmv1.GetManagementAccessResponse, error)
	ListStaff(ctx context.Context, in *crmv1.ListStaffRequest, opts ...grpc.CallOption) (*crmv1.ListStaffResponse, error)
}

type masterGatewayClient interface {
	GetMasterBooking(ctx context.Context, in *masterv1.GetMasterBookingRequest, opts ...grpc.CallOption) (*masterv1.MasterBookingResponse, error)
	GetMasterBookingsBatch(ctx context.Context, in *masterv1.GetMasterBookingsBatchRequest, opts ...grpc.CallOption) (*masterv1.GetMasterBookingsBatchResponse, error)
}

type chatMessageLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// inMemoryPruneInterval controls how often the background goroutine inside the
// in-memory fallback limiter evicts fully-stale keys (N6: prevents unbounded
// map growth on long QA sessions).
var inMemoryPruneInterval = limits.ChatInMemoryPruneInterval

// newInMemoryChatLimiter returns a sliding-window rate limiter with a
// background prune goroutine that stops when ctx is cancelled.
func newInMemoryChatLimiter(ctx context.Context) *ratelimit.SlidingWindow {
	return ratelimit.NewWithPrune(ctx, inMemoryPruneInterval)
}

// sendRateWindow and sendRateMax are kept as package-local aliases so the
// allowSend call site stays readable; the canonical values live in limits.
var (
	sendRateWindow = limits.ChatSendRateWindow
	sendRateMax    = limits.ChatSendRateMax
	wsTicketTTL    = limits.ChatWSTicketTTL
)

func NewChatHandler(
	ctx context.Context,
	log zerolog.Logger,
	client chatGatewayClient,
	booking bookingGatewayClient,
	venue venueGatewayClient,
	master masterGatewayClient,
	users userGatewayClient,
	crm crmGatewayClient,
	limiter chatMessageLimiter,
	ticketRedis *redis.Client,
	natsConn *nats.Conn,
) *ChatHandler {
	if limiter == nil {
		// N6: newInMemoryChatLimiter starts a background prune goroutine that
		// stops when ctx is cancelled (i.e. when the gateway shuts down).
		limiter = newInMemoryChatLimiter(ctx)
	}
	return &ChatHandler{
		log:         log,
		client:      client,
		users:       users,
		limiter:     limiter,
		ticketRedis: ticketRedis,
		natsConn:    natsConn,
		// P1#7: participant-resolution logic lives in chatThreadResolver, not
		// in the HTTP handler, keeping ChatHandler as a thin routing/auth layer.
		resolver: newChatThreadResolver(log, client, booking, venue, master, users, crm),
		upgrader: websocket.Upgrader{
			// CheckOrigin enforces the CORS allowlist for browser clients.
			//
			// Empty-Origin policy (N1 — intentional):
			//   Browsers always send an Origin header on cross-origin WebSocket
			//   upgrades (RFC 6455 §4).  A missing Origin means the caller is
			//   NOT a browser: native mobile apps, CLI tools, server-side test
			//   scripts, Postman.  These callers also cannot obtain CSRF cookies,
			//   so the CSRF risk that Origin-checking guards against does not apply.
			//
			//   The WS ticket mechanism (IssueWSTicket / Auth middleware) provides
			//   the primary auth layer.  When a ticket is issued to a server-side
			//   caller (no Origin), storedOrigin is "" and the middleware intentionally
			//   skips the origin-match check — mirroring this policy here.
			//
			//   If you introduce browser-only flows that must never allow headless
			//   access, require a non-empty Origin at ticket-issuance time instead
			//   of here.
			CheckOrigin: func(r *http.Request) bool {
				origin := strings.TrimSpace(r.Header.Get("Origin"))
				if origin == "" {
					// Non-browser caller (mobile / CLI / server): allow.
					// Auth is enforced by the WS ticket; see comment above.
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
	// P2#20: normalise all UUID fields to lowercase for a consistent API surface.
	// last_read_message_id was already lowercased below; apply the same here.
	out := map[string]any{
		"id":                   strings.ToLower(t.GetId()),
		"kind":                 t.GetKind(),
		"ref_id":               t.GetRefId(),
		"participant_user_ids": t.GetParticipantUserIds(),
		"last_message_id":      strings.ToLower(t.GetLastMessageId()),
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
		"id":             strings.ToLower(m.GetId()),
		"thread_id":      strings.ToLower(m.GetThreadId()),
		"author_user_id": strings.ToLower(m.GetAuthorUserId()),
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
		if err != nil {
			// P2#24: marshal errors are unexpected (payload is always a known map)
			// but worth a warning so they don't vanish silently in production.
			h.log.Warn().Err(err).Msg("chat: emitToUsers: failed to marshal NATS fanout payload, falling back to local hub")
		} else {
			if pubErr := h.natsConn.Publish("chat.fanout", b); pubErr != nil {
				h.log.Warn().Err(pubErr).Msg("chat: emitToUsers: NATS publish failed, falling back to local hub")
			} else {
				return
			}
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
	// P0#3: bind the ticket to the issuing Origin so that a stolen ticket
	// from a different origin (e.g. via Referer leak) cannot be replayed.
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	payload, err := json.Marshal(map[string]string{
		"user_id": userID,
		"role":    middleware.RoleFromCtx(r.Context()),
		"email":   middleware.EmailFromCtx(r.Context()),
		"origin":  origin,
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
		"ticket":         id,
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

// ensureVenueBookingThread and ensureMasterBookingThread have been moved to
// chat_thread_resolver.go (P1#7).  ChatHandler delegates to h.resolver.

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
	if !readJSONOrRespond(w, r, &body) {
		return
	}
	var (
		t   *chatv1.ChatThread
		err error
	)
	// Delegate to resolver (P1#7): ChatHandler stays thin, resolver owns
	// the "who participates?" business logic.
	switch strings.TrimSpace(body.Kind) {
	case "venue_booking":
		t, err = h.resolver.ensureVenueBookingThread(r, userID, strings.TrimSpace(body.RefID))
	case "master_booking":
		t, err = h.resolver.ensureMasterBookingThread(r, userID, strings.TrimSpace(body.RefID))
	default:
		writeCatalog(w, apicatalog.GatewayRequestInvalidBody.WithMessage("unsupported thread kind"))
		return
	}
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	item := chatThreadToJSON(t)
	attachPeerDisplayToThreadJSON(item, t.GetId(), h.resolver.peerDisplayNamesBatch(r.Context(), userID, []*chatv1.ChatThread{t}))
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
	peerByThread := h.resolver.peerDisplayNamesBatch(r.Context(), userID, rawThreads)
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
	if !readJSONOrRespond(w, r, &body) {
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
	// Both "type" (v1 WS clients) and "event" (v2 WS clients) are included in
	// the fanout payload so that clients on either version receive the event
	// they expect from a single broadcast. New clients should read "event".
	payload := map[string]any{
		"type":    "message_new",
		"event":   "chat.message.created",
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
	// See SendMessage: both keys present for v1/v2 client compatibility.
	payload := map[string]any{
		"type":   "read_updated",
		"event":  "chat.thread.read_updated",
		"thread": chatThreadToJSON(resp.GetThread()),
	}
	h.emitToUsers(resp.GetThread().GetParticipantUserIds(), payload)
	writeJSON(w, http.StatusOK, map[string]any{"thread": chatThreadToJSON(resp.GetThread())})
}

var (
	// wsReadLimit caps incoming JSON frames to guard against memory exhaustion
	// (P1#14).  Canonical value in limits.ChatWSReadLimit.
	wsReadLimit = limits.ChatWSReadLimit

	// wsPingInterval / wsPongWait / wsWriteWait — canonical values in limits.
	wsPingInterval = limits.ChatWSPingInterval
	wsPongWait     = limits.ChatWSPongWait
	wsWriteWait    = limits.ChatWSWriteWait
)

func init() {
	// PongWait must be strictly greater than PingInterval so the server always
	// allows at least one full round-trip before declaring the connection dead.
	// A violation (e.g. via env override) would cause spurious disconnects for
	// every client — catch it at startup rather than in production logs.
	if wsPongWait <= wsPingInterval {
		panic(fmt.Sprintf(
			"chat WS config invariant violated: CHAT_WS_PONG_WAIT (%s) must be > CHAT_WS_PING_INTERVAL (%s)",
			wsPongWait, wsPingInterval,
		))
	}
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

	// P1#14: cap incoming frame size so a rogue client cannot OOM the server.
	rawConn.SetReadLimit(wsReadLimit)

	// P1#15: server-side ping/pong keepalive.
	// Reset the read deadline each time a pong is received.
	_ = rawConn.SetReadDeadline(time.Now().Add(wsPongWait))
	rawConn.SetPongHandler(func(string) error {
		return rawConn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	c := h.hub.Add(userID, rawConn)
	defer h.hub.Remove(userID, c)

	// Keepalive pings are emitted by the hub's writeLoop (the single writer to
	// this connection). gorilla/websocket panics on concurrent writes, so the
	// ping must NOT run in a separate goroutine here — that panic fires outside
	// the HTTP recoverer and crashes the whole gateway process.

	// best-effort: SendJSON returns non-nil only when json.Marshal fails, which
	// cannot happen for the map[string]any literals below (all primitive types).
	// Connection drops and full send-buffers are handled inside SendJSON itself
	// via Client.close() — the next ReadJSON call will then return an error and
	// exit this goroutine cleanly. No action needed on the caller side.
	_ = c.SendJSON(map[string]any{"type": "connected", "event": "chat.connected"})

	for {
		var in struct {
			Type        string `json:"type"`
			ThreadID    string `json:"thread_id"`
			Text        string `json:"text"`
			ClientMsgID string `json:"client_msg_id"` // P0#5: propagate for idempotency
		}
		if err := rawConn.ReadJSON(&in); err != nil {
			return
		}
		// N11: normalise thread_id to lowercase once here so every case branch
		// sends a canonical UUID to gRPC regardless of what the client sent.
		// uuid.Parse in the usecase would handle it anyway, but being explicit
		// here keeps the WS path consistent with the HTTP handlers.
		in.ThreadID = strings.ToLower(strings.TrimSpace(in.ThreadID))
		switch in.Type {
		case "ping":
			_ = c.SendJSON(map[string]any{"type": "pong", "event": "chat.pong"}) // best-effort, see above
		case "send_message":
			if !h.allowSend(r.Context(), userID, in.ThreadID) {
				_ = c.SendJSON(map[string]any{"type": "error", "event": "chat.error", "error": "rate_limit"}) // best-effort, see above
				continue
			}
			resp, err := h.client.SendMessage(r.Context(), &chatv1.SendMessageRequest{
				ThreadId:    in.ThreadID,
				UserId:      userID,
				Text:        in.Text,
				ClientMsgId: strings.TrimSpace(in.ClientMsgID), // P0#5: dedup on retry
			})
			if err != nil {
				_ = c.SendJSON(map[string]any{"type": "error", "event": "chat.error", "error": "send_failed"}) // best-effort, see above
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
				_ = c.SendJSON(map[string]any{"type": "error", "event": "chat.error", "error": "mark_read_failed"}) // best-effort, see above
				continue
			}
			h.emitToUsers(resp.GetThread().GetParticipantUserIds(), map[string]any{
				"type":   "read_updated",
				"event":  "chat.thread.read_updated",
				"thread": chatThreadToJSON(resp.GetThread()),
			})
		default:
			_ = c.SendJSON(map[string]any{"type": "error", "event": "chat.error", "error": "unknown_command"}) // best-effort, see above
		}
	}
}
