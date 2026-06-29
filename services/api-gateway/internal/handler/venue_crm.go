package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CRM-related HTTP handlers proxy to crm-service. They live on VenueHandler
// because routes nest under /owner/venues/{venueId}/... — the URL space is
// venue-scoped even though the backing service is now separate.

type venueStaffUserDisplay struct {
	name  string
	email string
}

func uniqueNonEmptyUserIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func venueStaffLoadUserDisplays(ctx context.Context, c userv1.UserServiceClient, userIDs []string) map[string]venueStaffUserDisplay {
	out := make(map[string]venueStaffUserDisplay, len(userIDs))
	if c == nil || len(userIDs) == 0 {
		return out
	}
	for _, id := range userIDs {
		u, err := c.GetUser(ctx, &userv1.GetUserRequest{Id: id})
		if err != nil || u == nil {
			continue
		}
		out[id] = venueStaffUserDisplay{
			name:  strings.TrimSpace(u.GetName()),
			email: strings.TrimSpace(u.GetEmail()),
		}
	}
	return out
}

func (h *VenueHandler) ListVenueStaff(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	resp, err := h.crm.ListStaff(r.Context(), &crmv1.ListStaffRequest{
		VenueId: venueID,
		ActorId: actorID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	members := resp.GetMembers()
	idList := make([]string, 0, len(members)*2)
	for _, m := range members {
		idList = append(idList, m.GetUserId(), m.GetInvitedBy())
	}
	uniq := uniqueNonEmptyUserIDs(idList)
	profiles := venueStaffLoadUserDisplays(r.Context(), h.userClient, uniq)

	out := make([]map[string]any, 0, len(members))
	for _, m := range members {
		uid := strings.TrimSpace(m.GetUserId())
		inv := strings.TrimSpace(m.GetInvitedBy())
		up := profiles[uid]
		ip := profiles[inv]
		row := map[string]any{
			"user_id":    uid,
			"role":       m.GetRole(),
			"invited_by": inv,
			"created_at": m.GetCreatedAt().AsTime(),
		}
		if up.name != "" {
			row["user_name"] = up.name
		}
		if up.email != "" {
			row["user_email"] = up.email
		}
		if ip.name != "" {
			row["inviter_name"] = ip.name
		}
		if ip.email != "" {
			row["inviter_email"] = ip.email
		}
		if inv != "" && inv == actorID {
			row["inviter_is_you"] = true
		}
		out = append(out, row)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"staff": out})
}

func (h *VenueHandler) AddVenueStaffByEmail(w http.ResponseWriter, r *http.Request) {
	if h.userClient == nil {
		httpx.WriteCatalog(w, apicatalog.GatewayDependencyUserServiceUnavailable.WithMessage("user service not configured"))
		return
	}
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestEmailRequired)
		return
	}

	u, err := h.userClient.GetUserByEmail(r.Context(), &userv1.GetUserByEmailRequest{Email: email})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			httpx.WriteCatalog(w, apicatalog.GatewayCrmStaffEmailNotRegistered)
			return
		}
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	if u == nil || strings.TrimSpace(u.GetId()) == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayCrmStaffEmailNotRegistered)
		return
	}

	_, err = h.crm.AddStaff(r.Context(), &crmv1.AddStaffRequest{
		VenueId: venueID,
		ActorId: actorID,
		UserId:  u.GetId(),
		Role:    req.Role,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	// Bell notification for the invited worker (best-effort, never fails the
	// invite). Fired before the response is written so r.Context() is still live.
	if h.staffNotifier != nil {
		h.staffNotifier.NotifyStaffInvited(r.Context(), u.GetId(), venueID, req.Role)
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"user_id": u.GetId(), "email": u.GetEmail(), "role": req.Role})
}

func (h *VenueHandler) RemoveVenueStaff(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	targetID := chi.URLParam(r, "userId")

	_, err := h.crm.RemoveStaff(r.Context(), &crmv1.RemoveStaffRequest{
		VenueId: venueID,
		ActorId: actorID,
		UserId:  targetID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// crmTaskToMap renders a CRM task proto as the JSON shape the owner cabinet
// consumes. Optional fields are omitted when unset so the frontend can treat
// their absence as "not applicable".
func crmTaskToMap(t *crmv1.Task) map[string]any {
	m := map[string]any{
		"id":         t.GetId(),
		"venue_id":   t.GetVenueId(),
		"title":      t.GetTitle(),
		"body":       t.GetBody(),
		"status":     t.GetStatus(),
		"priority":   t.GetPriority(),
		"created_by": t.GetCreatedBy(),
		"created_at": t.GetCreatedAt().AsTime(),
		"updated_at": t.GetUpdatedAt().AsTime(),
	}
	if t.GetBookingId() != "" {
		m["booking_id"] = t.GetBookingId()
	}
	if t.GetAssigneeUserId() != "" {
		m["assignee_user_id"] = t.GetAssigneeUserId()
	}
	if t.GetDueAt() != nil {
		m["due_at"] = t.GetDueAt().AsTime()
	}
	if t.GetCompletedBy() != "" {
		m["completed_by"] = t.GetCompletedBy()
	}
	if t.GetCompletedAt() != nil {
		m["completed_at"] = t.GetCompletedAt().AsTime()
	}
	return m
}

// parseCRMDueAt converts an optional RFC3339 string to a proto timestamp. A nil
// pointer or empty/blank string yields (nil, true) — "no deadline". A malformed
// value yields (nil, false) so the caller can reject the request.
func parseCRMDueAt(raw *string) (*timestamppb.Timestamp, bool) {
	if raw == nil {
		return nil, true
	}
	s := strings.TrimSpace(*raw)
	if s == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, false
	}
	return timestamppb.New(parsed), true
}

// notifyTaskAssignee fires the assignment bell when a task has an assignee other
// than the actor. Best-effort: never blocks or fails the request.
func (h *VenueHandler) notifyTaskAssignee(ctx context.Context, t *crmv1.Task, venueID, actorID string) {
	if h.taskNotifier == nil || t == nil {
		return
	}
	assignee := strings.TrimSpace(t.GetAssigneeUserId())
	if assignee == "" || assignee == actorID {
		return
	}
	h.taskNotifier.NotifyTaskAssigned(ctx, assignee, venueID, t.GetId(), t.GetTitle())
}

func (h *VenueHandler) ListVenueCRMTasks(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	statusFilter := r.URL.Query().Get("status")

	resp, err := h.crm.ListTasks(r.Context(), &crmv1.ListTasksRequest{
		VenueId: venueID,
		ActorId: actorID,
		Status:  statusFilter,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	tasks := make([]map[string]any, 0, len(resp.GetTasks()))
	for _, t := range resp.GetTasks() {
		tasks = append(tasks, crmTaskToMap(t))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (h *VenueHandler) CreateVenueCRMTask(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")

	var req struct {
		Title          string  `json:"title"`
		Body           string  `json:"body"`
		BookingID      *string `json:"booking_id"`
		AssigneeUserID *string `json:"assignee_user_id"`
		Priority       string  `json:"priority"`
		DueAt          *string `json:"due_at"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}
	dueAt, ok := parseCRMDueAt(req.DueAt)
	if !ok {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidBody.WithMessage("неверный формат срока (ожидается RFC3339)"))
		return
	}

	grpcReq := &crmv1.CreateTaskRequest{
		VenueId:  venueID,
		ActorId:  actorID,
		Title:    req.Title,
		Body:     req.Body,
		Priority: req.Priority,
		DueAt:    dueAt,
	}
	if req.BookingID != nil && strings.TrimSpace(*req.BookingID) != "" {
		grpcReq.BookingId = req.BookingID
	}
	if req.AssigneeUserID != nil && strings.TrimSpace(*req.AssigneeUserID) != "" {
		grpcReq.AssigneeUserId = req.AssigneeUserID
	}

	resp, err := h.crm.CreateTask(r.Context(), grpcReq)
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	t := resp.GetTask()
	if t == nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"task": map[string]any{}})
		return
	}
	h.notifyTaskAssignee(r.Context(), t, venueID, actorID)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"task": crmTaskToMap(t)})
}

func (h *VenueHandler) CompleteVenueCRMTask(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	taskID := chi.URLParam(r, "taskId")

	_, err := h.crm.CompleteTask(r.Context(), &crmv1.CompleteTaskRequest{
		VenueId: venueID,
		ActorId: actorID,
		TaskId:  taskID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *VenueHandler) UpdateVenueCRMTask(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	taskID := chi.URLParam(r, "taskId")

	var req struct {
		Title          string  `json:"title"`
		Body           string  `json:"body"`
		Priority       string  `json:"priority"`
		AssigneeUserID *string `json:"assignee_user_id"`
		DueAt          *string `json:"due_at"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}
	dueAt, ok := parseCRMDueAt(req.DueAt)
	if !ok {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidBody.WithMessage("неверный формат срока (ожидается RFC3339)"))
		return
	}

	grpcReq := &crmv1.UpdateTaskRequest{
		VenueId:  venueID,
		ActorId:  actorID,
		TaskId:   taskID,
		Title:    req.Title,
		Body:     req.Body,
		Priority: req.Priority,
		DueAt:    dueAt,
	}
	if req.AssigneeUserID != nil && strings.TrimSpace(*req.AssigneeUserID) != "" {
		grpcReq.AssigneeUserId = req.AssigneeUserID
	}

	resp, err := h.crm.UpdateTask(r.Context(), grpcReq)
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	t := resp.GetTask()
	h.notifyTaskAssignee(r.Context(), t, venueID, actorID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"task": crmTaskToMap(t)})
}

func (h *VenueHandler) ReopenVenueCRMTask(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	taskID := chi.URLParam(r, "taskId")

	resp, err := h.crm.ReopenTask(r.Context(), &crmv1.ReopenTaskRequest{
		VenueId: venueID,
		ActorId: actorID,
		TaskId:  taskID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"task": crmTaskToMap(resp.GetTask())})
}

func (h *VenueHandler) CancelVenueCRMTask(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	taskID := chi.URLParam(r, "taskId")

	_, err := h.crm.CancelTask(r.Context(), &crmv1.CancelTaskRequest{
		VenueId: venueID,
		ActorId: actorID,
		TaskId:  taskID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// crmGuestToMap renders a guest profile for the owner cabinet, enriching it with
// the display name/email from user-service (PII stays out of crm-service; see
// docs/CRM_DESIGN.md §4.2). Optional timestamps are omitted when unset.
func crmGuestToMap(g *crmv1.GuestProfile, d venueStaffUserDisplay) map[string]any {
	segments := g.GetSegments()
	if segments == nil {
		segments = []string{}
	}
	m := map[string]any{
		"user_id":             g.GetUserId(),
		"venue_id":            g.GetVenueId(),
		"bookings_count":      g.GetBookingsCount(),
		"visits_count":        g.GetVisitsCount(),
		"cancellations_count": g.GetCancellationsCount(),
		"no_show_count":       g.GetNoShowCount(),
		"total_spent":         g.GetTotalSpent(),
		"segments":            segments,
	}
	if g.GetFirstVisitAt() != nil {
		m["first_visit_at"] = g.GetFirstVisitAt().AsTime()
	}
	if g.GetLastVisitAt() != nil {
		m["last_visit_at"] = g.GetLastVisitAt().AsTime()
	}
	if g.GetLastBookingAt() != nil {
		m["last_booking_at"] = g.GetLastBookingAt().AsTime()
	}
	if d.name != "" {
		m["user_name"] = d.name
	}
	if d.email != "" {
		m["user_email"] = d.email
	}
	return m
}

func (h *VenueHandler) ListVenueGuests(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	resp, err := h.crm.ListGuests(r.Context(), &crmv1.ListGuestsRequest{
		VenueId: venueID,
		ActorId: actorID,
		Segment: q.Get("segment"),
		Sort:    q.Get("sort"),
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	guests := resp.GetGuests()
	ids := make([]string, 0, len(guests))
	for _, g := range guests {
		ids = append(ids, g.GetUserId())
	}
	displays := venueStaffLoadUserDisplays(r.Context(), h.userClient, uniqueNonEmptyUserIDs(ids))

	out := make([]map[string]any, 0, len(guests))
	for _, g := range guests {
		out = append(out, crmGuestToMap(g, displays[g.GetUserId()]))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"guests": out, "total": resp.GetTotal()})
}

func (h *VenueHandler) GetVenueGuest(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	userID := chi.URLParam(r, "userId")

	resp, err := h.crm.GetGuest(r.Context(), &crmv1.GetGuestRequest{
		VenueId: venueID,
		ActorId: actorID,
		UserId:  userID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	display := venueStaffLoadUserDisplays(r.Context(), h.userClient, uniqueNonEmptyUserIDs([]string{userID}))[userID]

	bookings := make([]map[string]any, 0, len(resp.GetRecentBookings()))
	for _, b := range resp.GetRecentBookings() {
		bm := map[string]any{
			"booking_id":  b.GetBookingId(),
			"status":      b.GetStatus(),
			"total_price": b.GetTotalPrice(),
			"guests":      b.GetGuests(),
		}
		if b.GetVisitDate() != nil {
			bm["visit_date"] = b.GetVisitDate().AsTime()
		}
		bookings = append(bookings, bm)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"guest":           crmGuestToMap(resp.GetProfile(), display),
		"recent_bookings": bookings,
	})
}
