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
	"github.com/tienlao/agregator/services/venue-service/internal/geocode"
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
	// Geocoder resolves an address to coordinates when the caller supplies none.
	// Optional: nil disables geocoding and venues keep the coordinates they came
	// with (possibly none).
	Geocoder geocoder
}

// geocoder resolves a postal address to coordinates. Declared here, at the point
// of use, and satisfied by *geocode.Client; nil means the feature is off.
type geocoder interface {
	Geocode(ctx context.Context, address string) (geocode.Point, error)
}

type VenueUseCase struct {
	repo           domain.VenueRepository
	redis          *goredis.Client
	log            zerolog.Logger
	sf             singleflight.Group
	venueCacheTTL  time.Duration
	searchCacheTTL time.Duration
	geocoder       geocoder
}

func NewVenueUseCase(repo domain.VenueRepository, redis *goredis.Client) *VenueUseCase {
	return NewVenueUseCaseWithConfig(repo, redis, zerolog.Nop(), Config{})
}

// NewVenueUseCaseWithConfig creates a VenueUseCase with configurable cache TTLs
// and logger. Use this in main; tests use NewVenueUseCase (Nop logger).
func NewVenueUseCaseWithConfig(repo domain.VenueRepository, redis *goredis.Client, log zerolog.Logger, cfg Config) *VenueUseCase {
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
		geocoder:       cfg.Geocoder,
	}
}

// resolveCoordinates fills in venue.Latitude/Longitude from its address when the
// caller sent none. It never fails the write: an owner filing a card must not be
// blocked because an external geocoder is slow or down, and a venue without
// coordinates is still a usable listing — it just stays out of "бани рядом"
// until someone sets the point on the map in the edit form.
//
// A caller-supplied point always wins: the edit form has a real map, and a
// guessed address match must not silently move a pin the owner placed.
func (uc *VenueUseCase) resolveCoordinates(ctx context.Context, venue *domain.Venue) {
	if uc.geocoder == nil || venue.Latitude != 0 || venue.Longitude != 0 {
		return
	}
	address := strings.TrimSpace(venue.Address)
	if address == "" {
		return
	}
	// City first: "ул. Баумана, 5" exists in dozens of Russian cities, and the
	// geocoder will confidently return the wrong one. Matters from the first day
	// outside Kazan.
	query := address
	if city := strings.TrimSpace(venue.City); city != "" {
		query = city + ", " + address
	}

	point, err := uc.geocoder.Geocode(ctx, query)
	if err != nil {
		// Debug, not Warn: a miss is an ordinary outcome for a sloppy address and
		// would otherwise fill the log with noise on every such creation.
		uc.log.Debug().Err(err).Str("city", venue.City).Msg("geocode failed, venue stays without coordinates")
		return
	}
	venue.Latitude = point.Lat
	venue.Longitude = point.Lng
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
	if len(venue.Halls) > maxVenueHalls {
		return pkgerrors.InvalidArgument("превышен лимит залов")
	}
	for i := range venue.Halls {
		name := strings.TrimSpace(venue.Halls[i].Name)
		if name == "" {
			return pkgerrors.InvalidArgument("каждый зал должен иметь название")
		}
		venue.Halls[i].Name = name
	}
	uc.resolveCoordinates(ctx, venue)
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
	// Same gap on update: an API client may change the address without touching
	// the point, and the edit form's map only covers the browser path.
	uc.resolveCoordinates(ctx, venue)
	// repo.Update atomically sends an active/rejected card back to pending_review.
	if err := uc.repo.Update(ctx, venue); err != nil {
		return err
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

// PopularCities returns active-venue counts per city (most first), capped at limit.
func (uc *VenueUseCase) PopularCities(ctx context.Context, limit int32) ([]domain.CityCount, error) {
	return uc.repo.PopularCities(ctx, limit)
}

// geoCacheBucketDigits rounds coordinates in the cache key to ~110 m. At full
// precision every GPS reading is a unique key, so "бани рядом" would miss the
// cache on every request. The query itself still runs on exact coordinates —
// only the key is bucketed, so viewers within ~110 m share a cached page.
const geoCacheBucketDigits = 3

func searchParamsCacheString(ver int64, p domain.SearchParams) string {
	am := append([]string(nil), p.Amenities...)
	sort.Strings(am)
	cc := append([]string(nil), p.EffectiveCities()...)
	sort.Strings(cc)
	return fmt.Sprintf("%d|q=%s|c=%s|lat=%.*f|lng=%.*f|r=%.6f|vt=%s|pmin=%d|pmax=%d|rmin=%.6f|a=%s|page=%d|ps=%d",
		ver, p.Query, strings.Join(cc, ","),
		geoCacheBucketDigits, p.Lat, geoCacheBucketDigits, p.Lng,
		p.RadiusKM, p.VenueType, p.PriceMin, p.PriceMax, p.RatingMin, strings.Join(am, ","), p.Page, p.PageSize)
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

// SuspendByOwner suspends every venue owned by ownerID (account-deletion
// cascade). It reads the owner's venues first so each one's cache entry can be
// invalidated after the bulk status update; the catalog version is bumped too,
// dropping the suspended venues from cached public listings. Returns the number
// of venues transitioned to suspended.
func (uc *VenueUseCase) SuspendByOwner(ctx context.Context, ownerID uuid.UUID) (int64, error) {
	venues, err := uc.repo.ListByOwner(ctx, ownerID)
	if err != nil {
		return 0, err
	}
	count, err := uc.repo.SuspendByOwner(ctx, ownerID)
	if err != nil {
		return 0, err
	}
	for i := range venues {
		uc.invalidateCache(ctx, venues[i].ID, venues[i].Slug)
	}
	return count, nil
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

// effectiveHalls resolves which halls a reservation or availability check
// applies to: the requested halls in per-hall mode, or none (whole-venue) in
// whole mode. Concentrates the mode gate so reserve and check stay consistent.
//
// requireHall guards the reservation path: in per-hall mode a booking with no
// hall selected would write a whole-venue (hall_id NULL) row keyed on venue_id,
// which never collides with per-hall rows keyed on hall_id — so it would not
// fence the individual halls. We reject that whenever the venue actually has
// halls to pick from. Availability checks pass requireHall=false (a hall-less
// check just asks about the whole venue).
func (uc *VenueUseCase) effectiveHalls(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, requireHall bool) ([]uuid.UUID, error) {
	mode, err := uc.repo.BookingMode(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if mode != domain.BookingModePerHall {
		return nil, nil
	}
	if requireHall && len(hallIDs) == 0 {
		existing, err := uc.repo.HallIDs(ctx, venueID)
		if err != nil {
			return nil, err
		}
		if len(existing) > 0 {
			return nil, pkgerrors.InvalidArgument("в этом заведении нужно выбрать зал для брони")
		}
	}
	return hallIDs, nil
}

func (uc *VenueUseCase) CheckSlot(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, date, timeFrom, timeTo string) (bool, error) {
	halls, err := uc.effectiveHalls(ctx, venueID, hallIDs, false)
	if err != nil {
		return false, err
	}
	return uc.repo.CheckSlot(ctx, venueID, halls, date, timeFrom, timeTo)
}

func (uc *VenueUseCase) BatchCheckSlots(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, date string, slots [][2]string) ([]bool, error) {
	halls, err := uc.effectiveHalls(ctx, venueID, hallIDs, false)
	if err != nil {
		return nil, err
	}
	return uc.repo.BatchCheckSlots(ctx, venueID, halls, date, slots)
}

// ReserveSlot reserves the interval for a booking. In per-hall mode the booking's
// selected halls are each reserved independently; in whole mode (the default) a
// single whole-venue reservation is taken and hallIDs are ignored. Falling back
// to a whole-venue reservation whenever no halls apply is deliberately
// conservative — it never under-reserves.
func (uc *VenueUseCase) ReserveSlot(ctx context.Context, venueID, bookingID uuid.UUID, date, timeFrom, timeTo string, hallIDs []uuid.UUID) error {
	halls, err := uc.effectiveHalls(ctx, venueID, hallIDs, true)
	if err != nil {
		return err
	}
	return uc.repo.ReserveSlot(ctx, venueID, bookingID, date, timeFrom, timeTo, halls)
}

func (uc *VenueUseCase) ReleaseSlot(ctx context.Context, venueID, bookingID uuid.UUID) error {
	return uc.repo.ReleaseSlot(ctx, venueID, bookingID)
}

// GetBookingMode returns the venue's booking mode for a venue member.
func (uc *VenueUseCase) GetBookingMode(ctx context.Context, actorID, venueID uuid.UUID) (string, error) {
	if err := uc.ensureVenueMember(ctx, venueID, actorID); err != nil {
		return "", err
	}
	return uc.repo.BookingMode(ctx, venueID)
}

// SetBookingMode changes the venue's booking mode. Owner/staff only.
func (uc *VenueUseCase) SetBookingMode(ctx context.Context, actorID, venueID uuid.UUID, mode string) error {
	if err := uc.ensureVenueMember(ctx, venueID, actorID); err != nil {
		return err
	}
	if mode != domain.BookingModeWhole && mode != domain.BookingModePerHall {
		return pkgerrors.InvalidArgument("неизвестный режим бронирования")
	}
	if err := uc.repo.SetBookingMode(ctx, venueID, mode); err != nil {
		if errors.Is(err, repository.ErrBookingModeHasReservations) {
			return pkgerrors.FailedPrecondition("нельзя сменить режим бронирования: есть предстоящие брони или блокировки")
		}
		return err
	}
	return nil
}

const (
	maxVenuePhotos          = domain.MaxVenuePhotos
	maxVenueVideos          = domain.MaxVenueVideos
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

// CreateManualSlotBlock reserves a time interval without an aggregator booking
// (external booking). In per-hall mode the block targets the requested halls and
// at least one must be given; in whole mode it blocks the venue as a single
// interval.
func (uc *VenueUseCase) CreateManualSlotBlock(ctx context.Context, ownerID, venueID uuid.UUID, hallIDs []uuid.UUID, date, timeFrom, timeTo, note string) ([]domain.ManualSlotBlock, error) {
	if err := uc.ensureVenueMember(ctx, venueID, ownerID); err != nil {
		return nil, err
	}
	if err := validateSlotDateAndTimes(date, timeFrom, timeTo); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(note) > maxManualBlockNoteRunes {
		return nil, pkgerrors.InvalidArgument("комментарий слишком длинный")
	}

	halls, err := uc.resolveBlockHalls(ctx, venueID, hallIDs)
	if err != nil {
		return nil, err
	}

	blocks, err := uc.repo.CreateManualSlotBlock(ctx, venueID, halls, date, timeFrom, timeTo, note)
	if err != nil {
		if errors.Is(err, repository.ErrSlotUnavailable) {
			return nil, pkgerrors.InvalidArgument("время пересекается с уже занятым слотом или другой блокировкой")
		}
		return nil, err
	}
	return blocks, nil
}

// resolveBlockHalls picks the halls a manual block targets: none (whole-venue)
// in whole mode; in per-hall mode the requested halls (validated to belong to
// the venue), with an explicit hall required — a hall-less block would write a
// venue-keyed row that never fences the per-hall rows, mirroring the booking
// guard in effectiveHalls. A per-hall venue with no halls at all falls back to
// a whole-venue block (there are no per-hall rows to fence).
func (uc *VenueUseCase) resolveBlockHalls(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID) ([]uuid.UUID, error) {
	mode, err := uc.repo.BookingMode(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if mode != domain.BookingModePerHall {
		return nil, nil
	}
	venueHalls, err := uc.repo.HallIDs(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if len(venueHalls) == 0 {
		// per_hall, но залов ещё нет (не заведены или все удалены): ведём себя
		// как whole — одна venue-wide строка, как fallback брони в effectiveHalls.
		// Коллизии нет: без залов per-hall строк не существует.
		return nil, nil
	}
	if len(hallIDs) == 0 {
		return nil, pkgerrors.InvalidArgument("в этом заведении нужно выбрать зал для блокировки")
	}
	known := make(map[uuid.UUID]struct{}, len(venueHalls))
	for _, h := range venueHalls {
		known[h] = struct{}{}
	}
	for _, h := range hallIDs {
		if _, ok := known[h]; !ok {
			return nil, pkgerrors.InvalidArgument("зал не принадлежит заведению")
		}
	}
	return hallIDs, nil
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

// GetVenueSchedule returns the venue's occupied intervals (aggregator bookings
// and manual owner blocks) for the schedule grid, in the inclusive date range.
// Any venue member (owner or staff) may view it; enrichment of booking details
// happens at the gateway.
func (uc *VenueUseCase) GetVenueSchedule(ctx context.Context, actorID, venueID uuid.UUID, dateFrom, dateTo string) ([]domain.ScheduleEntry, error) {
	if err := uc.ensureVenueMember(ctx, venueID, actorID); err != nil {
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
	return uc.repo.ListVenueSchedule(ctx, venueID, dateFrom, dateTo)
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
	if _, err := uc.repo.AddVenuePhoto(ctx, venueID, ownerID, url); err != nil {
		if guardErr := venueWriteGuardErr(err); guardErr != nil {
			return nil, guardErr
		}
		if errors.Is(err, repository.ErrVenuePhotosLimit) {
			return nil, pkgerrors.InvalidArgument("достигнут лимит фотографий")
		}
		return nil, err
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
	uc.invalidateCache(ctx, venueID, v.Slug)
	return deletedURL, nil
}

func (uc *VenueUseCase) AddVenueVideo(ctx context.Context, venueID, ownerID uuid.UUID, url string) (*domain.Venue, error) {
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
	if len(v.Videos) >= maxVenueVideos {
		return nil, pkgerrors.InvalidArgument("достигнут лимит видео")
	}
	if _, err := uc.repo.AddVenueVideo(ctx, venueID, ownerID, url); err != nil {
		if guardErr := venueWriteGuardErr(err); guardErr != nil {
			return nil, guardErr
		}
		if errors.Is(err, repository.ErrVenueVideosLimit) {
			return nil, pkgerrors.InvalidArgument("достигнут лимит видео")
		}
		return nil, err
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return uc.repo.GetByID(ctx, venueID)
}

func (uc *VenueUseCase) DeleteVenueVideo(ctx context.Context, venueID, ownerID, videoID uuid.UUID) (deletedURL string, err error) {
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
	deletedURL, err = uc.repo.DeleteVenueVideo(ctx, venueID, ownerID, videoID)
	if err != nil {
		if guardErr := venueWriteGuardErr(err); guardErr != nil {
			return "", guardErr
		}
		if errors.Is(err, repository.ErrVideoNotFound) {
			return "", pkgerrors.NotFound("видео не найдено")
		}
		return "", err
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
	if err := uc.repo.SetVenueCoverPhoto(ctx, venueID, ownerID, photoID); err != nil {
		if guardErr := venueWriteGuardErr(err); guardErr != nil {
			return nil, guardErr
		}
		if errors.Is(err, repository.ErrPhotoNotFound) {
			return nil, pkgerrors.NotFound("фото не найдено")
		}
		return nil, err
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
	uc.invalidateCache(ctx, venueID, v.Slug)
	return nil
}

func (uc *VenueUseCase) AddVenueHallPhoto(ctx context.Context, venueID, hallID, ownerID uuid.UUID, url string) (*domain.Venue, error) {
	v, err := uc.ensureVenueOwner(ctx, venueID, ownerID)
	if err != nil {
		return nil, err
	}
	if _, err := uc.repo.AddVenueHallPhoto(ctx, venueID, hallID, url); err != nil {
		if errors.Is(err, repository.ErrHallPhotosLimit) {
			return nil, pkgerrors.InvalidArgument("достигнут лимит фотографий зала")
		}
		if errors.Is(err, repository.ErrHallNotFound) || errors.Is(err, repository.ErrHallNotInVenue) {
			return nil, pkgerrors.NotFound("зал не найден")
		}
		return nil, err
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return uc.repo.GetByID(ctx, venueID)
}

func (uc *VenueUseCase) DeleteVenueHallPhoto(ctx context.Context, venueID, hallID, ownerID, photoID uuid.UUID) (deletedURL string, err error) {
	v, err := uc.ensureVenueOwner(ctx, venueID, ownerID)
	if err != nil {
		return "", err
	}
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
	uc.invalidateCache(ctx, venueID, v.Slug)
	return deletedURL, nil
}

func (uc *VenueUseCase) SetVenueHallCoverPhoto(ctx context.Context, venueID, hallID, ownerID, photoID uuid.UUID) (*domain.Venue, error) {
	v, err := uc.ensureVenueOwner(ctx, venueID, ownerID)
	if err != nil {
		return nil, err
	}
	if err := uc.repo.SetVenueHallCoverPhoto(ctx, venueID, hallID, photoID); err != nil {
		if errors.Is(err, repository.ErrPhotoNotFound) {
			return nil, pkgerrors.NotFound("фото не найдено")
		}
		if errors.Is(err, repository.ErrHallNotFound) || errors.Is(err, repository.ErrHallNotInVenue) {
			return nil, pkgerrors.NotFound("зал не найден")
		}
		return nil, err
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
