package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

func float64PtrFromNull(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	f := n.Float64
	return &f
}

func float64PtrArg(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func marshalTravelExcludeZonesJSON(z []domain.MasterTravelExcludeZone) (string, error) {
	if len(z) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(z)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalTravelExcludeZonesJSON(s string) ([]domain.MasterTravelExcludeZone, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var z []domain.MasterTravelExcludeZone
	if err := json.Unmarshal([]byte(s), &z); err != nil {
		return nil, err
	}
	return z, nil
}

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
  travel_radius_km, travel_base_latitude, travel_base_longitude, travel_exclude_zones_json,
  experience_years, specializations, hourly_rate, availability_json,
  payout_legal_form, yookassa_seller_account_id,
  payout_legal_name, payout_inn, payout_kpp, payout_ogrn, payout_ogrnip,
  payout_bank_name, payout_bik, payout_settlement_account, payout_correspondent_account, payout_verification_status,
  status, moderation_comment, moderated_by, moderated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32)
`
	zj, err := marshalTravelExcludeZonesJSON(m.TravelExcludeZones)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, q,
		m.ID, m.UserID, m.Slug, m.DisplayName, m.Bio, m.Phone, m.City, m.WorkFormat,
		m.TravelRadiusKm, float64PtrArg(m.TravelBaseLatitude), float64PtrArg(m.TravelBaseLongitude), zj,
		m.ExperienceYears, m.Specializations, m.HourlyRate, m.AvailabilityJSON,
		m.PayoutLegalForm, m.YookassaSellerAccountID,
		m.PayoutLegalName, m.PayoutINN, m.PayoutKPP, m.PayoutOGRN, m.PayoutOGRNIP,
		m.PayoutBankName, m.PayoutBIK, m.PayoutSettlementAccount, m.PayoutCorrespondentAccount, m.PayoutVerificationStatus,
		m.Status, m.ModerationComment, m.ModeratedBy, m.ModeratedAt,
	)
	return err
}

func (r *MasterRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.Master, error) {
	const q = `
SELECT id, user_id, slug, display_name, bio, phone, city, work_format,
  travel_radius_km, travel_base_latitude, travel_base_longitude, travel_exclude_zones_json,
  experience_years, specializations, hourly_rate, availability_json,
  payout_legal_form, yookassa_seller_account_id,
  payout_legal_name, payout_inn, payout_kpp, payout_ogrn, payout_ogrnip,
  payout_bank_name, payout_bik, payout_settlement_account, payout_correspondent_account, payout_verification_status,
  status, moderation_comment, moderated_by, moderated_at, created_at, updated_at
FROM masters WHERE user_id = $1
`
	return r.scanMaster(ctx, r.pool.QueryRow(ctx, q, userID))
}

func (r *MasterRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Master, error) {
	const q = `
SELECT id, user_id, slug, display_name, bio, phone, city, work_format,
  travel_radius_km, travel_base_latitude, travel_base_longitude, travel_exclude_zones_json,
  experience_years, specializations, hourly_rate, availability_json,
  payout_legal_form, yookassa_seller_account_id,
  payout_legal_name, payout_inn, payout_kpp, payout_ogrn, payout_ogrnip,
  payout_bank_name, payout_bik, payout_settlement_account, payout_correspondent_account, payout_verification_status,
  status, moderation_comment, moderated_by, moderated_at, created_at, updated_at
FROM masters WHERE id = $1
`
	return r.scanMaster(ctx, r.pool.QueryRow(ctx, q, id))
}

func (r *MasterRepo) GetBySlug(ctx context.Context, s string) (*domain.Master, error) {
	const q = `
SELECT id, user_id, slug, display_name, bio, phone, city, work_format,
  travel_radius_km, travel_base_latitude, travel_base_longitude, travel_exclude_zones_json,
  experience_years, specializations, hourly_rate, availability_json,
  payout_legal_form, yookassa_seller_account_id,
  payout_legal_name, payout_inn, payout_kpp, payout_ogrn, payout_ogrnip,
  payout_bank_name, payout_bik, payout_settlement_account, payout_correspondent_account, payout_verification_status,
  status, moderation_comment, moderated_by, moderated_at, created_at, updated_at
FROM masters WHERE slug = $1
`
	return r.scanMaster(ctx, r.pool.QueryRow(ctx, q, s))
}

func (r *MasterRepo) scanMaster(ctx context.Context, row pgx.Row) (*domain.Master, error) {
	var m domain.Master
	var modBy *uuid.UUID
	var modAt sql.NullTime
	var lat, lon sql.NullFloat64
	var zonesJSON string
	err := row.Scan(
		&m.ID, &m.UserID, &m.Slug, &m.DisplayName, &m.Bio, &m.Phone, &m.City, &m.WorkFormat,
		&m.TravelRadiusKm, &lat, &lon, &zonesJSON, &m.ExperienceYears, &m.Specializations, &m.HourlyRate, &m.AvailabilityJSON,
		&m.PayoutLegalForm, &m.YookassaSellerAccountID,
		&m.PayoutLegalName, &m.PayoutINN, &m.PayoutKPP, &m.PayoutOGRN, &m.PayoutOGRNIP,
		&m.PayoutBankName, &m.PayoutBIK, &m.PayoutSettlementAccount, &m.PayoutCorrespondentAccount, &m.PayoutVerificationStatus,
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
	m.TravelBaseLatitude = float64PtrFromNull(lat)
	m.TravelBaseLongitude = float64PtrFromNull(lon)
	zones, zerr := unmarshalTravelExcludeZonesJSON(zonesJSON)
	if zerr != nil {
		m.TravelExcludeZones = nil
	} else {
		m.TravelExcludeZones = zones
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
  travel_radius_km = $7, travel_base_latitude = $8, travel_base_longitude = $9,
  travel_exclude_zones_json = $10,
  experience_years = $11, specializations = $12,
  hourly_rate = $13, availability_json = $14, payout_legal_form = $15, yookassa_seller_account_id = $16,
  payout_legal_name = $17, payout_inn = $18, payout_kpp = $19, payout_ogrn = $20, payout_ogrnip = $21,
  payout_bank_name = $22, payout_bik = $23, payout_settlement_account = $24, payout_correspondent_account = $25, payout_verification_status = $26,
  status = $27, moderation_comment = $28, moderated_by = $29, moderated_at = $30,
  updated_at = now()
WHERE id = $1
`
	zj, err := marshalTravelExcludeZonesJSON(m.TravelExcludeZones)
	if err != nil {
		return err
	}
	ct, err := r.pool.Exec(ctx, q,
		m.ID, m.DisplayName, m.Bio, m.Phone, m.City, m.WorkFormat,
		m.TravelRadiusKm, float64PtrArg(m.TravelBaseLatitude), float64PtrArg(m.TravelBaseLongitude), zj,
		m.ExperienceYears, m.Specializations, m.HourlyRate, m.AvailabilityJSON,
		m.PayoutLegalForm, m.YookassaSellerAccountID,
		m.PayoutLegalName, m.PayoutINN, m.PayoutKPP, m.PayoutOGRN, m.PayoutOGRNIP,
		m.PayoutBankName, m.PayoutBIK, m.PayoutSettlementAccount, m.PayoutCorrespondentAccount, m.PayoutVerificationStatus,
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
  travel_radius_km, travel_base_latitude, travel_base_longitude, travel_exclude_zones_json,
  experience_years, specializations, hourly_rate, availability_json,
  payout_legal_form, yookassa_seller_account_id,
  payout_legal_name, payout_inn, payout_kpp, payout_ogrn, payout_ogrnip,
  payout_bank_name, payout_bik, payout_settlement_account, payout_correspondent_account, payout_verification_status,
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
		var lat, lon sql.NullFloat64
		var zonesJSON string
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.Slug, &m.DisplayName, &m.Bio, &m.Phone, &m.City, &m.WorkFormat,
			&m.TravelRadiusKm, &lat, &lon, &zonesJSON, &m.ExperienceYears, &m.Specializations, &m.HourlyRate, &m.AvailabilityJSON,
			&m.PayoutLegalForm, &m.YookassaSellerAccountID,
			&m.PayoutLegalName, &m.PayoutINN, &m.PayoutKPP, &m.PayoutOGRN, &m.PayoutOGRNIP,
			&m.PayoutBankName, &m.PayoutBIK, &m.PayoutSettlementAccount, &m.PayoutCorrespondentAccount, &m.PayoutVerificationStatus,
			&m.Status, &m.ModerationComment, &modBy, &modAt, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		m.ModeratedBy = modBy
		if modAt.Valid {
			t := modAt.Time
			m.ModeratedAt = &t
		}
		m.TravelBaseLatitude = float64PtrFromNull(lat)
		m.TravelBaseLongitude = float64PtrFromNull(lon)
		zones, zerr := unmarshalTravelExcludeZonesJSON(zonesJSON)
		if zerr != nil {
			m.TravelExcludeZones = nil
		} else {
			m.TravelExcludeZones = zones
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

func (r *MasterRepo) ListPublic(ctx context.Context, p domain.ListPublicMastersParams) ([]domain.Master, int32, error) {
	limit := p.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}

	where := []string{"m.status = 'active'"}
	args := []any{}
	argPos := 1

	q := strings.TrimSpace(p.Query)
	if q != "" {
		pat := "%" + q + "%"
		// Имя, био, город, slug, телефон, специализации (TEXT[]), название/описание услуг.
		ph := argPos
		where = append(where, fmt.Sprintf(
			"(m.display_name ILIKE $%[1]d OR m.bio ILIKE $%[1]d OR m.city ILIKE $%[1]d OR m.slug ILIKE $%[1]d OR m.phone ILIKE $%[1]d OR COALESCE(array_to_string(m.specializations, ' '), '') ILIKE $%[1]d OR EXISTS (SELECT 1 FROM master_services ms WHERE ms.master_id = m.id AND (ms.name ILIKE $%[1]d OR ms.description ILIKE $%[1]d)))",
			ph,
		))
		args = append(args, pat)
		argPos++
	}

	var cityArgs []string
	for _, c := range p.Cities {
		t := strings.TrimSpace(c)
		if t == "" {
			continue
		}
		cityArgs = append(cityArgs, strings.ToLower(t))
	}
	if len(cityArgs) > 0 {
		where = append(where, fmt.Sprintf("LOWER(TRIM(m.city)) = ANY($%d::text[])", argPos))
		args = append(args, cityArgs)
		argPos++
	}

	wf := strings.TrimSpace(p.WorkFormat)
	if wf != "" && strings.EqualFold(wf, "all") {
		wf = ""
	}
	if wf != "" {
		where = append(where, fmt.Sprintf("m.work_format = $%d", argPos))
		args = append(args, wf)
		argPos++
	}

	effKop := `COALESCE((SELECT MIN(ms.price) FROM master_services ms WHERE ms.master_id = m.id AND ms.price > 0), m.hourly_rate)`
	if p.PriceMinKopecks > 0 {
		where = append(where, fmt.Sprintf("(%s) >= $%d", effKop, argPos))
		args = append(args, p.PriceMinKopecks)
		argPos++
	}
	if p.PriceMaxKopecks > 0 {
		where = append(where, fmt.Sprintf("(%s) <= $%d", effKop, argPos))
		args = append(args, p.PriceMaxKopecks)
		argPos++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	countQ := "SELECT COUNT(*) FROM masters m " + whereSQL

	filterArgs := append([]any{}, args...)
	var total int32
	if err := r.pool.QueryRow(ctx, countQ, filterArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limIdx := len(filterArgs) + 1
	offIdx := len(filterArgs) + 2
	dataQ := fmt.Sprintf(`
SELECT m.id, m.user_id, m.slug, m.display_name, m.bio, m.phone, m.city, m.work_format,
  m.travel_radius_km, m.travel_base_latitude, m.travel_base_longitude, m.travel_exclude_zones_json,
  m.experience_years, m.specializations, m.hourly_rate, m.availability_json,
  m.payout_legal_form, m.yookassa_seller_account_id,
  m.payout_legal_name, m.payout_inn, m.payout_kpp, m.payout_ogrn, m.payout_ogrnip,
  m.payout_bank_name, m.payout_bik, m.payout_settlement_account, m.payout_correspondent_account, m.payout_verification_status,
  m.status, m.moderation_comment, m.moderated_by, m.moderated_at, m.created_at, m.updated_at
FROM masters m
%s
ORDER BY m.display_name ASC
LIMIT $%d OFFSET $%d`, whereSQL, limIdx, offIdx)

	dataArgs := append(filterArgs, limit, offset)
	rows, err := r.pool.Query(ctx, dataQ, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []domain.Master
	for rows.Next() {
		var m domain.Master
		var modBy *uuid.UUID
		var modAt sql.NullTime
		var lat, lon sql.NullFloat64
		var zonesJSON string
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.Slug, &m.DisplayName, &m.Bio, &m.Phone, &m.City, &m.WorkFormat,
			&m.TravelRadiusKm, &lat, &lon, &zonesJSON, &m.ExperienceYears, &m.Specializations, &m.HourlyRate, &m.AvailabilityJSON,
			&m.PayoutLegalForm, &m.YookassaSellerAccountID,
			&m.PayoutLegalName, &m.PayoutINN, &m.PayoutKPP, &m.PayoutOGRN, &m.PayoutOGRNIP,
			&m.PayoutBankName, &m.PayoutBIK, &m.PayoutSettlementAccount, &m.PayoutCorrespondentAccount, &m.PayoutVerificationStatus,
			&m.Status, &m.ModerationComment, &modBy, &modAt, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		m.ModeratedBy = modBy
		if modAt.Valid {
			t := modAt.Time
			m.ModeratedAt = &t
		}
		m.TravelBaseLatitude = float64PtrFromNull(lat)
		m.TravelBaseLongitude = float64PtrFromNull(lon)
		zones, zerr := unmarshalTravelExcludeZonesJSON(zonesJSON)
		if zerr != nil {
			m.TravelExcludeZones = nil
		} else {
			m.TravelExcludeZones = zones
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
INSERT INTO master_bookings (id, master_id, client_user_id, master_service_id, date, time_from, time_to, comment, status, payment_id, payment_url, total_price)
VALUES ($1,$2,$3,$4,$5::date,$6::time,$7::time,$8,$9,$10,$11,$12)`,
		b.ID, b.MasterID, b.ClientUserID, b.MasterServiceID, b.Date, b.TimeFrom, b.TimeTo, b.Comment, b.Status, b.PaymentID, b.PaymentURL, b.TotalPrice,
	)
	return err
}

func (r *MasterRepo) GetBookingByID(ctx context.Context, bookingID uuid.UUID) (*domain.MasterBooking, error) {
	row := r.pool.QueryRow(ctx, `
SELECT id, master_id, client_user_id, master_service_id, date::text, time_from::text, time_to::text, comment, status, payment_id, payment_url, total_price, created_at
FROM master_bookings WHERE id = $1`, bookingID)
	var b domain.MasterBooking
	var svcID *uuid.UUID
	if err := row.Scan(
		&b.ID, &b.MasterID, &b.ClientUserID, &svcID, &b.Date, &b.TimeFrom, &b.TimeTo, &b.Comment, &b.Status, &b.PaymentID, &b.PaymentURL, &b.TotalPrice, &b.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	b.MasterServiceID = svcID
	return &b, nil
}

func (r *MasterRepo) GetBookingByPaymentID(ctx context.Context, paymentID string) (*domain.MasterBooking, error) {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return nil, nil
	}
	row := r.pool.QueryRow(ctx, `
SELECT id, master_id, client_user_id, master_service_id, date::text, time_from::text, time_to::text, comment, status, payment_id, payment_url, total_price, created_at
FROM master_bookings WHERE payment_id = $1`, paymentID)
	var b domain.MasterBooking
	var svcID *uuid.UUID
	if err := row.Scan(
		&b.ID, &b.MasterID, &b.ClientUserID, &svcID, &b.Date, &b.TimeFrom, &b.TimeTo, &b.Comment, &b.Status, &b.PaymentID, &b.PaymentURL, &b.TotalPrice, &b.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	b.MasterServiceID = svcID
	return &b, nil
}

func (r *MasterRepo) SetBookingPayment(ctx context.Context, bookingID uuid.UUID, paymentID, paymentURL string, totalPrice int64, status string) error {
	paymentID = strings.TrimSpace(paymentID)
	paymentURL = strings.TrimSpace(paymentURL)
	ct, err := r.pool.Exec(ctx, `
UPDATE master_bookings
SET payment_id = $2,
    payment_url = $3,
    total_price = $4,
    status = $5,
    updated_at = now()
WHERE id = $1`,
		bookingID, paymentID, paymentURL, totalPrice, status,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *MasterRepo) ListBookingsByMaster(ctx context.Context, masterID uuid.UUID, statusFilter string) ([]domain.MasterBooking, error) {
	q := `
SELECT id, master_id, client_user_id, master_service_id, date::text, time_from::text, time_to::text, comment, status, payment_id, payment_url, total_price, created_at
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
		if err := rows.Scan(&b.ID, &b.MasterID, &b.ClientUserID, &svcID, &b.Date, &b.TimeFrom, &b.TimeTo, &b.Comment, &b.Status, &b.PaymentID, &b.PaymentURL, &b.TotalPrice, &b.CreatedAt); err != nil {
			return nil, err
		}
		b.MasterServiceID = svcID
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *MasterRepo) ListBookingsByClient(ctx context.Context, clientUserID uuid.UUID, statusFilter string) ([]domain.MasterBooking, error) {
	q := `
SELECT id, master_id, client_user_id, master_service_id, date::text, time_from::text, time_to::text, comment, status, payment_id, payment_url, total_price, created_at
FROM master_bookings WHERE client_user_id = $1`
	args := []any{clientUserID}
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
		if err := rows.Scan(&b.ID, &b.MasterID, &b.ClientUserID, &svcID, &b.Date, &b.TimeFrom, &b.TimeTo, &b.Comment, &b.Status, &b.PaymentID, &b.PaymentURL, &b.TotalPrice, &b.CreatedAt); err != nil {
			return nil, err
		}
		b.MasterServiceID = svcID
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *MasterRepo) HasCompletedBookingByClientMaster(ctx context.Context, clientUserID, masterID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM master_bookings
  WHERE client_user_id = $1
    AND master_id = $2
    AND status = 'completed'
)`, clientUserID, masterID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *MasterRepo) UpdateBookingStatus(ctx context.Context, bookingID uuid.UUID, status string) error {
	ct, err := r.pool.Exec(ctx, `UPDATE master_bookings SET status = $2, updated_at = now() WHERE id = $1`, bookingID, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
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
