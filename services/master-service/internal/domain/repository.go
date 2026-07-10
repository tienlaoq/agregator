package domain

import (
	"context"

	"github.com/google/uuid"
)

// MasterRepository is the single authoritative repository interface for the
// master-service domain. It lives here — in the domain layer — so that usecase
// and delivery layers depend inward on the domain, not on infrastructure.
//
// The concrete implementation is in internal/repository/postgres.go;
// test doubles implement this interface directly.
type MasterRepository interface {
	Insert(ctx context.Context, m *Master) error
	GetByUserID(ctx context.Context, userID uuid.UUID) (*Master, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Master, error)
	GetBySlug(ctx context.Context, slug string) (*Master, error)
	UpdateProfile(ctx context.Context, m *Master) error
	UpdateStatus(ctx context.Context, masterID uuid.UUID, status, comment string, moderatedBy *uuid.UUID) error
	// SuspendByUser sets the master profile owned by userID to status
	// "suspended" (account-deletion cascade). Returns true when a row was
	// transitioned, false when the user has no profile or it was already
	// suspended. Never returns ErrNotFound — absence is a normal no-op.
	SuspendByUser(ctx context.Context, userID uuid.UUID) (bool, error)
	// SubmitForReviewAtomic atomically transitions the master's status to
	// pending_review (only from draft / needs_revision / rejected) and records a
	// ModerationHistoryEntry, all in a single transaction. Returns ErrSubmitNotAllowed
	// when the UPDATE touches 0 rows (wrong status or master not found), which
	// prevents duplicate history entries when two concurrent submit calls race.
	SubmitForReviewAtomic(ctx context.Context, masterID, userID uuid.UUID) error
	ListByStatus(ctx context.Context, statusFilter string, limit, offset int32) ([]Master, int32, error)
	ListPublic(ctx context.Context, params ListPublicMastersParams) ([]Master, int32, error)
	ReplaceServices(ctx context.Context, masterID uuid.UUID, items []MasterServiceUpsert) ([]MasterService, error)
	// ReplaceCredentials deletes all of a master's certificates/awards and
	// re-inserts the supplied list in a single transaction. An empty list
	// clears the credentials. SortOrder falls back to list index when unset.
	ReplaceCredentials(ctx context.Context, masterID uuid.UUID, items []MasterCredentialUpsert) ([]MasterCredential, error)
	InsertModerationHistory(ctx context.Context, e *ModerationHistoryEntry) error
	// UpdateStatusWithHistory atomically updates masters.status and appends a
	// ModerationHistoryEntry in a single transaction. Use this instead of calling
	// UpdateStatus + InsertModerationHistory separately — the two-call pattern
	// risks a partial write where the status is updated but the audit record is lost.
	// Returns ErrNotFound when masterID does not exist.
	UpdateStatusWithHistory(ctx context.Context, masterID uuid.UUID, status, comment string, moderatedBy *uuid.UUID, entry *ModerationHistoryEntry) error
	// ModerateAtomic performs a conditional status transition for moderation:
	//   UPDATE masters … WHERE id = $masterID AND status = $expectedOldStatus
	// If the row's status no longer equals expectedOldStatus (a concurrent
	// moderator already acted), 0 rows are affected and ErrModerationConflict
	// is returned so the caller can surface an Aborted error and ask the client
	// to refresh. On success it also appends a ModerationHistoryEntry atomically.
	// Returns ErrNotFound when masterID does not exist at all.
	ModerateAtomic(ctx context.Context, masterID uuid.UUID, expectedOldStatus, newStatus, comment string, moderatedBy *uuid.UUID) error
	ListModerationHistory(ctx context.Context, masterID uuid.UUID, limit int32) ([]ModerationHistoryEntry, error)

	InsertBooking(ctx context.Context, b *MasterBooking) error
	GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*MasterBooking, error)
	// GetBookingsByIDs fetches multiple master bookings in a single query. Unknown IDs are omitted.
	GetBookingsByIDs(ctx context.Context, bookingIDs []uuid.UUID) ([]MasterBooking, error)
	// GetMasterUserIDsByIDs returns map[masterID]userID for access-control checks in batch operations.
	GetMasterUserIDsByIDs(ctx context.Context, masterIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error)
	SetBookingPayment(ctx context.Context, bookingID uuid.UUID, paymentID, paymentURL string, totalPrice int64, status string) error
	// DeleteBooking hard-deletes a booking row. Used as a compensating action
	// when CreatePayment fails or SetBookingPayment fails after a successful
	// CreatePayment — in both cases the booking must not remain as a stale
	// pending record. Callers should use a fresh context (context.Background)
	// so a cancelled request context does not prevent the cleanup.
	DeleteBooking(ctx context.Context, bookingID uuid.UUID) error
	// ConfirmBookingByPayment atomically transitions payment_pending→confirmed.
	// Returns ErrPaymentMismatch or ErrBookingNotPending on logical conflicts.
	ConfirmBookingByPayment(ctx context.Context, bookingID uuid.UUID, paymentID string) error
	// CancelBookingByPayment atomically transitions payment_pending→cancelled.
	// Returns ErrPaymentMismatch or ErrBookingNotPending on logical conflicts.
	CancelBookingByPayment(ctx context.Context, bookingID uuid.UUID, paymentID string) error
	ListBookingsByMaster(ctx context.Context, masterID uuid.UUID, statusFilter string) ([]MasterBooking, error)
	// ListClientsByMaster aggregates the master's bookings into one projection
	// per distinct client_user_id. It returns every client; segment filtering,
	// sorting and pagination are applied in the usecase (segments are computed,
	// not stored).
	ListClientsByMaster(ctx context.Context, masterID uuid.UUID) ([]MasterClient, error)

	// InsertSlotBlock persists a new schedule block for the master.
	InsertSlotBlock(ctx context.Context, b *MasterSlotBlock) error
	// ListSlotBlocksByMaster returns the master's upcoming blocks (date >= today),
	// ordered by date then time_from, whole-day blocks first.
	ListSlotBlocksByMaster(ctx context.Context, masterID uuid.UUID) ([]MasterSlotBlock, error)
	// DeleteSlotBlock removes a block owned by masterID. Returns ErrNotFound when
	// no row matches (unknown id or not owned by this master).
	DeleteSlotBlock(ctx context.Context, masterID, blockID uuid.UUID) error
	ListBookingsByClient(ctx context.Context, clientUserID uuid.UUID, statusFilter string) ([]MasterBooking, error)
	UpdateBookingStatus(ctx context.Context, bookingID uuid.UUID, status string) error
	HasCompletedBookingByClientMaster(ctx context.Context, clientUserID, masterID uuid.UUID) (bool, error)

	// NewSlug builds a unique URL-slug candidate from displayName. userID is
	// used as a fallback base when displayName produces an empty slug (emoji-only,
	// whitespace, or a string consisting entirely of stop characters) so that
	// every master gets a human-readable, user-specific URL instead of the
	// generic "master-XXXXXXXX" that a shared fallback would produce.
	NewSlug(ctx context.Context, displayName string, userID uuid.UUID) (string, error)

	CountPhotosByMaster(ctx context.Context, masterID uuid.UUID) (int32, error)
	AddMasterPhoto(ctx context.Context, masterID uuid.UUID, url string) (*MasterPhoto, error)
	DeleteMasterPhoto(ctx context.Context, masterID, photoID uuid.UUID) (deletedURL string, err error)
	SetMasterCoverPhoto(ctx context.Context, masterID, photoID uuid.UUID) error
}
