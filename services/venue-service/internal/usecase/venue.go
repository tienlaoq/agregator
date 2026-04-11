package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
	"github.com/tienlao/agregator/services/venue-service/internal/repository"
)

const (
	cacheTTL              = 10 * time.Minute
	maxVenueServices      = 50
	maxServiceDurationMin = 10080 // 7 days in minutes
)

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
	sl, err := NormalizeSocialLinksJSON(venue.SocialLinks)
	if err != nil {
		return err
	}
	venue.SocialLinks = sl
	return uc.repo.Create(ctx, venue)
}

func (uc *VenueUseCase) Update(ctx context.Context, venue *domain.Venue) error {
	if err := ValidateVenueVerificationForUpdate(venue); err != nil {
		return err
	}
	sl, err := NormalizeSocialLinksJSON(venue.SocialLinks)
	if err != nil {
		return err
	}
	venue.SocialLinks = sl
	prevStatus := venue.Status
	if err := uc.repo.Update(ctx, venue); err != nil {
		return err
	}
	if prevStatus == domain.StatusActive || prevStatus == domain.StatusRejected {
		if err := uc.repo.ResetToPendingReview(ctx, venue.ID); err != nil {
			return err
		}
		venue.Status = domain.StatusPendingReview
		venue.ModerationComment = ""
		venue.IsActive = false
		venue.ModeratedAt = nil
		venue.ModeratedBy = nil
	}
	uc.invalidateCache(ctx, venue.ID, venue.Slug)
	return nil
}

// ReplaceVenueServices replaces the full services list (owner cabinet). Caller must verify ownership.
func (uc *VenueUseCase) ReplaceVenueServices(ctx context.Context, venueID, ownerID uuid.UUID, services []domain.VenueService) error {
	if len(services) > maxVenueServices {
		return pkgerrors.InvalidArgument("too many services")
	}
	for _, s := range services {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			return pkgerrors.InvalidArgument("service name is required")
		}
		if utf8.RuneCountInString(name) > 255 {
			return pkgerrors.InvalidArgument("service name is too long")
		}
		if s.Price < 0 {
			return pkgerrors.InvalidArgument("invalid service price")
		}
		if s.DurationMin < 0 || s.DurationMin > maxServiceDurationMin {
			return pkgerrors.InvalidArgument("invalid service duration")
		}
	}

	existing, err := uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return err
	}
	if existing == nil {
		return pkgerrors.NotFound("venue not found")
	}
	if existing.OwnerID != ownerID {
		return pkgerrors.PermissionDenied("not the venue owner")
	}

	normalized := make([]domain.VenueService, len(services))
	for i, s := range services {
		normalized[i] = domain.VenueService{
			Name:        strings.TrimSpace(s.Name),
			DurationMin: s.DurationMin,
			Price:       s.Price,
			Description: strings.TrimSpace(s.Description),
		}
	}

	if err := uc.repo.ReplaceVenueServices(ctx, venueID, normalized); err != nil {
		return err
	}
	uc.invalidateCache(ctx, venueID, existing.Slug)
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

func (uc *VenueUseCase) SubmitVenueForReview(ctx context.Context, venueID, ownerID uuid.UUID) (*domain.Venue, error) {
	ok, err := uc.repo.SubmitDraftForReview(ctx, venueID, ownerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, pkgerrors.NotFound("venue not found or not a draft")
	}
	v, err := uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, pkgerrors.NotFound("venue not found")
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return v, nil
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

const (
	maxVenuePhotos          = 24
	maxVenueHalls           = 15
	maxManualBlockNoteRunes = 500
	maxManualBlockRangeDays = 120
)

func (uc *VenueUseCase) ensureVenueOwner(ctx context.Context, venueID, ownerID uuid.UUID) (*domain.Venue, error) {
	v, err := uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, pkgerrors.NotFound("venue not found")
	}
	if v.OwnerID != ownerID {
		return nil, pkgerrors.PermissionDenied("not the venue owner")
	}
	return v, nil
}

func validateSlotDateAndTimes(date, timeFrom, timeTo string) error {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return pkgerrors.InvalidArgument("неверная дата, ожидается YYYY-MM-DD")
	}
	tf, err := time.Parse("15:04", timeFrom)
	if err != nil {
		return pkgerrors.InvalidArgument("неверное время «с», ожидается ЧЧ:ММ")
	}
	tt, err := time.Parse("15:04", timeTo)
	if err != nil {
		return pkgerrors.InvalidArgument("неверное время «до», ожидается ЧЧ:ММ")
	}
	if !tt.After(tf) {
		return pkgerrors.InvalidArgument("время «до» должно быть позже «с»")
	}
	return nil
}

// CreateManualSlotBlock reserves a time interval without an aggregator booking (external booking).
func (uc *VenueUseCase) CreateManualSlotBlock(ctx context.Context, ownerID, venueID uuid.UUID, date, timeFrom, timeTo, note string) (uuid.UUID, error) {
	if _, err := uc.ensureVenueOwner(ctx, venueID, ownerID); err != nil {
		return uuid.Nil, err
	}
	if err := validateSlotDateAndTimes(date, timeFrom, timeTo); err != nil {
		return uuid.Nil, err
	}
	if utf8.RuneCountInString(note) > maxManualBlockNoteRunes {
		return uuid.Nil, pkgerrors.InvalidArgument("комментарий слишком длинный")
	}
	id, err := uc.repo.CreateManualSlotBlock(ctx, venueID, date, timeFrom, timeTo, note)
	if err != nil {
		if errors.Is(err, repository.ErrSlotUnavailable) {
			return uuid.Nil, pkgerrors.InvalidArgument("время пересекается с уже занятым слотом или другой блокировкой")
		}
		return uuid.Nil, err
	}
	return id, nil
}

// DeleteManualSlotBlock removes a manual block created by the owner.
func (uc *VenueUseCase) DeleteManualSlotBlock(ctx context.Context, ownerID, venueID, blockID uuid.UUID) error {
	if _, err := uc.ensureVenueOwner(ctx, venueID, ownerID); err != nil {
		return err
	}
	ok, err := uc.repo.DeleteManualSlotBlock(ctx, venueID, blockID)
	if err != nil {
		return err
	}
	if !ok {
		return pkgerrors.NotFound("блокировка не найдена")
	}
	return nil
}

// ListManualSlotBlocks returns manual blocks for the venue in the inclusive date range.
func (uc *VenueUseCase) ListManualSlotBlocks(ctx context.Context, ownerID, venueID uuid.UUID, dateFrom, dateTo string) ([]domain.ManualSlotBlock, error) {
	if _, err := uc.ensureVenueOwner(ctx, venueID, ownerID); err != nil {
		return nil, err
	}
	d0, err := time.Parse("2006-01-02", dateFrom)
	if err != nil {
		return nil, pkgerrors.InvalidArgument("неверная дата «с», ожидается YYYY-MM-DD")
	}
	d1, err := time.Parse("2006-01-02", dateTo)
	if err != nil {
		return nil, pkgerrors.InvalidArgument("неверная дата «по», ожидается YYYY-MM-DD")
	}
	if d1.Before(d0) {
		return nil, pkgerrors.InvalidArgument("дата «по» не может быть раньше даты «с»")
	}
	if d1.Sub(d0) > maxManualBlockRangeDays*24*time.Hour {
		return nil, pkgerrors.InvalidArgument("слишком большой диапазон дат")
	}
	return uc.repo.ListManualSlotBlocks(ctx, venueID, dateFrom, dateTo)
}

func (uc *VenueUseCase) AddVenuePhoto(ctx context.Context, venueID, ownerID uuid.UUID, url string) (*domain.Venue, error) {
	v, err := uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, pkgerrors.NotFound("venue not found")
	}
	if v.OwnerID != ownerID {
		return nil, pkgerrors.PermissionDenied("not the venue owner")
	}
	if len(v.Photos) >= maxVenuePhotos {
		return nil, pkgerrors.InvalidArgument("maximum number of photos reached")
	}
	prevStatus := v.Status
	if _, err := uc.repo.AddVenuePhoto(ctx, venueID, url); err != nil {
		if strings.Contains(err.Error(), "maximum number") {
			return nil, pkgerrors.InvalidArgument("maximum number of photos reached")
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	if prevStatus == domain.StatusActive || prevStatus == domain.StatusRejected {
		if err := uc.repo.ResetToPendingReview(ctx, venueID); err != nil {
			return nil, err
		}
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return uc.repo.GetByID(ctx, venueID)
}

func (uc *VenueUseCase) DeleteVenuePhoto(ctx context.Context, venueID, ownerID, photoID uuid.UUID) (deletedURL string, err error) {
	v, err := uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", pkgerrors.NotFound("venue not found")
	}
	if v.OwnerID != ownerID {
		return "", pkgerrors.PermissionDenied("not the venue owner")
	}
	prevStatus := v.Status
	deletedURL, err = uc.repo.DeleteVenuePhoto(ctx, venueID, photoID)
	if err != nil {
		if strings.Contains(err.Error(), "photo not found") {
			return "", pkgerrors.NotFound("photo not found")
		}
		return "", pkgerrors.Internal(err.Error())
	}
	if prevStatus == domain.StatusActive || prevStatus == domain.StatusRejected {
		if err := uc.repo.ResetToPendingReview(ctx, venueID); err != nil {
			return deletedURL, err
		}
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return deletedURL, nil
}

func (uc *VenueUseCase) SetVenueCoverPhoto(ctx context.Context, venueID, ownerID, photoID uuid.UUID) (*domain.Venue, error) {
	v, err := uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, pkgerrors.NotFound("venue not found")
	}
	if v.OwnerID != ownerID {
		return nil, pkgerrors.PermissionDenied("not the venue owner")
	}
	prevStatus := v.Status
	if err := uc.repo.SetVenueCoverPhoto(ctx, venueID, photoID); err != nil {
		if strings.Contains(err.Error(), "photo not found") {
			return nil, pkgerrors.NotFound("photo not found")
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	if prevStatus == domain.StatusActive || prevStatus == domain.StatusRejected {
		if err := uc.repo.ResetToPendingReview(ctx, venueID); err != nil {
			return nil, err
		}
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return uc.repo.GetByID(ctx, venueID)
}

func (uc *VenueUseCase) ReplaceVenueHalls(ctx context.Context, venueID, ownerID uuid.UUID, items []domain.VenueHallUpsert) error {
	v, err := uc.ensureVenueOwner(ctx, venueID, ownerID)
	if err != nil {
		return err
	}
	if len(items) > maxVenueHalls {
		return pkgerrors.InvalidArgument("too many halls")
	}
	prevStatus := v.Status
	if err := uc.repo.ReplaceVenueHalls(ctx, venueID, items); err != nil {
		if strings.Contains(err.Error(), "hall name is required") {
			return pkgerrors.InvalidArgument("each hall must have a name")
		}
		if strings.Contains(err.Error(), "hall not found") || strings.Contains(err.Error(), "does not belong") {
			return pkgerrors.InvalidArgument("invalid hall id")
		}
		return pkgerrors.Internal(err.Error())
	}
	if prevStatus == domain.StatusActive || prevStatus == domain.StatusRejected {
		if err := uc.repo.ResetToPendingReview(ctx, venueID); err != nil {
			return err
		}
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return nil
}

func (uc *VenueUseCase) AddVenueHallPhoto(ctx context.Context, venueID, hallID, ownerID uuid.UUID, url string) (*domain.Venue, error) {
	v, err := uc.ensureVenueOwner(ctx, venueID, ownerID)
	if err != nil {
		return nil, err
	}
	prevStatus := v.Status
	if _, err := uc.repo.AddVenueHallPhoto(ctx, venueID, hallID, url); err != nil {
		if strings.Contains(err.Error(), "maximum number") {
			return nil, pkgerrors.InvalidArgument("maximum number of hall photos reached")
		}
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not belong") {
			return nil, pkgerrors.NotFound("hall not found")
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	if prevStatus == domain.StatusActive || prevStatus == domain.StatusRejected {
		if err := uc.repo.ResetToPendingReview(ctx, venueID); err != nil {
			return nil, err
		}
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return uc.repo.GetByID(ctx, venueID)
}

func (uc *VenueUseCase) DeleteVenueHallPhoto(ctx context.Context, venueID, hallID, ownerID, photoID uuid.UUID) (deletedURL string, err error) {
	v, err := uc.ensureVenueOwner(ctx, venueID, ownerID)
	if err != nil {
		return "", err
	}
	prevStatus := v.Status
	deletedURL, err = uc.repo.DeleteVenueHallPhoto(ctx, venueID, hallID, photoID)
	if err != nil {
		if strings.Contains(err.Error(), "photo not found") {
			return "", pkgerrors.NotFound("photo not found")
		}
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not belong") {
			return "", pkgerrors.NotFound("hall not found")
		}
		return "", pkgerrors.Internal(err.Error())
	}
	if prevStatus == domain.StatusActive || prevStatus == domain.StatusRejected {
		if err := uc.repo.ResetToPendingReview(ctx, venueID); err != nil {
			return deletedURL, err
		}
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return deletedURL, nil
}

func (uc *VenueUseCase) SetVenueHallCoverPhoto(ctx context.Context, venueID, hallID, ownerID, photoID uuid.UUID) (*domain.Venue, error) {
	v, err := uc.ensureVenueOwner(ctx, venueID, ownerID)
	if err != nil {
		return nil, err
	}
	prevStatus := v.Status
	if err := uc.repo.SetVenueHallCoverPhoto(ctx, venueID, hallID, photoID); err != nil {
		if strings.Contains(err.Error(), "photo not found") {
			return nil, pkgerrors.NotFound("photo not found")
		}
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not belong") {
			return nil, pkgerrors.NotFound("hall not found")
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	if prevStatus == domain.StatusActive || prevStatus == domain.StatusRejected {
		if err := uc.repo.ResetToPendingReview(ctx, venueID); err != nil {
			return nil, err
		}
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return uc.repo.GetByID(ctx, venueID)
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
