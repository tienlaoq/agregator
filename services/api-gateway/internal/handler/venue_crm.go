package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	resp, err := h.client.ListVenueStaff(r.Context(), &venuev1.ListVenueStaffRequest{
		VenueId: venueID,
		ActorId: actorID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
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
	writeJSON(w, http.StatusOK, map[string]any{"staff": out})
}

func (h *VenueHandler) AddVenueStaffByEmail(w http.ResponseWriter, r *http.Request) {
	if h.userClient == nil {
		writeCatalog(w, apicatalog.GatewayDependencyUserServiceUnavailable.WithMessage("user service not configured"))
		return
	}
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := readJSON(r, &req); err != nil {
		writeCatalog(w, apicatalog.GatewayRequestInvalidBody)
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		writeCatalog(w, apicatalog.GatewayRequestEmailRequired)
		return
	}

	u, err := h.userClient.GetUserByEmail(r.Context(), &userv1.GetUserByEmailRequest{Email: email})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			writeCatalog(w, apicatalog.GatewayCrmStaffEmailNotRegistered)
			return
		}
		grpcErrorToHTTP(w, err)
		return
	}
	if u == nil || strings.TrimSpace(u.GetId()) == "" {
		writeCatalog(w, apicatalog.GatewayCrmStaffEmailNotRegistered)
		return
	}

	_, err = h.client.AddVenueStaff(r.Context(), &venuev1.AddVenueStaffRequest{
		VenueId: venueID,
		ActorId: actorID,
		UserId:  u.GetId(),
		Role:    req.Role,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user_id": u.GetId(), "email": u.GetEmail(), "role": req.Role})
}

func (h *VenueHandler) RemoveVenueStaff(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	targetID := chi.URLParam(r, "userId")

	_, err := h.client.RemoveVenueStaff(r.Context(), &venuev1.RemoveVenueStaffRequest{
		VenueId: venueID,
		ActorId: actorID,
		UserId:  targetID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *VenueHandler) ListVenueCRMTasks(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	status := r.URL.Query().Get("status")

	resp, err := h.client.ListVenueCRMTasks(r.Context(), &venuev1.ListVenueCRMTasksRequest{
		VenueId: venueID,
		ActorId: actorID,
		Status:  status,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	tasks := make([]map[string]any, 0, len(resp.GetTasks()))
	for _, t := range resp.GetTasks() {
		m := map[string]any{
			"id":         t.GetId(),
			"venue_id":   t.GetVenueId(),
			"title":      t.GetTitle(),
			"body":       t.GetBody(),
			"status":     t.GetStatus(),
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
		tasks = append(tasks, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (h *VenueHandler) CreateVenueCRMTask(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")

	var req struct {
		Title          string  `json:"title"`
		Body           string  `json:"body"`
		BookingID      *string `json:"booking_id"`
		AssigneeUserID *string `json:"assignee_user_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeCatalog(w, apicatalog.GatewayRequestInvalidBody)
		return
	}

	grpcReq := &venuev1.CreateVenueCRMTaskRequest{
		VenueId: venueID,
		ActorId: actorID,
		Title:   req.Title,
		Body:    req.Body,
	}
	if req.BookingID != nil && strings.TrimSpace(*req.BookingID) != "" {
		grpcReq.BookingId = req.BookingID
	}
	if req.AssigneeUserID != nil && strings.TrimSpace(*req.AssigneeUserID) != "" {
		grpcReq.AssigneeUserId = req.AssigneeUserID
	}

	resp, err := h.client.CreateVenueCRMTask(r.Context(), grpcReq)
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	t := resp.GetTask()
	if t == nil {
		writeJSON(w, http.StatusOK, map[string]any{"task": map[string]any{}})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"task": map[string]any{
		"id": t.GetId(), "venue_id": t.GetVenueId(), "title": t.GetTitle(), "body": t.GetBody(),
		"status": t.GetStatus(), "created_by": t.GetCreatedBy(),
		"created_at": t.GetCreatedAt().AsTime(), "updated_at": t.GetUpdatedAt().AsTime(),
	}})
}

func (h *VenueHandler) CompleteVenueCRMTask(w http.ResponseWriter, r *http.Request) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		writeCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return
	}
	venueID := chi.URLParam(r, "venueId")
	taskID := chi.URLParam(r, "taskId")

	_, err := h.client.CompleteVenueCRMTask(r.Context(), &venuev1.CompleteVenueCRMTaskRequest{
		VenueId: venueID,
		ActorId: actorID,
		TaskId:  taskID,
	})
	if err != nil {
		grpcErrorToHTTP(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
