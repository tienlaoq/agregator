package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Management access roles returned by GetManagementAccess.
const (
	AccessOwner   = "owner"
	AccessManager = "manager"
	AccessStaff   = "staff"
)

// Staff roles persisted in venue_staff.role (subset of access roles — owner
// is implicit via venues.owner_id and is never stored here).
const (
	StaffRoleManager = "manager"
	StaffRoleStaff   = "staff"
)

// CRM task lifecycle.
const (
	TaskStatusOpen = "open"
	TaskStatusDone = "done"
)

// StaffMember represents a venue staff membership row.
type StaffMember struct {
	VenueID   uuid.UUID
	UserID    uuid.UUID
	Role      string
	InvitedBy uuid.UUID
	CreatedAt time.Time
}

// ManagedVenue is a (venue_id, access) pair returned by ListManagedVenues.
type ManagedVenue struct {
	VenueID uuid.UUID
	Access  string
}

// Task is a CRM action item attached to a venue (optionally to a booking).
type Task struct {
	ID             uuid.UUID
	VenueID        uuid.UUID
	BookingID      *uuid.UUID
	Title          string
	Body           string
	Status         string
	AssigneeUserID *uuid.UUID
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Repository is the persistence port for CRM.
type Repository interface {
	// VenueOwnerID returns the owner_id from the shared venues table.
	// Returns ErrVenueNotFound if the venue does not exist.
	VenueOwnerID(ctx context.Context, venueID uuid.UUID) (uuid.UUID, error)

	// GetManagementAccess returns "owner" | "manager" | "staff" | "" for the
	// given venue/user pair. Empty string means no access.
	GetManagementAccess(ctx context.Context, venueID, userID uuid.UUID) (string, error)

	// BatchGetManagementAccess resolves access for many venues in one query.
	// Venues with no access are omitted from the result map.
	BatchGetManagementAccess(ctx context.Context, userID uuid.UUID, venueIDs []uuid.UUID) (map[uuid.UUID]string, error)

	// ListManagedVenues returns all venues the user can manage as owner|manager|staff.
	ListManagedVenues(ctx context.Context, userID uuid.UUID) ([]ManagedVenue, error)

	ListStaff(ctx context.Context, venueID uuid.UUID) ([]StaffMember, error)
	AddStaff(ctx context.Context, venueID, userID uuid.UUID, role string, invitedBy uuid.UUID) error
	RemoveStaff(ctx context.Context, venueID, userID uuid.UUID) error

	ListTasks(ctx context.Context, venueID uuid.UUID, status string) ([]Task, error)
	CreateTask(ctx context.Context, t *Task) error
	// CompleteTask returns true when a row was transitioned from open to done.
	CompleteTask(ctx context.Context, venueID, taskID uuid.UUID) (bool, error)
}
