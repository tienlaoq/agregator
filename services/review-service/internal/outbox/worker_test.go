package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tienlao/agregator/services/review-service/internal/domain"
	"github.com/tienlao/agregator/services/review-service/internal/events"
)

// ---------------------------------------------------------------------------
// mockTx — minimal pgx.Tx stub; only Commit/Rollback are exercised by tests.
// ---------------------------------------------------------------------------

type mockTx struct {
	commitErr  error
	committed  bool
	rolledBack bool
}

func (m *mockTx) Begin(_ context.Context) (pgx.Tx, error) { return m, nil }
func (m *mockTx) Commit(_ context.Context) error          { m.committed = true; return m.commitErr }
func (m *mockTx) Rollback(_ context.Context) error        { m.rolledBack = true; return nil }
func (m *mockTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (m *mockTx) SendBatch(_ context.Context, _ *pgx.Batch) pgx.BatchResults { return nil }
func (m *mockTx) LargeObjects() pgx.LargeObjects                             { return pgx.LargeObjects{} }
func (m *mockTx) Prepare(_ context.Context, _, _ string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (m *mockTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (m *mockTx) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) { return nil, nil }
func (m *mockTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row        { return nil }
func (m *mockTx) Conn() *pgx.Conn                                               { return nil }

// ---------------------------------------------------------------------------
// mockOutboxRepo
// ---------------------------------------------------------------------------

type mockOutboxRepo struct {
	tx                 *mockTx
	beginErr           error
	claimEntries       []*domain.OutboxEntry
	claimErr           error
	markedIDs          []string
	markErr            error
	resetStaleLocksN   int
	resetStaleLocksErr error
}

func (m *mockOutboxRepo) BeginTx(_ context.Context) (pgx.Tx, error) {
	if m.beginErr != nil {
		return nil, m.beginErr
	}
	if m.tx == nil {
		m.tx = &mockTx{}
	}
	return m.tx, nil
}

func (m *mockOutboxRepo) CreateTx(_ context.Context, _ pgx.Tx, _ *domain.OutboxEntry) error {
	return nil
}

func (m *mockOutboxRepo) ClaimBatch(_ context.Context, _ pgx.Tx, _ int) ([]*domain.OutboxEntry, error) {
	if m.claimErr != nil {
		return nil, m.claimErr
	}
	return m.claimEntries, nil
}

func (m *mockOutboxRepo) MarkPublished(_ context.Context, ids []string) error {
	m.markedIDs = append(m.markedIDs, ids...)
	return m.markErr
}

func (m *mockOutboxRepo) ResetStaleLocks(_ context.Context) error {
	m.resetStaleLocksN++
	return m.resetStaleLocksErr
}

// ---------------------------------------------------------------------------
// mockPublisher
// ---------------------------------------------------------------------------

type mockPublisher struct {
	errAtIndex int // -1 = never fail; N = fail on the Nth call (0-based)
	callCount  int
	published  []events.ReviewCreatedEvent
}

func (m *mockPublisher) PublishReviewCreated(_ context.Context, evt events.ReviewCreatedEvent) error {
	idx := m.callCount
	m.callCount++
	if m.errAtIndex >= 0 && idx == m.errAtIndex {
		return errors.New("nats: publish failed")
	}
	m.published = append(m.published, evt)
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestWorker(repo *mockOutboxRepo, pub *mockPublisher) *Worker {
	return &Worker{
		repo:      repo,
		publisher: pub,
		interval:  defaultInterval,
		batchSize: defaultBatchSize,
	}
}

// validEntry builds an OutboxEntry with a correctly marshalled ReviewCreatedEvent payload.
func validEntry(id string) *domain.OutboxEntry {
	payload, _ := json.Marshal(events.ReviewCreatedEvent{
		ReviewID:   id,
		UserID:     "user-" + id,
		TargetType: events.TargetTypeVenue,
		VenueID:    "venue-1",
		Rating:     5,
	})
	return &domain.OutboxEntry{ID: id, EventType: events.SubjectReviewCreated, Payload: payload}
}

// ---------------------------------------------------------------------------
// tick — phase 1 (claim) failure paths
// ---------------------------------------------------------------------------

func TestWorker_tick_beginTxFails_returnsEarly(t *testing.T) {
	repo := &mockOutboxRepo{beginErr: errors.New("db: max connections reached")}
	pub := &mockPublisher{errAtIndex: -1}
	newTestWorker(repo, pub).tick(context.Background())

	assert.Equal(t, 0, pub.callCount, "publisher must not be called when BeginTx fails")
	assert.Nil(t, repo.markedIDs)
}

func TestWorker_tick_claimBatchFails_rollsBack(t *testing.T) {
	repo := &mockOutboxRepo{claimErr: errors.New("db: deadlock detected")}
	pub := &mockPublisher{errAtIndex: -1}
	newTestWorker(repo, pub).tick(context.Background())

	assert.Equal(t, 0, pub.callCount)
	assert.Nil(t, repo.markedIDs)
	require.NotNil(t, repo.tx)
	assert.True(t, repo.tx.rolledBack, "claim tx must be rolled back on ClaimBatch error")
	assert.False(t, repo.tx.committed)
}

func TestWorker_tick_emptyBatch_rollsBack(t *testing.T) {
	repo := &mockOutboxRepo{claimEntries: nil}
	pub := &mockPublisher{errAtIndex: -1}
	newTestWorker(repo, pub).tick(context.Background())

	assert.Equal(t, 0, pub.callCount, "publisher must not be called on empty batch")
	assert.Nil(t, repo.markedIDs, "MarkPublished must not be called on empty batch")
	require.NotNil(t, repo.tx)
	assert.True(t, repo.tx.rolledBack, "claim tx must be rolled back for empty batch")
	assert.False(t, repo.tx.committed)
}

func TestWorker_tick_commitClaimFails_returnsEarly(t *testing.T) {
	repo := &mockOutboxRepo{
		claimEntries: []*domain.OutboxEntry{validEntry("1")},
		tx:           &mockTx{commitErr: errors.New("db: commit failed")},
	}
	pub := &mockPublisher{errAtIndex: -1}
	newTestWorker(repo, pub).tick(context.Background())

	assert.Equal(t, 0, pub.callCount, "publish must not start when claim commit fails")
	assert.Nil(t, repo.markedIDs)
}

// ---------------------------------------------------------------------------
// tick — phase 2 (publish) paths
// ---------------------------------------------------------------------------

func TestWorker_tick_allPublished(t *testing.T) {
	repo := &mockOutboxRepo{
		claimEntries: []*domain.OutboxEntry{validEntry("1"), validEntry("2")},
	}
	pub := &mockPublisher{errAtIndex: -1}
	newTestWorker(repo, pub).tick(context.Background())

	require.NotNil(t, repo.tx)
	assert.True(t, repo.tx.committed, "claim tx must be committed when entries exist")
	assert.Equal(t, 2, pub.callCount)
	require.Len(t, pub.published, 2)
	assert.Equal(t, "1", pub.published[0].ReviewID)
	assert.Equal(t, "2", pub.published[1].ReviewID)
	assert.Equal(t, []string{"1", "2"}, repo.markedIDs)
}

func TestWorker_tick_firstPublishFails_noMark(t *testing.T) {
	repo := &mockOutboxRepo{
		claimEntries: []*domain.OutboxEntry{validEntry("1")},
	}
	pub := &mockPublisher{errAtIndex: 0} // fail on first call
	newTestWorker(repo, pub).tick(context.Background())

	assert.Equal(t, 1, pub.callCount)
	assert.Empty(t, pub.published, "no events published when first attempt fails")
	assert.Nil(t, repo.markedIDs, "MarkPublished must not be called when nothing was published")
}

func TestWorker_tick_partialPublish_stopsAtError(t *testing.T) {
	repo := &mockOutboxRepo{
		claimEntries: []*domain.OutboxEntry{validEntry("1"), validEntry("2"), validEntry("3")},
	}
	pub := &mockPublisher{errAtIndex: 1} // fail on second publish (entry "2")
	newTestWorker(repo, pub).tick(context.Background())

	// Publisher called for "1" (success) and "2" (fail); "3" never attempted.
	assert.Equal(t, 2, pub.callCount)
	require.Len(t, pub.published, 1)
	assert.Equal(t, "1", pub.published[0].ReviewID)
	// Only "1" marked; "2" and "3" keep locked_at and are retried after stale sweep.
	assert.Equal(t, []string{"1"}, repo.markedIDs)
}

func TestWorker_tick_unknownEventType_notMarked(t *testing.T) {
	// Unknown event type: publish() returns an error → the publish loop breaks →
	// published slice is empty → MarkPublished is NOT called.
	// The entry keeps its locked_at and will be retried after the stale lock
	// threshold, producing a loud log warning on every retry until the worker is
	// updated or the row is manually deleted. This is intentional: returning nil
	// would silently mark the event as published and lose it forever.
	unknownEntry := &domain.OutboxEntry{
		ID:        "x-1",
		EventType: "unknown.type",
		Payload:   json.RawMessage(`{}`),
	}
	repo := &mockOutboxRepo{claimEntries: []*domain.OutboxEntry{unknownEntry}}
	pub := &mockPublisher{errAtIndex: -1}
	newTestWorker(repo, pub).tick(context.Background())

	assert.Equal(t, 0, pub.callCount, "real publisher not invoked for unknown event type")
	assert.Nil(t, repo.markedIDs, "unknown entry must NOT be marked — it must stay in the queue")
}

func TestWorker_tick_invalidJSONPayload_stopsPublish(t *testing.T) {
	badEntry := &domain.OutboxEntry{
		ID:        "bad-1",
		EventType: events.SubjectReviewCreated,
		Payload:   json.RawMessage(`not-valid-json`),
	}
	repo := &mockOutboxRepo{claimEntries: []*domain.OutboxEntry{badEntry}}
	pub := &mockPublisher{errAtIndex: -1}
	newTestWorker(repo, pub).tick(context.Background())

	assert.Equal(t, 0, pub.callCount, "publisher not called when JSON unmarshal fails")
	assert.Nil(t, repo.markedIDs, "bad entry must not be marked — will retry after stale lock reset")
}

// ---------------------------------------------------------------------------
// tick — phase 3 (mark) failure path
// ---------------------------------------------------------------------------

func TestWorker_tick_markPublishedFails_noPanic(t *testing.T) {
	// All entries published to NATS, but MarkPublished fails.
	// Tick must not panic: at-least-once guarantee means duplicates are acceptable;
	// the entries will be re-published when locked_at expires.
	repo := &mockOutboxRepo{
		claimEntries: []*domain.OutboxEntry{validEntry("1"), validEntry("2")},
		markErr:      errors.New("db: connection reset"),
	}
	pub := &mockPublisher{errAtIndex: -1}

	assert.NotPanics(t, func() {
		newTestWorker(repo, pub).tick(context.Background())
	})
	assert.Equal(t, 2, pub.callCount, "both entries must be published before mark attempt")
}

// ---------------------------------------------------------------------------
// maybeSweepStaleLocks
// ---------------------------------------------------------------------------

func TestWorker_maybeSweepStaleLocks_runsWhenNeverRun(t *testing.T) {
	repo := &mockOutboxRepo{}
	w := newTestWorker(repo, &mockPublisher{errAtIndex: -1})
	// lastStaleSweep is zero — sweep must run.
	w.maybeSweepStaleLocks(context.Background())
	assert.Equal(t, 1, repo.resetStaleLocksN)
}

func TestWorker_maybeSweepStaleLocks_skipsIfRecent(t *testing.T) {
	repo := &mockOutboxRepo{}
	w := newTestWorker(repo, &mockPublisher{errAtIndex: -1})
	w.lastStaleSweep = time.Now() // swept just now
	w.maybeSweepStaleLocks(context.Background())
	assert.Equal(t, 0, repo.resetStaleLocksN, "sweep must be skipped within staleSweepInterval")
}

func TestWorker_maybeSweepStaleLocks_errorStillUpdatesTimestamp(t *testing.T) {
	// Even on error the timestamp is updated so a failing DB isn't hammered
	// once per tick. The error is logged (best-effort semantics).
	repo := &mockOutboxRepo{resetStaleLocksErr: errors.New("db: error")}
	w := newTestWorker(repo, &mockPublisher{errAtIndex: -1})
	before := time.Now()

	w.maybeSweepStaleLocks(context.Background())

	assert.Equal(t, 1, repo.resetStaleLocksN)
	assert.False(t, w.lastStaleSweep.Before(before),
		"lastStaleSweep must be updated even when ResetStaleLocks fails")
}
