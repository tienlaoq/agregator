package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

const cacheTTL = 10 * time.Minute

type VenueUseCase struct {
	repo  domain.VenueRepository
	redis *goredis.Client
}

func NewVenueUseCase(repo domain.VenueRepository, redis *goredis.Client) *VenueUseCase {
	return &VenueUseCase{repo: repo, redis: redis}
}

func (uc *VenueUseCase) Create(ctx context.Context, venue *domain.Venue) error {
	if err := ValidateVenueVerificationForCreate(venue); err != nil {
		return err
	}
	return uc.repo.Create(ctx, venue)
}

func (uc *VenueUseCase) Update(ctx context.Context, venue *domain.Venue) error {
	if err := ValidateVenueVerificationForUpdate(venue); err != nil {
		return err
	}
	if err := uc.repo.Update(ctx, venue); err != nil {
		return err
	}
	if venue.Status == domain.StatusRejected {
		_ = uc.repo.ResetToPendingReview(ctx, venue.ID)
		venue.Status = domain.StatusPendingReview
	}
	uc.invalidateCache(ctx, venue.ID, venue.Slug)
	return nil
}

func (uc *VenueUseCase) GetByID(ctx context.Context, id uuid.UUID) (*domain.Venue, error) {
	key := fmt.Sprintf("venue:id:%s", id)
	if v, err := uc.getFromCache(ctx, key); err == nil && v != nil {
		return v, nil
	}

	v, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if v != nil {
		uc.setCache(ctx, key, v)
	}
	return v, nil
}

func (uc *VenueUseCase) GetBySlug(ctx context.Context, slug string) (*domain.Venue, error) {
	key := fmt.Sprintf("venue:slug:%s", slug)
	if v, err := uc.getFromCache(ctx, key); err == nil && v != nil {
		return v, nil
	}

	v, err := uc.repo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if v != nil {
		uc.setCache(ctx, key, v)
	}
	return v, nil
}

func (uc *VenueUseCase) List(ctx context.Context, page, pageSize int32, venueType, sortBy string) (*domain.ListResult, error) {
	return uc.repo.List(ctx, page, pageSize, venueType, sortBy)
}

func (uc *VenueUseCase) Search(ctx context.Context, params domain.SearchParams) (*domain.ListResult, error) {
	return uc.repo.Search(ctx, params)
}

func (uc *VenueUseCase) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]domain.Venue, error) {
	return uc.repo.ListByOwner(ctx, ownerID)
}

func (uc *VenueUseCase) ListByStatus(ctx context.Context, status string, page, pageSize int32) (*domain.ListResult, error) {
	return uc.repo.ListByStatus(ctx, status, page, pageSize)
}

func (uc *VenueUseCase) Moderate(ctx context.Context, venueID uuid.UUID, action, comment string, moderatedBy uuid.UUID) (*domain.Venue, error) {
	var newStatus string
	switch action {
	case "approve":
		newStatus = domain.StatusActive
	case "reject":
		newStatus = domain.StatusRejected
	case "suspend":
		newStatus = domain.StatusSuspended
	default:
		return nil, fmt.Errorf("unknown moderation action: %s", action)
	}

	if (action == "reject" || action == "suspend") && comment == "" {
		return nil, fmt.Errorf("comment is required for %s action", action)
	}

	existing, err := uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("venue not found: %s", venueID)
	}
	oldStatus := existing.Status

	if err := uc.repo.UpdateStatus(ctx, venueID, newStatus, comment, moderatedBy); err != nil {
		return nil, err
	}

	_ = uc.repo.InsertModerationHistory(ctx, &domain.ModerationHistoryEntry{
		VenueID:   venueID,
		OldStatus: oldStatus,
		NewStatus: newStatus,
		Comment:   comment,
		ChangedBy: moderatedBy,
	})

	v, err := uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return nil, err
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return v, nil
}

func (uc *VenueUseCase) GetModerationHistory(ctx context.Context, venueID uuid.UUID) ([]domain.ModerationHistoryEntry, error) {
	return uc.repo.GetModerationHistory(ctx, venueID)
}

func (uc *VenueUseCase) UpdateRating(ctx context.Context, venueID uuid.UUID, avgRating float64, reviewCount int32) error {
	if err := uc.repo.UpdateRating(ctx, venueID, avgRating, reviewCount); err != nil {
		return err
	}
	uc.redis.Del(ctx, fmt.Sprintf("venue:id:%s", venueID))
	return nil
}

func (uc *VenueUseCase) CheckSlot(ctx context.Context, venueID uuid.UUID, date, timeFrom, timeTo string) (bool, error) {
	return uc.repo.CheckSlot(ctx, venueID, date, timeFrom, timeTo)
}

func (uc *VenueUseCase) ReserveSlot(ctx context.Context, venueID, bookingID uuid.UUID, date, timeFrom, timeTo string) error {
	return uc.repo.ReserveSlot(ctx, venueID, bookingID, date, timeFrom, timeTo)
}

func (uc *VenueUseCase) ReleaseSlot(ctx context.Context, venueID, bookingID uuid.UUID) error {
	return uc.repo.ReleaseSlot(ctx, venueID, bookingID)
}

func (uc *VenueUseCase) invalidateCache(ctx context.Context, id uuid.UUID, slug string) {
	uc.redis.Del(ctx, fmt.Sprintf("venue:id:%s", id))
	if slug != "" {
		uc.redis.Del(ctx, fmt.Sprintf("venue:slug:%s", slug))
	}
}

func (uc *VenueUseCase) getFromCache(ctx context.Context, key string) (*domain.Venue, error) {
	data, err := uc.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var v domain.Venue
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (uc *VenueUseCase) setCache(ctx context.Context, key string, v *domain.Venue) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	uc.redis.Set(ctx, key, data, cacheTTL)
}
