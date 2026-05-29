package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

// OutboxRepo implements domain.OutboxRepository using PostgreSQL.
type OutboxRepo struct {
	pool *pgxpool.Pool
}

func NewOutboxRepo(pool *pgxpool.Pool) *OutboxRepo {
	return &OutboxRepo{pool: pool}
}

// AppendTx inserts an outbox row inside the given transaction.
// tx must be a pgx.Tx — it is passed as any to avoid a pgx import in domain.
func (r *OutboxRepo) AppendTx(ctx context.Context, tx any, event *domain.OutboxEvent) error {
	pgxTx, ok := tx.(pgx.Tx)
	if !ok {
		return fmt.Errorf("outbox AppendTx: tx must be pgx.Tx, got %T", tx)
	}
	const q = `
		INSERT INTO payment_outbox (subject, payload, payment_id)
		VALUES ($1, $2, $3)`
	_, err := pgxTx.Exec(ctx, q, string(event.Subject), event.Payload, event.PaymentID)
	return err
}

// RelayBatch selects up to limit pending rows with FOR UPDATE SKIP LOCKED,
// calls publish for each one, marks sent or failed, then commits — all inside
// a single transaction so that row locks are held for the minimum required
// duration and two pods never process the same row concurrently.
//
// See domain.OutboxRepository.RelayBatch for the full contract.
func (r *OutboxRepo) RelayBatch(ctx context.Context, limit int, publish func(*domain.OutboxEvent) error) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("outbox RelayBatch: begin tx: %w", err)
	}
	// Always rollback on early return; a committed tx makes Rollback a no-op.
	defer func() { _ = tx.Rollback(ctx) }()

	const selectQ = `
		SELECT id, subject, payload, payment_id, created_at,
		       sent_at, attempt_count, failed_at, COALESCE(last_error, '')
		  FROM payment_outbox
		 WHERE sent_at IS NULL AND failed_at IS NULL
		 ORDER BY id
		 LIMIT $1
		   FOR UPDATE SKIP LOCKED`
	rows, err := tx.Query(ctx, selectQ, limit)
	if err != nil {
		return 0, fmt.Errorf("outbox RelayBatch: select: %w", err)
	}

	var batch []*domain.OutboxEvent
	for rows.Next() {
		e := &domain.OutboxEvent{}
		if err := rows.Scan(
			&e.ID, &e.Subject, &e.Payload, &e.PaymentID, &e.CreatedAt,
			&e.SentAt, &e.AttemptCount, &e.FailedAt, &e.LastError,
		); err != nil {
			rows.Close()
			return 0, fmt.Errorf("outbox RelayBatch: scan: %w", err)
		}
		batch = append(batch, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("outbox RelayBatch: rows: %w", err)
	}

	for _, e := range batch {
		if publishErr := publish(e); publishErr != nil {
			// Truncate error message to avoid unbounded text in the DB.
			msg := publishErr.Error()
			if len(msg) > 500 {
				msg = msg[:500]
			}
			const failQ = `
				UPDATE payment_outbox
				   SET attempt_count = attempt_count + 1,
				       last_error     = $2,
				       failed_at      = CASE WHEN attempt_count + 1 >= $3 THEN now() ELSE NULL END
				 WHERE id = $1`
			if _, err := tx.Exec(ctx, failQ, e.ID, msg, outboxMaxAttempts); err != nil {
				// Best-effort: log-worthy but don't abort the batch for other rows.
				_ = err // caller's publish logging handles the original error
			}
		} else {
			const sentQ = `UPDATE payment_outbox SET sent_at = now() WHERE id = $1`
			if _, err := tx.Exec(ctx, sentQ, e.ID); err != nil {
				// Best-effort: the NATS publish already landed; a duplicate will
				// be re-sent on the next tick (SKIP LOCKED won't see it until
				// this tx commits, at which point sent_at is still NULL).
				// Don't abort the batch for other rows.
				_ = err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("outbox RelayBatch: commit: %w", err)
	}
	return len(batch), nil
}

// MarkFailed increments attempt_count and stores the error message.
// When attempt_count reaches maxAttempts it also sets failed_at = now()
// so the row is excluded from future polls.
const outboxMaxAttempts = 10

func (r *OutboxRepo) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	// Truncate to avoid unbounded text in the DB.
	if len(errMsg) > 500 {
		errMsg = errMsg[:500]
	}
	const q = `
		UPDATE payment_outbox
		   SET attempt_count = attempt_count + 1,
		       last_error     = $2,
		       failed_at      = CASE WHEN attempt_count + 1 >= $3 THEN now() ELSE NULL END
		 WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id, errMsg, outboxMaxAttempts)
	return err
}

// ── UpdateStatusWithOutbox ────────────────────────────────────────────────────

// UpdateStatusWithOutbox runs UPDATE payments + INSERT payment_outbox in a
// single transaction, guaranteeing that either both writes land or neither does.
//
// It is implemented on PaymentRepo (not OutboxRepo) because it needs to UPDATE
// the payments table; the outbox insert is delegated to OutboxRepo.AppendTx.
func (r *PaymentRepo) UpdateStatusWithOutbox(
	ctx context.Context,
	id string,
	newStatus domain.PaymentStatus,
	providerID string,
	event *domain.OutboxEvent,
) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	const q = `
		UPDATE payments
		   SET status = $2, provider_id = $3, updated_at = now()
		 WHERE id = $1
		   AND status NOT IN ('succeeded', 'cancelled', 'refunded')`
	ct, err := tx.Exec(ctx, q, id, newStatus.String(), providerID)
	if err != nil {
		return false, err
	}
	if ct.RowsAffected() == 0 {
		// Already terminal or not found — no event to write, commit empty tx.
		err = tx.Commit(ctx)
		return false, err
	}

	// Insert the outbox row in the same transaction.
	const qOutbox = `
		INSERT INTO payment_outbox (subject, payload, payment_id)
		VALUES ($1, $2, $3)`
	if _, err = tx.Exec(ctx, qOutbox, string(event.Subject), event.Payload, event.PaymentID); err != nil {
		return false, err
	}

	if err = tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit tx: %w", err)
	}
	return true, nil
}

// Compile-time check: OutboxRepo satisfies domain.OutboxRepository.
var _ domain.OutboxRepository = (*OutboxRepo)(nil)
