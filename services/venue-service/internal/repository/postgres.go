package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

// ErrSlotUnavailable is returned when the requested interval overlaps an existing reservation.
var ErrSlotUnavailable = errors.New("slot unavailable")

// ErrVenueNotFound is returned when a locked venue row no longer exists.
var ErrVenueNotFound = errors.New("venue not found")

// ErrVenueOwnershipMismatch is returned when a locked venue is no longer owned by the actor.
var ErrVenueOwnershipMismatch = errors.New("venue ownership mismatch")

// ErrPhotoNotFound is returned when a venue or hall photo row does not exist.
var ErrPhotoNotFound = errors.New("photo not found")

// ErrHallNotFound is returned when a venue hall row does not exist.
var ErrHallNotFound = errors.New("hall not found")

// ErrHallNotInVenue is returned when a hall exists but belongs to a different venue.
var ErrHallNotInVenue = errors.New("hall does not belong to venue")

// ErrHallNameRequired is returned when a hall upsert is missing a name.
var ErrHallNameRequired = errors.New("hall name is required")

// ErrVenuePhotosLimit is returned when adding a venue photo would exceed MaxVenuePhotos.
var ErrVenuePhotosLimit = errors.New("venue photos limit reached")

// ErrHallPhotosLimit is returned when adding a hall photo would exceed the per-hall limit.
var ErrHallPhotosLimit = errors.New("hall photos limit reached")

type venueRepo struct {
	pool *pgxpool.Pool
}

func NewVenueRepo(pool *pgxpool.Pool) domain.VenueRepository {
	return &venueRepo{pool: pool}
}

func lockVenueForOwner(ctx context.Context, tx pgx.Tx, venueID, ownerID uuid.UUID) error {
	var currentOwnerID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT owner_id FROM venues WHERE id = $1 FOR UPDATE`, venueID).Scan(&currentOwnerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrVenueNotFound
		}
		return fmt.Errorf("lock venue: %w", err)
	}
	if currentOwnerID != ownerID {
		return ErrVenueOwnershipMismatch
	}
	return nil
}

func (r *venueRepo) Create(ctx context.Context, venue *domain.Venue) error {
	venue.Slug = slug.Make(venue.Name)

	existing, _ := r.GetBySlug(ctx, venue.Slug)
	if existing != nil {
		venue.Slug = venue.Slug + "-" + uuid.New().String()[:8]
	}

	if venue.WorkingHours == "" {
		venue.WorkingHours = "{}"
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if venue.Status == "" {
		venue.Status = domain.StatusPendingReview
	}

	if venue.SocialLinks == "" {
		venue.SocialLinks = "{}"
	}

	// Payout rails are owned by payment-service (escrow flow); venues carry only
	// the provider-agnostic payout profile fields.
	err = tx.QueryRow(ctx, `
		INSERT INTO venues (owner_id, slug, name, type, description, address, city, location, price_from, capacity, amenities, working_hours, phone, status,
			legal_entity_name, inn, ogrn, public_listing_url, verification_note, social_links,
			payout_legal_form)
		VALUES ($1, $2, $3, $4, $5, $6, $7, ST_SetSRID(ST_MakePoint($8, $9), 4326)::geography, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21::jsonb, $22)
		RETURNING id, created_at, updated_at`,
		venue.OwnerID, venue.Slug, venue.Name, venue.Type, venue.Description,
		venue.Address, venue.City, venue.Longitude, venue.Latitude,
		venue.PriceFrom, venue.Capacity, venue.Amenities, venue.WorkingHours, venue.Phone,
		venue.Status,
		venue.LegalEntityName, venue.INN, venue.OGRN, venue.PublicListingURL, venue.VerificationNote, venue.SocialLinks,
		venue.PayoutLegalForm,
	).Scan(&venue.ID, &venue.CreatedAt, &venue.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert venue: %w", err)
	}

	for i := range venue.Services {
		svc := &venue.Services[i]
		err = tx.QueryRow(ctx, `
			INSERT INTO venue_services (venue_id, name, duration_min, price, description, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id`,
			venue.ID, svc.Name, svc.DurationMin, svc.Price, svc.Description, int32(i),
		).Scan(&svc.ID)
		if err != nil {
			return fmt.Errorf("insert venue_service: %w", err)
		}
		svc.VenueID = venue.ID
	}

	return tx.Commit(ctx)
}

func (r *venueRepo) ReplaceVenueServices(ctx context.Context, venueID, ownerID uuid.UUID, services []domain.VenueService) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockVenueForOwner(ctx, tx, venueID, ownerID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM venue_services WHERE venue_id = $1`, venueID); err != nil {
		return fmt.Errorf("delete venue_services: %w", err)
	}

	for i := range services {
		svc := &services[i]
		svc.VenueID = venueID
		svc.SortOrder = int32(i)
		err = tx.QueryRow(ctx, `
			INSERT INTO venue_services (venue_id, name, duration_min, price, description, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id`,
			venueID, svc.Name, svc.DurationMin, svc.Price, svc.Description, int32(i),
		).Scan(&svc.ID)
		if err != nil {
			return fmt.Errorf("insert venue_service: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *venueRepo) Update(ctx context.Context, venue *domain.Venue) error {
	if venue.WorkingHours == "" {
		venue.WorkingHours = "{}"
	}
	if venue.SocialLinks == "" {
		venue.SocialLinks = "{}"
	}

	_, err := r.pool.Exec(ctx, `
		UPDATE venues SET
			name = $2, description = $3, address = $4, city = $5,
			location = ST_SetSRID(ST_MakePoint($6, $7), 4326)::geography,
			price_from = $8, capacity = $9, amenities = $10,
			working_hours = $11, phone = $12,
			legal_entity_name = $13, inn = $14, ogrn = $15,
			public_listing_url = $16, social_links = $17::jsonb, verification_note = $18,
			payout_legal_form = $19,
			updated_at = now()
		WHERE id = $1`,
		venue.ID, venue.Name, venue.Description, venue.Address, venue.City,
		venue.Longitude, venue.Latitude,
		venue.PriceFrom, venue.Capacity, venue.Amenities,
		venue.WorkingHours, venue.Phone,
		venue.LegalEntityName, venue.INN, venue.OGRN, venue.PublicListingURL, venue.SocialLinks, venue.VerificationNote,
		venue.PayoutLegalForm,
	)
	if err != nil {
		return fmt.Errorf("update venue: %w", err)
	}
	return nil
}

func (r *venueRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Venue, error) {
	return r.getVenue(ctx, "WHERE v.id = $1", id)
}

func (r *venueRepo) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Venue, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT v.id, v.owner_id, v.slug, v.name, v.type, v.description, v.address, v.city,
			ST_Y(v.location::geometry) AS latitude, ST_X(v.location::geometry) AS longitude,
			v.price_from, v.capacity, v.amenities, v.working_hours, v.phone,
			v.avg_rating, v.review_count, v.is_active, v.status, v.moderation_comment,
			v.moderated_at, v.moderated_by,
			v.legal_entity_name, v.inn, v.ogrn, v.public_listing_url, v.verification_note,
			v.social_links::text,
			COALESCE(v.payout_legal_form, ''),
			v.created_at, v.updated_at
		FROM venues v WHERE v.id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, fmt.Errorf("get venues batch: %w", err)
	}
	defer rows.Close()
	var venues []domain.Venue
	for rows.Next() {
		var v domain.Venue
		if err := rows.Scan(
			&v.ID, &v.OwnerID, &v.Slug, &v.Name, &v.Type, &v.Description, &v.Address, &v.City,
			&v.Latitude, &v.Longitude,
			&v.PriceFrom, &v.Capacity, &v.Amenities, &v.WorkingHours, &v.Phone,
			&v.AvgRating, &v.ReviewCount, &v.IsActive, &v.Status, &v.ModerationComment,
			&v.ModeratedAt, &v.ModeratedBy,
			&v.LegalEntityName, &v.INN, &v.OGRN, &v.PublicListingURL, &v.VerificationNote,
			&v.SocialLinks,
			&v.PayoutLegalForm,
			&v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan venue batch: %w", err)
		}
		venues = append(venues, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.attachVenueDetailsBatch(ctx, venues); err != nil {
		return nil, err
	}
	return venues, nil
}

func (r *venueRepo) GetBySlug(ctx context.Context, s string) (*domain.Venue, error) {
	return r.getVenue(ctx, "WHERE v.slug = $1", s)
}

func (r *venueRepo) getVenue(ctx context.Context, where string, arg any) (*domain.Venue, error) {
	v := &domain.Venue{}
	err := r.pool.QueryRow(ctx, `
		SELECT v.id, v.owner_id, v.slug, v.name, v.type, v.description, v.address, v.city,
			ST_Y(v.location::geometry) AS latitude, ST_X(v.location::geometry) AS longitude,
			v.price_from, v.capacity, v.amenities, v.working_hours, v.phone,
			v.avg_rating, v.review_count, v.is_active, v.status, v.moderation_comment,
			v.moderated_at, v.moderated_by,
			v.legal_entity_name, v.inn, v.ogrn, v.public_listing_url, v.verification_note,
			v.social_links::text,
			COALESCE(v.payout_legal_form, ''),
			v.created_at, v.updated_at
		FROM venues v `+where, arg,
	).Scan(
		&v.ID, &v.OwnerID, &v.Slug, &v.Name, &v.Type, &v.Description, &v.Address, &v.City,
		&v.Latitude, &v.Longitude,
		&v.PriceFrom, &v.Capacity, &v.Amenities, &v.WorkingHours, &v.Phone,
		&v.AvgRating, &v.ReviewCount, &v.IsActive, &v.Status, &v.ModerationComment,
		&v.ModeratedAt, &v.ModeratedBy,
		&v.LegalEntityName, &v.INN, &v.OGRN, &v.PublicListingURL, &v.VerificationNote,
		&v.SocialLinks,
		&v.PayoutLegalForm,
		&v.CreatedAt, &v.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get venue: %w", err)
	}

	services, err := r.getServices(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	v.Services = services

	photos, err := r.getPhotos(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	v.Photos = photos

	halls, err := r.getHallsWithPhotos(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	v.Halls = halls

	return v, nil
}

func (r *venueRepo) getServices(ctx context.Context, venueID uuid.UUID) ([]domain.VenueService, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, venue_id, name, duration_min, price, description, sort_order
		FROM venue_services WHERE venue_id = $1
		ORDER BY sort_order ASC, id ASC`, venueID)
	if err != nil {
		return nil, fmt.Errorf("get venue_services: %w", err)
	}
	defer rows.Close()

	var services []domain.VenueService
	for rows.Next() {
		var s domain.VenueService
		if err := rows.Scan(&s.ID, &s.VenueID, &s.Name, &s.DurationMin, &s.Price, &s.Description, &s.SortOrder); err != nil {
			return nil, fmt.Errorf("scan venue_service: %w", err)
		}
		services = append(services, s)
	}
	return services, rows.Err()
}

func (r *venueRepo) getPhotos(ctx context.Context, venueID uuid.UUID) ([]domain.VenuePhoto, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, venue_id, url, sort_order, is_cover
		FROM venue_photos WHERE venue_id = $1 ORDER BY sort_order`, venueID)
	if err != nil {
		return nil, fmt.Errorf("get venue_photos: %w", err)
	}
	defer rows.Close()

	var photos []domain.VenuePhoto
	for rows.Next() {
		var p domain.VenuePhoto
		if err := rows.Scan(&p.ID, &p.VenueID, &p.URL, &p.SortOrder, &p.IsCover); err != nil {
			return nil, fmt.Errorf("scan venue_photo: %w", err)
		}
		photos = append(photos, p)
	}
	return photos, rows.Err()
}

// attachVenueDetails подгружает услуги, фото карточки и залы с фото (модерация, кабинет владельца).
func (r *venueRepo) attachVenueDetails(ctx context.Context, v *domain.Venue) error {
	services, err := r.getServices(ctx, v.ID)
	if err != nil {
		return fmt.Errorf("attach venue services: %w", err)
	}
	v.Services = services
	photos, err := r.getPhotos(ctx, v.ID)
	if err != nil {
		return fmt.Errorf("attach venue photos: %w", err)
	}
	v.Photos = photos
	halls, err := r.getHallsWithPhotos(ctx, v.ID)
	if err != nil {
		return fmt.Errorf("attach venue halls: %w", err)
	}
	v.Halls = halls
	return nil
}

func (r *venueRepo) getServicesByVenueIDs(ctx context.Context, venueIDs []uuid.UUID) (map[uuid.UUID][]domain.VenueService, error) {
	if len(venueIDs) == 0 {
		return map[uuid.UUID][]domain.VenueService{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, venue_id, name, duration_min, price, description, sort_order
		FROM venue_services WHERE venue_id = ANY($1::uuid[])
		ORDER BY venue_id, sort_order ASC, id ASC`, venueIDs)
	if err != nil {
		return nil, fmt.Errorf("get venue_services batch: %w", err)
	}
	defer rows.Close()

	out := make(map[uuid.UUID][]domain.VenueService)
	for rows.Next() {
		var s domain.VenueService
		if err := rows.Scan(&s.ID, &s.VenueID, &s.Name, &s.DurationMin, &s.Price, &s.Description, &s.SortOrder); err != nil {
			return nil, fmt.Errorf("scan venue_service: %w", err)
		}
		out[s.VenueID] = append(out[s.VenueID], s)
	}
	return out, rows.Err()
}

// getHallsWithPhotosByVenueIDs loads halls and their photos for many venues (owner / CRM lists).
func (r *venueRepo) getHallsWithPhotosByVenueIDs(ctx context.Context, venueIDs []uuid.UUID) (map[uuid.UUID][]domain.VenueHall, error) {
	out := make(map[uuid.UUID][]domain.VenueHall)
	if len(venueIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, venue_id, name, price_from, capacity, amenities, sort_order
		FROM venue_halls WHERE venue_id = ANY($1::uuid[])
		ORDER BY venue_id, sort_order ASC, id ASC`, venueIDs)
	if err != nil {
		return nil, fmt.Errorf("get venue_halls batch: %w", err)
	}
	defer rows.Close()

	type hallRow struct {
		h       domain.VenueHall
		venueID uuid.UUID
	}
	var flat []hallRow
	for rows.Next() {
		var h domain.VenueHall
		if err := rows.Scan(&h.ID, &h.VenueID, &h.Name, &h.PriceFrom, &h.Capacity, &h.Amenities, &h.SortOrder); err != nil {
			return nil, fmt.Errorf("scan venue_hall: %w", err)
		}
		flat = append(flat, hallRow{h: h, venueID: h.VenueID})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(flat) == 0 {
		return out, nil
	}

	hallIDs := make([]uuid.UUID, len(flat))
	for i := range flat {
		hallIDs[i] = flat[i].h.ID
	}
	prows, err := r.pool.Query(ctx, `
		SELECT id, hall_id, url, sort_order, is_cover
		FROM venue_hall_photos WHERE hall_id = ANY($1::uuid[])
		ORDER BY hall_id, sort_order ASC, id ASC`, hallIDs)
	if err != nil {
		return nil, fmt.Errorf("get venue_hall_photos batch: %w", err)
	}
	defer prows.Close()

	byHall := make(map[uuid.UUID][]domain.VenueHallPhoto)
	for prows.Next() {
		var p domain.VenueHallPhoto
		if err := prows.Scan(&p.ID, &p.HallID, &p.URL, &p.SortOrder, &p.IsCover); err != nil {
			return nil, fmt.Errorf("scan venue_hall_photo: %w", err)
		}
		byHall[p.HallID] = append(byHall[p.HallID], p)
	}
	if err := prows.Err(); err != nil {
		return nil, err
	}
	for _, hr := range flat {
		h := hr.h
		h.Photos = byHall[h.ID]
		if h.Photos == nil {
			h.Photos = []domain.VenueHallPhoto{}
		}
		out[hr.venueID] = append(out[hr.venueID], h)
	}
	return out, nil
}

func (r *venueRepo) attachVenueDetailsBatch(ctx context.Context, venues []domain.Venue) error {
	if len(venues) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(venues))
	for i := range venues {
		ids[i] = venues[i].ID
	}
	svcBy, err := r.getServicesByVenueIDs(ctx, ids)
	if err != nil {
		return err
	}
	photosBy, err := r.getPhotosByVenueIDs(ctx, ids)
	if err != nil {
		return err
	}
	hallsBy, err := r.getHallsWithPhotosByVenueIDs(ctx, ids)
	if err != nil {
		return err
	}
	for i := range venues {
		v := &venues[i]
		if s := svcBy[v.ID]; s != nil {
			v.Services = s
		} else {
			v.Services = []domain.VenueService{}
		}
		if p := photosBy[v.ID]; p != nil {
			v.Photos = p
		} else {
			v.Photos = []domain.VenuePhoto{}
		}
		if h := hallsBy[v.ID]; h != nil {
			v.Halls = h
		} else {
			v.Halls = []domain.VenueHall{}
		}
	}
	return nil
}

// getPhotosByVenueIDs одним запросом подгружает фото для списка заведений (каталог / поиск).
func (r *venueRepo) getPhotosByVenueIDs(ctx context.Context, venueIDs []uuid.UUID) (map[uuid.UUID][]domain.VenuePhoto, error) {
	if len(venueIDs) == 0 {
		return map[uuid.UUID][]domain.VenuePhoto{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, venue_id, url, sort_order, is_cover
		FROM venue_photos
		WHERE venue_id = ANY($1::uuid[])
		ORDER BY venue_id, sort_order ASC, id ASC`, venueIDs)
	if err != nil {
		return nil, fmt.Errorf("get venue_photos batch: %w", err)
	}
	defer rows.Close()

	out := make(map[uuid.UUID][]domain.VenuePhoto)
	for rows.Next() {
		var p domain.VenuePhoto
		if err := rows.Scan(&p.ID, &p.VenueID, &p.URL, &p.SortOrder, &p.IsCover); err != nil {
			return nil, fmt.Errorf("scan venue_photo: %w", err)
		}
		out[p.VenueID] = append(out[p.VenueID], p)
	}
	return out, rows.Err()
}

// attachPhotosToVenues заполняет Photos для публичных списков (каталог / поиск).
func (r *venueRepo) attachPhotosToVenues(ctx context.Context, venues []domain.Venue) error {
	if len(venues) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(venues))
	for i := range venues {
		ids[i] = venues[i].ID
	}
	byVenue, err := r.getPhotosByVenueIDs(ctx, ids)
	if err != nil {
		return err
	}
	for i := range venues {
		venues[i].Photos = byVenue[venues[i].ID]
	}
	return nil
}

func (r *venueRepo) List(ctx context.Context, page, pageSize int32, venueType, sortBy string) (*domain.ListResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	conditions := []string{"status = 'active'"}
	args := []any{}
	argIdx := 1

	if venueType != "" {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, venueType)
		argIdx++
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	orderBy := "ORDER BY created_at DESC"
	switch sortBy {
	case "rating":
		orderBy = "ORDER BY avg_rating DESC"
	case "price":
		orderBy = "ORDER BY price_from ASC"
	}

	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, owner_id, slug, name, type, description, address, city,
			ST_Y(location::geometry) AS latitude, ST_X(location::geometry) AS longitude,
			price_from, capacity, amenities, working_hours, phone,
			avg_rating, review_count, is_active, status, moderation_comment,
			moderated_at, moderated_by,
			legal_entity_name, inn, ogrn, public_listing_url, verification_note,
			social_links::text,
			COALESCE(payout_legal_form, ''),
			created_at, updated_at,
			(COUNT(*) OVER())::bigint AS __total
		FROM venues %s %s LIMIT $%d OFFSET $%d`, where, orderBy, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list venues: %w", err)
	}
	defer rows.Close()

	venues, total, err := r.scanVenuesWithTotal(rows)
	if err != nil {
		return nil, err
	}
	if err := r.attachPhotosToVenues(ctx, venues); err != nil {
		return nil, err
	}

	return &domain.ListResult{Venues: venues, Total: total, Page: page, PageSize: pageSize}, nil
}

func (r *venueRepo) Search(ctx context.Context, params domain.SearchParams) (*domain.ListResult, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 || params.PageSize > 100 {
		params.PageSize = 20
	}

	conditions := []string{"status = 'active'"}
	args := []any{}
	argIdx := 1

	if pat, ok := escapedILikePattern(params.Query); ok {
		conditions = append(conditions, fmt.Sprintf(
			"(to_tsvector('russian', name || ' ' || COALESCE(description, '') || ' ' || address || ' ' || COALESCE(city, '')) @@ plainto_tsquery('russian', $%d) OR name ILIKE $%d ESCAPE '\\' OR address ILIKE $%d ESCAPE '\\' OR city ILIKE $%d ESCAPE '\\')",
			argIdx, argIdx+1, argIdx+2, argIdx+3))
		args = append(args, params.Query, pat, pat, pat)
		argIdx += 4
	}

	if eff := params.EffectiveCities(); len(eff) > 0 {
		var ors []string
		for _, cc := range eff {
			if pat, ok := escapedILikePattern(cc); ok {
				ors = append(ors, fmt.Sprintf("city ILIKE $%d ESCAPE '\\'", argIdx))
				args = append(args, pat)
				argIdx++
			}
		}
		if len(ors) == 1 {
			conditions = append(conditions, ors[0])
		} else {
			conditions = append(conditions, "("+strings.Join(ors, " OR ")+")")
		}
	}

	if params.Lat != 0 && params.Lng != 0 && params.RadiusKM > 0 {
		conditions = append(conditions, fmt.Sprintf(
			"ST_DWithin(location, ST_SetSRID(ST_MakePoint($%d, $%d), 4326)::geography, $%d)", argIdx, argIdx+1, argIdx+2))
		args = append(args, params.Lng, params.Lat, params.RadiusKM*1000)
		argIdx += 3
	}

	if params.VenueType != "" {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, params.VenueType)
		argIdx++
	}

	if params.PriceMin > 0 {
		conditions = append(conditions, fmt.Sprintf("price_from >= $%d", argIdx))
		args = append(args, params.PriceMin)
		argIdx++
	}
	if params.PriceMax > 0 {
		conditions = append(conditions, fmt.Sprintf("price_from <= $%d", argIdx))
		args = append(args, params.PriceMax)
		argIdx++
	}

	if params.RatingMin > 0 {
		conditions = append(conditions, fmt.Sprintf("avg_rating >= $%d", argIdx))
		args = append(args, params.RatingMin)
		argIdx++
	}

	if len(params.Amenities) > 0 {
		conditions = append(conditions, fmt.Sprintf("amenities @> $%d", argIdx))
		args = append(args, params.Amenities)
		argIdx++
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	offset := (params.Page - 1) * params.PageSize
	args = append(args, params.PageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, owner_id, slug, name, type, description, address, city,
			ST_Y(location::geometry) AS latitude, ST_X(location::geometry) AS longitude,
			price_from, capacity, amenities, working_hours, phone,
			avg_rating, review_count, is_active, status, moderation_comment,
			moderated_at, moderated_by,
			legal_entity_name, inn, ogrn, public_listing_url, verification_note,
			social_links::text,
			COALESCE(payout_legal_form, ''),
			created_at, updated_at,
			(COUNT(*) OVER())::bigint AS __total
		FROM venues %s ORDER BY avg_rating DESC LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search venues: %w", err)
	}
	defer rows.Close()

	venues, total, err := r.scanVenuesWithTotal(rows)
	if err != nil {
		return nil, err
	}
	if err := r.attachPhotosToVenues(ctx, venues); err != nil {
		return nil, err
	}

	return &domain.ListResult{Venues: venues, Total: total, Page: params.Page, PageSize: params.PageSize}, nil
}

func (r *venueRepo) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]domain.Venue, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, owner_id, slug, name, type, description, address, city,
			ST_Y(location::geometry) AS latitude, ST_X(location::geometry) AS longitude,
			price_from, capacity, amenities, working_hours, phone,
			avg_rating, review_count, is_active, status, moderation_comment,
			moderated_at, moderated_by,
			legal_entity_name, inn, ogrn, public_listing_url, verification_note,
			social_links::text,
			COALESCE(payout_legal_form, ''),
			created_at, updated_at
		FROM venues WHERE owner_id = $1 ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list by owner: %w", err)
	}
	defer rows.Close()
	venues, err := r.scanVenues(rows)
	if err != nil {
		return nil, err
	}
	if err := r.attachVenueDetailsBatch(ctx, venues); err != nil {
		return nil, err
	}
	return venues, nil
}

// IsVenueMember returns true when userID is the venue owner or a row in
// venue_staff. Strangler-fig phase B: we still read venue_staff directly here
// because the table physically lives in the same Postgres database as venues.
// When crm-service gets its own database, replace this with a gRPC call to
// crm.GetManagementAccess.
func (r *venueRepo) IsVenueMember(ctx context.Context, venueID, userID uuid.UUID) (bool, error) {
	var found int
	err := r.pool.QueryRow(ctx, `
		SELECT 1
		FROM venues v
		LEFT JOIN venue_staff vs ON vs.venue_id = v.id AND vs.user_id = $2
		WHERE v.id = $1 AND (v.owner_id = $2 OR vs.user_id IS NOT NULL)
		LIMIT 1`,
		venueID, userID,
	).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("is venue member: %w", err)
	}
	return true, nil
}

// StaffUserIDsForVenue returns staff user_ids (excluding the owner) so the
// notifier can fan out moderation emails. Same phase-B caveat as
// IsVenueMember — read directly from the shared venue_staff table.
func (r *venueRepo) StaffUserIDsForVenue(ctx context.Context, venueID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id FROM venue_staff WHERE venue_id = $1`, venueID)
	if err != nil {
		return nil, fmt.Errorf("staff user ids: %w", err)
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var u uuid.UUID
		if err := rows.Scan(&u); err != nil {
			return nil, fmt.Errorf("scan staff user id: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// escapedILikePattern builds a LIKE pattern with ILIKE ESCAPE '\' (wildcards in user input neutralized).
func escapedILikePattern(q string) (pattern string, ok bool) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", false
	}
	q = strings.ReplaceAll(q, `\`, `\\`)
	q = strings.ReplaceAll(q, `%`, `\%`)
	q = strings.ReplaceAll(q, `_`, `\_`)
	return "%" + q + "%", true
}

func (r *venueRepo) ListByStatus(ctx context.Context, status string, page, pageSize int32, nameQuery string) (*domain.ListResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	if status == "" {
		status = domain.StatusPendingReview
	}

	pat, filterName := escapedILikePattern(nameQuery)

	const venueAdminSelect = `
		SELECT id, owner_id, slug, name, type, description, address, city,
			ST_Y(location::geometry) AS latitude, ST_X(location::geometry) AS longitude,
			price_from, capacity, amenities, working_hours, phone,
			avg_rating, review_count, is_active, status, moderation_comment,
			moderated_at, moderated_by,
			legal_entity_name, inn, ogrn, public_listing_url, verification_note,
			social_links::text,
			COALESCE(payout_legal_form, ''),
			created_at, updated_at
		FROM venues`

	var total int32
	var rows pgx.Rows
	var err error
	offset := (page - 1) * pageSize

	if filterName {
		err = r.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM venues WHERE status = $1 AND name ILIKE $2 ESCAPE '\'`,
			status, pat,
		).Scan(&total)
		if err != nil {
			return nil, fmt.Errorf("count by status: %w", err)
		}
		rows, err = r.pool.Query(ctx, venueAdminSelect+`
		WHERE status = $1 AND name ILIKE $2 ESCAPE '\'
		ORDER BY created_at ASC LIMIT $3 OFFSET $4`, status, pat, pageSize, offset)
	} else {
		err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM venues WHERE status = $1`, status).Scan(&total)
		if err != nil {
			return nil, fmt.Errorf("count by status: %w", err)
		}
		rows, err = r.pool.Query(ctx, venueAdminSelect+`
		WHERE status = $1
		ORDER BY created_at ASC LIMIT $2 OFFSET $3`, status, pageSize, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list by status: %w", err)
	}
	defer rows.Close()

	venues, err := r.scanVenues(rows)
	if err != nil {
		return nil, err
	}
	for i := range venues {
		if err := r.attachVenueDetails(ctx, &venues[i]); err != nil {
			return nil, err
		}
	}
	return &domain.ListResult{Venues: venues, Total: total, Page: page, PageSize: pageSize}, nil
}

func (r *venueRepo) UpdateStatus(ctx context.Context, venueID uuid.UUID, status, comment string, moderatedBy uuid.UUID) error {
	isActive := status == domain.StatusActive
	_, err := r.pool.Exec(ctx, `
		UPDATE venues
		SET status = $2, moderation_comment = $3, is_active = $4,
		    moderated_at = now(), moderated_by = $5, updated_at = now()
		WHERE id = $1`, venueID, status, comment, isActive, moderatedBy)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

func (r *venueRepo) SuspendByOwner(ctx context.Context, ownerID uuid.UUID) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE venues
		SET status = $2, is_active = false, updated_at = now()
		WHERE owner_id = $1 AND status <> $2`,
		ownerID, domain.StatusSuspended)
	if err != nil {
		return 0, fmt.Errorf("suspend venues by owner: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *venueRepo) ResetToPendingReview(ctx context.Context, venueID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE venues
		SET status = $2, moderation_comment = '', is_active = false,
		    moderated_at = NULL, moderated_by = NULL,
		    updated_at = now()
		WHERE id = $1 AND status IN ($3, $4)`,
		venueID, domain.StatusPendingReview, domain.StatusActive, domain.StatusRejected)
	if err != nil {
		return fmt.Errorf("reset to pending_review: %w", err)
	}
	return nil
}

func (r *venueRepo) SubmitDraftForReview(ctx context.Context, venueID, ownerID uuid.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE venues
		SET status = $3, is_active = false, moderation_comment = '',
		    moderated_at = NULL, moderated_by = NULL, updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND status = $4`,
		venueID, ownerID, domain.StatusPendingReview, domain.StatusDraft)
	if err != nil {
		return false, fmt.Errorf("submit draft for review: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *venueRepo) AddVenuePhoto(ctx context.Context, venueID, ownerID uuid.UUID, url string) (*domain.VenuePhoto, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockVenueForOwner(ctx, tx, venueID, ownerID); err != nil {
		return nil, err
	}

	var n int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM venue_photos WHERE venue_id = $1`, venueID).Scan(&n); err != nil {
		return nil, fmt.Errorf("count photos: %w", err)
	}
	if n >= domain.MaxVenuePhotos {
		return nil, ErrVenuePhotosLimit
	}

	var sortOrder int32
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(sort_order), -1) + 1 FROM venue_photos WHERE venue_id = $1`, venueID,
	).Scan(&sortOrder); err != nil {
		return nil, fmt.Errorf("next sort_order: %w", err)
	}

	isCover := n == 0
	var p domain.VenuePhoto
	err = tx.QueryRow(ctx, `
		INSERT INTO venue_photos (venue_id, url, sort_order, is_cover)
		VALUES ($1, $2, $3, $4)
		RETURNING id, venue_id, url, sort_order, is_cover`,
		venueID, url, sortOrder, isCover,
	).Scan(&p.ID, &p.VenueID, &p.URL, &p.SortOrder, &p.IsCover)
	if err != nil {
		return nil, fmt.Errorf("insert venue_photo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &p, nil
}

func (r *venueRepo) DeleteVenuePhoto(ctx context.Context, venueID, ownerID, photoID uuid.UUID) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockVenueForOwner(ctx, tx, venueID, ownerID); err != nil {
		return "", err
	}

	var deletedURL string
	err = tx.QueryRow(ctx, `
		DELETE FROM venue_photos WHERE id = $1 AND venue_id = $2 RETURNING url`,
		photoID, venueID,
	).Scan(&deletedURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrPhotoNotFound
		}
		return "", fmt.Errorf("delete venue_photo: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE venue_photos SET is_cover = false WHERE venue_id = $1`, venueID); err != nil {
		return "", fmt.Errorf("clear covers: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE venue_photos SET is_cover = true
		WHERE id = (SELECT id FROM venue_photos WHERE venue_id = $1 ORDER BY sort_order ASC LIMIT 1)`,
		venueID,
	); err != nil {
		return "", fmt.Errorf("set cover: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return deletedURL, nil
}

func (r *venueRepo) SetVenueCoverPhoto(ctx context.Context, venueID, ownerID, photoID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockVenueForOwner(ctx, tx, venueID, ownerID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `UPDATE venue_photos SET is_cover = false WHERE venue_id = $1`, venueID); err != nil {
		return fmt.Errorf("clear covers: %w", err)
	}
	ct, err := tx.Exec(ctx, `
		UPDATE venue_photos SET is_cover = true WHERE id = $1 AND venue_id = $2`,
		photoID, venueID,
	)
	if err != nil {
		return fmt.Errorf("set cover: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrPhotoNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func derivedVenueFieldsFromHalls(halls []domain.VenueHall) (priceFrom int64, capacity int32, amenities []string) {
	seen := make(map[string]struct{})
	for _, h := range halls {
		if h.PriceFrom > 0 && (priceFrom == 0 || h.PriceFrom < priceFrom) {
			priceFrom = h.PriceFrom
		}
		if h.Capacity > capacity {
			capacity = h.Capacity
		}
		for _, a := range h.Amenities {
			t := strings.TrimSpace(a)
			if t == "" {
				continue
			}
			k := strings.ToLower(t)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			amenities = append(amenities, t)
		}
	}
	sort.Strings(amenities)
	return priceFrom, capacity, amenities
}

func (r *venueRepo) getHallsWithPhotos(ctx context.Context, venueID uuid.UUID) ([]domain.VenueHall, error) {
	m, err := r.getHallsWithPhotosByVenueIDs(ctx, []uuid.UUID{venueID})
	if err != nil {
		return nil, err
	}
	return m[venueID], nil
}

func (r *venueRepo) ReplaceVenueHalls(ctx context.Context, venueID, ownerID uuid.UUID, items []domain.VenueHallUpsert) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockVenueForOwner(ctx, tx, venueID, ownerID); err != nil {
		return err
	}

	var kept []uuid.UUID
	for i, it := range items {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			return ErrHallNameRequired
		}
		amenities := it.Amenities
		if amenities == nil {
			amenities = []string{}
		}
		if it.ID != nil {
			var vid uuid.UUID
			err := tx.QueryRow(ctx, `SELECT venue_id FROM venue_halls WHERE id = $1`, *it.ID).Scan(&vid)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrHallNotFound
				}
				return fmt.Errorf("hall lookup: %w", err)
			}
			if vid != venueID {
				return ErrHallNotInVenue
			}
			if _, err := tx.Exec(ctx, `
				UPDATE venue_halls SET name = $1, price_from = $2, capacity = $3, amenities = $4, sort_order = $5, updated_at = now()
				WHERE id = $6 AND venue_id = $7`,
				name, it.PriceFrom, it.Capacity, amenities, int32(i), *it.ID, venueID,
			); err != nil {
				return fmt.Errorf("update venue_hall: %w", err)
			}
			kept = append(kept, *it.ID)
		} else {
			var newID uuid.UUID
			err := tx.QueryRow(ctx, `
				INSERT INTO venue_halls (venue_id, name, price_from, capacity, amenities, sort_order)
				VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
				venueID, name, it.PriceFrom, it.Capacity, amenities, int32(i),
			).Scan(&newID)
			if err != nil {
				return fmt.Errorf("insert venue_hall: %w", err)
			}
			kept = append(kept, newID)
		}
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM venue_halls WHERE venue_id = $1 AND NOT (id = ANY($2::uuid[]))`,
		venueID, kept,
	); err != nil {
		return fmt.Errorf("delete removed halls: %w", err)
	}

	var hallRows []domain.VenueHall
	hrows, err := tx.Query(ctx, `
		SELECT id, venue_id, name, price_from, capacity, amenities, sort_order
		FROM venue_halls WHERE venue_id = $1`, venueID)
	if err != nil {
		return fmt.Errorf("list halls for derived: %w", err)
	}
	defer hrows.Close()
	for hrows.Next() {
		var h domain.VenueHall
		if err := hrows.Scan(&h.ID, &h.VenueID, &h.Name, &h.PriceFrom, &h.Capacity, &h.Amenities, &h.SortOrder); err != nil {
			return fmt.Errorf("scan hall row: %w", err)
		}
		hallRows = append(hallRows, h)
	}
	if err := hrows.Err(); err != nil {
		return err
	}
	pf, cap, am := derivedVenueFieldsFromHalls(hallRows)
	if _, err := tx.Exec(ctx, `
		UPDATE venues SET price_from = $2, capacity = $3, amenities = $4, updated_at = now() WHERE id = $1`,
		venueID, pf, cap, am,
	); err != nil {
		return fmt.Errorf("update venue derived from halls: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

const maxVenueHallPhotos = 16

func (r *venueRepo) AddVenueHallPhoto(ctx context.Context, venueID, hallID uuid.UUID, url string) (*domain.VenueHallPhoto, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var vid uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT venue_id FROM venue_halls WHERE id = $1 FOR UPDATE`, hallID).Scan(&vid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrHallNotFound
		}
		return nil, fmt.Errorf("lock hall: %w", err)
	}
	if vid != venueID {
		return nil, ErrHallNotInVenue
	}

	var n int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM venue_hall_photos WHERE hall_id = $1`, hallID).Scan(&n); err != nil {
		return nil, fmt.Errorf("count hall photos: %w", err)
	}
	if n >= maxVenueHallPhotos {
		return nil, ErrHallPhotosLimit
	}

	var sortOrder int32
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(sort_order), -1) + 1 FROM venue_hall_photos WHERE hall_id = $1`, hallID,
	).Scan(&sortOrder); err != nil {
		return nil, fmt.Errorf("next sort_order: %w", err)
	}

	isCover := n == 0
	var p domain.VenueHallPhoto
	err = tx.QueryRow(ctx, `
		INSERT INTO venue_hall_photos (hall_id, url, sort_order, is_cover)
		VALUES ($1, $2, $3, $4)
		RETURNING id, hall_id, url, sort_order, is_cover`,
		hallID, url, sortOrder, isCover,
	).Scan(&p.ID, &p.HallID, &p.URL, &p.SortOrder, &p.IsCover)
	if err != nil {
		return nil, fmt.Errorf("insert venue_hall_photo: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &p, nil
}

func (r *venueRepo) DeleteVenueHallPhoto(ctx context.Context, venueID, hallID, photoID uuid.UUID) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var vid uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT venue_id FROM venue_halls WHERE id = $1`, hallID).Scan(&vid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrHallNotFound
		}
		return "", fmt.Errorf("hall lookup: %w", err)
	}
	if vid != venueID {
		return "", ErrHallNotInVenue
	}

	var deletedURL string
	err = tx.QueryRow(ctx, `
		DELETE FROM venue_hall_photos WHERE id = $1 AND hall_id = $2 RETURNING url`,
		photoID, hallID,
	).Scan(&deletedURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrPhotoNotFound
		}
		return "", fmt.Errorf("delete venue_hall_photo: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE venue_hall_photos SET is_cover = false WHERE hall_id = $1`, hallID); err != nil {
		return "", fmt.Errorf("clear hall covers: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE venue_hall_photos SET is_cover = true
		WHERE id = (SELECT id FROM venue_hall_photos WHERE hall_id = $1 ORDER BY sort_order ASC LIMIT 1)`,
		hallID,
	); err != nil {
		return "", fmt.Errorf("set hall cover: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return deletedURL, nil
}

func (r *venueRepo) SetVenueHallCoverPhoto(ctx context.Context, venueID, hallID, photoID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var vid uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT venue_id FROM venue_halls WHERE id = $1`, hallID).Scan(&vid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrHallNotFound
		}
		return fmt.Errorf("hall lookup: %w", err)
	}
	if vid != venueID {
		return ErrHallNotInVenue
	}

	if _, err := tx.Exec(ctx, `UPDATE venue_hall_photos SET is_cover = false WHERE hall_id = $1`, hallID); err != nil {
		return fmt.Errorf("clear hall covers: %w", err)
	}
	ct, err := tx.Exec(ctx, `
		UPDATE venue_hall_photos SET is_cover = true WHERE id = $1 AND hall_id = $2`,
		photoID, hallID,
	)
	if err != nil {
		return fmt.Errorf("set hall cover: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrPhotoNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (r *venueRepo) InsertModerationHistory(ctx context.Context, entry *domain.ModerationHistoryEntry) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO venue_moderation_history (venue_id, old_status, new_status, comment, changed_by)
		VALUES ($1, $2, $3, $4, $5)`,
		entry.VenueID, entry.OldStatus, entry.NewStatus, entry.Comment, entry.ChangedBy)
	if err != nil {
		return fmt.Errorf("insert moderation history: %w", err)
	}
	return nil
}

func (r *venueRepo) GetModerationHistory(ctx context.Context, venueID uuid.UUID) ([]domain.ModerationHistoryEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, venue_id, old_status, new_status, comment, changed_by, created_at
		FROM venue_moderation_history
		WHERE venue_id = $1
		ORDER BY created_at DESC`, venueID)
	if err != nil {
		return nil, fmt.Errorf("get moderation history: %w", err)
	}
	defer rows.Close()

	var entries []domain.ModerationHistoryEntry
	for rows.Next() {
		var e domain.ModerationHistoryEntry
		if err := rows.Scan(&e.ID, &e.VenueID, &e.OldStatus, &e.NewStatus, &e.Comment, &e.ChangedBy, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan moderation history: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *venueRepo) UpdateRating(ctx context.Context, venueID uuid.UUID, avgRating float64, reviewCount int32) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE venues SET avg_rating = $2, review_count = $3, updated_at = now()
		WHERE id = $1`, venueID, avgRating, reviewCount)
	if err != nil {
		return fmt.Errorf("update rating: %w", err)
	}
	return nil
}

// reservationKeys returns the resource keys a candidate reservation occupies,
// mirroring the reserved_slots exclusion constraint: the given halls in per-hall
// mode, or the venue itself (matching hall_id NULL rows via COALESCE) when no
// halls apply.
func reservationKeys(venueID uuid.UUID, hallIDs []uuid.UUID) []uuid.UUID {
	if len(hallIDs) == 0 {
		return []uuid.UUID{venueID}
	}
	return hallIDs
}

func (r *venueRepo) CheckSlot(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, date, timeFrom, timeTo string) (bool, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM reserved_slots
		WHERE venue_id = $1 AND date = $2
			AND COALESCE(hall_id, venue_id) = ANY($3::uuid[])
			AND time_from < $5::time AND time_to > $4::time`,
		venueID, date, reservationKeys(venueID, hallIDs), timeFrom, timeTo,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check slot: %w", err)
	}
	return count == 0, nil
}

// BatchCheckSlots checks multiple (time_from, time_to) pairs for the same
// venue and date in a single round-trip. It uses unnest to expand the input
// arrays server-side and LEFT JOIN to find conflicting rows in reserved_slots.
//
// Note: manual_slot_blocks are stored in reserved_slots with booking_id = NULL.
// The JOIN condition does not filter on booking_id, so manual blocks are
// included in the conflict check alongside booking-driven reservations.
//
// SQL sketch:
//
//	WITH slots(ord, tf, tt) AS (
//	    SELECT * FROM unnest($3::int[], $4::time[], $5::time[])
//	)
//	SELECT s.ord, COUNT(rs.*) = 0
//	FROM slots s
//	LEFT JOIN reserved_slots rs
//	  ON rs.venue_id = $1 AND rs.date = $2
//	 AND rs.time_from < s.tt AND rs.time_to > s.tf
//	GROUP BY s.ord
//	ORDER BY s.ord;
func (r *venueRepo) BatchCheckSlots(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, date string, slots [][2]string) ([]bool, error) {
	if len(slots) == 0 {
		return nil, nil
	}

	// Build parallel arrays for unnest.
	ords := make([]int, len(slots))
	froms := make([]string, len(slots))
	tos := make([]string, len(slots))
	for i, s := range slots {
		ords[i] = i
		froms[i] = s[0]
		tos[i] = s[1]
	}

	rows, err := r.pool.Query(ctx, `
		WITH slots(ord, tf, tt) AS (
		    SELECT * FROM unnest($3::int[], $4::time[], $5::time[])
		)
		SELECT s.ord, COUNT(rs.venue_id) = 0 AS available
		FROM slots s
		LEFT JOIN reserved_slots rs
		  ON rs.venue_id = $1
		 AND rs.date = $2
		 AND COALESCE(rs.hall_id, rs.venue_id) = ANY($6::uuid[])
		 AND rs.time_from < s.tt
		 AND rs.time_to   > s.tf
		GROUP BY s.ord
		ORDER BY s.ord`,
		venueID, date, ords, froms, tos, reservationKeys(venueID, hallIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("batch check slots: %w", err)
	}
	defer rows.Close()

	result := make([]bool, len(slots))
	for rows.Next() {
		var ord int
		var avail bool
		if err := rows.Scan(&ord, &avail); err != nil {
			return nil, fmt.Errorf("batch check slots scan: %w", err)
		}
		if ord >= 0 && ord < len(result) {
			result[ord] = avail
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("batch check slots rows: %w", err)
	}
	return result, nil
}

func (r *venueRepo) ReserveSlot(ctx context.Context, venueID, bookingID uuid.UUID, date, timeFrom, timeTo string, hallIDs []uuid.UUID) error {
	const perHallInsert = `
		INSERT INTO reserved_slots (venue_id, booking_id, hall_id, date, time_from, time_to)
		VALUES ($1, $2, $3, $4::date, $5::time, $6::time)`

	if len(hallIDs) == 0 {
		_, err := r.pool.Exec(ctx, `
			INSERT INTO reserved_slots (venue_id, booking_id, hall_id, date, time_from, time_to)
			VALUES ($1, $2, NULL, $3::date, $4::time, $5::time)`,
			venueID, bookingID, date, timeFrom, timeTo,
		)
		return reserveSlotErr(err)
	}

	// Per-hall: reserve every selected hall atomically. Any overlap on any hall
	// aborts the whole reservation, leaving no rows.
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("reserve slot begin: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, hallID := range hallIDs {
		if _, err := tx.Exec(ctx, perHallInsert, venueID, bookingID, hallID, date, timeFrom, timeTo); err != nil {
			return reserveSlotErr(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("reserve slot commit: %w", err)
	}
	return nil
}

// reserveSlotErr maps a Postgres exclusion violation (23P01) on reserved_slots
// to the domain ErrSlotUnavailable; other errors are wrapped.
func reserveSlotErr(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
		return ErrSlotUnavailable
	}
	return fmt.Errorf("reserve slot: %w", err)
}

// BookingMode returns the venue's booking mode ("whole" | "per_hall").
func (r *venueRepo) BookingMode(ctx context.Context, venueID uuid.UUID) (string, error) {
	var mode string
	err := r.pool.QueryRow(ctx, `SELECT booking_mode FROM venues WHERE id = $1`, venueID).Scan(&mode)
	if err != nil {
		return "", fmt.Errorf("booking mode: %w", err)
	}
	return mode, nil
}

// SetBookingMode updates the venue's booking mode.
func (r *venueRepo) SetBookingMode(ctx context.Context, venueID uuid.UUID, mode string) error {
	if _, err := r.pool.Exec(ctx, `UPDATE venues SET booking_mode = $2 WHERE id = $1`, venueID, mode); err != nil {
		return fmt.Errorf("set booking mode: %w", err)
	}
	return nil
}

func (r *venueRepo) ReleaseSlot(ctx context.Context, venueID, bookingID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM reserved_slots WHERE venue_id = $1 AND booking_id = $2`,
		venueID, bookingID,
	)
	if err != nil {
		return fmt.Errorf("release slot: %w", err)
	}
	return nil
}

// HallIDs returns the ids of all halls defined for the venue.
func (r *venueRepo) HallIDs(ctx context.Context, venueID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM venue_halls WHERE venue_id = $1`, venueID)
	if err != nil {
		return nil, fmt.Errorf("hall ids: %w", err)
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan hall id: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *venueRepo) CreateManualSlotBlock(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, date, timeFrom, timeTo, note string) ([]domain.ManualSlotBlock, error) {
	// Whole-venue block: a single hall_id NULL row (whole mode).
	if len(hallIDs) == 0 {
		b := domain.ManualSlotBlock{VenueID: venueID, Date: date, TimeFrom: timeFrom, TimeTo: timeTo}
		err := r.pool.QueryRow(ctx, `
			INSERT INTO reserved_slots (venue_id, booking_id, hall_id, date, time_from, time_to, block_note)
			VALUES ($1, NULL, NULL, $2::date, $3::time, $4::time, NULLIF(trim($5), ''))
			RETURNING id, COALESCE(block_note, '')`,
			venueID, date, timeFrom, timeTo, note,
		).Scan(&b.ID, &b.Note)
		if err != nil {
			return nil, manualBlockErr(err)
		}
		return []domain.ManualSlotBlock{b}, nil
	}

	// Per-hall block: one row per hall, atomically.
	const perHallInsert = `
		INSERT INTO reserved_slots (venue_id, booking_id, hall_id, date, time_from, time_to, block_note)
		VALUES ($1, NULL, $2, $3::date, $4::time, $5::time, NULLIF(trim($6), ''))
		RETURNING id, COALESCE(block_note, '')`
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("create manual block begin: %w", err)
	}
	defer tx.Rollback(ctx)
	out := make([]domain.ManualSlotBlock, 0, len(hallIDs))
	for _, hallID := range hallIDs {
		hid := hallID
		b := domain.ManualSlotBlock{VenueID: venueID, HallID: &hid, Date: date, TimeFrom: timeFrom, TimeTo: timeTo}
		if err := tx.QueryRow(ctx, perHallInsert, venueID, hallID, date, timeFrom, timeTo, note).Scan(&b.ID, &b.Note); err != nil {
			return nil, manualBlockErr(err)
		}
		out = append(out, b)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("create manual block commit: %w", err)
	}
	return out, nil
}

func manualBlockErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
		return ErrSlotUnavailable
	}
	return fmt.Errorf("create manual slot block: %w", err)
}

func (r *venueRepo) DeleteManualSlotBlock(ctx context.Context, venueID, blockID uuid.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM reserved_slots
		WHERE id = $1 AND venue_id = $2 AND booking_id IS NULL`,
		blockID, venueID,
	)
	if err != nil {
		return false, fmt.Errorf("delete manual slot block: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *venueRepo) ListManualSlotBlocks(ctx context.Context, venueID uuid.UUID, dateFrom, dateTo string) ([]domain.ManualSlotBlock, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, venue_id, hall_id, date::text,
			to_char(time_from, 'HH24:MI'),
			to_char(time_to, 'HH24:MI'),
			COALESCE(block_note, '')
		FROM reserved_slots
		WHERE venue_id = $1 AND booking_id IS NULL
			AND date >= $2::date AND date <= $3::date
		ORDER BY date, time_from`,
		venueID, dateFrom, dateTo,
	)
	if err != nil {
		return nil, fmt.Errorf("list manual slot blocks: %w", err)
	}
	defer rows.Close()

	var out []domain.ManualSlotBlock
	for rows.Next() {
		var (
			b    domain.ManualSlotBlock
			hall uuid.NullUUID
		)
		if err := rows.Scan(&b.ID, &b.VenueID, &hall, &b.Date, &b.TimeFrom, &b.TimeTo, &b.Note); err != nil {
			return nil, fmt.Errorf("scan manual slot block: %w", err)
		}
		if hall.Valid {
			id := hall.UUID
			b.HallID = &id
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *venueRepo) ListVenueSchedule(ctx context.Context, venueID uuid.UUID, dateFrom, dateTo string) ([]domain.ScheduleEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, booking_id, hall_id, date::text,
			to_char(time_from, 'HH24:MI'),
			to_char(time_to, 'HH24:MI'),
			COALESCE(block_note, '')
		FROM reserved_slots
		WHERE venue_id = $1 AND date >= $2::date AND date <= $3::date
		ORDER BY date, time_from`,
		venueID, dateFrom, dateTo,
	)
	if err != nil {
		return nil, fmt.Errorf("list venue schedule: %w", err)
	}
	defer rows.Close()

	var out []domain.ScheduleEntry
	for rows.Next() {
		var (
			e       domain.ScheduleEntry
			booking uuid.NullUUID
			hall    uuid.NullUUID
		)
		if err := rows.Scan(&e.ID, &booking, &hall, &e.Date, &e.TimeFrom, &e.TimeTo, &e.Note); err != nil {
			return nil, fmt.Errorf("scan schedule entry: %w", err)
		}
		if booking.Valid {
			id := booking.UUID
			e.BookingID = &id
		}
		if hall.Valid {
			id := hall.UUID
			e.HallID = &id
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *venueRepo) scanVenuesWithTotal(rows pgx.Rows) ([]domain.Venue, int32, error) {
	var venues []domain.Venue
	var total int32
	first := true
	for rows.Next() {
		var v domain.Venue
		var totalRaw int64
		if err := rows.Scan(
			&v.ID, &v.OwnerID, &v.Slug, &v.Name, &v.Type, &v.Description, &v.Address, &v.City,
			&v.Latitude, &v.Longitude,
			&v.PriceFrom, &v.Capacity, &v.Amenities, &v.WorkingHours, &v.Phone,
			&v.AvgRating, &v.ReviewCount, &v.IsActive, &v.Status, &v.ModerationComment,
			&v.ModeratedAt, &v.ModeratedBy,
			&v.LegalEntityName, &v.INN, &v.OGRN, &v.PublicListingURL, &v.VerificationNote,
			&v.SocialLinks,
			&v.PayoutLegalForm,
			&v.CreatedAt, &v.UpdatedAt,
			&totalRaw,
		); err != nil {
			return nil, 0, fmt.Errorf("scan venue: %w", err)
		}
		if first {
			if totalRaw > math.MaxInt32 {
				total = math.MaxInt32
			} else {
				total = int32(totalRaw)
			}
			first = false
		}
		venues = append(venues, v)
	}
	if first {
		total = 0
	}
	return venues, total, rows.Err()
}

func (r *venueRepo) scanVenues(rows pgx.Rows) ([]domain.Venue, error) {
	var venues []domain.Venue
	for rows.Next() {
		var v domain.Venue
		if err := rows.Scan(
			&v.ID, &v.OwnerID, &v.Slug, &v.Name, &v.Type, &v.Description, &v.Address, &v.City,
			&v.Latitude, &v.Longitude,
			&v.PriceFrom, &v.Capacity, &v.Amenities, &v.WorkingHours, &v.Phone,
			&v.AvgRating, &v.ReviewCount, &v.IsActive, &v.Status, &v.ModerationComment,
			&v.ModeratedAt, &v.ModeratedBy,
			&v.LegalEntityName, &v.INN, &v.OGRN, &v.PublicListingURL, &v.VerificationNote,
			&v.SocialLinks,
			&v.PayoutLegalForm,
			&v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan venue: %w", err)
		}
		venues = append(venues, v)
	}
	return venues, rows.Err()
}
