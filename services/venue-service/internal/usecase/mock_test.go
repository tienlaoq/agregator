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
	ReplaceVenueServicesFn    func(ctx context.Context, venueID uuid.UUID, services []domain.VenueService) error
	GetByIDFn                 func(ctx context.Context, id uuid.UUID) (*domain.Venue, error)
	GetBySlugFn               func(ctx context.Context, slug string) (*domain.Venue, error)
	ListFn                    func(ctx context.Context, page, pageSize int32, venueType, sortBy string) (*domain.ListResult, error)
	SearchFn                  func(ctx context.Context, params domain.SearchParams) (*domain.ListResult, error)
	ListByOwnerFn             func(ctx context.Context, ownerID uuid.UUID) ([]domain.Venue, error)
	ListByStatusFn            func(ctx context.Context, status string, page, pageSize int32, nameQuery string) (*domain.ListResult, error)
	UpdateStatusFn            func(ctx context.Context, venueID uuid.UUID, status, comment string, moderatedBy uuid.UUID) error
	ResetToPendingReviewFn    func(ctx context.Context, venueID uuid.UUID) error
	SubmitDraftForReviewFn    func(ctx context.Context, venueID, ownerID uuid.UUID) (bool, error)
	InsertModerationHistoryFn func(ctx context.Context, entry *domain.ModerationHistoryEntry) error
	GetModerationHistoryFn    func(ctx context.Context, venueID uuid.UUID) ([]domain.ModerationHistoryEntry, error)
	UpdateRatingFn            func(ctx context.Context, venueID uuid.UUID, avgRating float64, reviewCount int32) error
	CheckSlotFn               func(ctx context.Context, venueID uuid.UUID, date, timeFrom, timeTo string) (bool, error)
	ReserveSlotFn             func(ctx context.Context, venueID, bookingID uuid.UUID, date, timeFrom, timeTo string) error
	ReleaseSlotFn             func(ctx context.Context, venueID, bookingID uuid.UUID) error
	CreateManualSlotBlockFn   func(ctx context.Context, venueID uuid.UUID, date, timeFrom, timeTo, note string) (uuid.UUID, error)
	DeleteManualSlotBlockFn   func(ctx context.Context, venueID, blockID uuid.UUID) (bool, error)
	ListManualSlotBlocksFn    func(ctx context.Context, venueID uuid.UUID, dateFrom, dateTo string) ([]domain.ManualSlotBlock, error)
	AddVenuePhotoFn           func(ctx context.Context, venueID uuid.UUID, url string) (*domain.VenuePhoto, error)
	DeleteVenuePhotoFn        func(ctx context.Context, venueID, photoID uuid.UUID) (string, error)
	SetVenueCoverPhotoFn      func(ctx context.Context, venueID, photoID uuid.UUID) error
	ReplaceVenueHallsFn       func(ctx context.Context, venueID uuid.UUID, items []domain.VenueHallUpsert) error
	AddVenueHallPhotoFn       func(ctx context.Context, venueID, hallID uuid.UUID, url string) (*domain.VenueHallPhoto, error)
	DeleteVenueHallPhotoFn    func(ctx context.Context, venueID, hallID, photoID uuid.UUID) (string, error)
	SetVenueHallCoverPhotoFn  func(ctx context.Context, venueID, hallID, photoID uuid.UUID) error

	ListForManagingUserFn        func(ctx context.Context, userID uuid.UUID) ([]domain.Venue, error)
	GetVenueManagementAccessFn   func(ctx context.Context, venueID, userID uuid.UUID) (string, error)
	AddVenueStaffFn              func(ctx context.Context, venueID, userID uuid.UUID, role string, invitedBy uuid.UUID) error
	RemoveVenueStaffFn           func(ctx context.Context, venueID, userID uuid.UUID) error
	ListVenueStaffFn             func(ctx context.Context, venueID uuid.UUID) ([]domain.VenueStaff, error)
	CreateVenueCRMTaskFn         func(ctx context.Context, t *domain.VenueCRMTask) error
	ListVenueCRMTasksFn          func(ctx context.Context, venueID uuid.UUID, status string) ([]domain.VenueCRMTask, error)
	CompleteVenueCRMTaskFn       func(ctx context.Context, venueID, taskID uuid.UUID) (bool, error)
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

func (m *mockVenueRepo) ReplaceVenueServices(ctx context.Context, venueID uuid.UUID, services []domain.VenueService) error {
	if m.ReplaceVenueServicesFn != nil {
		return m.ReplaceVenueServicesFn(ctx, venueID, services)
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

func (m *mockVenueRepo) ListByStatus(ctx context.Context, status string, page, pageSize int32, nameQuery string) (*domain.ListResult, error) {
	if m.ListByStatusFn != nil {
		return m.ListByStatusFn(ctx, status, page, pageSize, nameQuery)
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

func (m *mockVenueRepo) CreateManualSlotBlock(ctx context.Context, venueID uuid.UUID, date, timeFrom, timeTo, note string) (uuid.UUID, error) {
	if m.CreateManualSlotBlockFn != nil {
		return m.CreateManualSlotBlockFn(ctx, venueID, date, timeFrom, timeTo, note)
	}
	return uuid.Nil, nil
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

func (m *mockVenueRepo) AddVenuePhoto(ctx context.Context, venueID uuid.UUID, url string) (*domain.VenuePhoto, error) {
	if m.AddVenuePhotoFn != nil {
		return m.AddVenuePhotoFn(ctx, venueID, url)
	}
	return &domain.VenuePhoto{VenueID: venueID, URL: url}, nil
}

func (m *mockVenueRepo) DeleteVenuePhoto(ctx context.Context, venueID, photoID uuid.UUID) (string, error) {
	if m.DeleteVenuePhotoFn != nil {
		return m.DeleteVenuePhotoFn(ctx, venueID, photoID)
	}
	return "", nil
}

func (m *mockVenueRepo) SetVenueCoverPhoto(ctx context.Context, venueID, photoID uuid.UUID) error {
	if m.SetVenueCoverPhotoFn != nil {
		return m.SetVenueCoverPhotoFn(ctx, venueID, photoID)
	}
	return nil
}

func (m *mockVenueRepo) ReplaceVenueHalls(ctx context.Context, venueID uuid.UUID, items []domain.VenueHallUpsert) error {
	if m.ReplaceVenueHallsFn != nil {
		return m.ReplaceVenueHallsFn(ctx, venueID, items)
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

func (m *mockVenueRepo) ListForManagingUser(ctx context.Context, userID uuid.UUID) ([]domain.Venue, error) {
	if m.ListForManagingUserFn != nil {
		return m.ListForManagingUserFn(ctx, userID)
	}
	if m.ListByOwnerFn != nil {
		vs, err := m.ListByOwnerFn(ctx, userID)
		if err != nil {
			return nil, err
		}
		for i := range vs {
			vs[i].ManagementAccess = domain.ManagementAccessOwner
		}
		return vs, nil
	}
	return nil, nil
}

func (m *mockVenueRepo) GetVenueManagementAccess(ctx context.Context, venueID, userID uuid.UUID) (string, error) {
	if m.GetVenueManagementAccessFn != nil {
		return m.GetVenueManagementAccessFn(ctx, venueID, userID)
	}
	if m.GetByIDFn == nil {
		return "", nil
	}
	v, err := m.GetByIDFn(ctx, venueID)
	if err != nil || v == nil {
		return "", err
	}
	if v.OwnerID == userID {
		return domain.ManagementAccessOwner, nil
	}
	return "", nil
}

func (m *mockVenueRepo) AddVenueStaff(ctx context.Context, venueID, userID uuid.UUID, role string, invitedBy uuid.UUID) error {
	if m.AddVenueStaffFn != nil {
		return m.AddVenueStaffFn(ctx, venueID, userID, role, invitedBy)
	}
	return nil
}

func (m *mockVenueRepo) RemoveVenueStaff(ctx context.Context, venueID, userID uuid.UUID) error {
	if m.RemoveVenueStaffFn != nil {
		return m.RemoveVenueStaffFn(ctx, venueID, userID)
	}
	return nil
}

func (m *mockVenueRepo) ListVenueStaff(ctx context.Context, venueID uuid.UUID) ([]domain.VenueStaff, error) {
	if m.ListVenueStaffFn != nil {
		return m.ListVenueStaffFn(ctx, venueID)
	}
	return nil, nil
}

func (m *mockVenueRepo) CreateVenueCRMTask(ctx context.Context, t *domain.VenueCRMTask) error {
	if m.CreateVenueCRMTaskFn != nil {
		return m.CreateVenueCRMTaskFn(ctx, t)
	}
	return nil
}

func (m *mockVenueRepo) ListVenueCRMTasks(ctx context.Context, venueID uuid.UUID, status string) ([]domain.VenueCRMTask, error) {
	if m.ListVenueCRMTasksFn != nil {
		return m.ListVenueCRMTasksFn(ctx, venueID, status)
	}
	return nil, nil
}

func (m *mockVenueRepo) CompleteVenueCRMTask(ctx context.Context, venueID, taskID uuid.UUID) (bool, error) {
	if m.CompleteVenueCRMTaskFn != nil {
		return m.CompleteVenueCRMTaskFn(ctx, venueID, taskID)
	}
	return false, nil
}
