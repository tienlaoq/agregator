package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tienlao/agregator/services/review-service/internal/domain"
)

// OutboxRepo implements domain.OutboxRepository using pgx.
type OutboxRepo struct {
	pool *pgxpool.Pool
}

func NewOutboxRepo(pool *pgxpool.Pool) *OutboxRepo {
	return &OutboxRepo{pool: pool}
}

// CreateTx inserts one outbox entry inside the provided transaction.
func (r *OutboxRepo) CreateTx(ctx context.Context, tx pgx.Tx, entry *domain.OutboxEntry) error {
	const q = `
		INSERT INTO outbox_events (aggregate_id, event_type, payload)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`
	return tx.QueryRow(ctx, q, entry.AggregateID, entry.EventType, entry.Payload).
		Scan(&entry.ID, &entry.CreatedAt)
}

// BeginTx starts a transaction for the claim or mark phases.
func (r *OutboxRepo) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

// ClaimBatch selects up to limit rows that are unpublished and either
// unlocked or have a stale lock, stamps locked_at = now() on them within tx,
// and returns them. The caller is responsible for Commit / Rollback.
//
// Using FOR UPDATE SKIP LOCKED + immediate locked_at stamp means the
// Postgres row-lock is held only for the duration of this short transaction,
// not for the entire NATS publish cycle.
func (r *OutboxRepo) ClaimBatch(ctx context.Context, tx pgx.Tx, limit int) ([]*domain.OutboxEntry, error) {
	const selectQ = `
		SELECT id, aggregate_id, event_type, payload, created_at
		FROM outbox_events
		WHERE published_at IS NULL
		  AND (locked_at IS NULL OR locked_at < now() - $2::interval)
		ORDER BY created_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	rows, err := tx.Query(ctx, selectQ, limit, domain.StaleLockThreshold.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*domain.OutboxEntry
	for rows.Next() {
		e := &domain.OutboxEntry{}
		if err := rows.Scan(&e.ID, &e.AggregateID, &e.EventType, &e.Payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	// Stamp locked_at so other workers skip these rows after we COMMIT
	// and release the FOR UPDATE row-lock.
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	_, err = tx.Exec(ctx,
		`UPDATE outbox_events SET locked_at = now() WHERE id = ANY($1)`,
		ids,
	)
	return entries, err
}

// MarkPublished sets published_at = now() and clears locked_at for the given
// IDs. It opens and commits its own short transaction internally so the caller
// does not need to manage one.
func (r *OutboxRepo) MarkPublished(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx,
		`UPDATE outbox_events
		    SET published_at = now(), locked_at = NULL
		  WHERE id = ANY($1)`,
		ids,
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ResetStaleLocks clears locked_at for rows that have been locked longer than
// StaleLockThreshold and are still unpublished. This reclaims batches whose
// worker crashed or timed out before completing the publish + mark phase.
func (r *OutboxRepo) ResetStaleLocks(ctx context.Context) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE outbox_events
		    SET locked_at = NULL
		  WHERE published_at IS NULL
		    AND locked_at < now() - $1::interval`,
		domain.StaleLockThreshold.String(),
	)
	return err
}
