package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/crm-service/internal/domain"
	"github.com/tienlao/agregator/services/crm-service/internal/repository"
)

var errBoom = errors.New("boom")

func TestGetManagementAccess_Passthrough(t *testing.T) {
	venueID, userID := uuid.New(), uuid.New()
	repo := &mockRepo{
		GetManagementAccessFunc: func(_ context.Context, v, u uuid.UUID) (string, error) {
			assert.Equal(t, venueID, v)
			assert.Equal(t, userID, u)
			return domain.AccessManager, nil
		},
	}
	got, err := New(repo).GetManagementAccess(context.Background(), venueID, userID)
	require.NoError(t, err)
	assert.Equal(t, domain.AccessManager, got)
}

func TestBatchGetManagementAccess_Passthrough(t *testing.T) {
	userID := uuid.New()
	want := map[uuid.UUID]string{uuid.New(): domain.AccessOwner}
	repo := &mockRepo{
		BatchGetManagementAccessFunc: func(_ context.Context, u uuid.UUID, _ []uuid.UUID) (map[uuid.UUID]string, error) {
			assert.Equal(t, userID, u)
			return want, nil
		},
	}
	got, err := New(repo).BatchGetManagementAccess(context.Background(), userID, nil)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestListManagedVenues_Passthrough(t *testing.T) {
	want := []domain.ManagedVenue{{VenueID: uuid.New(), Access: domain.AccessStaff}}
	repo := &mockRepo{
		ListManagedVenuesFunc: func(_ context.Context, _ uuid.UUID) ([]domain.ManagedVenue, error) {
			return want, nil
		},
	}
	got, err := New(repo).ListManagedVenues(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestListStaff(t *testing.T) {
	venueID, actorID := uuid.New(), uuid.New()
	tests := []struct {
		name     string
		access   string
		accesser error
		wantCode codes.Code
		wantErr  bool
	}{
		{name: "owner_ok", access: domain.AccessOwner},
		{name: "staff_ok", access: domain.AccessStaff},
		{name: "no_access", access: "", wantErr: true, wantCode: codes.PermissionDenied},
		{name: "repo_error", accesser: errBoom, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{
				GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
					return tt.access, tt.accesser
				},
				ListStaffFunc: func(_ context.Context, _ uuid.UUID) ([]domain.StaffMember, error) {
					return []domain.StaffMember{{VenueID: venueID}}, nil
				},
			}
			got, err := New(repo).ListStaff(context.Background(), venueID, actorID)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantCode != codes.OK {
					assert.Equal(t, tt.wantCode, status.Code(err))
				}
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, 1)
		})
	}
}

func TestAddStaff(t *testing.T) {
	venueID := uuid.New()
	owner := uuid.New()
	target := uuid.New()

	tests := []struct {
		name       string
		actor      uuid.UUID
		target     uuid.UUID
		role       string
		ownerErr   error
		wantErr    bool
		wantCode   codes.Code
		wantRole   string // expected normalized role passed to repo
		expectRepo bool
	}{
		{name: "owner_adds_manager", actor: owner, target: target, role: "manager", wantRole: domain.StaffRoleManager, expectRepo: true},
		{name: "normalizes_role", actor: owner, target: target, role: "  STAFF ", wantRole: domain.StaffRoleStaff, expectRepo: true},
		{name: "not_owner", actor: uuid.New(), target: target, role: "staff", wantErr: true, wantCode: codes.PermissionDenied},
		{name: "venue_not_found", actor: owner, target: target, role: "staff", ownerErr: repository.ErrVenueNotFound, wantErr: true, wantCode: codes.NotFound},
		{name: "owner_err_passthrough", actor: owner, target: target, role: "staff", ownerErr: errBoom, wantErr: true},
		{name: "target_is_owner", actor: owner, target: owner, role: "staff", wantErr: true, wantCode: codes.InvalidArgument},
		{name: "invalid_role", actor: owner, target: target, role: "admin", wantErr: true, wantCode: codes.InvalidArgument},
		{name: "empty_role", actor: owner, target: target, role: "", wantErr: true, wantCode: codes.InvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotRole string
			var called bool
			repo := &mockRepo{
				VenueOwnerIDFunc: func(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
					return owner, tt.ownerErr
				},
				AddStaffFunc: func(_ context.Context, v, u uuid.UUID, role string, invitedBy uuid.UUID) error {
					called = true
					gotRole = role
					assert.Equal(t, venueID, v)
					assert.Equal(t, tt.target, u)
					assert.Equal(t, tt.actor, invitedBy)
					return nil
				},
			}
			err := New(repo).AddStaff(context.Background(), venueID, tt.actor, tt.target, tt.role)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantCode != codes.OK {
					assert.Equal(t, tt.wantCode, status.Code(err))
				}
				assert.False(t, called, "repo must not be called on error")
				return
			}
			require.NoError(t, err)
			assert.True(t, tt.expectRepo && called)
			assert.Equal(t, tt.wantRole, gotRole)
		})
	}
}

func TestRemoveStaff(t *testing.T) {
	venueID := uuid.New()
	owner := uuid.New()
	target := uuid.New()

	tests := []struct {
		name      string
		actor     uuid.UUID
		target    uuid.UUID
		ownerErr  error
		removeErr error
		wantErr   bool
		wantCode  codes.Code
	}{
		{name: "owner_removes", actor: owner, target: target},
		{name: "not_owner", actor: uuid.New(), target: target, wantErr: true, wantCode: codes.PermissionDenied},
		{name: "target_is_owner", actor: owner, target: owner, wantErr: true, wantCode: codes.InvalidArgument},
		{name: "staff_not_found", actor: owner, target: target, removeErr: repository.ErrStaffNotFound, wantErr: true, wantCode: codes.NotFound},
		{name: "repo_err_passthrough", actor: owner, target: target, removeErr: errBoom, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{
				VenueOwnerIDFunc: func(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
					return owner, tt.ownerErr
				},
				RemoveStaffFunc: func(_ context.Context, _, _ uuid.UUID) error {
					return tt.removeErr
				},
			}
			err := New(repo).RemoveStaff(context.Background(), venueID, tt.actor, tt.target)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantCode != codes.OK {
					assert.Equal(t, tt.wantCode, status.Code(err))
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestListTasks(t *testing.T) {
	venueID, actorID := uuid.New(), uuid.New()
	tests := []struct {
		name       string
		access     string
		status     string
		wantStatus string // normalized status forwarded to repo
		wantErr    bool
		wantCode   codes.Code
	}{
		{name: "no_filter", access: domain.AccessOwner, status: "", wantStatus: ""},
		{name: "open", access: domain.AccessStaff, status: "open", wantStatus: "open"},
		{name: "done_normalized", access: domain.AccessManager, status: " DONE ", wantStatus: "done"},
		{name: "invalid_status", access: domain.AccessOwner, status: "weird", wantErr: true, wantCode: codes.InvalidArgument},
		{name: "no_access", access: "", status: "", wantErr: true, wantCode: codes.PermissionDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotStatus string
			repo := &mockRepo{
				GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
					return tt.access, nil
				},
				ListTasksFunc: func(_ context.Context, _ uuid.UUID, status string) ([]domain.Task, error) {
					gotStatus = status
					return []domain.Task{{ID: uuid.New()}}, nil
				},
			}
			got, err := New(repo).ListTasks(context.Background(), venueID, actorID, tt.status)
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.wantCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, 1)
			assert.Equal(t, tt.wantStatus, gotStatus)
		})
	}
}

func TestCreateTask(t *testing.T) {
	venueID, actorID := uuid.New(), uuid.New()
	bookingID, assignee := uuid.New(), uuid.New()

	tests := []struct {
		name      string
		access    string
		title     string
		body      string
		wantErr   bool
		wantCode  codes.Code
		wantTitle string
	}{
		{name: "ok", access: domain.AccessOwner, title: "  Позвонить клиенту ", body: "детали", wantTitle: "Позвонить клиенту"},
		{name: "no_access", access: "", title: "x", wantErr: true, wantCode: codes.PermissionDenied},
		{name: "empty_title", access: domain.AccessOwner, title: "   ", wantErr: true, wantCode: codes.InvalidArgument},
		{name: "title_too_long", access: domain.AccessOwner, title: strings.Repeat("я", maxTaskTitleRunes+1), wantErr: true, wantCode: codes.InvalidArgument},
		{name: "body_too_long", access: domain.AccessOwner, title: "ok", body: strings.Repeat("б", maxTaskBodyRunes+1), wantErr: true, wantCode: codes.InvalidArgument},
		{name: "title_at_limit", access: domain.AccessOwner, title: strings.Repeat("я", maxTaskTitleRunes), wantTitle: strings.Repeat("я", maxTaskTitleRunes)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured *domain.Task
			repo := &mockRepo{
				GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
					return tt.access, nil
				},
				CreateTaskFunc: func(_ context.Context, task *domain.Task) error {
					captured = task
					task.ID = uuid.New()
					return nil
				},
			}
			got, err := New(repo).CreateTask(context.Background(), venueID, actorID, tt.title, tt.body, &bookingID, &assignee)
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.wantCode, status.Code(err))
				assert.Nil(t, captured, "repo must not be called on validation error")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantTitle, got.Title)
			assert.Equal(t, domain.TaskStatusOpen, got.Status)
			assert.Equal(t, actorID, got.CreatedBy)
			assert.Equal(t, venueID, got.VenueID)
			require.NotNil(t, got.BookingID)
			assert.Equal(t, bookingID, *got.BookingID)
			require.NotNil(t, got.AssigneeUserID)
			assert.Equal(t, assignee, *got.AssigneeUserID)
		})
	}
}

func TestCreateTask_RepoError(t *testing.T) {
	repo := &mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return domain.AccessOwner, nil
		},
		CreateTaskFunc: func(_ context.Context, _ *domain.Task) error { return errBoom },
	}
	got, err := New(repo).CreateTask(context.Background(), uuid.New(), uuid.New(), "title", "", nil, nil)
	require.ErrorIs(t, err, errBoom)
	assert.Nil(t, got)
}

func TestCompleteTask(t *testing.T) {
	venueID, actorID, taskID := uuid.New(), uuid.New(), uuid.New()
	tests := []struct {
		name       string
		access     string
		completed  bool
		completeEr error
		wantErr    bool
		wantCode   codes.Code
	}{
		{name: "ok", access: domain.AccessStaff, completed: true},
		{name: "not_found_or_closed", access: domain.AccessOwner, completed: false, wantErr: true, wantCode: codes.NotFound},
		{name: "no_access", access: "", wantErr: true, wantCode: codes.PermissionDenied},
		{name: "repo_err", access: domain.AccessOwner, completeEr: errBoom, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{
				GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
					return tt.access, nil
				},
				CompleteTaskFunc: func(_ context.Context, _, _ uuid.UUID) (bool, error) {
					return tt.completed, tt.completeEr
				},
			}
			err := New(repo).CompleteTask(context.Background(), venueID, actorID, taskID)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantCode != codes.OK {
					assert.Equal(t, tt.wantCode, status.Code(err))
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestEnsureMember_RepoError(t *testing.T) {
	repo := &mockRepo{
		GetManagementAccessFunc: func(_ context.Context, _, _ uuid.UUID) (string, error) {
			return "", errBoom
		},
	}
	_, err := New(repo).ListStaff(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, errBoom)
}
