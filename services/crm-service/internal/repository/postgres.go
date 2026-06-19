package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tienlao/agregator/services/crm-service/internal/domain"
)

// Sentinel errors returned by repository methods. They are mapped to
// domain-meaningful gRPC codes in the delivery layer.
var (
	ErrStaffNotFound = errors.New("crm: staff not found")
	ErrVenueNotFound = errors.New("crm: venue not found")
	ErrTaskNotFound  = errors.New("crm: task not found")
	ErrGuestNotFound = errors.New("crm: guest profile not found")
)

// guestProfileCols is the canonical select column list for crm_guest_profiles,
// kept in sync with scanGuestProfile.
const guestProfileCols = `venue_id, user_id, bookings_count, visits_count,
	cancellations_count, no_show_count, total_spent, first_visit_at, last_visit_at, last_booking_at`

func scanGuestProfile(row pgx.Row, p *domain.GuestProfile) error {
	return row.Scan(
		&p.VenueID, &p.UserID, &p.BookingsCount, &p.VisitsCount,
		&p.CancellationsCount, &p.NoShowCount, &p.TotalSpent,
		&p.FirstVisitAt, &p.LastVisitAt, &p.LastBookingAt,
	)
}

// taskColumns is the canonical select/returning column list for venue_crm_tasks,
// kept in sync with scanTask so every task read maps the same shape.
const taskColumns = `id, venue_id, booking_id, title, body, status, priority,
	assignee_user_id, due_at, created_by, completed_by, completed_at, created_at, updated_at`

// scanTask reads one venue_crm_tasks row (in taskColumns order) into t. Accepts
// both pgx.Row (QueryRow) and pgx.Rows, which share the Scan signature.
func scanTask(row pgx.Row, t *domain.Task) error {
	return row.Scan(
		&t.ID, &t.VenueID, &t.BookingID, &t.Title, &t.Body, &t.Status, &t.Priority,
		&t.AssigneeUserID, &t.DueAt, &t.CreatedBy, &t.CompletedBy, &t.CompletedAt,
		&t.CreatedAt, &t.UpdatedAt,
	)
}

type crmRepo struct {
	pool *pgxpool.Pool
}

// New returns a Repository backed by the given pgx pool.
func New(pool *pgxpool.Pool) domain.Repository {
	return &crmRepo{pool: pool}
}

func (r *crmRepo) VenueOwnerID(ctx context.Context, venueID uuid.UUID) (uuid.UUID, error) {
	var owner uuid.UUID
	err := r.pool.QueryRow(ctx,
		`SELECT owner_id FROM venues WHERE id = $1`, venueID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrVenueNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("get venue owner: %w", err)
	}
	return owner, nil
}

func (r *crmRepo) GetManagementAccess(ctx context.Context, venueID, userID uuid.UUID) (string, error) {
	var access string
	err := r.pool.QueryRow(ctx, `
		SELECT CASE WHEN v.owner_id = $2 THEN 'owner' ELSE vs.role END
		FROM venues v
		LEFT JOIN venue_staff vs ON vs.venue_id = v.id AND vs.user_id = $2
		WHERE v.id = $1 AND (v.owner_id = $2 OR vs.user_id IS NOT NULL)`,
		venueID, userID,
	).Scan(&access)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get management access: %w", err)
	}
	return access, nil
}

func (r *crmRepo) BatchGetManagementAccess(ctx context.Context, userID uuid.UUID, venueIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string, len(venueIDs))
	if len(venueIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT v.id,
			CASE WHEN v.owner_id = $1 THEN 'owner' ELSE vs.role END AS access
		FROM venues v
		LEFT JOIN venue_staff vs ON vs.venue_id = v.id AND vs.user_id = $1
		WHERE v.id = ANY($2) AND (v.owner_id = $1 OR vs.user_id IS NOT NULL)`,
		userID, venueIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("batch get management access: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var access string
		if err := rows.Scan(&id, &access); err != nil {
			return nil, fmt.Errorf("scan batch access: %w", err)
		}
		out[id] = access
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *crmRepo) ListManagedVenues(ctx context.Context, userID uuid.UUID) ([]domain.ManagedVenue, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT v.id,
			CASE WHEN v.owner_id = $1 THEN 'owner' ELSE vs.role END AS access
		FROM venues v
		LEFT JOIN venue_staff vs ON vs.venue_id = v.id AND vs.user_id = $1
		WHERE v.owner_id = $1 OR vs.user_id IS NOT NULL
		ORDER BY v.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list managed venues: %w", err)
	}
	defer rows.Close()
	var out []domain.ManagedVenue
	for rows.Next() {
		var mv domain.ManagedVenue
		if err := rows.Scan(&mv.VenueID, &mv.Access); err != nil {
			return nil, fmt.Errorf("scan managed venue: %w", err)
		}
		out = append(out, mv)
	}
	return out, rows.Err()
}

func (r *crmRepo) ListStaff(ctx context.Context, venueID uuid.UUID) ([]domain.StaffMember, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT venue_id, user_id, role, invited_by, created_at
		FROM venue_staff WHERE venue_id = $1 ORDER BY created_at ASC`, venueID)
	if err != nil {
		return nil, fmt.Errorf("list staff: %w", err)
	}
	defer rows.Close()
	var out []domain.StaffMember
	for rows.Next() {
		var s domain.StaffMember
		if err := rows.Scan(&s.VenueID, &s.UserID, &s.Role, &s.InvitedBy, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan staff: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *crmRepo) AddStaff(ctx context.Context, venueID, userID uuid.UUID, role string, invitedBy uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO venue_staff (venue_id, user_id, role, invited_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (venue_id, user_id) DO UPDATE SET role = EXCLUDED.role, invited_by = EXCLUDED.invited_by`,
		venueID, userID, role, invitedBy,
	)
	if err != nil {
		return fmt.Errorf("add staff: %w", err)
	}
	return nil
}

func (r *crmRepo) RemoveStaff(ctx context.Context, venueID, userID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM venue_staff WHERE venue_id = $1 AND user_id = $2`, venueID, userID)
	if err != nil {
		return fmt.Errorf("remove staff: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrStaffNotFound
	}
	return nil
}

func (r *crmRepo) ListTasks(ctx context.Context, venueID uuid.UUID, status string) ([]domain.Task, error) {
	q := `SELECT ` + taskColumns + ` FROM venue_crm_tasks WHERE venue_id = $1`
	args := []any{venueID}
	if status != "" {
		q += ` AND status = $2`
		args = append(args, status)
	}
	// Open tasks first, soonest deadline first (NULLs last), then newest.
	q += ` ORDER BY (status = 'open') DESC, due_at ASC NULLS LAST, created_at DESC`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	var out []domain.Task
	for rows.Next() {
		var t domain.Task
		if err := scanTask(rows, &t); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *crmRepo) GetTask(ctx context.Context, venueID, taskID uuid.UUID) (*domain.Task, error) {
	var t domain.Task
	err := scanTask(r.pool.QueryRow(ctx,
		`SELECT `+taskColumns+` FROM venue_crm_tasks WHERE id = $1 AND venue_id = $2`,
		taskID, venueID), &t)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	return &t, nil
}

func (r *crmRepo) CreateTask(ctx context.Context, t *domain.Task) error {
	err := scanTask(r.pool.QueryRow(ctx, `
		INSERT INTO venue_crm_tasks (venue_id, booking_id, title, body, status, priority, assignee_user_id, due_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+taskColumns,
		t.VenueID, t.BookingID, t.Title, t.Body, t.Status, t.Priority, t.AssigneeUserID, t.DueAt, t.CreatedBy,
	), t)
	if err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	return nil
}

func (r *crmRepo) UpdateTask(ctx context.Context, t *domain.Task) error {
	err := scanTask(r.pool.QueryRow(ctx, `
		UPDATE venue_crm_tasks
		SET title = $3, body = $4, priority = $5, assignee_user_id = $6, due_at = $7, updated_at = now()
		WHERE id = $1 AND venue_id = $2 AND status <> 'cancelled'
		RETURNING `+taskColumns,
		t.ID, t.VenueID, t.Title, t.Body, t.Priority, t.AssigneeUserID, t.DueAt,
	), t)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTaskNotFound
	}
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return nil
}

func (r *crmRepo) CompleteTask(ctx context.Context, venueID, taskID, completedBy uuid.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE venue_crm_tasks
		SET status = 'done', completed_by = $3, completed_at = now(), updated_at = now()
		WHERE id = $1 AND venue_id = $2 AND status = 'open'`,
		taskID, venueID, completedBy)
	if err != nil {
		return false, fmt.Errorf("complete task: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *crmRepo) ReopenTask(ctx context.Context, venueID, taskID uuid.UUID) (*domain.Task, error) {
	var t domain.Task
	err := scanTask(r.pool.QueryRow(ctx, `
		UPDATE venue_crm_tasks
		SET status = 'open', completed_by = NULL, completed_at = NULL, updated_at = now()
		WHERE id = $1 AND venue_id = $2 AND status = 'done'
		RETURNING `+taskColumns,
		taskID, venueID), &t)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reopen task: %w", err)
	}
	return &t, nil
}

func (r *crmRepo) CancelTask(ctx context.Context, venueID, taskID uuid.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE venue_crm_tasks SET status = 'cancelled', updated_at = now()
		WHERE id = $1 AND venue_id = $2 AND status <> 'cancelled'`,
		taskID, venueID)
	if err != nil {
		return false, fmt.Errorf("cancel task: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ApplyBookingFact upserts the booking fact and recomputes the guest profile in
// one transaction. The fact upsert is guarded by status_rank so re-delivered or
// out-of-order events never regress the booking's state; the profile is then a
// deterministic aggregate of the facts (correct under at-least-once delivery).
func (r *crmRepo) ApplyBookingFact(ctx context.Context, f *domain.BookingFact) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO crm_guest_booking_facts
			(booking_id, venue_id, user_id, status, status_rank, total_price, visit_date, guests)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (booking_id) DO UPDATE SET
			status      = EXCLUDED.status,
			status_rank = EXCLUDED.status_rank,
			total_price = EXCLUDED.total_price,
			visit_date  = EXCLUDED.visit_date,
			guests      = EXCLUDED.guests,
			updated_at  = now()
		WHERE EXCLUDED.status_rank >= crm_guest_booking_facts.status_rank`,
		f.BookingID, f.VenueID, f.UserID, f.Status, f.StatusRank, f.TotalPrice, f.VisitDate, f.Guests,
	); err != nil {
		return fmt.Errorf("upsert booking fact: %w", err)
	}

	// Recompute the profile from all facts for this guest. no_show_count is left
	// untouched (reserved until booking.no_show exists).
	if _, err := tx.Exec(ctx, `
		INSERT INTO crm_guest_profiles
			(venue_id, user_id, bookings_count, visits_count, cancellations_count,
			 total_spent, first_visit_at, last_visit_at, last_booking_at, updated_at)
		SELECT $1, $2,
			count(*),
			count(*) FILTER (WHERE status = 'completed'),
			count(*) FILTER (WHERE status = 'cancelled'),
			COALESCE(sum(total_price) FILTER (WHERE status = 'completed'), 0),
			min(visit_date) FILTER (WHERE status = 'completed'),
			max(visit_date) FILTER (WHERE status = 'completed'),
			max(created_at),
			now()
		FROM crm_guest_booking_facts
		WHERE venue_id = $1 AND user_id = $2
		ON CONFLICT (venue_id, user_id) DO UPDATE SET
			bookings_count      = EXCLUDED.bookings_count,
			visits_count        = EXCLUDED.visits_count,
			cancellations_count = EXCLUDED.cancellations_count,
			total_spent         = EXCLUDED.total_spent,
			first_visit_at      = EXCLUDED.first_visit_at,
			last_visit_at       = EXCLUDED.last_visit_at,
			last_booking_at     = EXCLUDED.last_booking_at,
			updated_at          = now()`,
		f.VenueID, f.UserID,
	); err != nil {
		return fmt.Errorf("recompute guest profile: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *crmRepo) ListGuests(ctx context.Context, venueID uuid.UUID, params domain.GuestListParams) ([]domain.GuestProfile, int, error) {
	args := []any{venueID}
	where := "venue_id = $1"
	// Segment values are an allowlist (validated in usecase); only the VIP
	// threshold is a bound parameter — no value interpolation.
	switch params.Segment {
	case domain.SegmentNew:
		where += " AND bookings_count <= 1"
	case domain.SegmentRegular:
		where += " AND visits_count >= 3"
	case domain.SegmentVIP:
		args = append(args, params.VIPThreshold)
		where += fmt.Sprintf(" AND total_spent >= $%d", len(args))
	case domain.SegmentAtRisk:
		where += " AND visits_count >= 2 AND last_visit_at < (now() - interval '90 days')"
	}

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM crm_guest_profiles WHERE `+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count guests: %w", err)
	}

	orderBy := "last_visit_at DESC NULLS LAST, last_booking_at DESC NULLS LAST"
	switch params.Sort {
	case "ltv":
		orderBy = "total_spent DESC"
	case "visits":
		orderBy = "visits_count DESC"
	}

	limit := params.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	q := `SELECT ` + guestProfileCols + ` FROM crm_guest_profiles WHERE ` + where +
		` ORDER BY ` + orderBy + fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list guests: %w", err)
	}
	defer rows.Close()
	var out []domain.GuestProfile
	for rows.Next() {
		var p domain.GuestProfile
		if err := scanGuestProfile(rows, &p); err != nil {
			return nil, 0, fmt.Errorf("scan guest: %w", err)
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

func (r *crmRepo) GetGuestProfile(ctx context.Context, venueID, userID uuid.UUID) (*domain.GuestProfile, error) {
	var p domain.GuestProfile
	err := scanGuestProfile(r.pool.QueryRow(ctx,
		`SELECT `+guestProfileCols+` FROM crm_guest_profiles WHERE venue_id = $1 AND user_id = $2`,
		venueID, userID), &p)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGuestNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get guest profile: %w", err)
	}
	return &p, nil
}

func (r *crmRepo) ListGuestBookings(ctx context.Context, venueID, userID uuid.UUID, limit int) ([]domain.GuestBookingSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT booking_id, status, total_price, visit_date, guests
		FROM crm_guest_booking_facts
		WHERE venue_id = $1 AND user_id = $2
		ORDER BY visit_date DESC NULLS LAST, updated_at DESC
		LIMIT $3`, venueID, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list guest bookings: %w", err)
	}
	defer rows.Close()
	var out []domain.GuestBookingSummary
	for rows.Next() {
		var s domain.GuestBookingSummary
		if err := rows.Scan(&s.BookingID, &s.Status, &s.TotalPrice, &s.VisitDate, &s.Guests); err != nil {
			return nil, fmt.Errorf("scan guest booking: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
