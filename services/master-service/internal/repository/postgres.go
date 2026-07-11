package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// dbZone is the repository-local type that owns the DB wire format for a
// travel-exclude zone. Its json tags define exactly what is stored in the
// travel_exclude_zones_json TEXT column and are intentionally decoupled from
// domain.MasterTravelExcludeZone, which carries no serialisation concerns.
type dbZone struct {
	ID        string  `json:"id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	RadiusKm  float64 `json:"radius_km"`
	Label     string  `json:"label,omitempty"`
}

func domainZonesToDB(zones []domain.MasterTravelExcludeZone) []dbZone {
	if len(zones) == 0 {
		return nil
	}
	out := make([]dbZone, len(zones))
	for i, z := range zones {
		out[i] = dbZone{
			ID:        z.ID,
			Latitude:  z.Latitude,
			Longitude: z.Longitude,
			RadiusKm:  z.RadiusKm,
			Label:     z.Label,
		}
	}
	return out
}

func dbZonesToDomain(zones []dbZone) []domain.MasterTravelExcludeZone {
	if len(zones) == 0 {
		return nil
	}
	out := make([]domain.MasterTravelExcludeZone, len(zones))
	for i, z := range zones {
		out[i] = domain.MasterTravelExcludeZone{
			ID:        z.ID,
			Latitude:  z.Latitude,
			Longitude: z.Longitude,
			RadiusKm:  z.RadiusKm,
			Label:     z.Label,
		}
	}
	return out
}

func marshalTravelExcludeZonesJSON(z []domain.MasterTravelExcludeZone) (string, error) {
	db := domainZonesToDB(z)
	if len(db) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(db)
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
	var db []dbZone
	if err := json.Unmarshal([]byte(s), &db); err != nil {
		return nil, err
	}
	return dbZonesToDomain(db), nil
}

// Compile-time check: *MasterRepo must satisfy the domain interface.
// If a method is added to domain.MasterRepository but not implemented here,
// this line fails with a clear error instead of a runtime panic.
var _ domain.MasterRepository = (*MasterRepo)(nil)

// masterColumns is the canonical SELECT column list for the masters table.
// Use it in single-table queries (no alias). The order must match the
// arguments to scanMasterRow exactly — add a column to both or neither.
const masterColumns = `id, user_id, slug, display_name, bio, phone, city, work_format,
  travel_radius_km, travel_base_latitude, travel_base_longitude, travel_exclude_zones_json,
  experience_years, specializations, hourly_rate, availability_json,
  payout_legal_form,
  payout_legal_name, payout_inn, payout_kpp, payout_ogrn, payout_ogrnip,
  payout_bank_name, payout_bik, payout_settlement_account, payout_correspondent_account, payout_verification_status,
  status, moderation_comment, moderated_by, moderated_at, created_at, updated_at`

// masterColumnsAliased is the same list with the "m." table alias, for use in
// queries that join or alias the masters table (e.g. ListPublic).
const masterColumnsAliased = `m.id, m.user_id, m.slug, m.display_name, m.bio, m.phone, m.city, m.work_format,
  m.travel_radius_km, m.travel_base_latitude, m.travel_base_longitude, m.travel_exclude_zones_json,
  m.experience_years, m.specializations, m.hourly_rate, m.availability_json,
  m.payout_legal_form,
  m.payout_legal_name, m.payout_inn, m.payout_kpp, m.payout_ogrn, m.payout_ogrnip,
  m.payout_bank_name, m.payout_bik, m.payout_settlement_account, m.payout_correspondent_account, m.payout_verification_status,
  m.status, m.moderation_comment, m.moderated_by, m.moderated_at, m.created_at, m.updated_at`

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
  payout_legal_form,
  payout_legal_name, payout_inn, payout_kpp, payout_ogrn, payout_ogrnip,
  payout_bank_name, payout_bik, payout_settlement_account, payout_correspondent_account, payout_verification_status,
  status, moderation_comment, moderated_by, moderated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31)
RETURNING created_at, updated_at
`
	zj, err := marshalTravelExcludeZonesJSON(m.TravelExcludeZones)
	if err != nil {
		return err
	}
	err = r.pool.QueryRow(ctx, q,
		m.ID, m.UserID, m.Slug, m.DisplayName, m.Bio, m.Phone, m.City, m.WorkFormat,
		m.TravelRadiusKm, float64PtrArg(m.TravelBaseLatitude), float64PtrArg(m.TravelBaseLongitude), zj,
		m.ExperienceYears, m.Specializations, m.HourlyRate, m.AvailabilityJSON,
		m.PayoutLegalForm,
		m.PayoutLegalName, m.PayoutINN, m.PayoutKPP, m.PayoutOGRN, m.PayoutOGRNIP,
		m.PayoutBankName, m.PayoutBIK, m.PayoutSettlementAccount, m.PayoutCorrespondentAccount, m.PayoutVerificationStatus,
		m.Status, m.ModerationComment, m.ModeratedBy, m.ModeratedAt,
	).Scan(&m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "masters_slug_key":
				// Two goroutines raced through NewSlug and picked the same candidate.
				// Return ErrSlugConflict so the usecase retries with a fresh slug.
				return domain.ErrSlugConflict
			case "masters_user_id_key":
				// Two concurrent CreateMyProfile calls for the same userID raced
				// through the GetByUserID check. Return a domain sentinel so the
				// usecase can map it to AlreadyExists instead of surfacing a 5xx.
				return domain.ErrUserProfileExists
			}
		}
	}
	return err
}

func (r *MasterRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.Master, error) {
	return r.scanMaster(ctx, r.pool.QueryRow(ctx,
		"SELECT "+masterColumns+" FROM masters WHERE user_id = $1", userID))
}

func (r *MasterRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Master, error) {
	return r.scanMaster(ctx, r.pool.QueryRow(ctx,
		"SELECT "+masterColumns+" FROM masters WHERE id = $1", id))
}

// GetMasterUserIDsByIDs returns a map[masterID]userID for the given master ids.
// Only id and user_id are fetched — used for access-control checks in batch operations.
func (r *MasterRepo) GetMasterUserIDsByIDs(ctx context.Context, masterIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	if len(masterIDs) == 0 {
		return map[uuid.UUID]uuid.UUID{}, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT id, user_id FROM masters WHERE id = ANY($1::uuid[])`, masterIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uuid.UUID]uuid.UUID, len(masterIDs))
	for rows.Next() {
		var mid, uid uuid.UUID
		if err := rows.Scan(&mid, &uid); err != nil {
			return nil, err
		}
		out[mid] = uid
	}
	return out, rows.Err()
}

func (r *MasterRepo) GetBySlug(ctx context.Context, s string) (*domain.Master, error) {
	return r.scanMaster(ctx, r.pool.QueryRow(ctx,
		"SELECT "+masterColumns+" FROM masters WHERE slug = $1", s))
}

// scanMasterRow scans a single row from a masters SELECT into a domain.Master.
// It handles nullable fields (moderated_by, moderated_at, lat/lon, zones JSON)
// but does NOT load associated services or photos — call attachAssociations
// (or scanMaster for single-row GetBy* lookups) after scanning.
//
// Returns (nil, nil) on pgx.ErrNoRows so callers can treat "not found" as nil.
func scanMasterRow(row pgx.Row) (*domain.Master, error) {
	var m domain.Master
	var modBy *uuid.UUID
	var modAt sql.NullTime
	var lat, lon sql.NullFloat64
	var zonesJSON string
	err := row.Scan(
		&m.ID, &m.UserID, &m.Slug, &m.DisplayName, &m.Bio, &m.Phone, &m.City, &m.WorkFormat,
		&m.TravelRadiusKm, &lat, &lon, &zonesJSON, &m.ExperienceYears, &m.Specializations, &m.HourlyRate, &m.AvailabilityJSON,
		&m.PayoutLegalForm,
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
	if zerr == nil {
		m.TravelExcludeZones = zones
	}
	return &m, nil
}

// scanMasterRowWithTotal is like scanMasterRow but also scans the extra
// COUNT(*) OVER () column appended by list queries that use window-function
// pagination. total is written on every row; callers should declare it outside
// the loop and pass the same pointer each iteration — the final value is the
// window-function result (identical for all rows in the result-set).
//
// When the result-set is empty (OFFSET >= total rows), the loop body never
// executes and *total remains at its zero value (0), which correctly signals
// "no rows on this page".
func scanMasterRowWithTotal(row pgx.Row, total *int32) (*domain.Master, error) {
	var m domain.Master
	var modBy *uuid.UUID
	var modAt sql.NullTime
	var lat, lon sql.NullFloat64
	var zonesJSON string
	err := row.Scan(
		&m.ID, &m.UserID, &m.Slug, &m.DisplayName, &m.Bio, &m.Phone, &m.City, &m.WorkFormat,
		&m.TravelRadiusKm, &lat, &lon, &zonesJSON, &m.ExperienceYears, &m.Specializations, &m.HourlyRate, &m.AvailabilityJSON,
		&m.PayoutLegalForm,
		&m.PayoutLegalName, &m.PayoutINN, &m.PayoutKPP, &m.PayoutOGRN, &m.PayoutOGRNIP,
		&m.PayoutBankName, &m.PayoutBIK, &m.PayoutSettlementAccount, &m.PayoutCorrespondentAccount, &m.PayoutVerificationStatus,
		&m.Status, &m.ModerationComment, &modBy, &modAt, &m.CreatedAt, &m.UpdatedAt,
		total,
	)
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
	if zerr == nil {
		m.TravelExcludeZones = zones
	}
	return &m, nil
}

// scanMaster scans a single-row lookup and eagerly loads services and photos
// via attachAssociations (reuses the batch path with a one-element slice so
// there is no separate single-row code path to maintain).
// Use this for GetBy* methods. For list queries use scanMasterRow in the loop
// and call attachAssociations on the full slice afterwards.
func (r *MasterRepo) scanMaster(ctx context.Context, row pgx.Row) (*domain.Master, error) {
	m, err := scanMasterRow(row)
	if err != nil || m == nil {
		return m, err
	}
	list := []domain.Master{*m}
	if err := r.attachAssociations(ctx, list); err != nil {
		return nil, err
	}
	return &list[0], nil
}

// batchLoadServices fetches services for a set of masters in a single query and
// returns them grouped by master ID. The ORDER BY preserves sort_order within
// each master so callers do not need to re-sort.
func (r *MasterRepo) batchLoadServices(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]domain.MasterService, error) {
	out := make(map[uuid.UUID][]domain.MasterService, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
SELECT id, master_id, name, description, duration_min, price, sort_order
FROM master_services
WHERE master_id = ANY($1::uuid[])
ORDER BY master_id, sort_order, id`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s domain.MasterService
		if err := rows.Scan(&s.ID, &s.MasterID, &s.Name, &s.Description, &s.DurationMin, &s.Price, &s.SortOrder); err != nil {
			return nil, err
		}
		out[s.MasterID] = append(out[s.MasterID], s)
	}
	return out, rows.Err()
}

// batchLoadPhotos fetches photos for a set of masters in a single query and
// returns them grouped by master ID, ordered by sort_order ASC, id ASC.
func (r *MasterRepo) batchLoadPhotos(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]domain.MasterPhoto, error) {
	out := make(map[uuid.UUID][]domain.MasterPhoto, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
SELECT id, master_id, url, sort_order, is_cover
FROM master_photos
WHERE master_id = ANY($1::uuid[])
ORDER BY master_id, sort_order ASC, id ASC`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p domain.MasterPhoto
		if err := rows.Scan(&p.ID, &p.MasterID, &p.URL, &p.SortOrder, &p.IsCover); err != nil {
			return nil, err
		}
		out[p.MasterID] = append(out[p.MasterID], p)
	}
	return out, rows.Err()
}

// batchLoadCredentials fetches certificates/awards for a set of masters in a
// single query and returns them grouped by master ID, ordered by sort_order ASC
// then id so callers do not need to re-sort.
func (r *MasterRepo) batchLoadCredentials(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]domain.MasterCredential, error) {
	out := make(map[uuid.UUID][]domain.MasterCredential, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
SELECT id, master_id, kind, title, issuer, year, sort_order
FROM master_credentials
WHERE master_id = ANY($1::uuid[])
ORDER BY master_id, sort_order, id`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c domain.MasterCredential
		if err := rows.Scan(&c.ID, &c.MasterID, &c.Kind, &c.Title, &c.Issuer, &c.Year, &c.SortOrder); err != nil {
			return nil, err
		}
		out[c.MasterID] = append(out[c.MasterID], c)
	}
	return out, rows.Err()
}

// attachAssociations populates Services, Photos and Credentials on every element
// of list using a fixed number of batch queries regardless of list length. It
// mutates list in-place and is safe to call with an empty slice (no queries are
// issued).
func (r *MasterRepo) attachAssociations(ctx context.Context, list []domain.Master) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(list))
	for i := range list {
		ids[i] = list[i].ID
	}
	svcsMap, err := r.batchLoadServices(ctx, ids)
	if err != nil {
		return err
	}
	phMap, err := r.batchLoadPhotos(ctx, ids)
	if err != nil {
		return err
	}
	credMap, err := r.batchLoadCredentials(ctx, ids)
	if err != nil {
		return err
	}
	// Index-based loop is intentional: list is a []domain.Master value slice
	// (not a pointer slice), so `for _, m := range list { m.Services = ... }`
	// would mutate a copy and the caller would see no change. list[i].Field = …
	// addresses the actual element in the backing array.
	for i := range list {
		list[i].Services = svcsMap[list[i].ID]    // nil if master has no services
		list[i].Photos = phMap[list[i].ID]        // nil if master has no photos
		list[i].Credentials = credMap[list[i].ID] // nil if master has no credentials
	}
	return nil
}

// updateMasterSQL updates every editable column on the masters row and returns
// the new updated_at. Shared by UpdateProfile and UpdateProfileWithAssociations
// so the column list lives in one place. The argument order must match
// updateMasterArgs exactly.
const updateMasterSQL = `
UPDATE masters SET
  display_name = $2, bio = $3, phone = $4, city = $5, work_format = $6,
  travel_radius_km = $7, travel_base_latitude = $8, travel_base_longitude = $9,
  travel_exclude_zones_json = $10,
  experience_years = $11, specializations = $12,
  hourly_rate = $13, availability_json = $14, payout_legal_form = $15,
  payout_legal_name = $16, payout_inn = $17, payout_kpp = $18, payout_ogrn = $19, payout_ogrnip = $20,
  payout_bank_name = $21, payout_bik = $22, payout_settlement_account = $23, payout_correspondent_account = $24, payout_verification_status = $25,
  status = $26, moderation_comment = $27, moderated_by = $28, moderated_at = $29,
  updated_at = now()
WHERE id = $1
RETURNING updated_at
`

// updateMasterArgs returns the positional args for updateMasterSQL. zj is the
// pre-marshalled travel_exclude_zones JSON.
func updateMasterArgs(m *domain.Master, zj string) []any {
	return []any{
		m.ID, m.DisplayName, m.Bio, m.Phone, m.City, m.WorkFormat,
		m.TravelRadiusKm, float64PtrArg(m.TravelBaseLatitude), float64PtrArg(m.TravelBaseLongitude), zj,
		m.ExperienceYears, m.Specializations, m.HourlyRate, m.AvailabilityJSON,
		m.PayoutLegalForm,
		m.PayoutLegalName, m.PayoutINN, m.PayoutKPP, m.PayoutOGRN, m.PayoutOGRNIP,
		m.PayoutBankName, m.PayoutBIK, m.PayoutSettlementAccount, m.PayoutCorrespondentAccount, m.PayoutVerificationStatus,
		m.Status, m.ModerationComment, m.ModeratedBy, m.ModeratedAt,
	}
}

func (r *MasterRepo) UpdateProfile(ctx context.Context, m *domain.Master) error {
	zj, err := marshalTravelExcludeZonesJSON(m.TravelExcludeZones)
	if err != nil {
		return err
	}
	err = r.pool.QueryRow(ctx, updateMasterSQL, updateMasterArgs(m, zj)...).Scan(&m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

// UpdateProfileWithAssociations updates the master row and, when the apply flags
// are set, replaces services and/or credentials — all in ONE transaction. A
// failure in any step rolls back the whole save, so the caller never observes a
// profile whose scalar fields and status were committed while its services or
// credentials are stale. m.UpdatedAt is populated from the UPDATE's RETURNING.
func (r *MasterRepo) UpdateProfileWithAssociations(ctx context.Context, m *domain.Master, applyServices bool, services []domain.MasterServiceUpsert, applyCredentials bool, credentials []domain.MasterCredentialUpsert) error {
	zj, err := marshalTravelExcludeZonesJSON(m.TravelExcludeZones)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	err = tx.QueryRow(ctx, updateMasterSQL, updateMasterArgs(m, zj)...).Scan(&m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if applyServices {
		if _, err := replaceServicesTx(ctx, tx, m.ID, services); err != nil {
			return err
		}
	}
	if applyCredentials {
		if _, err := replaceCredentialsTx(ctx, tx, m.ID, credentials); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
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
	ct, err := r.pool.Exec(ctx, q, masterID, status, comment, moderatedBy)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *MasterRepo) SuspendByUser(ctx context.Context, userID uuid.UUID) (bool, error) {
	const q = `
UPDATE masters SET
  status = $2,
  moderated_at = now(),
  updated_at = now()
WHERE user_id = $1 AND status <> $2
`
	ct, err := r.pool.Exec(ctx, q, userID, domain.StatusSuspended)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

func (r *MasterRepo) ListByStatus(ctx context.Context, statusFilter string, limit, offset int32) ([]domain.Master, int32, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{}
	where := ""
	if statusFilter != "" {
		where = " WHERE status = $1"
		args = append(args, statusFilter)
	}
	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	args = append(args, limit, offset)
	dataQ := fmt.Sprintf(
		"SELECT %s, COUNT(*) OVER () AS _total FROM masters%s ORDER BY updated_at DESC LIMIT $%d OFFSET $%d",
		masterColumns, where, limitArg, offsetArg)
	rows, err := r.pool.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var (
		list  []domain.Master
		total int32
	)
	for rows.Next() {
		m, err := scanMasterRowWithTotal(rows, &total)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := r.attachAssociations(ctx, list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
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
		// Primary filter: GIN-indexed tsvector covering display_name, bio, city,
		// specializations. plainto_tsquery is safe with raw user input (no special
		// tsquery syntax). The idx_masters_fts index (migration 011) makes this
		// O(log N + k) instead of a full sequential scan.
		//
		// Edge case — stop-words only (e.g. "и", "но", "то"):
		// plainto_tsquery('russian', 'и') returns an empty tsquery, and
		// tsvector @@ ''::tsquery is always false, which would produce zero
		// results even though the ILIKE branches should still match. The guard
		//   plainto_tsquery('russian', $1) <> ''::tsquery
		// disables the FTS branch for stop-word-only queries so the fallback
		// ILIKE branches remain active and return sensible results.
		//
		// Secondary filter: service name/description via EXISTS. The comment
		// "candidate set is already small" was optimistic: OR branches are not
		// cascading — the planner is free to evaluate EXISTS first if its cost
		// estimate is lower, which means it may scan all active masters' services
		// before touching the tsvector or ILIKE branches. In practice the planner
		// prefers the GIN-indexed tsvector arm first (lower estimated rows), but
		// this is not guaranteed and can regress as statistics change.
		// A GIN index on master_services(to_tsvector('russian', name||description))
		// would make the EXISTS arm index-safe. Tracked in TECH_DEBT.md [MASTER-FTS-MEILI].
		where = append(where, fmt.Sprintf(
			`(
  (
    plainto_tsquery('russian', $%[1]d) <> ''::tsquery
    -- masters_search_tsv (migration 011) is an IMMUTABLE wrapper so this
    -- expression matches the GIN index idx_masters_fts and the planner can use it.
    AND masters_search_tsv(m.display_name, m.bio, m.city, m.specializations)
        @@ plainto_tsquery('russian', $%[1]d)
  )
  OR m.slug  ILIKE '%%' || $%[1]d || '%%'
  OR m.phone ILIKE '%%' || $%[1]d || '%%'
  OR EXISTS (
    SELECT 1 FROM master_services ms
    WHERE ms.master_id = m.id
      AND (ms.name ILIKE '%%' || $%[1]d || '%%' OR ms.description ILIKE '%%' || $%[1]d || '%%')
  )
)`,
			argPos,
		))
		args = append(args, q)
		argPos++
	}

	var cityArgs []string
	for _, c := range p.Cities {
		// city is stored normalised (lowercase, trimmed) since migration 013.
		// Normalise the filter values the same way so idx_masters_city is usable.
		t := strings.ToLower(strings.TrimSpace(c))
		if t == "" {
			continue
		}
		cityArgs = append(cityArgs, t)
	}
	if len(cityArgs) > 0 {
		where = append(where, fmt.Sprintf("m.city = ANY($%d::text[])", argPos))
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

	// Price filter uses the denormalised column maintained by
	// trg_master_min_price_services / trg_master_min_price_hourly (migration 012).
	// COALESCE(..., 0) treats unconfigured profiles (NULL column) as price=0 so
	// they are excluded by any min-price filter, matching the old subquery behaviour.
	if p.PriceMinKopecks > 0 {
		where = append(where, fmt.Sprintf("COALESCE(m.min_effective_price_kopecks, 0) >= $%d", argPos))
		args = append(args, p.PriceMinKopecks)
		argPos++
	}
	if p.PriceMaxKopecks > 0 {
		where = append(where, fmt.Sprintf("COALESCE(m.min_effective_price_kopecks, 0) <= $%d", argPos))
		args = append(args, p.PriceMaxKopecks)
		argPos++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")

	// Single-query pagination: COUNT(*) OVER () is computed once by the planner
	// and attached to every row, eliminating the separate COUNT round-trip and
	// ensuring both the data and the total share a single execution plan.
	//
	// Edge case: when OFFSET >= total the result-set is empty and total cannot
	// be read from any row. Callers must treat an empty page as the end of
	// pagination (total=0 means "no results on this page", not "zero masters
	// exist"). The proto field ListMastersResponse.Total documents this.
	limIdx := len(args) + 1
	offIdx := len(args) + 2
	dataQ := fmt.Sprintf(
		"SELECT %s, COUNT(*) OVER () AS _total FROM masters m\n%s\nORDER BY m.display_name ASC\nLIMIT $%d OFFSET $%d",
		masterColumnsAliased, whereSQL, limIdx, offIdx)

	dataArgs := append(append([]any{}, args...), limit, offset)
	rows, err := r.pool.Query(ctx, dataQ, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var (
		list  []domain.Master
		total int32
	)
	for rows.Next() {
		m, err := scanMasterRowWithTotal(rows, &total)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := r.attachAssociations(ctx, list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// replaceServicesTx deletes and re-inserts a master's services within tx. Shared
// by ReplaceServices (own tx) and UpdateProfileWithAssociations (shared tx).
func replaceServicesTx(ctx context.Context, tx pgx.Tx, masterID uuid.UUID, items []domain.MasterServiceUpsert) ([]domain.MasterService, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM master_services WHERE master_id = $1`, masterID); err != nil {
		return nil, err
	}
	out := make([]domain.MasterService, 0, len(items))
	for i, it := range items {
		// Use the caller-supplied sort_order when explicitly set (including 0 —
		// "pin at the top"). Fall back to list index only when the caller omitted
		// sort_order entirely, i.e. "I don't care, order by position in the list".
		so := it.SortOrder
		if !it.SortOrderSet {
			so = int32(i)
		}
		var s domain.MasterService
		err := tx.QueryRow(ctx, `
INSERT INTO master_services (id, master_id, name, description, duration_min, price, sort_order)
VALUES ($1,$2,$3,$4,$5,$6,$7)
RETURNING id, master_id, name, description, duration_min, price, sort_order`,
			uuid.New(), masterID, it.Name, it.Description, it.DurationMin, it.Price, so,
		).Scan(&s.ID, &s.MasterID, &s.Name, &s.Description, &s.DurationMin, &s.Price, &s.SortOrder)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// replaceCredentialsTx deletes and re-inserts a master's credentials within tx.
// Shared by ReplaceCredentials (own tx) and UpdateProfileWithAssociations.
func replaceCredentialsTx(ctx context.Context, tx pgx.Tx, masterID uuid.UUID, items []domain.MasterCredentialUpsert) ([]domain.MasterCredential, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM master_credentials WHERE master_id = $1`, masterID); err != nil {
		return nil, err
	}
	out := make([]domain.MasterCredential, 0, len(items))
	for i, it := range items {
		var c domain.MasterCredential
		err := tx.QueryRow(ctx, `
INSERT INTO master_credentials (id, master_id, kind, title, issuer, year, sort_order)
VALUES ($1,$2,$3,$4,$5,$6,$7)
RETURNING id, master_id, kind, title, issuer, year, sort_order`,
			uuid.New(), masterID, it.Kind, it.Title, it.Issuer, it.Year, int32(i),
		).Scan(&c.ID, &c.MasterID, &c.Kind, &c.Title, &c.Issuer, &c.Year, &c.SortOrder)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *MasterRepo) ReplaceServices(ctx context.Context, masterID uuid.UUID, items []domain.MasterServiceUpsert) ([]domain.MasterService, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	out, err := replaceServicesTx(ctx, tx, masterID, items)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// ReplaceCredentials atomically swaps a master's full credential list. Like
// ReplaceServices it DELETEs the existing rows and re-INSERTs the supplied
// items inside one transaction, so a failed save never leaves a partial list.
func (r *MasterRepo) ReplaceCredentials(ctx context.Context, masterID uuid.UUID, items []domain.MasterCredentialUpsert) ([]domain.MasterCredential, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	out, err := replaceCredentialsTx(ctx, tx, masterID, items)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
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

// UpdateStatusWithHistory atomically updates masters.status and appends a
// moderation history entry in a single transaction. This prevents the partial-
// write window where UpdateStatus succeeds but InsertModerationHistory fails,
// which would lose an audit record required for 152-ФЗ compliance.
func (r *MasterRepo) UpdateStatusWithHistory(ctx context.Context, masterID uuid.UUID, status, comment string, moderatedBy *uuid.UUID, entry *domain.ModerationHistoryEntry) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	ct, err := tx.Exec(ctx, `
UPDATE masters SET
  status = $2,
  moderation_comment = $3,
  moderated_by = $4,
  moderated_at = now(),
  updated_at = now()
WHERE id = $1`, masterID, status, comment, moderatedBy)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	_, err = tx.Exec(ctx, `
INSERT INTO master_moderation_history (id, master_id, old_status, new_status, comment, changed_by)
VALUES ($1,$2,$3,$4,$5,$6)`,
		entry.ID, entry.MasterID, entry.OldStatus, entry.NewStatus, entry.Comment, entry.ChangedBy,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ModerateAtomic performs a conditional moderation transition:
//
//	UPDATE masters SET status = $newStatus … WHERE id = $masterID AND status = $expectedOldStatus
//
// The WHERE clause on the old status is the concurrency guard: if two
// moderators race, the second UPDATE touches 0 rows and ErrModerationConflict
// is returned. The history entry is written in the same transaction so partial
// writes are impossible.
func (r *MasterRepo) ModerateAtomic(ctx context.Context, masterID uuid.UUID, expectedOldStatus, newStatus, comment string, moderatedBy *uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// First lock the row and verify it still exists with the expected status.
	// Using SELECT … FOR UPDATE before the UPDATE lets us distinguish
	// "row not found" from "row found but status changed" — two semantically
	// different outcomes that both produce 0 rows in a bare UPDATE.
	var currentStatus string
	err = tx.QueryRow(ctx,
		`SELECT status FROM masters WHERE id = $1 FOR UPDATE`,
		masterID,
	).Scan(&currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if currentStatus != expectedOldStatus {
		// Another moderator already changed the status; abort so the caller
		// can surface a Conflict/Aborted response and ask for a refresh.
		return domain.ErrModerationConflict
	}

	_, err = tx.Exec(ctx, `
UPDATE masters SET
  status             = $2,
  moderation_comment = $3,
  moderated_by       = $4,
  moderated_at       = now(),
  updated_at         = now()
WHERE id = $1`, masterID, newStatus, comment, moderatedBy)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
INSERT INTO master_moderation_history (id, master_id, old_status, new_status, comment, changed_by)
VALUES ($1,$2,$3,$4,$5,$6)`,
		uuid.New(), masterID, expectedOldStatus, newStatus, comment, moderatedBy,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// SubmitForReviewAtomic atomically transitions the master's status to
// pending_review (only from draft / needs_revision / rejected) and records
// a ModerationHistoryEntry, all within a single transaction.
//
// The WHERE clause guards against the duplicate-submit race:
//
//	AND status IN ('draft','needs_revision','rejected')
//
// If the row is already in pending_review (second concurrent call), the UPDATE
// touches 0 rows, the transaction is rolled back, and ErrSubmitNotAllowed is
// returned — preventing a duplicate history entry.
func (r *MasterRepo) SubmitForReviewAtomic(ctx context.Context, masterID, userID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Use a CTE to capture the old status before UPDATE so it can be stored
	// in the history entry. RETURNING status in a plain UPDATE reflects the
	// new value; we need the old one for the audit record.
	var oldStatus string
	err = tx.QueryRow(ctx, `
WITH old AS (
    SELECT status FROM masters WHERE id = $1 FOR UPDATE
)
UPDATE masters
SET    status             = $2,
       moderation_comment = '',
       moderated_by       = NULL,
       moderated_at       = NULL,
       updated_at         = now()
FROM   old
WHERE  masters.id = $1
  AND  old.status IN ('draft', 'needs_revision', 'rejected')
RETURNING old.status`,
		masterID, domain.StatusPendingReview,
	).Scan(&oldStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		// 0 rows: master not found or status not submittable (e.g. duplicate submit).
		return domain.ErrSubmitNotAllowed
	}
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
INSERT INTO master_moderation_history (id, master_id, old_status, new_status, comment, changed_by)
VALUES ($1,$2,$3,$4,$5,$6)`,
		uuid.New(), masterID, oldStatus, domain.StatusPendingReview, "submitted by master", userID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
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

	// COUNT and limit check share the same transaction so concurrent uploads
	// cannot both read cnt < MaxMasterPhotos and both succeed — the second will
	// block on the row-level lock acquired by the first INSERT, then re-read
	// the updated count and return ErrPhotoLimitReached.
	var cnt int32
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM master_photos WHERE master_id = $1`, masterID).Scan(&cnt); err != nil {
		return nil, err
	}
	if cnt >= domain.MaxMasterPhotos {
		return nil, domain.ErrPhotoLimitReached
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
			return "", domain.ErrNotFound
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
	// Single UPDATE: SET is_cover = (id = $photoID) atomically clears all other
	// covers and sets the new one in one statement, eliminating the window between
	// the old two-statement pattern where no photo was the cover.
	//
	// With the partial unique index uq_master_photos_one_cover (migration 015),
	// a two-statement approach would still be safe (clear-then-set within a
	// transaction, constraint checked at COMMIT), but a single statement is
	// simpler and avoids any constraint check ordering concerns.
	ct, err := r.pool.Exec(ctx, `
UPDATE master_photos
SET    is_cover = (id = $2)
WHERE  master_id = $1`, masterID, photoID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
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
	// 23P01 = exclusion_violation from master_bookings_no_overlap (migration 024):
	// another non-cancelled booking already covers an overlapping interval.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
		return domain.ErrSlotTaken
	}
	return err
}

// HasSlotBlockConflict reports whether [timeFrom, timeTo) on date overlaps any
// of the master's slot blocks. A whole-day block (time_from IS NULL) matches any
// interval on that date. The interval predicate uses only constant literals plus
// bound params, so no user input is interpolated; masterID is bound as $1.
func (r *MasterRepo) HasSlotBlockConflict(ctx context.Context, masterID uuid.UUID, date, timeFrom, timeTo string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1 FROM master_slot_blocks
  WHERE master_id = $1 AND date = $2::date
    AND (
      time_from IS NULL
      OR tsrange(($2::date + time_from)::timestamp, ($2::date + time_to)::timestamp, '[)')
         && tsrange(($2::date + $3::time)::timestamp, ($2::date + $4::time)::timestamp, '[)')
    )
)`, masterID, date, timeFrom, timeTo).Scan(&exists)
	return exists, err
}

// DeleteBooking hard-deletes a booking row. It is a compensating action for
// the CreateBooking saga and must only be called on a booking that is still
// in "pending" status (i.e. before any payment has been initiated or after a
// failed CreatePayment call). Using a fresh context (context.Background) is
// recommended so a cancelled request context does not block the cleanup.
func (r *MasterRepo) DeleteBooking(ctx context.Context, bookingID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM master_bookings WHERE id = $1`, bookingID)
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

func (r *MasterRepo) GetBookingsByIDs(ctx context.Context, bookingIDs []uuid.UUID) ([]domain.MasterBooking, error) {
	if len(bookingIDs) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
SELECT id, master_id, client_user_id, master_service_id, date::text, time_from::text, time_to::text, comment, status, payment_id, payment_url, total_price, created_at
FROM master_bookings WHERE id = ANY($1::uuid[])`, bookingIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MasterBooking
	for rows.Next() {
		var b domain.MasterBooking
		var svcID *uuid.UUID
		if err := rows.Scan(
			&b.ID, &b.MasterID, &b.ClientUserID, &svcID, &b.Date, &b.TimeFrom, &b.TimeTo, &b.Comment, &b.Status, &b.PaymentID, &b.PaymentURL, &b.TotalPrice, &b.CreatedAt,
		); err != nil {
			return nil, err
		}
		b.MasterServiceID = svcID
		out = append(out, b)
	}
	return out, rows.Err()
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
		return domain.ErrNotFound
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

// ListClientsByMaster aggregates master_bookings into one row per distinct
// client_user_id. "Visited" bookings — those that generated income and count
// toward segments/LTV — are completed rows plus confirmed rows whose date has
// already passed (mirrors HasCompletedBookingByClientMaster's definition).
// The visited predicate uses only constant literals, so no user input is
// interpolated; masterID is bound as $1.
func (r *MasterRepo) ListClientsByMaster(ctx context.Context, masterID uuid.UUID) ([]domain.MasterClient, error) {
	const visited = `(status = 'completed' OR (status = 'confirmed' AND date < CURRENT_DATE))`
	q := `
SELECT
  client_user_id,
  (COUNT(*))::int AS bookings_count,
  (COUNT(*) FILTER (WHERE ` + visited + `))::int AS visits_count,
  COALESCE(SUM(total_price) FILTER (WHERE ` + visited + `), 0)::bigint AS total_spent,
  MIN(date) FILTER (WHERE ` + visited + `) AS first_visit,
  MAX(date) FILTER (WHERE ` + visited + `) AS last_visit,
  MAX(created_at) AS last_booking
FROM master_bookings
WHERE master_id = $1
GROUP BY client_user_id`
	rows, err := r.pool.Query(ctx, q, masterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MasterClient
	for rows.Next() {
		var c domain.MasterClient
		var firstVisit, lastVisit, lastBooking *time.Time
		if err := rows.Scan(&c.UserID, &c.BookingsCount, &c.VisitsCount, &c.TotalSpent, &firstVisit, &lastVisit, &lastBooking); err != nil {
			return nil, err
		}
		c.FirstVisitAt = firstVisit
		c.LastVisitAt = lastVisit
		c.LastBookingAt = lastBooking
		out = append(out, c)
	}
	return out, rows.Err()
}

// InsertSlotBlock persists a schedule block. Empty TimeFrom/TimeTo are stored as
// SQL NULL (whole-day block); the CHECK constraint enforces the invariant.
func (r *MasterRepo) InsertSlotBlock(ctx context.Context, b *domain.MasterSlotBlock) error {
	const q = `
INSERT INTO master_slot_blocks (id, master_id, date, time_from, time_to, note)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING created_at`
	return r.pool.QueryRow(ctx, q,
		b.ID, b.MasterID, b.Date, nullableTime(b.TimeFrom), nullableTime(b.TimeTo), b.Note,
	).Scan(&b.CreatedAt)
}

// ListSlotBlocksByMaster returns upcoming blocks (date >= today) ordered by date,
// then whole-day blocks (NULL time_from) before timed ones.
func (r *MasterRepo) ListSlotBlocksByMaster(ctx context.Context, masterID uuid.UUID) ([]domain.MasterSlotBlock, error) {
	const q = `
SELECT id, master_id, date::text,
       COALESCE(to_char(time_from, 'HH24:MI'), ''),
       COALESCE(to_char(time_to,   'HH24:MI'), ''),
       note, created_at
FROM master_slot_blocks
WHERE master_id = $1 AND date >= CURRENT_DATE
ORDER BY date, time_from NULLS FIRST`
	rows, err := r.pool.Query(ctx, q, masterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MasterSlotBlock
	for rows.Next() {
		var b domain.MasterSlotBlock
		if err := rows.Scan(&b.ID, &b.MasterID, &b.Date, &b.TimeFrom, &b.TimeTo, &b.Note, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// DeleteSlotBlock removes a block scoped to its owning master. The master_id
// predicate is the ownership check — a block belonging to another master is
// indistinguishable from a missing one (both yield ErrNotFound).
func (r *MasterRepo) DeleteSlotBlock(ctx context.Context, masterID, blockID uuid.UUID) error {
	const q = `DELETE FROM master_slot_blocks WHERE id = $1 AND master_id = $2`
	tag, err := r.pool.Exec(ctx, q, blockID, masterID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// nullableTime maps an empty "HH:MM" string to SQL NULL (whole-day block) and a
// non-empty one to the string itself (pgx casts to TIME).
func nullableTime(hhmm string) any {
	if hhmm == "" {
		return nil
	}
	return hhmm
}

// HasCompletedBookingByClientMaster returns true when the client has at least
// one booking with the master that is considered "completed":
//
//   - status = 'completed' — explicit terminal status (set by a future
//     confirmed→completed cron-job or admin action).
//   - status = 'confirmed' AND (date || ' ' || time_to)::timestamp < now() —
//     the appointment time has already passed but the row has not yet been
//     transitioned by the cron-job. This implicit path keeps reviews working
//     end-to-end before the cron-job is implemented (migration 020 adds
//     'completed' to the CHECK constraint so explicit transitions can land too).
//
// Moscow time is not used here: date and time_to are stored as local wall-clock
// values without a timezone, so the comparison is done in the DB server's local
// clock. The master booking window guard (validateBookingSlot) ensures dates are
// always in the near future, so the implicit path fires within hours of the slot.
func (r *MasterRepo) HasCompletedBookingByClientMaster(ctx context.Context, clientUserID, masterID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM master_bookings
  WHERE client_user_id = $1
    AND master_id      = $2
    AND (
      status = 'completed'
      OR (
        status = 'confirmed'
        AND (date + time_to) < now()
      )
    )
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
		return domain.ErrNotFound
	}
	return nil
}

// ConfirmBookingByPayment atomically transitions a master booking from
// payment_pending → confirmed, associating paymentID in the same statement.
//
// The WHERE clause guards against two concurrent NATS deliveries of the same
// payment.completed event:
//
//	AND status = 'payment_pending'      — only one winner; the other sees 0 rows
//	AND (payment_id = '' OR payment_id = $2) — reject a different payment's event
//
// Return values:
//
//	nil          — transition applied (or booking was already confirmed — idempotent)
//	ErrNoRows    — booking not found at all
//	domain.ErrPaymentMismatch — booking belongs to a different payment_id
//	domain.ErrBookingNotPending — booking is in a terminal/unexpected status
func (r *MasterRepo) ConfirmBookingByPayment(ctx context.Context, bookingID uuid.UUID, paymentID string) error {
	paymentID = strings.TrimSpace(paymentID)
	ct, err := r.pool.Exec(ctx, `
UPDATE master_bookings
SET    status     = 'confirmed',
       payment_id = $2,
       updated_at = now()
WHERE  id         = $1
  AND  status     = 'payment_pending'
  AND  (payment_id = '' OR payment_id = $2)`,
		bookingID, paymentID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 1 {
		return nil // fast path — transition applied
	}

	// 0 rows: need to distinguish between the cases.
	return r.classifyBookingPaymentNoOp(ctx, bookingID, paymentID, "confirmed")
}

// CancelBookingByPayment atomically transitions a master booking from
// payment_pending → cancelled. Same concurrency contract as ConfirmBookingByPayment.
func (r *MasterRepo) CancelBookingByPayment(ctx context.Context, bookingID uuid.UUID, paymentID string) error {
	paymentID = strings.TrimSpace(paymentID)
	ct, err := r.pool.Exec(ctx, `
UPDATE master_bookings
SET    status     = 'cancelled',
       payment_id = $2,
       updated_at = now()
WHERE  id         = $1
  AND  status     = 'payment_pending'
  AND  (payment_id = '' OR payment_id = $2)`,
		bookingID, paymentID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 1 {
		return nil
	}

	return r.classifyBookingPaymentNoOp(ctx, bookingID, paymentID, "cancelled")
}

// classifyBookingPaymentNoOp is called when an atomic payment UPDATE touched 0
// rows. It reads the current row to return a meaningful error:
//
//   - row not found → domain.ErrNotFound
//   - already in targetStatus → nil (idempotent re-delivery)
//   - payment_id mismatch → domain.ErrPaymentMismatch
//   - wrong status (e.g. already cancelled when confirming) → domain.ErrBookingNotPending
func (r *MasterRepo) classifyBookingPaymentNoOp(ctx context.Context, bookingID uuid.UUID, paymentID, targetStatus string) error {
	var curStatus, curPaymentID string
	err := r.pool.QueryRow(ctx,
		`SELECT status, payment_id FROM master_bookings WHERE id = $1`,
		bookingID,
	).Scan(&curStatus, &curPaymentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	// Idempotent re-delivery: already in the desired terminal state.
	if curStatus == targetStatus {
		return nil
	}
	// A different payment's event arrived for this booking.
	// Use exact equality to match the SQL WHERE clause (payment_id = $2),
	// which is also case-sensitive. EqualFold would diverge: an event with a
	// differently-cased ID would pass the classify check but had already failed
	// the UPDATE, producing ErrBookingNotPending instead of ErrPaymentMismatch.
	if curPaymentID != "" && curPaymentID != paymentID {
		return domain.ErrPaymentMismatch
	}
	// Booking is in a state we cannot transition from (e.g. already cancelled
	// when payment.completed arrives late, or already confirmed when
	// payment.failed arrives).
	//
	// Wrap with the observed status so that the subscriber's Warn log
	// (which prints err) captures it. This is valuable for post-hoc debugging
	// of the UPDATE→SELECT race window: the UPDATE touched 0 rows, then a
	// concurrent transition may have changed the status before SELECT ran.
	// errors.Is(err, domain.ErrBookingNotPending) still returns true because
	// %w preserves the sentinel for switch/errors.Is matching in the caller.
	return fmt.Errorf("%w: found status=%q", domain.ErrBookingNotPending, curStatus)
}

// NewSlug builds a unique URL slug candidate from displayName.
//
// The function never touches the database — it only produces a candidate string.
// Uniqueness is enforced by the DB UNIQUE constraint on masters.slug; if Insert
// returns domain.ErrSlugConflict the usecase calls NewSlug again for a fresh
// candidate and retries the insert (see usecase.CreateMyProfile).
//
// The suffix is 4 bytes from crypto/rand encoded as 8 lower-hex characters.
// This gives 2³² ≈ 4 billion distinct suffixes per base slug, making collisions
// astronomically unlikely in any realistic deployment.
func (r *MasterRepo) NewSlug(_ context.Context, displayName string, userID uuid.UUID) (string, error) {
	return newSlugCandidate(displayName, userID)
}

func newSlugCandidate(displayName string, userID uuid.UUID) (string, error) {
	base := slug.Make(strings.TrimSpace(displayName))
	if base == "" {
		// displayName produced no slug-safe characters (emoji-only, whitespace,
		// or all stop chars). Fall back to the first 8 hex chars of the userID
		// so each master gets a unique, user-specific base rather than the
		// shared "master" prefix that would collide for every such profile.
		// userIDStr is a standard UUID: xxxxxxxx-xxxx-...; strip dashes to get
		// pure hex then take the first 8 characters (32-bit entropy from UUID v4).
		uid := strings.ReplaceAll(userID.String(), "-", "")
		base = uid[:8]
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("slug: crypto/rand read: %w", err)
	}
	return base + "-" + hex.EncodeToString(b[:]), nil
}
