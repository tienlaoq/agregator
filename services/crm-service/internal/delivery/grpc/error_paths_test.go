package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"

	"github.com/tienlao/agregator/services/crm-service/internal/domain"
)

// TestHandlers_InvalidActorID checks that every handler taking an actor_id
// rejects a malformed one with InvalidArgument before any repo access. venue_id
// is valid so the failure is unambiguously the actor.
func TestHandlers_InvalidActorID(t *testing.T) {
	ctx := context.Background()
	v := uuid.NewString()
	s := newServer(&mockRepo{})

	calls := map[string]func() error{
		"ListStaff": func() error { _, e := s.ListStaff(ctx, &crmv1.ListStaffRequest{VenueId: v, ActorId: "bad"}); return e },
		"AddStaff": func() error {
			_, e := s.AddStaff(ctx, &crmv1.AddStaffRequest{VenueId: v, ActorId: "bad", UserId: uuid.NewString()})
			return e
		},
		"RemoveStaff": func() error {
			_, e := s.RemoveStaff(ctx, &crmv1.RemoveStaffRequest{VenueId: v, ActorId: "bad", UserId: uuid.NewString()})
			return e
		},
		"ListTasks": func() error { _, e := s.ListTasks(ctx, &crmv1.ListTasksRequest{VenueId: v, ActorId: "bad"}); return e },
		"CreateTask": func() error {
			_, e := s.CreateTask(ctx, &crmv1.CreateTaskRequest{VenueId: v, ActorId: "bad", Title: "x"})
			return e
		},
		"UpdateTask": func() error {
			_, e := s.UpdateTask(ctx, &crmv1.UpdateTaskRequest{VenueId: v, ActorId: "bad", TaskId: uuid.NewString()})
			return e
		},
		"CompleteTask": func() error {
			_, e := s.CompleteTask(ctx, &crmv1.CompleteTaskRequest{VenueId: v, ActorId: "bad", TaskId: uuid.NewString()})
			return e
		},
		"ReopenTask": func() error {
			_, e := s.ReopenTask(ctx, &crmv1.ReopenTaskRequest{VenueId: v, ActorId: "bad", TaskId: uuid.NewString()})
			return e
		},
		"CancelTask": func() error {
			_, e := s.CancelTask(ctx, &crmv1.CancelTaskRequest{VenueId: v, ActorId: "bad", TaskId: uuid.NewString()})
			return e
		},
		"GetGuest": func() error {
			_, e := s.GetGuest(ctx, &crmv1.GetGuestRequest{VenueId: v, ActorId: "bad", UserId: uuid.NewString()})
			return e
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, codes.InvalidArgument, status.Code(call()))
		})
	}
}

// TestHandlers_InternalError checks that a non-status repository failure is
// masked as Internal (never leaked) once authorization has passed.
func TestHandlers_InternalError(t *testing.T) {
	ctx := context.Background()
	venue := uuid.NewString()
	owner := uuid.New()

	// ownerAccess grants owner-level management and makes the actor the venue
	// owner, so both member and owner authorization paths succeed.
	ownerAccess := func() *mockRepo {
		return &mockRepo{
			GetManagementAccessFunc: func(context.Context, uuid.UUID, uuid.UUID) (string, error) { return domain.AccessOwner, nil },
			VenueOwnerIDFunc:        func(context.Context, uuid.UUID) (uuid.UUID, error) { return owner, nil },
		}
	}

	tests := []struct {
		name string
		repo *mockRepo
		call func(s *Server) error
	}{
		{
			name: "GetManagementAccess",
			repo: &mockRepo{GetManagementAccessFunc: func(context.Context, uuid.UUID, uuid.UUID) (string, error) { return "", errBoom }},
			call: func(s *Server) error {
				_, e := s.GetManagementAccess(ctx, &crmv1.GetManagementAccessRequest{VenueId: venue, UserId: uuid.NewString()})
				return e
			},
		},
		{
			name: "ListManagedVenues",
			repo: &mockRepo{ListManagedVenuesFunc: func(context.Context, uuid.UUID) ([]domain.ManagedVenue, error) { return nil, errBoom }},
			call: func(s *Server) error {
				_, e := s.ListManagedVenues(ctx, &crmv1.ListManagedVenuesRequest{UserId: uuid.NewString()})
				return e
			},
		},
		{
			name: "ListStaff",
			repo: func() *mockRepo {
				r := ownerAccess()
				r.ListStaffFunc = func(context.Context, uuid.UUID) ([]domain.StaffMember, error) { return nil, errBoom }
				return r
			}(),
			call: func(s *Server) error {
				_, e := s.ListStaff(ctx, &crmv1.ListStaffRequest{VenueId: venue, ActorId: owner.String()})
				return e
			},
		},
		{
			name: "AddStaff",
			repo: func() *mockRepo {
				r := ownerAccess()
				r.AddStaffFunc = func(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID) error { return errBoom }
				return r
			}(),
			call: func(s *Server) error {
				_, e := s.AddStaff(ctx, &crmv1.AddStaffRequest{VenueId: venue, ActorId: owner.String(), UserId: uuid.NewString(), Role: "manager"})
				return e
			},
		},
		{
			name: "RemoveStaff",
			repo: func() *mockRepo {
				r := ownerAccess()
				r.RemoveStaffFunc = func(context.Context, uuid.UUID, uuid.UUID) error { return errBoom }
				return r
			}(),
			call: func(s *Server) error {
				_, e := s.RemoveStaff(ctx, &crmv1.RemoveStaffRequest{VenueId: venue, ActorId: owner.String(), UserId: uuid.NewString()})
				return e
			},
		},
		{
			name: "ListTasks",
			repo: func() *mockRepo {
				r := ownerAccess()
				r.ListTasksFunc = func(context.Context, uuid.UUID, string) ([]domain.Task, error) { return nil, errBoom }
				return r
			}(),
			call: func(s *Server) error {
				_, e := s.ListTasks(ctx, &crmv1.ListTasksRequest{VenueId: venue, ActorId: owner.String()})
				return e
			},
		},
		{
			name: "CreateTask",
			repo: func() *mockRepo {
				r := ownerAccess()
				r.CreateTaskFunc = func(context.Context, *domain.Task) error { return errBoom }
				return r
			}(),
			call: func(s *Server) error {
				_, e := s.CreateTask(ctx, &crmv1.CreateTaskRequest{VenueId: venue, ActorId: owner.String(), Title: "x"})
				return e
			},
		},
		{
			name: "UpdateTask",
			repo: func() *mockRepo {
				r := ownerAccess()
				r.UpdateTaskFunc = func(context.Context, *domain.Task) error { return errBoom }
				return r
			}(),
			call: func(s *Server) error {
				_, e := s.UpdateTask(ctx, &crmv1.UpdateTaskRequest{VenueId: venue, ActorId: owner.String(), TaskId: uuid.NewString(), Title: "x"})
				return e
			},
		},
		{
			name: "CompleteTask",
			repo: func() *mockRepo {
				r := ownerAccess()
				r.CompleteTaskFunc = func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error) { return false, errBoom }
				return r
			}(),
			call: func(s *Server) error {
				_, e := s.CompleteTask(ctx, &crmv1.CompleteTaskRequest{VenueId: venue, ActorId: owner.String(), TaskId: uuid.NewString()})
				return e
			},
		},
		{
			name: "ReopenTask",
			repo: func() *mockRepo {
				r := ownerAccess()
				r.ReopenTaskFunc = func(context.Context, uuid.UUID, uuid.UUID) (*domain.Task, error) { return nil, errBoom }
				return r
			}(),
			call: func(s *Server) error {
				_, e := s.ReopenTask(ctx, &crmv1.ReopenTaskRequest{VenueId: venue, ActorId: owner.String(), TaskId: uuid.NewString()})
				return e
			},
		},
		{
			name: "CancelTask",
			repo: func() *mockRepo {
				r := ownerAccess()
				r.CancelTaskFunc = func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return false, errBoom }
				return r
			}(),
			call: func(s *Server) error {
				_, e := s.CancelTask(ctx, &crmv1.CancelTaskRequest{VenueId: venue, ActorId: owner.String(), TaskId: uuid.NewString()})
				return e
			},
		},
		{
			name: "ListGuests",
			repo: func() *mockRepo {
				r := ownerAccess()
				r.ListGuestsFunc = func(context.Context, uuid.UUID, domain.GuestListParams) ([]domain.GuestProfile, int, error) {
					return nil, 0, errBoom
				}
				return r
			}(),
			call: func(s *Server) error {
				_, e := s.ListGuests(ctx, &crmv1.ListGuestsRequest{VenueId: venue, ActorId: owner.String()})
				return e
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, codes.Internal, status.Code(tt.call(newServer(tt.repo))))
		})
	}
}
