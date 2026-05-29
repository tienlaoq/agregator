// Package outbox provides a relay worker that reads pending rows from the
// payment_outbox table and publishes them to NATS JetStream.
//
// The outbox pattern guarantees at-least-once delivery:
//   - The payment status UPDATE and the outbox INSERT share a single DB
//     transaction (see repository.PaymentRepo.UpdateStatusWithOutbox).
//   - If the process crashes between commit and NATS publish the row stays
//     pending and the worker retries on the next tick.
//   - If NATS publish succeeds but MarkSent (inside RelayBatch) fails the row
//     is re-published on the next tick; downstream consumers must be idempotent
//     (deduplicate on payment_id).
//
// Multi-pod safety:
//   - RelayBatch uses SELECT … FOR UPDATE SKIP LOCKED so that two worker pods
//     never lock the same row concurrently.  Each batch is processed inside a
//     single transaction; locks are released on commit.
package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

const (
	// fetchBatchSize is the maximum number of outbox rows fetched per tick.
	fetchBatchSize = 50

	// defaultPollInterval is how often the worker wakes up to check for
	// pending rows.  Short enough to keep event lag low; long enough to avoid
	// hammering the DB under normal load.
	defaultPollInterval = 2 * time.Second

	// maxAttempts mirrors repository.outboxMaxAttempts — after this many
	// consecutive failures the row is marked failed and excluded from polls.
	maxAttempts = 10
)

// Worker polls the outbox table and relays events to NATS JetStream.
type Worker struct {
	outbox       domain.OutboxRepository
	js           nats.JetStreamContext
	log          zerolog.Logger
	pollInterval time.Duration
}

// NewWorker creates a Worker.  pollInterval of 0 uses the default (2 s).
func NewWorker(
	outbox domain.OutboxRepository,
	js nats.JetStreamContext,
	log zerolog.Logger,
	pollInterval time.Duration,
) *Worker {
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}
	return &Worker{
		outbox:       outbox,
		js:           js,
		log:          log.With().Str("component", "outbox-worker").Logger(),
		pollInterval: pollInterval,
	}
}

// Run starts the relay loop.  It blocks until ctx is cancelled.
// Call it in a goroutine: go worker.Run(ctx).
func (w *Worker) Run(ctx context.Context) {
	w.log.Info().Dur("poll_interval", w.pollInterval).Msg("outbox worker started")
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info().Msg("outbox worker stopped")
			return
		case <-ticker.C:
			w.relay(ctx)
		}
	}
}

// relay fetches a batch of pending rows under FOR UPDATE SKIP LOCKED and
// publishes each one to NATS.  All row-level marking (sent / failed) happens
// inside the same DB transaction as the SELECT, so two pods never process the
// same row concurrently.
func (w *Worker) relay(ctx context.Context) {
	n, err := w.outbox.RelayBatch(ctx, fetchBatchSize, func(e *domain.OutboxEvent) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return w.publishOne(ctx, e)
	})
	if err != nil {
		w.log.Error().Err(err).Msg("outbox: relay batch failed")
		return
	}
	if n > 0 {
		w.log.Debug().Int("count", n).Msg("outbox: batch processed")
	}
}

// publishOne attempts to deliver a single outbox event to NATS.
// Returns nil on success, non-nil on failure.  The returned error is recorded
// by RelayBatch as the row's last_error; it is not surfaced to relay's caller.
func (w *Worker) publishOne(ctx context.Context, e *domain.OutboxEvent) error {
	_, err := w.js.PublishMsg(&nats.Msg{
		Subject: string(e.Subject),
		Data:    e.Payload,
	}, nats.Context(ctx))

	if err != nil {
		w.log.Warn().
			Int64("outbox_id", e.ID).
			Str("subject", string(e.Subject)).
			Str("payment_id", e.PaymentID).
			Int("attempt", e.AttemptCount+1).
			Err(err).
			Msg("outbox: publish failed")
		return fmt.Errorf("attempt %d: %w", e.AttemptCount+1, err)
	}

	w.log.Info().
		Int64("outbox_id", e.ID).
		Str("subject", string(e.Subject)).
		Str("payment_id", e.PaymentID).
		Msg("outbox: event published")
	return nil
}
