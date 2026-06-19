package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	pkgerrors "github.com/tienlao/agregator/pkg/errors"

	"github.com/tienlao/agregator/services/crm-service/internal/domain"
	"github.com/tienlao/agregator/services/crm-service/internal/repository"
	"github.com/tienlao/agregator/services/crm-service/internal/usecase"
)

var errBoom = errors.New("boom")

func newServer(repo *mockRepo) *Server {
	return NewServer(usecase.New(repo), zerolog.Nop())
}

func TestParseUUIDArg(t *testing.T) {
	valid := uuid.New()
	got, err := parseUUIDArg(valid.String(), "venue_id")
	require.NoError(t, err)
	assert.Equal(t, valid, got)

	_, err = parseUUIDArg("not-a-uuid", "venue_id")
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestOptionalUUIDArg(t *testing.T) {
	id := uuid.New()
	tests := []struct {
		name    string
		raw     string
		wantNil bool
		wantErr bool
	}{
		{name: "empty", raw: "", wantNil: true},
		{name: "whitespace", raw: "   ", wantNil: true},
		{name: "valid", raw: id.String()},
		{name: "invalid", raw: "bad", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := optionalUUIDArg(tt.raw, "booking_id")
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, codes.InvalidArgument, status.Code(err))
				return
			}
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, id, *got)
		})
	}
}

func TestPassthroughOrInternal(t *testing.T) {
	s := newServer(&mockRepo{})

	domainErr := pkgerrors.NotFound("missing")
	assert.Equal(t, domainErr, s.passthroughOrInternal(domainErr, "msg"))

	got := s.passthroughOrInternal(errBoom, "msg")
	assert.Equal(t, codes.Internal, status.Code(got))
	assert.Equal(t, internalErrMsg, status.Convert(got).Message())
}

func TestTaskToProto(t *testing.T) {
	now := time.Now()
	booking := uuid.New()
	assignee := uuid.New()
	completer := uuid.New()
	full := &domain.Task{
		ID: uuid.New(), VenueID: uuid.New(), BookingID: &booking,
		Title: "t", Body: "b", Status: domain.TaskStatusDone, Priority: domain.TaskPriorityHigh,
		AssigneeUserID: &assignee, DueAt: &now, CreatedBy: uuid.New(),
		CompletedBy: &completer, CompletedAt: &now,
		CreatedAt: now, UpdatedAt: now,
	}
	p := taskToProto(full)
	assert.Equal(t, full.ID.String(), p.GetId())
	assert.Equal(t, booking.String(), p.GetBookingId())
	assert.Equal(t, assignee.String(), p.GetAssigneeUserId())
	assert.Equal(t, "t", p.GetTitle())
	assert.Equal(t, domain.TaskPriorityHigh, p.GetPriority())
	assert.Equal(t, completer.String(), p.GetCompletedBy())
	assert.NotNil(t, p.GetDueAt())
	assert.NotNil(t, p.GetCompletedAt())

	bare := &domain.Task{ID: uuid.New(), VenueID: uuid.New(), CreatedBy: uuid.New(), CreatedAt: now, UpdatedAt: now}
	pb := taskToProto(bare)
	assert.Nil(t, pb.BookingId)
	assert.Nil(t, pb.AssigneeUserId)
	assert.Nil(t, pb.DueAt)
	assert.Nil(t, pb.CompletedAt)
	assert.Empty(t, pb.GetCompletedBy())
}

func TestGetManagementAccess(t *testing.T) {
	venueID, userID := uuid.New(), uuid.New()
	s := newServer(&mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return domain.AccessManager, nil
		},
	})
	resp, err := s.GetManagementAccess(context.Background(), &crmv1.GetManagementAccessRequest{
		VenueId: venueID.String(), UserId: userID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, domain.AccessManager, resp.GetAccess())

	_, err = s.GetManagementAccess(context.Background(), &crmv1.GetManagementAccessRequest{VenueId: "bad", UserId: userID.String()})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGetManagementAccess_RepoErrorIsInternal(t *testing.T) {
	s := newServer(&mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return "", errBoom
		},
	})
	_, err := s.GetManagementAccess(context.Background(), &crmv1.GetManagementAccessRequest{
		VenueId: uuid.NewString(), UserId: uuid.NewString(),
	})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestBatchGetManagementAccess(t *testing.T) {
	v1, v2 := uuid.New(), uuid.New()
	s := newServer(&mockRepo{
		BatchGetManagementAccessFunc: func(_ context.Context, _ uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error) {
			assert.Len(t, ids, 2)
			return map[uuid.UUID]string{v1: domain.AccessOwner}, nil
		},
	})
	resp, err := s.BatchGetManagementAccess(context.Background(), &crmv1.BatchGetManagementAccessRequest{
		UserId: uuid.NewString(), VenueIds: []string{v1.String(), v2.String()},
	})
	require.NoError(t, err)
	assert.Equal(t, domain.AccessOwner, resp.GetAccess()[v1.String()])

	_, err = s.BatchGetManagementAccess(context.Background(), &crmv1.BatchGetManagementAccessRequest{
		UserId: uuid.NewString(), VenueIds: []string{"bad"},
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestListManagedVenues(t *testing.T) {
	mv := domain.ManagedVenue{VenueID: uuid.New(), Access: domain.AccessStaff}
	s := newServer(&mockRepo{
		ListManagedVenuesFunc: func(_ context.Context, _ uuid.UUID) ([]domain.ManagedVenue, error) {
			return []domain.ManagedVenue{mv}, nil
		},
	})
	resp, err := s.ListManagedVenues(context.Background(), &crmv1.ListManagedVenuesRequest{UserId: uuid.NewString()})
	require.NoError(t, err)
	require.Len(t, resp.GetVenues(), 1)
	assert.Equal(t, mv.VenueID.String(), resp.GetVenues()[0].GetVenueId())
	assert.Equal(t, domain.AccessStaff, resp.GetVenues()[0].GetAccess())
}

func TestListStaff(t *testing.T) {
	venueID := uuid.New()
	member := domain.StaffMember{
		VenueID: venueID, UserID: uuid.New(), Role: domain.StaffRoleManager,
		InvitedBy: uuid.New(), CreatedAt: time.Now(),
	}
	s := newServer(&mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return domain.AccessOwner, nil
		},
		ListStaffFunc: func(_ context.Context, _ uuid.UUID) ([]domain.StaffMember, error) {
			return []domain.StaffMember{member}, nil
		},
	})
	resp, err := s.ListStaff(context.Background(), &crmv1.ListStaffRequest{
		VenueId: venueID.String(), ActorId: uuid.NewString(),
	})
	require.NoError(t, err)
	require.Len(t, resp.GetMembers(), 1)
	assert.Equal(t, member.UserID.String(), resp.GetMembers()[0].GetUserId())
}

func TestListStaff_PermissionDeniedPassthrough(t *testing.T) {
	s := newServer(&mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return "", nil // no access
		},
	})
	_, err := s.ListStaff(context.Background(), &crmv1.ListStaffRequest{
		VenueId: uuid.NewString(), ActorId: uuid.NewString(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestListStaff_InvalidArgs(t *testing.T) {
	s := newServer(&mockRepo{})
	_, err := s.ListStaff(context.Background(), &crmv1.ListStaffRequest{VenueId: "bad", ActorId: uuid.NewString()})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = s.ListStaff(context.Background(), &crmv1.ListStaffRequest{VenueId: uuid.NewString(), ActorId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestAddStaff(t *testing.T) {
	owner := uuid.New()
	s := newServer(&mockRepo{
		VenueOwnerIDFunc: func(_ context.Context, _ uuid.UUID) (uuid.UUID, error) { return owner, nil },
	})
	_, err := s.AddStaff(context.Background(), &crmv1.AddStaffRequest{
		VenueId: uuid.NewString(), ActorId: owner.String(), UserId: uuid.NewString(), Role: "manager",
	})
	require.NoError(t, err)

	// invalid role surfaces as InvalidArgument from usecase via passthrough.
	_, err = s.AddStaff(context.Background(), &crmv1.AddStaffRequest{
		VenueId: uuid.NewString(), ActorId: owner.String(), UserId: uuid.NewString(), Role: "root",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestRemoveStaff(t *testing.T) {
	owner := uuid.New()
	s := newServer(&mockRepo{
		VenueOwnerIDFunc: func(_ context.Context, _ uuid.UUID) (uuid.UUID, error) { return owner, nil },
		RemoveStaffFunc:  func(_ context.Context, _, _ uuid.UUID) error { return repository.ErrStaffNotFound },
	})
	_, err := s.RemoveStaff(context.Background(), &crmv1.RemoveStaffRequest{
		VenueId: uuid.NewString(), ActorId: owner.String(), UserId: uuid.NewString(),
	})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestListTasks(t *testing.T) {
	s := newServer(&mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return domain.AccessOwner, nil
		},
		ListTasksFunc: func(_ context.Context, _ uuid.UUID, _ string) ([]domain.Task, error) {
			return []domain.Task{{ID: uuid.New(), Status: domain.TaskStatusOpen, CreatedAt: time.Now(), UpdatedAt: time.Now()}}, nil
		},
	})
	resp, err := s.ListTasks(context.Background(), &crmv1.ListTasksRequest{
		VenueId: uuid.NewString(), ActorId: uuid.NewString(), Status: "open",
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetTasks(), 1)
}

func TestCreateTask(t *testing.T) {
	booking := uuid.New()
	s := newServer(&mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return domain.AccessOwner, nil
		},
		CreateTaskFunc: func(_ context.Context, task *domain.Task) error {
			task.ID = uuid.New()
			task.CreatedAt = time.Now()
			task.UpdatedAt = time.Now()
			return nil
		},
	})
	bookingStr := booking.String()
	resp, err := s.CreateTask(context.Background(), &crmv1.CreateTaskRequest{
		VenueId: uuid.NewString(), ActorId: uuid.NewString(),
		Title: "Позвонить", Body: "детали", BookingId: &bookingStr,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.GetTask())
	assert.Equal(t, "Позвонить", resp.GetTask().GetTitle())
	assert.Equal(t, booking.String(), resp.GetTask().GetBookingId())

	// empty title → InvalidArgument from usecase.
	_, err = s.CreateTask(context.Background(), &crmv1.CreateTaskRequest{
		VenueId: uuid.NewString(), ActorId: uuid.NewString(), Title: "  ",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// invalid optional booking_id → InvalidArgument at delivery.
	bad := "nope"
	_, err = s.CreateTask(context.Background(), &crmv1.CreateTaskRequest{
		VenueId: uuid.NewString(), ActorId: uuid.NewString(), Title: "x", BookingId: &bad,
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCompleteTask(t *testing.T) {
	s := newServer(&mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return domain.AccessOwner, nil
		},
		CompleteTaskFunc: func(_ context.Context, _, _, _ uuid.UUID) (bool, error) { return true, nil },
	})
	_, err := s.CompleteTask(context.Background(), &crmv1.CompleteTaskRequest{
		VenueId: uuid.NewString(), ActorId: uuid.NewString(), TaskId: uuid.NewString(),
	})
	require.NoError(t, err)

	_, err = s.CompleteTask(context.Background(), &crmv1.CompleteTaskRequest{
		VenueId: uuid.NewString(), ActorId: uuid.NewString(), TaskId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestUpdateTask(t *testing.T) {
	s := newServer(&mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return domain.AccessManager, nil
		},
		UpdateTaskFunc: func(_ context.Context, task *domain.Task) error {
			task.Status = domain.TaskStatusOpen
			task.CreatedAt = time.Now()
			task.UpdatedAt = time.Now()
			return nil
		},
	})
	resp, err := s.UpdateTask(context.Background(), &crmv1.UpdateTaskRequest{
		VenueId: uuid.NewString(), ActorId: uuid.NewString(), TaskId: uuid.NewString(),
		Title: "Обновлено", Priority: "high",
	})
	require.NoError(t, err)
	assert.Equal(t, "Обновлено", resp.GetTask().GetTitle())
	assert.Equal(t, "high", resp.GetTask().GetPriority())

	// staff role surfaces PermissionDenied from the usecase.
	sd := newServer(&mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return domain.AccessStaff, nil
		},
	})
	_, err = sd.UpdateTask(context.Background(), &crmv1.UpdateTaskRequest{
		VenueId: uuid.NewString(), ActorId: uuid.NewString(), TaskId: uuid.NewString(), Title: "x",
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestReopenTask(t *testing.T) {
	s := newServer(&mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return domain.AccessOwner, nil
		},
		ReopenTaskFunc: func(_ context.Context, _, _ uuid.UUID) (*domain.Task, error) {
			return &domain.Task{ID: uuid.New(), Status: domain.TaskStatusOpen, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
		},
	})
	resp, err := s.ReopenTask(context.Background(), &crmv1.ReopenTaskRequest{
		VenueId: uuid.NewString(), ActorId: uuid.NewString(), TaskId: uuid.NewString(),
	})
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusOpen, resp.GetTask().GetStatus())

	_, err = s.ReopenTask(context.Background(), &crmv1.ReopenTaskRequest{
		VenueId: "bad", ActorId: uuid.NewString(), TaskId: uuid.NewString(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCancelTask(t *testing.T) {
	s := newServer(&mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return domain.AccessOwner, nil
		},
		CancelTaskFunc: func(_ context.Context, _, _ uuid.UUID) (bool, error) { return true, nil },
	})
	_, err := s.CancelTask(context.Background(), &crmv1.CancelTaskRequest{
		VenueId: uuid.NewString(), ActorId: uuid.NewString(), TaskId: uuid.NewString(),
	})
	require.NoError(t, err)

	// not found maps to NotFound.
	sn := newServer(&mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return domain.AccessOwner, nil
		},
		CancelTaskFunc: func(_ context.Context, _, _ uuid.UUID) (bool, error) { return false, nil },
	})
	_, err = sn.CancelTask(context.Background(), &crmv1.CancelTaskRequest{
		VenueId: uuid.NewString(), ActorId: uuid.NewString(), TaskId: uuid.NewString(),
	})
	assert.Equal(t, codes.NotFound, status.Code(err))
}
