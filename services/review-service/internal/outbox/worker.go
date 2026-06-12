// Package outbox implements a polling worker that reads pending rows from the
// outbox_events table and publishes them to NATS JetStream with at-least-once
// semantics. Multiple instances are safe because ClaimBatch uses
// SELECT … FOR UPDATE SKIP LOCKED to prevent concurrent workers from claiming
// the same rows, and locked_at prevents rows from being re-claimed while a
// worker is publishing.
//
// Three-phase tick to avoid holding a Postgres row-lock during NATS I/O:
//
//  1. Claim  — short tx: lock rows + stamp locked_at + COMMIT.
//  2. Publish — NATS publish outside any DB transaction.
//  3. Mark   — short tx: set published_at + clear locked_at + COMMIT.
//
// If a worker crashes between phase 2 and 3, the row retains locked_at.
// ResetStaleLocks (called every staleSweepInterval) reclaims such rows so
// another worker can re-publish them. JetStream duplicate suppression
// (Nats-Msg-Id = ReviewID, default 2-minute window) deduplicates fast retries;
// subscribers MUST still deduplicate on ReviewID for retries outside that window.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/tienlao/agregator/services/review-service/internal/domain"
	"github.com/tienlao/agregator/services/review-service/internal/events"
)

const (
	defaultBatchSize   = 50
	defaultInterval    = 5 * time.Second
	staleSweepInterval = time.Minute
)

// Publisher is the subset of events.Publisher used by the worker.
type Publisher interface {
	PublishReviewCreated(ctx context.Context, event events.ReviewCreatedEvent) error
}

// Worker polls the outbox table and publishes pending events to NATS.
type Worker struct {
	repo           domain.OutboxRepository
	publisher      Publisher
	interval       time.Duration
	batchSize      int
	lastStaleSweep time.Time
}

// NewWorker creates a Worker with the given polling interval and batch size.
// Use defaultBatchSize / defaultInterval as fallbacks when no config is present.
func NewWorker(repo domain.OutboxRepository, publisher Publisher, batchSize int, interval time.Duration) *Worker {
	return &Worker{
		repo:      repo,
		publisher: publisher,
		interval:  interval,
		batchSize: batchSize,
	}
}

// Run starts the polling loop and blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	log.Info().Dur("interval", w.interval).Int("batch", w.batchSize).
		Msg("outbox worker started")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("outbox worker stopped")
			return
		case <-ticker.C:
			w.maybeSweepStaleLocks(ctx)
			w.tick(ctx)
		}
	}
}

// maybeSweepStaleLocks calls ResetStaleLocks at most once per staleSweepInterval.
// Stale lock cleanup is best-effort; errors are logged but do not stop the worker.
func (w *Worker) maybeSweepStaleLocks(ctx context.Context) {
	if time.Since(w.lastStaleSweep) < staleSweepInterval {
		return
	}
	if err := w.repo.ResetStaleLocks(ctx); err != nil {
		log.Warn().Err(err).Msg("outbox: stale lock sweep failed")
	}
	w.lastStaleSweep = time.Now()
}

// tick runs one claim → publish → mark cycle.
//
// Phase 1 — claim: short transaction acquires FOR UPDATE SKIP LOCKED on the
// batch and stamps locked_at = now(), then commits immediately. The Postgres
// row-lock is released as soon as we commit; locked_at prevents other workers
// from re-claiming the same rows until the stale threshold passes.
//
// Phase 2 — publish: NATS I/O happens entirely outside any DB transaction.
// A slow ACK or a NATS timeout no longer blocks a Postgres connection.
//
// Phase 3 — mark: another short transaction sets published_at and clears
// locked_at. MarkPublished manages its own transaction internally.
func (w *Worker) tick(ctx context.Context) {
	// Phase 1: claim batch.
	tx, err := w.repo.BeginTx(ctx)
	if err != nil {
		log.Error().Err(err).Msg("outbox: begin claim tx failed")
		return
	}
	// defer handles rollback in all early-return paths; after a successful
	// Commit pgx returns ErrTxClosed which is intentionally ignored.
	defer func() { _ = tx.Rollback(ctx) }()

	entries, err := w.repo.ClaimBatch(ctx, tx, w.batchSize)
	if err != nil {
		log.Error().Err(err).Msg("outbox: claim batch failed")
		return
	}
	if len(entries) == 0 {
		return
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("outbox: commit claim tx failed")
		return
	}

	// Phase 2: publish outside any DB transaction.
	var published []string
	for _, e := range entries {
		if pubErr := w.publish(ctx, e); pubErr != nil {
			// Stop on first error; unpublished rows keep their locked_at and
			// will be reclaimed by the stale lock sweeper after StaleLockThreshold.
			log.Warn().Err(pubErr).Str("outbox_id", e.ID).Str("event_type", e.EventType).
				Msg("outbox: publish failed; will retry after stale lock reset")
			break
		}
		published = append(published, e.ID)
	}

	if len(published) == 0 {
		return
	}

	// Phase 3: mark published.
	// Use a detached context so that a concurrent shutdown signal does not
	// cancel the mark step for events that already reached NATS — reducing
	// the duplicate-delivery window on graceful shutdown.
	if err := w.repo.MarkPublished(context.WithoutCancel(ctx), published); err != nil {
		// Events reached NATS but the DB mark failed — they will be re-published
		// once the lock expires (at-least-once guarantee, not exactly-once).
		log.Error().Err(err).Int("count", len(published)).
			Msg("outbox: mark published failed; duplicate events possible on retry")
	}
}

func (w *Worker) publish(ctx context.Context, e *domain.OutboxEntry) error {
	switch e.EventType {
	case events.SubjectReviewCreated:
		var evt events.ReviewCreatedEvent
		if err := json.Unmarshal(e.Payload, &evt); err != nil {
			return err
		}
		return w.publisher.PublishReviewCreated(ctx, evt)
	default:
		// Return an error so the entry stays in the queue and the publish loop
		// stops. Each retry tick will log the warning, creating visible noise
		// until the worker is updated or the row is manually deleted.
		// Returning nil here would silently mark the event as published and
		// lose it forever.
		return fmt.Errorf("outbox: unknown event type %q (outbox_id=%s)", e.EventType, e.ID)
	}
}
