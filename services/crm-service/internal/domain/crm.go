package domain

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Management access roles returned by GetManagementAccess.
const (
	AccessOwner   = "owner"
	AccessManager = "manager"
	AccessStaff   = "staff"
)

// Capability is a CRM action gated by the actor's management role. Capabilities
// keep RBAC decisions in one place instead of scattering role string checks
// across the usecase.
type Capability int

const (
	// CapViewCRM — read staff, tasks and bookings (any member: owner|manager|staff).
	CapViewCRM Capability = iota
	// CapManageStaff — add/remove staff (owner only).
	CapManageStaff
	// CapManageAnyTask — edit, reopen, cancel or complete ANY task regardless of
	// authorship (owner|manager). Staff are limited to their own tasks; that is a
	// per-task ownership check the usecase layers on top of this role check.
	CapManageAnyTask
)

// Can reports whether the given management access role may perform c. An empty
// access string (no membership) is denied every capability.
func Can(access string, c Capability) bool {
	switch c {
	case CapViewCRM:
		return access == AccessOwner || access == AccessManager || access == AccessStaff
	case CapManageStaff:
		return access == AccessOwner
	case CapManageAnyTask:
		return access == AccessOwner || access == AccessManager
	default:
		return false
	}
}

// Staff roles persisted in venue_staff.role (subset of access roles — owner
// is implicit via venues.owner_id and is never stored here).
const (
	StaffRoleManager = "manager"
	StaffRoleStaff   = "staff"
)

// CRM task lifecycle.
const (
	TaskStatusOpen      = "open"
	TaskStatusDone      = "done"
	TaskStatusCancelled = "cancelled"
)

// Task priorities (low | normal | high). Empty input normalizes to normal.
const (
	TaskPriorityLow    = "low"
	TaskPriorityNormal = "normal"
	TaskPriorityHigh   = "high"
)

// NormalizePriority lowercases and validates a task priority, defaulting empty
// to normal. The bool is false for unrecognized values.
func NormalizePriority(p string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "":
		return TaskPriorityNormal, true
	case TaskPriorityLow:
		return TaskPriorityLow, true
	case TaskPriorityNormal:
		return TaskPriorityNormal, true
	case TaskPriorityHigh:
		return TaskPriorityHigh, true
	default:
		return "", false
	}
}

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
	Priority       string
	AssigneeUserID *uuid.UUID
	DueAt          *time.Time
	CreatedBy      uuid.UUID
	CompletedBy    *uuid.UUID
	CompletedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// OwnedBy reports whether userID created the task or is its assignee. Used to let
// staff complete their own tasks without granting CapManageAnyTask.
func (t *Task) OwnedBy(userID uuid.UUID) bool {
	return t.CreatedBy == userID || (t.AssigneeUserID != nil && *t.AssigneeUserID == userID)
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
	// GetTask returns a single task scoped to the venue, or ErrTaskNotFound.
	GetTask(ctx context.Context, venueID, taskID uuid.UUID) (*Task, error)
	CreateTask(ctx context.Context, t *Task) error
	// UpdateTask replaces the mutable content fields (title, body, priority,
	// due_at, assignee) of a non-cancelled task and refreshes t from the stored
	// row. Returns ErrTaskNotFound when no matching task exists.
	UpdateTask(ctx context.Context, t *Task) error
	// CompleteTask transitions open -> done, recording completedBy and the
	// completion time. Returns true when a row was transitioned.
	CompleteTask(ctx context.Context, venueID, taskID, completedBy uuid.UUID) (bool, error)
	// ReopenTask transitions done -> open, clearing completed_by/at, and returns
	// the refreshed task. Returns ErrTaskNotFound when the task is absent or not done.
	ReopenTask(ctx context.Context, venueID, taskID uuid.UUID) (*Task, error)
	// CancelTask soft-deletes a task (-> cancelled). Returns true when a row was
	// transitioned (i.e. it existed and was not already cancelled).
	CancelTask(ctx context.Context, venueID, taskID uuid.UUID) (bool, error)

	// ApplyBookingFact upserts one booking fact and recomputes the affected
	// guest profile, in a single transaction. Idempotent: re-delivering the same
	// event is a no-op; lower-ranked (out-of-order) events are ignored.
	ApplyBookingFact(ctx context.Context, f *BookingFact) error
	// ListGuests returns guest profiles for a venue, filtered/sorted/paginated
	// per params, plus the total count of matching guests.
	ListGuests(ctx context.Context, venueID uuid.UUID, params GuestListParams) ([]GuestProfile, int, error)
	// GetGuestProfile returns one guest's profile, or ErrGuestNotFound.
	GetGuestProfile(ctx context.Context, venueID, userID uuid.UUID) (*GuestProfile, error)
	// ListGuestBookings returns a guest's most recent booking facts (newest first).
	ListGuestBookings(ctx context.Context, venueID, userID uuid.UUID, limit int) ([]GuestBookingSummary, error)
}
