package grpc

import (
	"context"

	"github.com/google/uuid"

	"github.com/tienlao/agregator/services/crm-service/internal/domain"
)

// mockRepo is a func-field mock of domain.Repository used to drive the usecase
// behind the gRPC server. Unset fields return zero values.
type mockRepo struct {
	VenueOwnerIDFunc             func(ctx context.Context, venueID uuid.UUID) (uuid.UUID, error)
	GetManagementAccessFunc      func(ctx context.Context, venueID, userID uuid.UUID) (string, error)
	BatchGetManagementAccessFunc func(ctx context.Context, userID uuid.UUID, venueIDs []uuid.UUID) (map[uuid.UUID]string, error)
	ListManagedVenuesFunc        func(ctx context.Context, userID uuid.UUID) ([]domain.ManagedVenue, error)
	ListStaffFunc                func(ctx context.Context, venueID uuid.UUID) ([]domain.StaffMember, error)
	AddStaffFunc                 func(ctx context.Context, venueID, userID uuid.UUID, role string, invitedBy uuid.UUID) error
	RemoveStaffFunc              func(ctx context.Context, venueID, userID uuid.UUID) error
	ListTasksFunc                func(ctx context.Context, venueID uuid.UUID, status string) ([]domain.Task, error)
	CreateTaskFunc               func(ctx context.Context, t *domain.Task) error
	CompleteTaskFunc             func(ctx context.Context, venueID, taskID uuid.UUID) (bool, error)
}

func (m *mockRepo) VenueOwnerID(ctx context.Context, venueID uuid.UUID) (uuid.UUID, error) {
	if m.VenueOwnerIDFunc != nil {
		return m.VenueOwnerIDFunc(ctx, venueID)
	}
	return uuid.Nil, nil
}

func (m *mockRepo) GetManagementAccess(ctx context.Context, venueID, userID uuid.UUID) (string, error) {
	if m.GetManagementAccessFunc != nil {
		return m.GetManagementAccessFunc(ctx, venueID, userID)
	}
	return "", nil
}

func (m *mockRepo) BatchGetManagementAccess(ctx context.Context, userID uuid.UUID, venueIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	if m.BatchGetManagementAccessFunc != nil {
		return m.BatchGetManagementAccessFunc(ctx, userID, venueIDs)
	}
	return nil, nil
}

func (m *mockRepo) ListManagedVenues(ctx context.Context, userID uuid.UUID) ([]domain.ManagedVenue, error) {
	if m.ListManagedVenuesFunc != nil {
		return m.ListManagedVenuesFunc(ctx, userID)
	}
	return nil, nil
}

func (m *mockRepo) ListStaff(ctx context.Context, venueID uuid.UUID) ([]domain.StaffMember, error) {
	if m.ListStaffFunc != nil {
		return m.ListStaffFunc(ctx, venueID)
	}
	return nil, nil
}

func (m *mockRepo) AddStaff(ctx context.Context, venueID, userID uuid.UUID, role string, invitedBy uuid.UUID) error {
	if m.AddStaffFunc != nil {
		return m.AddStaffFunc(ctx, venueID, userID, role, invitedBy)
	}
	return nil
}

func (m *mockRepo) RemoveStaff(ctx context.Context, venueID, userID uuid.UUID) error {
	if m.RemoveStaffFunc != nil {
		return m.RemoveStaffFunc(ctx, venueID, userID)
	}
	return nil
}

func (m *mockRepo) ListTasks(ctx context.Context, venueID uuid.UUID, status string) ([]domain.Task, error) {
	if m.ListTasksFunc != nil {
		return m.ListTasksFunc(ctx, venueID, status)
	}
	return nil, nil
}

func (m *mockRepo) CreateTask(ctx context.Context, t *domain.Task) error {
	if m.CreateTaskFunc != nil {
		return m.CreateTaskFunc(ctx, t)
	}
	return nil
}

func (m *mockRepo) CompleteTask(ctx context.Context, venueID, taskID uuid.UUID) (bool, error) {
	if m.CompleteTaskFunc != nil {
		return m.CompleteTaskFunc(ctx, venueID, taskID)
	}
	return false, nil
}
