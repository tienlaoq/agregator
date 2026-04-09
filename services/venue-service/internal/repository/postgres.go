package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

type venueRepo struct {
	pool *pgxpool.Pool
}

func NewVenueRepo(pool *pgxpool.Pool) domain.VenueRepository {
	return &venueRepo{pool: pool}
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

	err = tx.QueryRow(ctx, `
		INSERT INTO venues (owner_id, slug, name, type, description, address, city, location, price_from, capacity, amenities, working_hours, phone, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, ST_SetSRID(ST_MakePoint($8, $9), 4326)::geography, $10, $11, $12, $13, $14, $15)
		RETURNING id, created_at, updated_at`,
		venue.OwnerID, venue.Slug, venue.Name, venue.Type, venue.Description,
		venue.Address, venue.City, venue.Longitude, venue.Latitude,
		venue.PriceFrom, venue.Capacity, venue.Amenities, venue.WorkingHours, venue.Phone,
		venue.Status,
	).Scan(&venue.ID, &venue.CreatedAt, &venue.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert venue: %w", err)
	}

	for i := range venue.Services {
		svc := &venue.Services[i]
		err = tx.QueryRow(ctx, `
			INSERT INTO venue_services (venue_id, name, duration_min, price, description)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id`,
			venue.ID, svc.Name, svc.DurationMin, svc.Price, svc.Description,
		).Scan(&svc.ID)
		if err != nil {
			return fmt.Errorf("insert venue_service: %w", err)
		}
		svc.VenueID = venue.ID
	}

	return tx.Commit(ctx)
}

func (r *venueRepo) Update(ctx context.Context, venue *domain.Venue) error {
	if venue.WorkingHours == "" {
		venue.WorkingHours = "{}"
	}

	_, err := r.pool.Exec(ctx, `
		UPDATE venues SET
			name = $2, description = $3, address = $4, city = $5,
			location = ST_SetSRID(ST_MakePoint($6, $7), 4326)::geography,
			price_from = $8, capacity = $9, amenities = $10,
			working_hours = $11, phone = $12, updated_at = now()
		WHERE id = $1`,
		venue.ID, venue.Name, venue.Description, venue.Address, venue.City,
		venue.Longitude, venue.Latitude,
		venue.PriceFrom, venue.Capacity, venue.Amenities,
		venue.WorkingHours, venue.Phone,
	)
	if err != nil {
		return fmt.Errorf("update venue: %w", err)
	}
	return nil
}

func (r *venueRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Venue, error) {
	return r.getVenue(ctx, "WHERE v.id = $1", id)
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
			v.created_at, v.updated_at
		FROM venues v `+where, arg,
	).Scan(
		&v.ID, &v.OwnerID, &v.Slug, &v.Name, &v.Type, &v.Description, &v.Address, &v.City,
		&v.Latitude, &v.Longitude,
		&v.PriceFrom, &v.Capacity, &v.Amenities, &v.WorkingHours, &v.Phone,
		&v.AvgRating, &v.ReviewCount, &v.IsActive, &v.Status, &v.ModerationComment,
		&v.ModeratedAt, &v.ModeratedBy,
		&v.CreatedAt, &v.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
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

	return v, nil
}

func (r *venueRepo) getServices(ctx context.Context, venueID uuid.UUID) ([]domain.VenueService, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, venue_id, name, duration_min, price, description
		FROM venue_services WHERE venue_id = $1`, venueID)
	if err != nil {
		return nil, fmt.Errorf("get venue_services: %w", err)
	}
	defer rows.Close()

	var services []domain.VenueService
	for rows.Next() {
		var s domain.VenueService
		if err := rows.Scan(&s.ID, &s.VenueID, &s.Name, &s.DurationMin, &s.Price, &s.Description); err != nil {
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

	var total int32
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM venues "+where, args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("count venues: %w", err)
	}

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
			created_at, updated_at
		FROM venues %s %s LIMIT $%d OFFSET $%d`, where, orderBy, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list venues: %w", err)
	}
	defer rows.Close()

	venues, err := r.scanVenues(rows)
	if err != nil {
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

	if params.Query != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(to_tsvector('russian', name || ' ' || COALESCE(description, '') || ' ' || address || ' ' || COALESCE(city, '')) @@ plainto_tsquery('russian', $%d) OR name ILIKE '%%' || $%d || '%%' OR address ILIKE '%%' || $%d || '%%' OR city ILIKE '%%' || $%d || '%%')",
			argIdx, argIdx+1, argIdx+2, argIdx+3))
		args = append(args, params.Query, params.Query, params.Query, params.Query)
		argIdx += 4
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

	var total int32
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM venues "+where, args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("count search: %w", err)
	}

	offset := (params.Page - 1) * params.PageSize
	args = append(args, params.PageSize, offset)
	query := fmt.Sprintf(`
		SELECT id, owner_id, slug, name, type, description, address, city,
			ST_Y(location::geometry) AS latitude, ST_X(location::geometry) AS longitude,
			price_from, capacity, amenities, working_hours, phone,
			avg_rating, review_count, is_active, status, moderation_comment,
			moderated_at, moderated_by,
			created_at, updated_at
		FROM venues %s ORDER BY avg_rating DESC LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search venues: %w", err)
	}
	defer rows.Close()

	venues, err := r.scanVenues(rows)
	if err != nil {
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
			created_at, updated_at
		FROM venues WHERE owner_id = $1 ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list by owner: %w", err)
	}
	defer rows.Close()
	return r.scanVenues(rows)
}

func (r *venueRepo) ListByStatus(ctx context.Context, status string, page, pageSize int32) (*domain.ListResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	if status == "" {
		status = domain.StatusPendingReview
	}

	var total int32
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM venues WHERE status = $1", status).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("count by status: %w", err)
	}

	offset := (page - 1) * pageSize
	rows, err := r.pool.Query(ctx, `
		SELECT id, owner_id, slug, name, type, description, address, city,
			ST_Y(location::geometry) AS latitude, ST_X(location::geometry) AS longitude,
			price_from, capacity, amenities, working_hours, phone,
			avg_rating, review_count, is_active, status, moderation_comment,
			moderated_at, moderated_by,
			created_at, updated_at
		FROM venues WHERE status = $1 ORDER BY created_at ASC LIMIT $2 OFFSET $3`, status, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("list by status: %w", err)
	}
	defer rows.Close()

	venues, err := r.scanVenues(rows)
	if err != nil {
		return nil, err
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

func (r *venueRepo) ResetToPendingReview(ctx context.Context, venueID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE venues
		SET status = $2, moderation_comment = '', updated_at = now()
		WHERE id = $1 AND status = $3`,
		venueID, domain.StatusPendingReview, domain.StatusRejected)
	if err != nil {
		return fmt.Errorf("reset to pending_review: %w", err)
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

func (r *venueRepo) CheckSlot(ctx context.Context, venueID uuid.UUID, date, timeFrom, timeTo string) (bool, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM reserved_slots
		WHERE venue_id = $1 AND date = $2
			AND time_from < $4::time AND time_to > $3::time`,
		venueID, date, timeFrom, timeTo,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check slot: %w", err)
	}
	return count == 0, nil
}

func (r *venueRepo) ReserveSlot(ctx context.Context, venueID, bookingID uuid.UUID, date, timeFrom, timeTo string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO reserved_slots (venue_id, booking_id, date, time_from, time_to)
		VALUES ($1, $2, $3, $4, $5)`,
		venueID, bookingID, date, timeFrom, timeTo,
	)
	if err != nil {
		return fmt.Errorf("reserve slot: %w", err)
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
			&v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan venue: %w", err)
		}
		venues = append(venues, v)
	}
	return venues, rows.Err()
}
