package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

type MasterRepo struct {
	pool *pgxpool.Pool
}

func NewMasterRepo(pool *pgxpool.Pool) *MasterRepo {
	return &MasterRepo{pool: pool}
}

func (r *MasterRepo) Insert(ctx context.Context, m *domain.Master) error {
	const q = `
INSERT INTO masters (
  id, user_id, slug, display_name, bio, phone, city, work_format,
  travel_radius_km, experience_years, specializations, hourly_rate, availability_json,
  payout_legal_form,
  status, moderation_comment, moderated_by, moderated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
`
	_, err := r.pool.Exec(ctx, q,
		m.ID, m.UserID, m.Slug, m.DisplayName, m.Bio, m.Phone, m.City, m.WorkFormat,
		m.TravelRadiusKm, m.ExperienceYears, m.Specializations, m.HourlyRate, m.AvailabilityJSON,
		m.PayoutLegalForm,
		m.Status, m.ModerationComment, m.ModeratedBy, m.ModeratedAt,
	)
	return err
}

func (r *MasterRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.Master, error) {
	const q = `
SELECT id, user_id, slug, display_name, bio, phone, city, work_format,
  travel_radius_km, experience_years, specializations, hourly_rate, availability_json,
  payout_legal_form,
  status, moderation_comment, moderated_by, moderated_at, created_at, updated_at
FROM masters WHERE user_id = $1
`
	return r.scanMaster(ctx, r.pool.QueryRow(ctx, q, userID))
}

func (r *MasterRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Master, error) {
	const q = `
SELECT id, user_id, slug, display_name, bio, phone, city, work_format,
  travel_radius_km, experience_years, specializations, hourly_rate, availability_json,
  payout_legal_form,
  status, moderation_comment, moderated_by, moderated_at, created_at, updated_at
FROM masters WHERE id = $1
`
	return r.scanMaster(ctx, r.pool.QueryRow(ctx, q, id))
}

func (r *MasterRepo) GetBySlug(ctx context.Context, s string) (*domain.Master, error) {
	const q = `
SELECT id, user_id, slug, display_name, bio, phone, city, work_format,
  travel_radius_km, experience_years, specializations, hourly_rate, availability_json,
  payout_legal_form,
  status, moderation_comment, moderated_by, moderated_at, created_at, updated_at
FROM masters WHERE slug = $1
`
	return r.scanMaster(ctx, r.pool.QueryRow(ctx, q, s))
}

func (r *MasterRepo) scanMaster(ctx context.Context, row pgx.Row) (*domain.Master, error) {
	var m domain.Master
	var modBy *uuid.UUID
	var modAt sql.NullTime
	err := row.Scan(
		&m.ID, &m.UserID, &m.Slug, &m.DisplayName, &m.Bio, &m.Phone, &m.City, &m.WorkFormat,
		&m.TravelRadiusKm, &m.ExperienceYears, &m.Specializations, &m.HourlyRate, &m.AvailabilityJSON,
		&m.PayoutLegalForm,
		&m.Status, &m.ModerationComment, &modBy, &modAt, &m.CreatedAt, &m.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.ModeratedBy = modBy
	if modAt.Valid {
		t := modAt.Time
		m.ModeratedAt = &t
	}
	svcs, err := r.loadServices(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	m.Services = svcs
	ph, err := r.loadPhotos(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	m.Photos = ph
	return &m, nil
}

func (r *MasterRepo) loadServices(ctx context.Context, masterID uuid.UUID) ([]domain.MasterService, error) {
	rows, err := r.pool.Query(ctx, `
SELECT id, master_id, name, description, duration_min, price, sort_order
FROM master_services WHERE master_id = $1 ORDER BY sort_order, id`, masterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MasterService
	for rows.Next() {
		var s domain.MasterService
		if err := rows.Scan(&s.ID, &s.MasterID, &s.Name, &s.Description, &s.DurationMin, &s.Price, &s.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *MasterRepo) UpdateProfile(ctx context.Context, m *domain.Master) error {
	const q = `
UPDATE masters SET
  display_name = $2, bio = $3, phone = $4, city = $5, work_format = $6,
  travel_radius_km = $7, experience_years = $8, specializations = $9,
  hourly_rate = $10, availability_json = $11, payout_legal_form = $12, status = $13,
  moderation_comment = $14, moderated_by = $15, moderated_at = $16,
  updated_at = now()
WHERE id = $1
`
	ct, err := r.pool.Exec(ctx, q,
		m.ID, m.DisplayName, m.Bio, m.Phone, m.City, m.WorkFormat,
		m.TravelRadiusKm, m.ExperienceYears, m.Specializations, m.HourlyRate, m.AvailabilityJSON,
		m.PayoutLegalForm,
		m.Status, m.ModerationComment, m.ModeratedBy, m.ModeratedAt,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *MasterRepo) UpdateStatus(ctx context.Context, masterID uuid.UUID, status, comment string, moderatedBy *uuid.UUID) error {
	const q = `
UPDATE masters SET
  status = $2,
  moderation_comment = $3,
  moderated_by = $4,
  moderated_at = now(),
  updated_at = now()
WHERE id = $1
`
	_, err := r.pool.Exec(ctx, q, masterID, status, comment, moderatedBy)
	return err
}

func (r *MasterRepo) ListByStatus(ctx context.Context, statusFilter string, limit, offset int32) ([]domain.Master, int32, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var total int32
	countQ := `SELECT COUNT(*) FROM masters`
	dataQ := `
SELECT id, user_id, slug, display_name, bio, phone, city, work_format,
  travel_radius_km, experience_years, specializations, hourly_rate, availability_json,
  payout_legal_form,
  status, moderation_comment, moderated_by, moderated_at, created_at, updated_at
FROM masters`
	args := []any{}
	where := ""
	if statusFilter != "" {
		where = " WHERE status = $1"
		args = append(args, statusFilter)
	}
	if err := r.pool.QueryRow(ctx, countQ+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	args = append(args, limit, offset)
	dataQ += where + fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d OFFSET $%d", limitArg, offsetArg)
	rows, err := r.pool.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []domain.Master
	for rows.Next() {
		var m domain.Master
		var modBy *uuid.UUID
		var modAt sql.NullTime
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.Slug, &m.DisplayName, &m.Bio, &m.Phone, &m.City, &m.WorkFormat,
			&m.TravelRadiusKm, &m.ExperienceYears, &m.Specializations, &m.HourlyRate, &m.AvailabilityJSON,
			&m.PayoutLegalForm,
			&m.Status, &m.ModerationComment, &modBy, &modAt, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		m.ModeratedBy = modBy
		if modAt.Valid {
			t := modAt.Time
			m.ModeratedAt = &t
		}
		svcs, err := r.loadServices(ctx, m.ID)
		if err != nil {
			return nil, 0, err
		}
		m.Services = svcs
		ph, err := r.loadPhotos(ctx, m.ID)
		if err != nil {
			return nil, 0, err
		}
		m.Photos = ph
		list = append(list, m)
	}
	return list, total, rows.Err()
}

func (r *MasterRepo) ListPublic(ctx context.Context, city string, limit, offset int32) ([]domain.Master, int32, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var total int32
	baseWhere := ` WHERE status = 'active'`
	args := []any{}
	if strings.TrimSpace(city) != "" {
		baseWhere += ` AND LOWER(city) = LOWER($1)`
		args = append(args, strings.TrimSpace(city))
	}
	countQ := `SELECT COUNT(*) FROM masters` + baseWhere
	dataQ := `
SELECT id, user_id, slug, display_name, bio, phone, city, work_format,
  travel_radius_km, experience_years, specializations, hourly_rate, availability_json,
  payout_legal_form,
  status, moderation_comment, moderated_by, moderated_at, created_at, updated_at
FROM masters` + baseWhere
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	args = append(args, limit, offset)
	dataQ += fmt.Sprintf(" ORDER BY display_name ASC LIMIT $%d OFFSET $%d", limitArg, offsetArg)
	rows, err := r.pool.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []domain.Master
	for rows.Next() {
		var m domain.Master
		var modBy *uuid.UUID
		var modAt sql.NullTime
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.Slug, &m.DisplayName, &m.Bio, &m.Phone, &m.City, &m.WorkFormat,
			&m.TravelRadiusKm, &m.ExperienceYears, &m.Specializations, &m.HourlyRate, &m.AvailabilityJSON,
			&m.PayoutLegalForm,
			&m.Status, &m.ModerationComment, &modBy, &modAt, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		m.ModeratedBy = modBy
		if modAt.Valid {
			t := modAt.Time
			m.ModeratedAt = &t
		}
		svcs, err := r.loadServices(ctx, m.ID)
		if err != nil {
			return nil, 0, err
		}
		m.Services = svcs
		ph, err := r.loadPhotos(ctx, m.ID)
		if err != nil {
			return nil, 0, err
		}
		m.Photos = ph
		list = append(list, m)
	}
	return list, total, rows.Err()
}

func (r *MasterRepo) ReplaceServices(ctx context.Context, masterID uuid.UUID, items []domain.MasterServiceUpsert) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM master_services WHERE master_id = $1`, masterID); err != nil {
		return err
	}
	for i, it := range items {
		so := it.SortOrder
		if so == 0 {
			so = int32(i)
		}
		_, err := tx.Exec(ctx, `
INSERT INTO master_services (id, master_id, name, description, duration_min, price, sort_order)
VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			uuid.New(), masterID, it.Name, it.Description, it.DurationMin, it.Price, so,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *MasterRepo) InsertModerationHistory(ctx context.Context, e *domain.ModerationHistoryEntry) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	_, err := r.pool.Exec(ctx, `
INSERT INTO master_moderation_history (id, master_id, old_status, new_status, comment, changed_by)
VALUES ($1,$2,$3,$4,$5,$6)`,
		e.ID, e.MasterID, e.OldStatus, e.NewStatus, e.Comment, e.ChangedBy,
	)
	return err
}

func (r *MasterRepo) ListModerationHistory(ctx context.Context, masterID uuid.UUID, limit int32) ([]domain.ModerationHistoryEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
SELECT id, master_id, old_status, new_status, comment, changed_by, created_at
FROM master_moderation_history WHERE master_id = $1 ORDER BY created_at DESC LIMIT $2`,
		masterID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ModerationHistoryEntry
	for rows.Next() {
		var e domain.ModerationHistoryEntry
		if err := rows.Scan(&e.ID, &e.MasterID, &e.OldStatus, &e.NewStatus, &e.Comment, &e.ChangedBy, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *MasterRepo) loadPhotos(ctx context.Context, masterID uuid.UUID) ([]domain.MasterPhoto, error) {
	rows, err := r.pool.Query(ctx, `
SELECT id, master_id, url, sort_order, is_cover
FROM master_photos WHERE master_id = $1 ORDER BY sort_order ASC, id ASC`, masterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MasterPhoto
	for rows.Next() {
		var p domain.MasterPhoto
		if err := rows.Scan(&p.ID, &p.MasterID, &p.URL, &p.SortOrder, &p.IsCover); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *MasterRepo) CountPhotosByMaster(ctx context.Context, masterID uuid.UUID) (int32, error) {
	var n int32
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM master_photos WHERE master_id = $1`, masterID).Scan(&n)
	return n, err
}

func (r *MasterRepo) AddMasterPhoto(ctx context.Context, masterID uuid.UUID, url string) (*domain.MasterPhoto, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var cnt int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM master_photos WHERE master_id = $1`, masterID).Scan(&cnt); err != nil {
		return nil, err
	}
	var maxSO int32
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sort_order), -1) FROM master_photos WHERE master_id = $1`, masterID).Scan(&maxSO); err != nil {
		return nil, err
	}
	sortOrder := maxSO + 1
	isCover := cnt == 0
	id := uuid.New()
	_, err = tx.Exec(ctx, `
INSERT INTO master_photos (id, master_id, url, sort_order, is_cover) VALUES ($1,$2,$3,$4,$5)`,
		id, masterID, url, sortOrder, isCover)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &domain.MasterPhoto{
		ID: id, MasterID: masterID, URL: url, SortOrder: sortOrder, IsCover: isCover,
	}, nil
}

func (r *MasterRepo) DeleteMasterPhoto(ctx context.Context, masterID, photoID uuid.UUID) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var url string
	var wasCover bool
	err = tx.QueryRow(ctx, `
DELETE FROM master_photos WHERE id = $1 AND master_id = $2 RETURNING url, is_cover`,
		photoID, masterID).Scan(&url, &wasCover)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", pgx.ErrNoRows
		}
		return "", err
	}
	if wasCover {
		var nextID uuid.UUID
		err = tx.QueryRow(ctx, `
SELECT id FROM master_photos WHERE master_id = $1 ORDER BY sort_order ASC, id ASC LIMIT 1`, masterID).Scan(&nextID)
		if err == nil {
			if _, err := tx.Exec(ctx, `UPDATE master_photos SET is_cover = true WHERE id = $1`, nextID); err != nil {
				return "", err
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return url, nil
}

func (r *MasterRepo) SetMasterCoverPhoto(ctx context.Context, masterID, photoID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var one int
	err = tx.QueryRow(ctx, `SELECT 1 FROM master_photos WHERE id = $1 AND master_id = $2`, photoID, masterID).Scan(&one)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE master_photos SET is_cover = false WHERE master_id = $1`, masterID); err != nil {
		return err
	}
	ct, err := tx.Exec(ctx, `UPDATE master_photos SET is_cover = true WHERE id = $1 AND master_id = $2`, photoID, masterID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}

func (r *MasterRepo) InsertBooking(ctx context.Context, b *domain.MasterBooking) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	_, err := r.pool.Exec(ctx, `
INSERT INTO master_bookings (id, master_id, client_user_id, master_service_id, date, time_from, time_to, comment, status)
VALUES ($1,$2,$3,$4,$5::date,$6::time,$7::time,$8,$9)`,
		b.ID, b.MasterID, b.ClientUserID, b.MasterServiceID, b.Date, b.TimeFrom, b.TimeTo, b.Comment, b.Status,
	)
	return err
}

func (r *MasterRepo) ListBookingsByMaster(ctx context.Context, masterID uuid.UUID, statusFilter string) ([]domain.MasterBooking, error) {
	q := `
SELECT id, master_id, client_user_id, master_service_id, date::text, time_from::text, time_to::text, comment, status, created_at
FROM master_bookings WHERE master_id = $1`
	args := []any{masterID}
	if statusFilter != "" {
		q += ` AND status = $2`
		args = append(args, statusFilter)
	}
	q += ` ORDER BY date DESC, time_from DESC`
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MasterBooking
	for rows.Next() {
		var b domain.MasterBooking
		var svcID *uuid.UUID
		if err := rows.Scan(&b.ID, &b.MasterID, &b.ClientUserID, &svcID, &b.Date, &b.TimeFrom, &b.TimeTo, &b.Comment, &b.Status, &b.CreatedAt); err != nil {
			return nil, err
		}
		b.MasterServiceID = svcID
		out = append(out, b)
	}
	return out, rows.Err()
}

// MakeUniqueSlug builds a URL slug from display name.
func (r *MasterRepo) NewSlug(ctx context.Context, displayName string) (string, error) {
	return makeUniqueSlug(ctx, r.pool, displayName)
}

func makeUniqueSlug(ctx context.Context, pool *pgxpool.Pool, displayName string) (string, error) {
	base := slug.Make(strings.TrimSpace(displayName))
	if base == "" {
		base = "master"
	}
	s := base
	for i := 0; i < 20; i++ {
		var n int
		err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM masters WHERE slug = $1`, s).Scan(&n)
		if err != nil {
			return "", err
		}
		if n == 0 {
			return s, nil
		}
		s = base + "-" + uuid.New().String()[:8]
	}
	return "", fmt.Errorf("could not allocate slug")
}
