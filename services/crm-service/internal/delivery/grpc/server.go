package grpc

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	pkgerrors "github.com/tienlao/agregator/pkg/errors"

	"github.com/tienlao/agregator/services/crm-service/internal/domain"
	"github.com/tienlao/agregator/services/crm-service/internal/usecase"
)

const internalErrMsg = "internal error"

// Server implements crmv1.CRMServiceServer.
type Server struct {
	crmv1.UnimplementedCRMServiceServer
	uc  *usecase.CRMUseCase
	log zerolog.Logger
}

func NewServer(uc *usecase.CRMUseCase, log zerolog.Logger) *Server {
	return &Server{uc: uc, log: log}
}

func (s *Server) internalErr(err error, msg string) error {
	s.log.Error().Err(err).Msg(msg)
	return pkgerrors.Internal(internalErrMsg)
}

// passthroughIfStatus returns err unchanged when it is already a gRPC status
// error (carries the domain code from usecase), otherwise wraps it into
// Internal via internalErr to avoid leaking implementation details.
func (s *Server) passthroughOrInternal(err error, msg string) error {
	if _, ok := status.FromError(err); ok {
		return err
	}
	return s.internalErr(err, msg)
}

func parseUUIDArg(raw, field string) (uuid.UUID, error) {
	u, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, pkgerrors.InvalidArgument("invalid " + field)
	}
	return u, nil
}

// optionalUUIDArg parses an optional UUID. Empty string yields a nil pointer.
func optionalUUIDArg(raw, field string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	u, err := uuid.Parse(raw)
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid " + field)
	}
	return &u, nil
}

func (s *Server) GetManagementAccess(ctx context.Context, req *crmv1.GetManagementAccessRequest) (*crmv1.GetManagementAccessResponse, error) {
	venueID, err := parseUUIDArg(req.GetVenueId(), "venue_id")
	if err != nil {
		return nil, err
	}
	userID, err := parseUUIDArg(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	access, err := s.uc.GetManagementAccess(ctx, venueID, userID)
	if err != nil {
		return nil, s.passthroughOrInternal(err, "get management access failed")
	}
	return &crmv1.GetManagementAccessResponse{Access: access}, nil
}

func (s *Server) BatchGetManagementAccess(ctx context.Context, req *crmv1.BatchGetManagementAccessRequest) (*crmv1.BatchGetManagementAccessResponse, error) {
	userID, err := parseUUIDArg(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(req.GetVenueIds()))
	for _, raw := range req.GetVenueIds() {
		id, perr := parseUUIDArg(raw, "venue_ids")
		if perr != nil {
			return nil, perr
		}
		ids = append(ids, id)
	}
	access, err := s.uc.BatchGetManagementAccess(ctx, userID, ids)
	if err != nil {
		return nil, s.passthroughOrInternal(err, "batch get management access failed")
	}
	out := &crmv1.BatchGetManagementAccessResponse{
		Access: make(map[string]string, len(access)),
	}
	for id, a := range access {
		out.Access[id.String()] = a
	}
	return out, nil
}

func (s *Server) ListManagedVenues(ctx context.Context, req *crmv1.ListManagedVenuesRequest) (*crmv1.ListManagedVenuesResponse, error) {
	userID, err := parseUUIDArg(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	venues, err := s.uc.ListManagedVenues(ctx, userID)
	if err != nil {
		return nil, s.passthroughOrInternal(err, "list managed venues failed")
	}
	out := &crmv1.ListManagedVenuesResponse{Venues: make([]*crmv1.ManagedVenue, len(venues))}
	for i, v := range venues {
		out.Venues[i] = &crmv1.ManagedVenue{
			VenueId: v.VenueID.String(),
			Access:  v.Access,
		}
	}
	return out, nil
}

func (s *Server) ListStaff(ctx context.Context, req *crmv1.ListStaffRequest) (*crmv1.ListStaffResponse, error) {
	venueID, err := parseUUIDArg(req.GetVenueId(), "venue_id")
	if err != nil {
		return nil, err
	}
	actorID, err := parseUUIDArg(req.GetActorId(), "actor_id")
	if err != nil {
		return nil, err
	}
	members, err := s.uc.ListStaff(ctx, venueID, actorID)
	if err != nil {
		return nil, s.passthroughOrInternal(err, "list staff failed")
	}
	out := &crmv1.ListStaffResponse{Members: make([]*crmv1.StaffMember, len(members))}
	for i := range members {
		m := &members[i]
		out.Members[i] = &crmv1.StaffMember{
			VenueId:   m.VenueID.String(),
			UserId:    m.UserID.String(),
			Role:      m.Role,
			InvitedBy: m.InvitedBy.String(),
			CreatedAt: timestamppb.New(m.CreatedAt),
		}
	}
	return out, nil
}

func (s *Server) AddStaff(ctx context.Context, req *crmv1.AddStaffRequest) (*crmv1.AddStaffResponse, error) {
	venueID, err := parseUUIDArg(req.GetVenueId(), "venue_id")
	if err != nil {
		return nil, err
	}
	actorID, err := parseUUIDArg(req.GetActorId(), "actor_id")
	if err != nil {
		return nil, err
	}
	targetID, err := parseUUIDArg(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	if err := s.uc.AddStaff(ctx, venueID, actorID, targetID, req.GetRole()); err != nil {
		return nil, s.passthroughOrInternal(err, "add staff failed")
	}
	return &crmv1.AddStaffResponse{}, nil
}

func (s *Server) RemoveStaff(ctx context.Context, req *crmv1.RemoveStaffRequest) (*crmv1.RemoveStaffResponse, error) {
	venueID, err := parseUUIDArg(req.GetVenueId(), "venue_id")
	if err != nil {
		return nil, err
	}
	actorID, err := parseUUIDArg(req.GetActorId(), "actor_id")
	if err != nil {
		return nil, err
	}
	targetID, err := parseUUIDArg(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	if err := s.uc.RemoveStaff(ctx, venueID, actorID, targetID); err != nil {
		return nil, s.passthroughOrInternal(err, "remove staff failed")
	}
	return &crmv1.RemoveStaffResponse{}, nil
}

func (s *Server) ListTasks(ctx context.Context, req *crmv1.ListTasksRequest) (*crmv1.ListTasksResponse, error) {
	venueID, err := parseUUIDArg(req.GetVenueId(), "venue_id")
	if err != nil {
		return nil, err
	}
	actorID, err := parseUUIDArg(req.GetActorId(), "actor_id")
	if err != nil {
		return nil, err
	}
	tasks, err := s.uc.ListTasks(ctx, venueID, actorID, req.GetStatus())
	if err != nil {
		return nil, s.passthroughOrInternal(err, "list tasks failed")
	}
	out := &crmv1.ListTasksResponse{Tasks: make([]*crmv1.Task, len(tasks))}
	for i := range tasks {
		out.Tasks[i] = taskToProto(&tasks[i])
	}
	return out, nil
}

func (s *Server) CreateTask(ctx context.Context, req *crmv1.CreateTaskRequest) (*crmv1.CreateTaskResponse, error) {
	venueID, err := parseUUIDArg(req.GetVenueId(), "venue_id")
	if err != nil {
		return nil, err
	}
	actorID, err := parseUUIDArg(req.GetActorId(), "actor_id")
	if err != nil {
		return nil, err
	}
	bookingID, err := optionalUUIDArg(req.GetBookingId(), "booking_id")
	if err != nil {
		return nil, err
	}
	assignee, err := optionalUUIDArg(req.GetAssigneeUserId(), "assignee_user_id")
	if err != nil {
		return nil, err
	}
	t, err := s.uc.CreateTask(ctx, venueID, actorID, req.GetTitle(), req.GetBody(), bookingID, assignee, req.GetPriority(), protoToTimePtr(req.GetDueAt()))
	if err != nil {
		return nil, s.passthroughOrInternal(err, "create task failed")
	}
	return &crmv1.CreateTaskResponse{Task: taskToProto(t)}, nil
}

func (s *Server) UpdateTask(ctx context.Context, req *crmv1.UpdateTaskRequest) (*crmv1.UpdateTaskResponse, error) {
	venueID, err := parseUUIDArg(req.GetVenueId(), "venue_id")
	if err != nil {
		return nil, err
	}
	actorID, err := parseUUIDArg(req.GetActorId(), "actor_id")
	if err != nil {
		return nil, err
	}
	taskID, err := parseUUIDArg(req.GetTaskId(), "task_id")
	if err != nil {
		return nil, err
	}
	assignee, err := optionalUUIDArg(req.GetAssigneeUserId(), "assignee_user_id")
	if err != nil {
		return nil, err
	}
	t, err := s.uc.UpdateTask(ctx, venueID, actorID, taskID, req.GetTitle(), req.GetBody(), assignee, req.GetPriority(), protoToTimePtr(req.GetDueAt()))
	if err != nil {
		return nil, s.passthroughOrInternal(err, "update task failed")
	}
	return &crmv1.UpdateTaskResponse{Task: taskToProto(t)}, nil
}

func (s *Server) ReopenTask(ctx context.Context, req *crmv1.ReopenTaskRequest) (*crmv1.ReopenTaskResponse, error) {
	venueID, err := parseUUIDArg(req.GetVenueId(), "venue_id")
	if err != nil {
		return nil, err
	}
	actorID, err := parseUUIDArg(req.GetActorId(), "actor_id")
	if err != nil {
		return nil, err
	}
	taskID, err := parseUUIDArg(req.GetTaskId(), "task_id")
	if err != nil {
		return nil, err
	}
	t, err := s.uc.ReopenTask(ctx, venueID, actorID, taskID)
	if err != nil {
		return nil, s.passthroughOrInternal(err, "reopen task failed")
	}
	return &crmv1.ReopenTaskResponse{Task: taskToProto(t)}, nil
}

func (s *Server) CancelTask(ctx context.Context, req *crmv1.CancelTaskRequest) (*crmv1.CancelTaskResponse, error) {
	venueID, err := parseUUIDArg(req.GetVenueId(), "venue_id")
	if err != nil {
		return nil, err
	}
	actorID, err := parseUUIDArg(req.GetActorId(), "actor_id")
	if err != nil {
		return nil, err
	}
	taskID, err := parseUUIDArg(req.GetTaskId(), "task_id")
	if err != nil {
		return nil, err
	}
	if err := s.uc.CancelTask(ctx, venueID, actorID, taskID); err != nil {
		return nil, s.passthroughOrInternal(err, "cancel task failed")
	}
	return &crmv1.CancelTaskResponse{}, nil
}

func (s *Server) ListGuests(ctx context.Context, req *crmv1.ListGuestsRequest) (*crmv1.ListGuestsResponse, error) {
	venueID, err := parseUUIDArg(req.GetVenueId(), "venue_id")
	if err != nil {
		return nil, err
	}
	actorID, err := parseUUIDArg(req.GetActorId(), "actor_id")
	if err != nil {
		return nil, err
	}
	views, total, err := s.uc.ListGuests(ctx, venueID, actorID,
		req.GetSegment(), req.GetSort(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, s.passthroughOrInternal(err, "list guests failed")
	}
	out := &crmv1.ListGuestsResponse{
		Guests: make([]*crmv1.GuestProfile, len(views)),
		Total:  int32(total),
	}
	for i := range views {
		out.Guests[i] = guestViewToProto(&views[i])
	}
	return out, nil
}

func (s *Server) GetGuest(ctx context.Context, req *crmv1.GetGuestRequest) (*crmv1.GetGuestResponse, error) {
	venueID, err := parseUUIDArg(req.GetVenueId(), "venue_id")
	if err != nil {
		return nil, err
	}
	actorID, err := parseUUIDArg(req.GetActorId(), "actor_id")
	if err != nil {
		return nil, err
	}
	userID, err := parseUUIDArg(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	view, bookings, err := s.uc.GetGuest(ctx, venueID, actorID, userID)
	if err != nil {
		return nil, s.passthroughOrInternal(err, "get guest failed")
	}
	out := &crmv1.GetGuestResponse{
		Profile:        guestViewToProto(view),
		RecentBookings: make([]*crmv1.GuestBookingSummary, len(bookings)),
	}
	for i := range bookings {
		out.RecentBookings[i] = guestBookingToProto(&bookings[i])
	}
	return out, nil
}

func guestViewToProto(v *usecase.GuestView) *crmv1.GuestProfile {
	p := &v.Profile
	return &crmv1.GuestProfile{
		VenueId:            p.VenueID.String(),
		UserId:             p.UserID.String(),
		BookingsCount:      p.BookingsCount,
		VisitsCount:        p.VisitsCount,
		CancellationsCount: p.CancellationsCount,
		NoShowCount:        p.NoShowCount,
		TotalSpent:         p.TotalSpent,
		FirstVisitAt:       timePtrToProto(p.FirstVisitAt),
		LastVisitAt:        timePtrToProto(p.LastVisitAt),
		LastBookingAt:      timePtrToProto(p.LastBookingAt),
		Segments:           v.Segments,
	}
}

func guestBookingToProto(b *domain.GuestBookingSummary) *crmv1.GuestBookingSummary {
	return &crmv1.GuestBookingSummary{
		BookingId:  b.BookingID.String(),
		Status:     b.Status,
		TotalPrice: b.TotalPrice,
		VisitDate:  timePtrToProto(b.VisitDate),
		Guests:     b.Guests,
	}
}

func (s *Server) CompleteTask(ctx context.Context, req *crmv1.CompleteTaskRequest) (*crmv1.CompleteTaskResponse, error) {
	venueID, err := parseUUIDArg(req.GetVenueId(), "venue_id")
	if err != nil {
		return nil, err
	}
	actorID, err := parseUUIDArg(req.GetActorId(), "actor_id")
	if err != nil {
		return nil, err
	}
	taskID, err := parseUUIDArg(req.GetTaskId(), "task_id")
	if err != nil {
		return nil, err
	}
	if err := s.uc.CompleteTask(ctx, venueID, actorID, taskID); err != nil {
		return nil, s.passthroughOrInternal(err, "complete task failed")
	}
	return &crmv1.CompleteTaskResponse{}, nil
}

func taskToProto(t *domain.Task) *crmv1.Task {
	p := &crmv1.Task{
		Id:          t.ID.String(),
		VenueId:     t.VenueID.String(),
		Title:       t.Title,
		Body:        t.Body,
		Status:      t.Status,
		Priority:    t.Priority,
		CreatedBy:   t.CreatedBy.String(),
		CreatedAt:   timestamppb.New(t.CreatedAt),
		UpdatedAt:   timestamppb.New(t.UpdatedAt),
		DueAt:       timePtrToProto(t.DueAt),
		CompletedAt: timePtrToProto(t.CompletedAt),
	}
	if t.BookingID != nil {
		s := t.BookingID.String()
		p.BookingId = &s
	}
	if t.AssigneeUserID != nil {
		s := t.AssigneeUserID.String()
		p.AssigneeUserId = &s
	}
	if t.CompletedBy != nil {
		p.CompletedBy = t.CompletedBy.String()
	}
	return p
}

// protoToTimePtr converts an optional proto timestamp to *time.Time (nil-safe).
func protoToTimePtr(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

// timePtrToProto converts an optional time to a proto timestamp (nil-safe).
func timePtrToProto(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}
