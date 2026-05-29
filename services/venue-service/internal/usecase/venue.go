package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"golang.org/x/sync/singleflight"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
	"github.com/tienlao/agregator/services/venue-service/internal/repository"
)

const (
	defaultVenueCacheTTL  = 10 * time.Minute
	defaultSearchCacheTTL = 2 * time.Minute
	maxVenueServices      = 50
	maxServiceDurationMin = 10080 // 7 days in minutes
)

// Config holds tunable parameters for VenueUseCase.
// Zero values fall back to package defaults.
type Config struct {
	// VenueCacheTTL controls how long individual venue cards are cached in Redis.
	// Default: 10 minutes.
	VenueCacheTTL time.Duration
	// SearchCacheTTL controls how long list/search results are cached in Redis.
	// Default: 2 minutes.
	SearchCacheTTL time.Duration
}

type VenueUseCase struct {
	repo           domain.VenueRepository
	redis          *goredis.Client
	log            zerolog.Logger
	sf             singleflight.Group
	venueCacheTTL  time.Duration
	searchCacheTTL time.Duration
}

func NewVenueUseCase(repo domain.VenueRepository, redis *goredis.Client) *VenueUseCase {
	return NewVenueUseCaseWithLogger(repo, redis, zerolog.Nop())
}

func NewVenueUseCaseWithLogger(repo domain.VenueRepository, redis *goredis.Client, log zerolog.Logger) *VenueUseCase {
	return newVenueUseCase(repo, redis, log, Config{})
}

// NewVenueUseCaseWithConfig creates a VenueUseCase with configurable cache TTLs.
// Use this in main; tests can continue to use NewVenueUseCase / NewVenueUseCaseWithLogger.
func NewVenueUseCaseWithConfig(repo domain.VenueRepository, redis *goredis.Client, log zerolog.Logger, cfg Config) *VenueUseCase {
	return newVenueUseCase(repo, redis, log, cfg)
}

func newVenueUseCase(repo domain.VenueRepository, redis *goredis.Client, log zerolog.Logger, cfg Config) *VenueUseCase {
	if cfg.VenueCacheTTL <= 0 {
		cfg.VenueCacheTTL = defaultVenueCacheTTL
	}
	if cfg.SearchCacheTTL <= 0 {
		cfg.SearchCacheTTL = defaultSearchCacheTTL
	}
	return &VenueUseCase{
		repo:           repo,
		redis:          redis,
		log:            log,
		venueCacheTTL:  cfg.VenueCacheTTL,
		searchCacheTTL: cfg.SearchCacheTTL,
	}
}

func (uc *VenueUseCase) Create(ctx context.Context, venue *domain.Venue) error {
	if err := validateVenueType(venue.Type); err != nil {
		return err
	}
	if err := validateVenueCoordinates(venue.Latitude, venue.Longitude); err != nil {
		return err
	}
	if err := ValidateVenueVerificationForCreate(venue); err != nil {
		return err
	}
	if err := NormalizeVenuePayoutProfile(venue); err != nil {
		return err
	}
	sl, err := NormalizeSocialLinksJSON(venue.SocialLinks)
	if err != nil {
		return err
	}
	venue.SocialLinks = sl
	if err := uc.repo.Create(ctx, venue); err != nil {
		return err
	}
	uc.invalidateCache(ctx, venue.ID, venue.Slug)
	return nil
}

func (uc *VenueUseCase) Update(ctx context.Context, venue *domain.Venue) error {
	if err := validateVenueType(venue.Type); err != nil {
		return err
	}
	if err := validateVenueCoordinates(venue.Latitude, venue.Longitude); err != nil {
		return err
	}
	if err := ValidateVenueVerificationForUpdate(venue); err != nil {
		return err
	}
	if err := NormalizeVenuePayoutProfile(venue); err != nil {
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

func venueWriteGuardErr(err error) error {
	if errors.Is(err, repository.ErrVenueNotFound) {
		return pkgerrors.NotFound("площадка не найдена")
	}
	if errors.Is(err, repository.ErrVenueOwnershipMismatch) {
		return pkgerrors.PermissionDenied("нет прав на управление площадкой")
	}
	return nil
}

// ReplaceVenueServices replaces the full services list (owner cabinet).
func (uc *VenueUseCase) ReplaceVenueServices(ctx context.Context, venueID, ownerID uuid.UUID, services []domain.VenueService) error {
	if len(services) > maxVenueServices {
		return pkgerrors.InvalidArgument("превышен лимит услуг")
	}
	for _, s := range services {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			return pkgerrors.InvalidArgument("укажите название услуги")
		}
		if utf8.RuneCountInString(name) > 255 {
			return pkgerrors.InvalidArgument("название услуги слишком длинное")
		}
		if s.Price < 0 {
			return pkgerrors.InvalidArgument("некорректная цена услуги")
		}
		if s.DurationMin < 0 || s.DurationMin > maxServiceDurationMin {
			return pkgerrors.InvalidArgument("некорректная длительность услуги")
		}
	}

	existing, err := uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return err
	}
	if existing == nil {
		return pkgerrors.NotFound("площадка не найдена")
	}
	if existing.OwnerID != ownerID {
		return pkgerrors.PermissionDenied("нет прав на управление площадкой")
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

	if err := uc.repo.ReplaceVenueServices(ctx, venueID, ownerID, normalized); err != nil {
		if guardErr := venueWriteGuardErr(err); guardErr != nil {
			return guardErr
		}
		return err
	}
	uc.invalidateCache(ctx, venueID, existing.Slug)
	return nil
}

func (uc *VenueUseCase) GetByID(ctx context.Context, id uuid.UUID) (*domain.Venue, error) {
	v, err, _ := uc.sf.Do("gid:"+id.String(), func() (any, error) {
		return uc.getByIDUncached(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v.(*domain.Venue), nil
}

// GetByIDs fetches multiple venues in a single DB round-trip.
// No singleflight/cache is applied — this path is used only by internal batch calls
// (e.g. chat peer-name resolution) where staleness is acceptable and the request
// set is already deduplicated by the caller.
func (uc *VenueUseCase) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Venue, error) {
	return uc.repo.GetByIDs(ctx, ids)
}

func (uc *VenueUseCase) getByIDUncached(ctx context.Context, id uuid.UUID) (*domain.Venue, error) {
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
	v, err, _ := uc.sf.Do("gslug:"+slug, func() (any, error) {
		return uc.getBySlugUncached(ctx, slug)
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v.(*domain.Venue), nil
}

func (uc *VenueUseCase) getBySlugUncached(ctx context.Context, slug string) (*domain.Venue, error) {
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

func (uc *VenueUseCase) catalogVersion(ctx context.Context) int64 {
	if uc.redis == nil {
		return 0
	}
	ver, err := uc.redis.Get(ctx, "venue:catalog:ver").Int64()
	if err == goredis.Nil {
		return 0
	}
	if err != nil {
		return 0
	}
	return ver
}

func (uc *VenueUseCase) List(ctx context.Context, page, pageSize int32, venueType, sortBy string) (*domain.ListResult, error) {
	if uc.redis == nil {
		return uc.repo.List(ctx, page, pageSize, venueType, sortBy)
	}
	ver := uc.catalogVersion(ctx)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%d|%s|%s", ver, page, pageSize, venueType, sortBy)))
	rkey := fmt.Sprintf("venue:list:%x", sum[:])
	if raw, err := uc.redis.Get(ctx, rkey).Bytes(); err == nil {
		var res domain.ListResult
		if json.Unmarshal(raw, &res) == nil {
			return &res, nil
		}
	}
	res, err := uc.repo.List(ctx, page, pageSize, venueType, sortBy)
	if err != nil {
		return nil, err
	}
	if data, err := json.Marshal(res); err == nil {
		_ = uc.redis.Set(ctx, rkey, data, uc.searchCacheTTL).Err()
	}
	return res, nil
}

func searchParamsCacheString(ver int64, p domain.SearchParams) string {
	am := append([]string(nil), p.Amenities...)
	sort.Strings(am)
	cc := append([]string(nil), p.EffectiveCities()...)
	sort.Strings(cc)
	return fmt.Sprintf("%d|q=%s|c=%s|lat=%.5f|lng=%.5f|r=%.6f|vt=%s|pmin=%d|pmax=%d|rmin=%.6f|a=%s|page=%d|ps=%d",
		ver, p.Query, strings.Join(cc, ","), p.Lat, p.Lng, p.RadiusKM, p.VenueType, p.PriceMin, p.PriceMax, p.RatingMin, strings.Join(am, ","), p.Page, p.PageSize)
}

func (uc *VenueUseCase) Search(ctx context.Context, params domain.SearchParams) (*domain.ListResult, error) {
	if uc.redis == nil {
		return uc.repo.Search(ctx, params)
	}
	ver := uc.catalogVersion(ctx)
	sum := sha256.Sum256([]byte(searchParamsCacheString(ver, params)))
	rkey := fmt.Sprintf("venue:search:%x", sum[:])
	if raw, err := uc.redis.Get(ctx, rkey).Bytes(); err == nil {
		var res domain.ListResult
		if json.Unmarshal(raw, &res) == nil {
			return &res, nil
		}
	}
	res, err := uc.repo.Search(ctx, params)
	if err != nil {
		return nil, err
	}
	if data, err := json.Marshal(res); err == nil {
		_ = uc.redis.Set(ctx, rkey, data, uc.searchCacheTTL).Err()
	}
	return res, nil
}

func (uc *VenueUseCase) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]domain.Venue, error) {
	return uc.repo.ListByOwner(ctx, ownerID)
}

func (uc *VenueUseCase) ListByStatus(ctx context.Context, status string, page, pageSize int32, nameQuery string) (*domain.ListResult, error) {
	return uc.repo.ListByStatus(ctx, status, page, pageSize, nameQuery)
}

func (uc *VenueUseCase) SubmitVenueForReview(ctx context.Context, venueID, ownerID uuid.UUID) (*domain.Venue, error) {
	v, err := uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, pkgerrors.NotFound("площадка не найдена")
	}
	if v.OwnerID != ownerID || v.Status != domain.StatusDraft {
		return nil, pkgerrors.NotFound("площадка не найдена или не является черновиком")
	}
	if err := ValidateVenueDraftReadyForReview(v); err != nil {
		return nil, err
	}
	ok, err := uc.repo.SubmitDraftForReview(ctx, venueID, ownerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, pkgerrors.NotFound("площадка не найдена или не является черновиком")
	}
	v, err = uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, pkgerrors.NotFound("площадка не найдена")
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return v, nil
}

func (uc *VenueUseCase) Moderate(ctx context.Context, venueID uuid.UUID, action, comment string, moderatedBy uuid.UUID) (*domain.Venue, error) {
	existing, err := uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, pkgerrors.NotFound("площадка не найдена")
	}

	action = strings.TrimSpace(strings.ToLower(action))
	comment = strings.TrimSpace(comment)

	var newStatus string
	switch action {
	case "approve":
		newStatus = domain.StatusActive
		if existing.Status != domain.StatusPendingReview && existing.Status != domain.StatusRejected {
			return nil, pkgerrors.InvalidArgument("одобрение доступно только для заявок на проверке или отклонённых карточек")
		}
	case "reject":
		newStatus = domain.StatusRejected
		if existing.Status != domain.StatusPendingReview {
			return nil, pkgerrors.InvalidArgument("отклонение доступно только для заявок на проверке")
		}
	case "suspend":
		newStatus = domain.StatusSuspended
		if existing.Status != domain.StatusActive {
			return nil, pkgerrors.InvalidArgument("приостановка доступна только для активных заведений")
		}
	case "resume":
		newStatus = domain.StatusActive
		if existing.Status != domain.StatusSuspended {
			return nil, pkgerrors.InvalidArgument("возобновление доступно только для приостановленных заведений")
		}
	default:
		return nil, pkgerrors.InvalidArgument("неизвестное действие модерации")
	}

	if (action == "reject" || action == "suspend") && comment == "" {
		return nil, pkgerrors.InvalidArgument("комментарий обязателен для этого действия")
	}

	oldStatus := existing.Status

	if err := uc.repo.UpdateStatus(ctx, venueID, newStatus, comment, moderatedBy); err != nil {
		return nil, err
	}

	if err := uc.repo.InsertModerationHistory(ctx, &domain.ModerationHistoryEntry{
		VenueID:   venueID,
		OldStatus: oldStatus,
		NewStatus: newStatus,
		Comment:   comment,
		ChangedBy: moderatedBy,
	}); err != nil {
		uc.log.Warn().
			Err(err).
			Str("venue_id", venueID.String()).
			Str("old_status", oldStatus).
			Str("new_status", newStatus).
			Str("moderated_by", moderatedBy.String()).
			Msg("moderation history insert failed")
	}

	v, err := uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, pkgerrors.NotFound("площадка не найдена")
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return v, nil
}

// RecipientUserIDsForVenue возвращает владельца и CRM-персонал (для уведомлений после модерации). Без проверки прав вызывающего — только из доверенного кода сервиса.
// Strangler-fig phase B: reads venue_staff directly from the shared DB via
// StaffUserIDsForVenue. In phase C this becomes a NATS event handed off to
// the notifier (no synchronous CRM call from venue-service).
func (uc *VenueUseCase) RecipientUserIDsForVenue(ctx context.Context, venueID uuid.UUID) ([]uuid.UUID, error) {
	v, err := uc.repo.GetByID(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, fmt.Errorf("venue not found: %s", venueID)
	}
	staff, err := uc.repo.StaffUserIDsForVenue(ctx, venueID)
	if err != nil {
		return nil, err
	}
	out := []uuid.UUID{v.OwnerID}
	seen := map[uuid.UUID]struct{}{v.OwnerID: {}}
	for _, id := range staff {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// ensureVenueMember authorises actor as owner or staff for the venue.
// Replaces the old ensureVenueCRMMember which lived in the (now deleted)
// usecase/crm.go and went through repo.GetVenueManagementAccess.
// Strangler-fig phase B: backed by the IsVenueMember shared-DB read.
func (uc *VenueUseCase) ensureVenueMember(ctx context.Context, venueID, userID uuid.UUID) error {
	ok, err := uc.repo.IsVenueMember(ctx, venueID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return pkgerrors.PermissionDenied("нет доступа к управлению заведением")
	}
	return nil
}

func (uc *VenueUseCase) GetModerationHistory(ctx context.Context, venueID uuid.UUID) ([]domain.ModerationHistoryEntry, error) {
	return uc.repo.GetModerationHistory(ctx, venueID)
}

func (uc *VenueUseCase) UpdateRating(ctx context.Context, venueID uuid.UUID, avgRating float64, reviewCount int32) error {
	if avgRating < 0 || avgRating > 5 {
		return pkgerrors.InvalidArgument("средняя оценка должна быть в диапазоне 0–5")
	}
	if reviewCount < 0 {
		return pkgerrors.InvalidArgument("количество отзывов не может быть отрицательным")
	}
	if err := uc.repo.UpdateRating(ctx, venueID, avgRating, reviewCount); err != nil {
		return err
	}
	uc.invalidateCache(ctx, venueID, "")
	return nil
}

func (uc *VenueUseCase) CheckSlot(ctx context.Context, venueID uuid.UUID, date, timeFrom, timeTo string) (bool, error) {
	return uc.repo.CheckSlot(ctx, venueID, date, timeFrom, timeTo)
}

func (uc *VenueUseCase) BatchCheckSlots(ctx context.Context, venueID uuid.UUID, date string, slots [][2]string) ([]bool, error) {
	return uc.repo.BatchCheckSlots(ctx, venueID, date, slots)
}

func (uc *VenueUseCase) ReserveSlot(ctx context.Context, venueID, bookingID uuid.UUID, date, timeFrom, timeTo string) error {
	return uc.repo.ReserveSlot(ctx, venueID, bookingID, date, timeFrom, timeTo)
}

func (uc *VenueUseCase) ReleaseSlot(ctx context.Context, venueID, bookingID uuid.UUID) error {
	return uc.repo.ReleaseSlot(ctx, venueID, bookingID)
}

const (
	maxVenuePhotos          = domain.MaxVenuePhotos
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
		return nil, pkgerrors.NotFound("площадка не найдена")
	}
	if v.OwnerID != ownerID {
		return nil, pkgerrors.PermissionDenied("нет прав на управление площадкой")
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
	if err := uc.ensureVenueMember(ctx, venueID, ownerID); err != nil {
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
	if err := uc.ensureVenueMember(ctx, venueID, ownerID); err != nil {
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
	if err := uc.ensureVenueMember(ctx, venueID, ownerID); err != nil {
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
		return nil, pkgerrors.NotFound("площадка не найдена")
	}
	if v.OwnerID != ownerID {
		return nil, pkgerrors.PermissionDenied("нет прав на управление площадкой")
	}
	if len(v.Photos) >= maxVenuePhotos {
		return nil, pkgerrors.InvalidArgument("достигнут лимит фотографий")
	}
	prevStatus := v.Status
	if _, err := uc.repo.AddVenuePhoto(ctx, venueID, ownerID, url); err != nil {
		if guardErr := venueWriteGuardErr(err); guardErr != nil {
			return nil, guardErr
		}
		if errors.Is(err, repository.ErrVenuePhotosLimit) {
			return nil, pkgerrors.InvalidArgument("достигнут лимит фотографий")
		}
		return nil, err
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
		return "", pkgerrors.NotFound("площадка не найдена")
	}
	if v.OwnerID != ownerID {
		return "", pkgerrors.PermissionDenied("нет прав на управление площадкой")
	}
	prevStatus := v.Status
	deletedURL, err = uc.repo.DeleteVenuePhoto(ctx, venueID, ownerID, photoID)
	if err != nil {
		if guardErr := venueWriteGuardErr(err); guardErr != nil {
			return "", guardErr
		}
		if errors.Is(err, repository.ErrPhotoNotFound) {
			return "", pkgerrors.NotFound("фото не найдено")
		}
		return "", err
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
		return nil, pkgerrors.NotFound("площадка не найдена")
	}
	if v.OwnerID != ownerID {
		return nil, pkgerrors.PermissionDenied("нет прав на управление площадкой")
	}
	prevStatus := v.Status
	if err := uc.repo.SetVenueCoverPhoto(ctx, venueID, ownerID, photoID); err != nil {
		if guardErr := venueWriteGuardErr(err); guardErr != nil {
			return nil, guardErr
		}
		if errors.Is(err, repository.ErrPhotoNotFound) {
			return nil, pkgerrors.NotFound("фото не найдено")
		}
		return nil, err
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
		return pkgerrors.InvalidArgument("превышен лимит залов")
	}
	prevStatus := v.Status
	if err := uc.repo.ReplaceVenueHalls(ctx, venueID, ownerID, items); err != nil {
		if guardErr := venueWriteGuardErr(err); guardErr != nil {
			return guardErr
		}
		if errors.Is(err, repository.ErrHallNameRequired) {
			return pkgerrors.InvalidArgument("каждый зал должен иметь название")
		}
		if errors.Is(err, repository.ErrHallNotFound) || errors.Is(err, repository.ErrHallNotInVenue) {
			return pkgerrors.InvalidArgument("некорректный идентификатор зала")
		}
		return err
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
		if errors.Is(err, repository.ErrHallPhotosLimit) {
			return nil, pkgerrors.InvalidArgument("достигнут лимит фотографий зала")
		}
		if errors.Is(err, repository.ErrHallNotFound) || errors.Is(err, repository.ErrHallNotInVenue) {
			return nil, pkgerrors.NotFound("зал не найден")
		}
		return nil, err
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
		if errors.Is(err, repository.ErrPhotoNotFound) {
			return "", pkgerrors.NotFound("фото не найдено")
		}
		if errors.Is(err, repository.ErrHallNotFound) || errors.Is(err, repository.ErrHallNotInVenue) {
			return "", pkgerrors.NotFound("зал не найден")
		}
		return "", err
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
		if errors.Is(err, repository.ErrPhotoNotFound) {
			return nil, pkgerrors.NotFound("фото не найдено")
		}
		if errors.Is(err, repository.ErrHallNotFound) || errors.Is(err, repository.ErrHallNotInVenue) {
			return nil, pkgerrors.NotFound("зал не найден")
		}
		return nil, err
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
	if uc.redis == nil {
		return
	}
	uc.redis.Del(ctx, fmt.Sprintf("venue:id:%s", id))
	if slug != "" {
		uc.redis.Del(ctx, fmt.Sprintf("venue:slug:%s", slug))
	}
	_ = uc.redis.Incr(ctx, "venue:catalog:ver").Err()
}

func (uc *VenueUseCase) getFromCache(ctx context.Context, key string) (*domain.Venue, error) {
	if uc.redis == nil {
		return nil, goredis.Nil
	}
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
	if uc.redis == nil {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	uc.redis.Set(ctx, key, data, uc.venueCacheTTL)
}
