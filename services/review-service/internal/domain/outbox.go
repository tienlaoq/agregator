package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// OutboxEntry is a row in outbox_events. The worker reads pending rows,
// publishes them to NATS, and marks them published.
type OutboxEntry struct {
	ID          string
	AggregateID string
	EventType   string
	// Payload is the raw JSON body of the event, stored as JSONB in Postgres.
	// pgx v5 scans JSONB → []byte (text representation) and writes []byte → JSONB
	// correctly; json.RawMessage is intentional — avoid pgtype.JSONB here to keep
	// the domain layer free of driver-specific types.
	// Postgres validates JSON on INSERT, so a malformed payload never reaches the worker.
	Payload     json.RawMessage
	CreatedAt   time.Time
	PublishedAt *time.Time
	// LockedAt is set by the worker when it claims a batch (phase 1).
	// It is cleared either when the row is marked published or by the stale
	// lock sweeper after StaleLockThreshold has elapsed without a publish.
	LockedAt *time.Time
}

// StaleLockThreshold is the maximum time a worker may hold a lock before
// another worker or the sweeper reclaims the row.
const StaleLockThreshold = 2 * time.Minute

// OutboxRepository defines the persistence operations for the outbox table.
// CreateTx writes a single entry within a caller-managed transaction so that
// the review insert and the outbox insert share the same atomic commit.
//
// The worker follows a three-phase pattern to avoid holding a Postgres row-lock
// while doing NATS I/O:
//
//  1. ClaimBatch — short tx: SELECT … FOR UPDATE SKIP LOCKED + SET locked_at = now() + COMMIT.
//     Row-lock is released immediately; locked_at prevents other workers from
//     picking the same rows.
//  2. Publish outside any transaction (network I/O).
//  3. MarkPublished — short tx: SET published_at = now() + COMMIT.
//
// ResetStaleLocks is called periodically to reclaim rows whose worker crashed
// or timed out before completing phase 3.
type OutboxRepository interface {
	// CreateTx inserts an outbox entry within the provided transaction.
	CreateTx(ctx context.Context, tx pgx.Tx, entry *OutboxEntry) error

	// BeginTx starts a transaction for the claim or mark phases.
	BeginTx(ctx context.Context) (pgx.Tx, error)

	// ClaimBatch selects up to limit rows that are unpublished and either
	// unlocked or have a stale lock, marks them locked_at = now() within tx,
	// and returns them. The caller must Commit (or Rollback) tx.
	ClaimBatch(ctx context.Context, tx pgx.Tx, limit int) ([]*OutboxEntry, error)

	// MarkPublished sets published_at = now() and clears locked_at for the
	// given IDs. It opens and commits its own transaction internally.
	MarkPublished(ctx context.Context, ids []string) error

	// ResetStaleLocks clears locked_at for rows that have been locked longer
	// than StaleLockThreshold and are still unpublished.
	ResetStaleLocks(ctx context.Context) error
}
