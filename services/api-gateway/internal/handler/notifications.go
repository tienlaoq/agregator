package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"

	notificationv1 "github.com/tienlao/agregator/gen/go/notification/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"github.com/tienlao/agregator/services/api-gateway/internal/realtime/chatws"
)

// notificationFanoutSubject is the NATS subject used to fan a freshly-created
// notification out to every gateway replica, so the one holding the target
// user's WebSocket can push it. Mirrors the chat "chat.fanout" mechanism.
const notificationFanoutSubject = "notification.fanout"

// notificationGatewayClient narrows the generated client to the methods the
// gateway needs, defined at the consumer site so test fakes stay small.
type notificationGatewayClient interface {
	Create(ctx context.Context, in *notificationv1.CreateRequest, opts ...grpc.CallOption) (*notificationv1.CreateResponse, error)
	List(ctx context.Context, in *notificationv1.ListRequest, opts ...grpc.CallOption) (*notificationv1.ListResponse, error)
	GetUnreadCount(ctx context.Context, in *notificationv1.GetUnreadCountRequest, opts ...grpc.CallOption) (*notificationv1.GetUnreadCountResponse, error)
	MarkRead(ctx context.Context, in *notificationv1.MarkReadRequest, opts ...grpc.CallOption) (*notificationv1.MarkReadResponse, error)
	MarkAllRead(ctx context.Context, in *notificationv1.MarkAllReadRequest, opts ...grpc.CallOption) (*notificationv1.MarkAllReadResponse, error)
}

// NotificationHandler serves the per-user "bell" inbox: REST reads/writes that
// proxy to notification-service, plus a WebSocket that pushes new notifications
// in realtime. The WS transport (hub, ticket auth, fanout) mirrors ChatHandler.
type NotificationHandler struct {
	log         zerolog.Logger
	client      notificationGatewayClient
	ticketRedis *redis.Client
	natsConn    *nats.Conn
	upgrader    websocket.Upgrader
	hub         *chatws.Hub
}

func NewNotificationHandler(
	log zerolog.Logger,
	client notificationGatewayClient,
	ticketRedis *redis.Client,
	natsConn *nats.Conn,
) *NotificationHandler {
	return &NotificationHandler{
		log:         log,
		client:      client,
		ticketRedis: ticketRedis,
		natsConn:    natsConn,
		upgrader: websocket.Upgrader{
			// Same Origin policy as the chat WS upgrader: enforce the CORS
			// allowlist for browser callers; allow empty-Origin (non-browser)
			// callers since the ws_ticket is the primary auth layer.
			CheckOrigin: func(r *http.Request) bool {
				origin := strings.TrimSpace(r.Header.Get("Origin"))
				if origin == "" {
					return true
				}
				originURL, err := url.Parse(origin)
				if err != nil || originURL.Host == "" {
					return false
				}
				for _, a := range middleware.CORSAllowedOrigins() {
					if u, err := url.Parse(a); err == nil && strings.EqualFold(u.Host, originURL.Host) {
						return true
					}
				}
				return middleware.IsLocalLoopbackOrigin(origin)
			},
		},
		hub: chatws.NewHub(),
	}
}

func notificationToJSON(n *notificationv1.Notification) map[string]any {
	if n == nil {
		return nil
	}
	out := map[string]any{
		"id":      strings.ToLower(n.GetId()),
		"user_id": strings.ToLower(n.GetUserId()),
		"type":    n.GetType(),
		"title":   n.GetTitle(),
		"body":    n.GetBody(),
		"read":    n.GetRead(),
	}
	if d := strings.TrimSpace(n.GetData()); d != "" {
		out["data"] = d
	}
	if n.GetCreatedAt() != nil {
		out["created_at"] = n.GetCreatedAt().AsTime().Format(time.RFC3339)
	}
	if n.GetReadAt() != nil {
		out["read_at"] = n.GetReadAt().AsTime().Format(time.RFC3339)
	}
	return out
}

// ── REST ────────────────────────────────────────────────────────────────────

func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 30, 100)
	offset := parsePositiveInt(r.URL.Query().Get("offset"), 0, 1000000)
	unreadOnly := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("unread_only")), "true")

	resp, err := h.client.List(r.Context(), &notificationv1.ListRequest{
		UserId:     userID,
		Limit:      limit,
		Offset:     offset,
		UnreadOnly: unreadOnly,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	items := make([]map[string]any, 0, len(resp.GetNotifications()))
	for _, n := range resp.GetNotifications() {
		items = append(items, notificationToJSON(n))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"notifications": items,
		"total":         resp.GetTotal(),
		"unread_count":  resp.GetUnreadCount(),
	})
}

func (h *NotificationHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	resp, err := h.client.GetUnreadCount(r.Context(), &notificationv1.GetUnreadCountRequest{UserId: userID})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unread_count": resp.GetUnreadCount()})
}

func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	notificationID := chi.URLParam(r, "notificationId")
	resp, err := h.client.MarkRead(r.Context(), &notificationv1.MarkReadRequest{
		UserId:         userID,
		NotificationId: notificationID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unread_count": resp.GetUnreadCount()})
}

func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	resp, err := h.client.MarkAllRead(r.Context(), &notificationv1.MarkAllReadRequest{UserId: userID})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unread_count": resp.GetUnreadCount()})
}

// IssueWSTicket mints a one-time Redis ticket for the notifications WebSocket.
// It uses the same "chat:wst:" key prefix that middleware.Auth validates, so a
// ticket authenticates the user regardless of which WS path consumes it.
func (h *NotificationHandler) IssueWSTicket(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	if userID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	if h.ticketRedis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "ws_ticket_unavailable"})
		return
	}
	id := uuid.NewString()
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
	if err := h.ticketRedis.Set(r.Context(), "chat:wst:"+id, payload, wsTicketTTL).Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "redis"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":         id,
		"expires_in_sec": int(wsTicketTTL.Seconds()),
	})
}

// ── Realtime fan-out ──────────────────────────────────────────────────────────

// emitToUsers publishes payload to the target users. With NATS it fans out to
// every gateway replica (so whichever one holds the user's socket delivers it);
// without NATS it falls back to the local hub only.
func (h *NotificationHandler) emitToUsers(userIDs []string, payload map[string]any) {
	if h.natsConn != nil && len(userIDs) > 0 {
		b, err := json.Marshal(struct {
			UserIDs []string       `json:"user_ids"`
			Payload map[string]any `json:"payload"`
		}{UserIDs: userIDs, Payload: payload})
		if err != nil {
			h.log.Warn().Err(err).Msg("notification: emitToUsers marshal failed, using local hub")
		} else if pubErr := h.natsConn.Publish(notificationFanoutSubject, b); pubErr != nil {
			h.log.Warn().Err(pubErr).Msg("notification: NATS publish failed, using local hub")
		} else {
			return
		}
	}
	h.hub.Broadcast(userIDs, payload)
}

// HandleFanoutMessage applies a NATS fan-out payload to the local hub. Wired to
// the notification.fanout subscription in the router on every gateway replica.
func (h *NotificationHandler) HandleFanoutMessage(data []byte) {
	var envelope struct {
		UserIDs []string       `json:"user_ids"`
		Payload map[string]any `json:"payload"`
	}
	if json.Unmarshal(data, &envelope) != nil || len(envelope.UserIDs) == 0 || envelope.Payload == nil {
		return
	}
	h.hub.Broadcast(envelope.UserIDs, envelope.Payload)
}

// Notify creates a notification for userID and pushes it to the user's open
// sockets. Best-effort: a failure is logged but never propagated, so the
// triggering action (e.g. a CRM staff invite) is not rolled back by an inbox
// hiccup. ctx should be a non-request context when called after the HTTP
// response, since the request context may already be cancelled.
func (h *NotificationHandler) Notify(ctx context.Context, userID, typ, title, body, data string) {
	if h.client == nil || strings.TrimSpace(userID) == "" {
		return
	}
	resp, err := h.client.Create(ctx, &notificationv1.CreateRequest{
		UserId: userID,
		Type:   typ,
		Title:  title,
		Body:   body,
		Data:   data,
	})
	if err != nil {
		h.log.Warn().Err(err).Str("user_id", userID).Str("type", typ).Msg("notification: create failed")
		return
	}
	h.emitToUsers([]string{userID}, map[string]any{
		"event":        "notification.created",
		"notification": notificationToJSON(resp.GetNotification()),
	})
}

// NotifyStaffInvited builds and delivers the "added to a venue's team"
// notification. Implements the staffInviteNotifier interface consumed by
// VenueHandler.AddVenueStaffByEmail.
func (h *NotificationHandler) NotifyStaffInvited(ctx context.Context, userID, venueID, role string) {
	roleLabel := "сотрудник"
	if strings.EqualFold(strings.TrimSpace(role), "manager") {
		roleLabel = "менеджер"
	}
	data, _ := json.Marshal(map[string]string{
		"kind":     "crm_staff_invited",
		"venue_id": venueID,
		"role":     strings.TrimSpace(role),
	})
	h.Notify(ctx, userID, "crm_staff_invited",
		"Вас добавили в команду заведения",
		"Вам выдан доступ к управлению заведением в роли «"+roleLabel+"». Откройте кабинет владельца, чтобы начать работу.",
		string(data),
	)
}

// NotifyTaskAssigned delivers the "a CRM task was assigned to you" bell to the
// assignee. Implements the crmTaskNotifier interface consumed by VenueHandler's
// task create/update paths. Best-effort: failures are logged, never surfaced.
func (h *NotificationHandler) NotifyTaskAssigned(ctx context.Context, assigneeID, venueID, taskID, title string) {
	data, _ := json.Marshal(map[string]string{
		"kind":     "crm_task_assigned",
		"venue_id": venueID,
		"task_id":  taskID,
	})
	body := "Вам назначена задача по заведению. Откройте CRM, чтобы посмотреть детали."
	if t := strings.TrimSpace(title); t != "" {
		body = "Вам назначена задача «" + t + "». Откройте CRM, чтобы посмотреть детали."
	}
	h.Notify(ctx, assigneeID, "crm_task_assigned", "Новая задача в CRM", body, string(data))
}

// ── Venue-owner notifications ────────────────────────────────────────────────
// Владелец может иметь несколько заведений, поэтому каждое уведомление
// называет заведение по имени и несёт venue_id в data.

// NotifyVenueBookingCreated delivers the "new booking at your venue" bell to
// the venue owner. Implements bookingOwnerNotifier (booking.go).
func (h *NotificationHandler) NotifyVenueBookingCreated(ctx context.Context, ownerID, venueID, venueName, bookingID, date, timeFrom, timeTo string, guests int32) {
	when := strings.TrimSpace(date)
	if timeFrom != "" {
		when += " " + timeFrom
		if timeTo != "" {
			when += "–" + timeTo
		}
	}
	body := when
	if guests > 0 {
		body += fmt.Sprintf(", гостей: %d", guests)
	}
	body += ". Откройте CRM заведения, чтобы посмотреть детали."
	data, _ := json.Marshal(map[string]string{
		"kind":       "venue_booking_created",
		"venue_id":   venueID,
		"booking_id": bookingID,
	})
	h.Notify(ctx, ownerID, "venue_booking_created",
		"Новая бронь — "+venueName,
		body,
		string(data),
	)
}

// NotifyVenueModerated delivers the "your venue's listing status changed" bell
// to the venue owner. Implements venueModerationNotifier (venue.go).
func (h *NotificationHandler) NotifyVenueModerated(ctx context.Context, ownerID, venueID, venueName, action, comment string) {
	var title, body string
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "approve":
		title = "Заведение «" + venueName + "» одобрено"
		body = "Карточка прошла модерацию и опубликована в каталоге."
	case "reject":
		title = "Заведение «" + venueName + "» не прошло модерацию"
		body = "Исправьте карточку и отправьте на проверку снова."
		if c := strings.TrimSpace(comment); c != "" {
			body = "Причина: " + c + " — исправьте карточку и отправьте на проверку снова."
		}
	case "suspend":
		title = "Заведение «" + venueName + "» приостановлено"
		body = "Карточка скрыта из каталога."
		if c := strings.TrimSpace(comment); c != "" {
			body = "Карточка скрыта из каталога. Причина: " + c
		}
	case "resume":
		title = "Заведение «" + venueName + "» снова активно"
		body = "Публикация в каталоге возобновлена."
	default:
		return
	}
	data, _ := json.Marshal(map[string]string{
		"kind":     "venue_moderation",
		"venue_id": venueID,
		"action":   strings.ToLower(strings.TrimSpace(action)),
	})
	h.Notify(ctx, ownerID, "venue_moderation", title, body, string(data))
}

// NotifyVenueReviewCreated delivers the "new review at your venue" bell to the
// venue owner. Implements reviewOwnerNotifier (review.go).
func (h *NotificationHandler) NotifyVenueReviewCreated(ctx context.Context, ownerID, venueID, venueName string, rating int32) {
	body := "Гость оставил отзыв о заведении."
	if rating >= 1 && rating <= 5 {
		body = fmt.Sprintf("Оценка %d из 5. Откройте карточку заведения, чтобы прочитать отзыв.", rating)
	}
	data, _ := json.Marshal(map[string]any{
		"kind":     "venue_review_created",
		"venue_id": venueID,
		"rating":   rating,
	})
	h.Notify(ctx, ownerID, "venue_review_created",
		"Новый отзыв — "+venueName,
		body,
		string(data),
	)
}

// ── Master notifications ─────────────────────────────────────────────────────
// Адресуются user_id владельца профиля мастера. Имя мастера выносится в
// заголовок, master_id кладётся в data для будущих ссылок.

// NotifyMasterBookingCreated delivers the "new booking with you" bell to the
// master. Implements masterBookingNotifier (master.go).
func (h *NotificationHandler) NotifyMasterBookingCreated(ctx context.Context, masterUserID, masterID, masterName, bookingID, date, timeFrom, timeTo string) {
	when := strings.TrimSpace(date)
	if timeFrom != "" {
		when += " " + timeFrom
		if timeTo != "" {
			when += "–" + timeTo
		}
	}
	body := strings.TrimSpace(when) + ". Откройте кабинет мастера, чтобы посмотреть детали."
	data, _ := json.Marshal(map[string]string{
		"kind":       "master_booking_created",
		"master_id":  masterID,
		"booking_id": bookingID,
	})
	title := "Новая запись"
	if strings.TrimSpace(masterName) != "" {
		title = "Новая запись — " + masterName
	}
	h.Notify(ctx, masterUserID, "master_booking_created", title, body, string(data))
}

// NotifyMasterModerated delivers the "your master profile status changed" bell
// to the master. Implements masterModerationNotifier (master.go).
func (h *NotificationHandler) NotifyMasterModerated(ctx context.Context, masterUserID, masterID, masterName, action, comment string) {
	who := strings.TrimSpace(masterName)
	if who == "" {
		who = "ваш профиль"
	} else {
		who = "профиль «" + who + "»"
	}
	var title, body string
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "approve":
		title = "Профиль мастера одобрен"
		body = "Ваш " + who + " прошёл модерацию и опубликован в каталоге мастеров."
	case "reject":
		title = "Профиль мастера не прошёл модерацию"
		body = "Исправьте профиль и отправьте на проверку снова."
		if c := strings.TrimSpace(comment); c != "" {
			body = "Причина: " + c + " — исправьте профиль и отправьте на проверку снова."
		}
	case "suspend":
		title = "Профиль мастера приостановлен"
		body = "Ваш " + who + " скрыт из каталога."
		if c := strings.TrimSpace(comment); c != "" {
			body = "Ваш " + who + " скрыт из каталога. Причина: " + c
		}
	case "resume":
		title = "Профиль мастера снова активен"
		body = "Публикация в каталоге мастеров возобновлена."
	default:
		return
	}
	data, _ := json.Marshal(map[string]string{
		"kind":      "master_moderation",
		"master_id": masterID,
		"action":    strings.ToLower(strings.TrimSpace(action)),
	})
	h.Notify(ctx, masterUserID, "master_moderation", title, body, string(data))
}

// NotifyMasterReviewCreated delivers the "new review about you" bell to the
// master. Implements masterReviewNotifier (review.go).
func (h *NotificationHandler) NotifyMasterReviewCreated(ctx context.Context, masterUserID, masterID, masterName string, rating int32) {
	body := "Гость оставил отзыв о вашей работе."
	if rating >= 1 && rating <= 5 {
		body = fmt.Sprintf("Оценка %d из 5. Откройте свой профиль, чтобы прочитать отзыв.", rating)
	}
	data, _ := json.Marshal(map[string]any{
		"kind":      "master_review_created",
		"master_id": masterID,
		"rating":    rating,
	})
	title := "Новый отзыв о вас"
	if strings.TrimSpace(masterName) != "" {
		title = "Новый отзыв — " + masterName
	}
	h.Notify(ctx, masterUserID, "master_review_created", title, body, string(data))
}

// ── WebSocket ─────────────────────────────────────────────────────────────────

// WS upgrades the connection and registers it in the per-user hub. The socket
// is push-only (the client sends keepalive pings); all inbox mutations go
// through the REST endpoints above. Mirrors ChatHandler.WS keepalive handling.
func (h *NotificationHandler) WS(w http.ResponseWriter, r *http.Request) {
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

	rawConn.SetReadLimit(wsReadLimit)
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

	_ = c.SendJSON(map[string]any{"event": "notification.connected"})

	for {
		var in struct {
			Type string `json:"type"`
		}
		if err := rawConn.ReadJSON(&in); err != nil {
			return
		}
		if in.Type == "ping" {
			_ = c.SendJSON(map[string]any{"event": "notification.pong"})
		}
	}
}
