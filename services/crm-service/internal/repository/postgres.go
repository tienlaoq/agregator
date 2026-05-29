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
)

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
	q := `
		SELECT id, venue_id, booking_id, title, body, status, assignee_user_id, created_by, created_at, updated_at
		FROM venue_crm_tasks WHERE venue_id = $1`
	args := []any{venueID}
	if status != "" {
		q += ` AND status = $2`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	var out []domain.Task
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(
			&t.ID, &t.VenueID, &t.BookingID, &t.Title, &t.Body, &t.Status,
			&t.AssigneeUserID, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *crmRepo) CreateTask(ctx context.Context, t *domain.Task) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO venue_crm_tasks (venue_id, booking_id, title, body, status, assignee_user_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`,
		t.VenueID, t.BookingID, t.Title, t.Body, t.Status, t.AssigneeUserID, t.CreatedBy,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	return nil
}

func (r *crmRepo) CompleteTask(ctx context.Context, venueID, taskID uuid.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE venue_crm_tasks SET status = 'done', updated_at = now()
		WHERE id = $1 AND venue_id = $2 AND status = 'open'`,
		taskID, venueID)
	if err != nil {
		return false, fmt.Errorf("complete task: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
