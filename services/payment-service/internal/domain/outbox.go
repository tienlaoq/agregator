package domain

import (
	"context"
	"time"
)

// OutboxSubject is the NATS subject for a payment domain event.
type OutboxSubject string

const (
	SubjectPaymentCompleted OutboxSubject = "payment.completed"
	SubjectPaymentFailed    OutboxSubject = "payment.failed"
	SubjectPaymentRefunded  OutboxSubject = "payment.refunded"
)

// OutboxEvent represents a pending message in the payment_outbox table.
// It is written atomically with the corresponding payments UPDATE and read
// by the outbox worker, which publishes it to NATS and marks it sent.
type OutboxEvent struct {
	ID           int64
	Subject      OutboxSubject
	Payload      []byte // JSON; same shape as events.paymentEvent
	PaymentID    string
	CreatedAt    time.Time
	SentAt       *time.Time
	AttemptCount int
	FailedAt     *time.Time
	LastError    string
}

// OutboxRepository provides atomic write + reliable read for the outbox table.
type OutboxRepository interface {
	// RelayBatch selects up to limit pending rows with FOR UPDATE SKIP LOCKED
	// inside a single transaction, calls publish for each row, marks it sent or
	// failed based on the error returned by publish, then commits.
	//
	// FOR UPDATE SKIP LOCKED means that rows already held by another worker pod
	// are skipped rather than waited on, so two pods never publish the same row
	// concurrently.  Each row's lock is released when the transaction commits at
	// the end of the batch — not row-by-row.
	//
	// publish is called while the transaction is open.  It must be fast and must
	// not start its own DB transaction.  If publish returns a non-nil error the
	// row is marked failed (attempt_count++ / failed_at when exhausted); if it
	// returns nil the row is marked sent.  Either way the outer transaction
	// commits so locks are always released.
	//
	// Returns the number of rows processed and any infrastructure error (tx open,
	// SELECT, UPDATE, or commit failure).  Errors from individual publish calls
	// are recorded in the row and do not surface here.
	RelayBatch(ctx context.Context, limit int, publish func(*OutboxEvent) error) (int, error)

	// MarkFailed increments attempt_count and records last_error.
	// When attempt_count reaches the repository's maxAttempts threshold,
	// failed_at is set so the row is excluded from future RelayBatch calls.
	//
	// Used by the worker for rows where the publish function itself is not
	// directly orchestrated by RelayBatch (e.g. in tests or manual recovery).
	MarkFailed(ctx context.Context, id int64, errMsg string) error
}
