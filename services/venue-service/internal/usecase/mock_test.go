package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

// mockVenueRepo implements domain.VenueRepository with function hooks for tests.
type mockVenueRepo struct {
	CreateFn                  func(ctx context.Context, venue *domain.Venue) error
	UpdateFn                  func(ctx context.Context, venue *domain.Venue) error
	GetByIDFn                 func(ctx context.Context, id uuid.UUID) (*domain.Venue, error)
	GetBySlugFn               func(ctx context.Context, slug string) (*domain.Venue, error)
	ListFn                    func(ctx context.Context, page, pageSize int32, venueType, sortBy string) (*domain.ListResult, error)
	SearchFn                  func(ctx context.Context, params domain.SearchParams) (*domain.ListResult, error)
	ListByOwnerFn             func(ctx context.Context, ownerID uuid.UUID) ([]domain.Venue, error)
	ListByStatusFn            func(ctx context.Context, status string, page, pageSize int32) (*domain.ListResult, error)
	UpdateStatusFn            func(ctx context.Context, venueID uuid.UUID, status, comment string, moderatedBy uuid.UUID) error
	ResetToPendingReviewFn    func(ctx context.Context, venueID uuid.UUID) error
	InsertModerationHistoryFn func(ctx context.Context, entry *domain.ModerationHistoryEntry) error
	GetModerationHistoryFn    func(ctx context.Context, venueID uuid.UUID) ([]domain.ModerationHistoryEntry, error)
	UpdateRatingFn            func(ctx context.Context, venueID uuid.UUID, avgRating float64, reviewCount int32) error
	CheckSlotFn               func(ctx context.Context, venueID uuid.UUID, date, timeFrom, timeTo string) (bool, error)
	ReserveSlotFn             func(ctx context.Context, venueID, bookingID uuid.UUID, date, timeFrom, timeTo string) error
	ReleaseSlotFn             func(ctx context.Context, venueID, bookingID uuid.UUID) error
}

func (m *mockVenueRepo) Create(ctx context.Context, venue *domain.Venue) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, venue)
	}
	return nil
}

func (m *mockVenueRepo) Update(ctx context.Context, venue *domain.Venue) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, venue)
	}
	return nil
}

func (m *mockVenueRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Venue, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockVenueRepo) GetBySlug(ctx context.Context, slug string) (*domain.Venue, error) {
	if m.GetBySlugFn != nil {
		return m.GetBySlugFn(ctx, slug)
	}
	return nil, nil
}

func (m *mockVenueRepo) List(ctx context.Context, page, pageSize int32, venueType, sortBy string) (*domain.ListResult, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, page, pageSize, venueType, sortBy)
	}
	return nil, nil
}

func (m *mockVenueRepo) Search(ctx context.Context, params domain.SearchParams) (*domain.ListResult, error) {
	if m.SearchFn != nil {
		return m.SearchFn(ctx, params)
	}
	return nil, nil
}

func (m *mockVenueRepo) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]domain.Venue, error) {
	if m.ListByOwnerFn != nil {
		return m.ListByOwnerFn(ctx, ownerID)
	}
	return nil, nil
}

func (m *mockVenueRepo) ListByStatus(ctx context.Context, status string, page, pageSize int32) (*domain.ListResult, error) {
	if m.ListByStatusFn != nil {
		return m.ListByStatusFn(ctx, status, page, pageSize)
	}
	return nil, nil
}

func (m *mockVenueRepo) UpdateStatus(ctx context.Context, venueID uuid.UUID, status, comment string, moderatedBy uuid.UUID) error {
	if m.UpdateStatusFn != nil {
		return m.UpdateStatusFn(ctx, venueID, status, comment, moderatedBy)
	}
	return nil
}

func (m *mockVenueRepo) ResetToPendingReview(ctx context.Context, venueID uuid.UUID) error {
	if m.ResetToPendingReviewFn != nil {
		return m.ResetToPendingReviewFn(ctx, venueID)
	}
	return nil
}

func (m *mockVenueRepo) InsertModerationHistory(ctx context.Context, entry *domain.ModerationHistoryEntry) error {
	if m.InsertModerationHistoryFn != nil {
		return m.InsertModerationHistoryFn(ctx, entry)
	}
	return nil
}

func (m *mockVenueRepo) GetModerationHistory(ctx context.Context, venueID uuid.UUID) ([]domain.ModerationHistoryEntry, error) {
	if m.GetModerationHistoryFn != nil {
		return m.GetModerationHistoryFn(ctx, venueID)
	}
	return nil, nil
}

func (m *mockVenueRepo) UpdateRating(ctx context.Context, venueID uuid.UUID, avgRating float64, reviewCount int32) error {
	if m.UpdateRatingFn != nil {
		return m.UpdateRatingFn(ctx, venueID, avgRating, reviewCount)
	}
	return nil
}

func (m *mockVenueRepo) CheckSlot(ctx context.Context, venueID uuid.UUID, date, timeFrom, timeTo string) (bool, error) {
	if m.CheckSlotFn != nil {
		return m.CheckSlotFn(ctx, venueID, date, timeFrom, timeTo)
	}
	return false, nil
}

func (m *mockVenueRepo) ReserveSlot(ctx context.Context, venueID, bookingID uuid.UUID, date, timeFrom, timeTo string) error {
	if m.ReserveSlotFn != nil {
		return m.ReserveSlotFn(ctx, venueID, bookingID, date, timeFrom, timeTo)
	}
	return nil
}

func (m *mockVenueRepo) ReleaseSlot(ctx context.Context, venueID, bookingID uuid.UUID) error {
	if m.ReleaseSlotFn != nil {
		return m.ReleaseSlotFn(ctx, venueID, bookingID)
	}
	return nil
}
