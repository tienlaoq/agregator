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
	ReplaceVenueServicesFn    func(ctx context.Context, venueID, ownerID uuid.UUID, services []domain.VenueService) error
	GetByIDFn                 func(ctx context.Context, id uuid.UUID) (*domain.Venue, error)
	GetByIDsFn                func(ctx context.Context, ids []uuid.UUID) ([]domain.Venue, error)
	GetBySlugFn               func(ctx context.Context, slug string) (*domain.Venue, error)
	ListFn                    func(ctx context.Context, page, pageSize int32, venueType, sortBy string) (*domain.ListResult, error)
	SearchFn                  func(ctx context.Context, params domain.SearchParams) (*domain.ListResult, error)
	PopularCitiesFn           func(ctx context.Context, limit int32) ([]domain.CityCount, error)
	ListByOwnerFn             func(ctx context.Context, ownerID uuid.UUID) ([]domain.Venue, error)
	SuspendByOwnerFn          func(ctx context.Context, ownerID uuid.UUID) (int64, error)
	ListByStatusFn            func(ctx context.Context, status string, page, pageSize int32, nameQuery string) (*domain.ListResult, error)
	UpdateStatusFn            func(ctx context.Context, venueID uuid.UUID, status, comment string, moderatedBy uuid.UUID) error
	ResetToPendingReviewFn    func(ctx context.Context, venueID uuid.UUID) error
	SubmitDraftForReviewFn    func(ctx context.Context, venueID, ownerID uuid.UUID) (bool, error)
	InsertModerationHistoryFn func(ctx context.Context, entry *domain.ModerationHistoryEntry) error
	GetModerationHistoryFn    func(ctx context.Context, venueID uuid.UUID) ([]domain.ModerationHistoryEntry, error)
	UpdateRatingFn            func(ctx context.Context, venueID uuid.UUID, avgRating float64, reviewCount int32) error
	CheckSlotFn               func(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, date, timeFrom, timeTo string) (bool, error)
	ReserveSlotFn             func(ctx context.Context, venueID, bookingID uuid.UUID, date, timeFrom, timeTo string, hallIDs []uuid.UUID) error
	BookingModeFn             func(ctx context.Context, venueID uuid.UUID) (string, error)
	SetBookingModeFn          func(ctx context.Context, venueID uuid.UUID, mode string) error
	ReleaseSlotFn             func(ctx context.Context, venueID, bookingID uuid.UUID) error
	CreateManualSlotBlockFn   func(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, date, timeFrom, timeTo, note string) ([]domain.ManualSlotBlock, error)
	HallIDsFn                 func(ctx context.Context, venueID uuid.UUID) ([]uuid.UUID, error)
	DeleteManualSlotBlockFn   func(ctx context.Context, venueID, blockID uuid.UUID) (bool, error)
	ListManualSlotBlocksFn    func(ctx context.Context, venueID uuid.UUID, dateFrom, dateTo string) ([]domain.ManualSlotBlock, error)
	ListVenueScheduleFn       func(ctx context.Context, venueID uuid.UUID, dateFrom, dateTo string) ([]domain.ScheduleEntry, error)
	AddVenuePhotoFn           func(ctx context.Context, venueID, ownerID uuid.UUID, url string) (*domain.VenuePhoto, error)
	DeleteVenuePhotoFn        func(ctx context.Context, venueID, ownerID, photoID uuid.UUID) (string, error)
	SetVenueCoverPhotoFn      func(ctx context.Context, venueID, ownerID, photoID uuid.UUID) error
	ReplaceVenueHallsFn       func(ctx context.Context, venueID, ownerID uuid.UUID, items []domain.VenueHallUpsert) error
	AddVenueHallPhotoFn       func(ctx context.Context, venueID, hallID uuid.UUID, url string) (*domain.VenueHallPhoto, error)
	DeleteVenueHallPhotoFn    func(ctx context.Context, venueID, hallID, photoID uuid.UUID) (string, error)
	SetVenueHallCoverPhotoFn  func(ctx context.Context, venueID, hallID, photoID uuid.UUID) error

	// CRM was extracted: only the legacy phase-B bridges remain.
	IsVenueMemberFn        func(ctx context.Context, venueID, userID uuid.UUID) (bool, error)
	StaffUserIDsForVenueFn func(ctx context.Context, venueID uuid.UUID) ([]uuid.UUID, error)
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

func (m *mockVenueRepo) ReplaceVenueServices(ctx context.Context, venueID, ownerID uuid.UUID, services []domain.VenueService) error {
	if m.ReplaceVenueServicesFn != nil {
		return m.ReplaceVenueServicesFn(ctx, venueID, ownerID, services)
	}
	return nil
}

func (m *mockVenueRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Venue, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockVenueRepo) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Venue, error) {
	if m.GetByIDsFn != nil {
		return m.GetByIDsFn(ctx, ids)
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

func (m *mockVenueRepo) PopularCities(ctx context.Context, limit int32) ([]domain.CityCount, error) {
	if m.PopularCitiesFn != nil {
		return m.PopularCitiesFn(ctx, limit)
	}
	return nil, nil
}

func (m *mockVenueRepo) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]domain.Venue, error) {
	if m.ListByOwnerFn != nil {
		return m.ListByOwnerFn(ctx, ownerID)
	}
	return nil, nil
}

func (m *mockVenueRepo) ListByStatus(ctx context.Context, status string, page, pageSize int32, nameQuery string) (*domain.ListResult, error) {
	if m.ListByStatusFn != nil {
		return m.ListByStatusFn(ctx, status, page, pageSize, nameQuery)
	}
	return nil, nil
}

func (m *mockVenueRepo) SuspendByOwner(ctx context.Context, ownerID uuid.UUID) (int64, error) {
	if m.SuspendByOwnerFn != nil {
		return m.SuspendByOwnerFn(ctx, ownerID)
	}
	return 0, nil
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

func (m *mockVenueRepo) SubmitDraftForReview(ctx context.Context, venueID, ownerID uuid.UUID) (bool, error) {
	if m.SubmitDraftForReviewFn != nil {
		return m.SubmitDraftForReviewFn(ctx, venueID, ownerID)
	}
	return false, nil
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

func (m *mockVenueRepo) CheckSlot(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, date, timeFrom, timeTo string) (bool, error) {
	if m.CheckSlotFn != nil {
		return m.CheckSlotFn(ctx, venueID, hallIDs, date, timeFrom, timeTo)
	}
	return false, nil
}

func (m *mockVenueRepo) BatchCheckSlots(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, date string, slots [][2]string) ([]bool, error) {
	return make([]bool, len(slots)), nil
}

func (m *mockVenueRepo) ReserveSlot(ctx context.Context, venueID, bookingID uuid.UUID, date, timeFrom, timeTo string, hallIDs []uuid.UUID) error {
	if m.ReserveSlotFn != nil {
		return m.ReserveSlotFn(ctx, venueID, bookingID, date, timeFrom, timeTo, hallIDs)
	}
	return nil
}

func (m *mockVenueRepo) BookingMode(ctx context.Context, venueID uuid.UUID) (string, error) {
	if m.BookingModeFn != nil {
		return m.BookingModeFn(ctx, venueID)
	}
	return domain.BookingModeWhole, nil
}

func (m *mockVenueRepo) SetBookingMode(ctx context.Context, venueID uuid.UUID, mode string) error {
	if m.SetBookingModeFn != nil {
		return m.SetBookingModeFn(ctx, venueID, mode)
	}
	return nil
}

func (m *mockVenueRepo) ReleaseSlot(ctx context.Context, venueID, bookingID uuid.UUID) error {
	if m.ReleaseSlotFn != nil {
		return m.ReleaseSlotFn(ctx, venueID, bookingID)
	}
	return nil
}

func (m *mockVenueRepo) CreateManualSlotBlock(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, date, timeFrom, timeTo, note string) ([]domain.ManualSlotBlock, error) {
	if m.CreateManualSlotBlockFn != nil {
		return m.CreateManualSlotBlockFn(ctx, venueID, hallIDs, date, timeFrom, timeTo, note)
	}
	return nil, nil
}

func (m *mockVenueRepo) HallIDs(ctx context.Context, venueID uuid.UUID) ([]uuid.UUID, error) {
	if m.HallIDsFn != nil {
		return m.HallIDsFn(ctx, venueID)
	}
	return nil, nil
}

func (m *mockVenueRepo) DeleteManualSlotBlock(ctx context.Context, venueID, blockID uuid.UUID) (bool, error) {
	if m.DeleteManualSlotBlockFn != nil {
		return m.DeleteManualSlotBlockFn(ctx, venueID, blockID)
	}
	return false, nil
}

func (m *mockVenueRepo) ListManualSlotBlocks(ctx context.Context, venueID uuid.UUID, dateFrom, dateTo string) ([]domain.ManualSlotBlock, error) {
	if m.ListManualSlotBlocksFn != nil {
		return m.ListManualSlotBlocksFn(ctx, venueID, dateFrom, dateTo)
	}
	return nil, nil
}

func (m *mockVenueRepo) ListVenueSchedule(ctx context.Context, venueID uuid.UUID, dateFrom, dateTo string) ([]domain.ScheduleEntry, error) {
	if m.ListVenueScheduleFn != nil {
		return m.ListVenueScheduleFn(ctx, venueID, dateFrom, dateTo)
	}
	return nil, nil
}

func (m *mockVenueRepo) AddVenuePhoto(ctx context.Context, venueID, ownerID uuid.UUID, url string) (*domain.VenuePhoto, error) {
	if m.AddVenuePhotoFn != nil {
		return m.AddVenuePhotoFn(ctx, venueID, ownerID, url)
	}
	return &domain.VenuePhoto{VenueID: venueID, URL: url}, nil
}

func (m *mockVenueRepo) DeleteVenuePhoto(ctx context.Context, venueID, ownerID, photoID uuid.UUID) (string, error) {
	if m.DeleteVenuePhotoFn != nil {
		return m.DeleteVenuePhotoFn(ctx, venueID, ownerID, photoID)
	}
	return "", nil
}

func (m *mockVenueRepo) SetVenueCoverPhoto(ctx context.Context, venueID, ownerID, photoID uuid.UUID) error {
	if m.SetVenueCoverPhotoFn != nil {
		return m.SetVenueCoverPhotoFn(ctx, venueID, ownerID, photoID)
	}
	return nil
}

func (m *mockVenueRepo) ReplaceVenueHalls(ctx context.Context, venueID, ownerID uuid.UUID, items []domain.VenueHallUpsert) error {
	if m.ReplaceVenueHallsFn != nil {
		return m.ReplaceVenueHallsFn(ctx, venueID, ownerID, items)
	}
	return nil
}

func (m *mockVenueRepo) AddVenueHallPhoto(ctx context.Context, venueID, hallID uuid.UUID, url string) (*domain.VenueHallPhoto, error) {
	if m.AddVenueHallPhotoFn != nil {
		return m.AddVenueHallPhotoFn(ctx, venueID, hallID, url)
	}
	return &domain.VenueHallPhoto{HallID: hallID, URL: url}, nil
}

func (m *mockVenueRepo) DeleteVenueHallPhoto(ctx context.Context, venueID, hallID, photoID uuid.UUID) (string, error) {
	if m.DeleteVenueHallPhotoFn != nil {
		return m.DeleteVenueHallPhotoFn(ctx, venueID, hallID, photoID)
	}
	return "", nil
}

func (m *mockVenueRepo) SetVenueHallCoverPhoto(ctx context.Context, venueID, hallID, photoID uuid.UUID) error {
	if m.SetVenueHallCoverPhotoFn != nil {
		return m.SetVenueHallCoverPhotoFn(ctx, venueID, hallID, photoID)
	}
	return nil
}

// IsVenueMember is the phase-B legacy bridge that replaced GetVenueManagementAccess.
// Default behaviour mirrors the old mock: the owner (resolved via GetByIDFn)
// is a member; everyone else is not. Tests that need staff membership wire
// IsVenueMemberFn explicitly.
func (m *mockVenueRepo) IsVenueMember(ctx context.Context, venueID, userID uuid.UUID) (bool, error) {
	if m.IsVenueMemberFn != nil {
		return m.IsVenueMemberFn(ctx, venueID, userID)
	}
	if m.GetByIDFn == nil {
		return false, nil
	}
	v, err := m.GetByIDFn(ctx, venueID)
	if err != nil || v == nil {
		return false, err
	}
	return v.OwnerID == userID, nil
}

// StaffUserIDsForVenue is the phase-B legacy bridge used by
// RecipientUserIDsForVenue. Default returns no staff (owner only).
func (m *mockVenueRepo) StaffUserIDsForVenue(ctx context.Context, venueID uuid.UUID) ([]uuid.UUID, error) {
	if m.StaffUserIDsForVenueFn != nil {
		return m.StaffUserIDsForVenueFn(ctx, venueID)
	}
	return nil, nil
}
