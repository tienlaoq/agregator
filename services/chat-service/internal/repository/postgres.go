package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tienlao/agregator/services/chat-service/internal/domain"
)

type Repo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// Pool exposes the underlying pgxpool.Pool for integration tests that need to
// run direct SQL assertions (e.g. COUNT rows, read trigger-maintained columns).
// Production code should never call this — use the domain methods instead.
func (r *Repo) Pool() *pgxpool.Pool { return r.pool }

func (r *Repo) EnsureSchema(ctx context.Context) error {
	const q = `
SELECT EXISTS (
  SELECT 1
  FROM information_schema.columns
  WHERE table_schema = 'public'
    AND table_name   = 'chat_reads'
    AND column_name  = 'last_read_message_id'
)`
	var ok bool
	if err := r.pool.QueryRow(ctx, q).Scan(&ok); err != nil {
		return fmt.Errorf("verify chat schema: %w", err)
	}
	if !ok {
		return fmt.Errorf("chat schema is outdated: missing chat_reads.last_read_message_id; apply migrations")
	}
	return nil
}

// scanThread scans id, kind, ref_id, last_message_id, last_message_at,
// created_at, updated_at — WITHOUT participant_user_ids (P1#11: removed column).
func scanThread(row pgx.Row) (*domain.Thread, error) {
	var t domain.Thread
	var lastID *uuid.UUID
	var lastAt *time.Time
	if err := row.Scan(&t.ID, &t.Kind, &t.RefID, &lastID, &lastAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	t.LastMessageID = lastID
	t.LastMessageAt = lastAt
	return &t, nil
}

// querier is the minimal interface shared by *pgxpool.Pool and pgx.Tx,
// allowing enrichment helpers to run either against the pool or inside an
// open transaction without duplicating SQL.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func loadParticipantReads(ctx context.Context, q querier, threadIDs []uuid.UUID) (map[uuid.UUID][]domain.ParticipantRead, error) {
	out := make(map[uuid.UUID][]domain.ParticipantRead)
	if len(threadIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `
SELECT thread_id, user_id::text, last_read_message_id
FROM   chat_reads
WHERE  thread_id = ANY($1)
ORDER  BY thread_id, user_id`, threadIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tid uuid.UUID
		var uid string
		var lastMsgID *uuid.UUID
		if err := rows.Scan(&tid, &uid, &lastMsgID); err != nil {
			return nil, err
		}
		out[tid] = append(out[tid], domain.ParticipantRead{
			UserID:            uid,
			LastReadMessageID: lastMsgID,
		})
	}
	return out, rows.Err()
}

func hydrateThreadReads(ctx context.Context, q querier, threads ...*domain.Thread) error {
	ids := make([]uuid.UUID, 0, len(threads))
	seen := make(map[uuid.UUID]struct{}, len(threads))
	for _, t := range threads {
		if t == nil {
			continue
		}
		if _, ok := seen[t.ID]; ok {
			continue
		}
		seen[t.ID] = struct{}{}
		ids = append(ids, t.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	m, err := loadParticipantReads(ctx, q, ids)
	if err != nil {
		return err
	}
	for _, t := range threads {
		if t == nil {
			continue
		}
		if list, ok := m[t.ID]; ok {
			t.ParticipantReads = list
		}
	}
	return nil
}

// listParticipantsByThreadIDs returns active participant user_ids (left_at IS
// NULL) grouped by thread.  It is the single source of truth for membership
// after migration 005 removed the redundant TEXT[] column (P1#11).
func listParticipantsByThreadIDs(ctx context.Context, q querier, threadIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	if len(threadIDs) == 0 {
		return map[uuid.UUID][]string{}, nil
	}
	rows, err := q.Query(ctx, `
SELECT thread_id, user_id::text
FROM   chat_thread_participants
WHERE  left_at IS NULL AND thread_id = ANY($1)
ORDER  BY thread_id, joined_at ASC`, threadIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uuid.UUID][]string, len(threadIDs))
	for rows.Next() {
		var tid uuid.UUID
		var uid string
		if err := rows.Scan(&tid, &uid); err != nil {
			return nil, err
		}
		out[tid] = append(out[tid], uid)
	}
	return out, rows.Err()
}

// EnsureThread upserts the thread row and its participant rows in one
// transaction.  The participant_user_ids TEXT[] column was removed in migration
// 005 (P1#11); the upsert therefore only touches chat_thread_participants.
func (r *Repo) EnsureThread(ctx context.Context, kind string, refID uuid.UUID, participantUserIDs []string) (*domain.Thread, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// N5: acquire a session-scoped advisory lock keyed on (kind, ref_id) before
	// touching the thread or its participants.  Without this, two concurrent
	// EnsureThread calls for the same booking can both see "no row yet", both
	// INSERT (one wins the conflict, one becomes a no-op UPDATE), and then race
	// on participant upserts — the later-committing transaction's participant set
	// wins, which may differ from the earlier one's.
	//
	// pg_advisory_xact_lock is released automatically at transaction end, so
	// no explicit unlock is needed.  The lock key is a stable int64 derived from
	// the logical identity of the thread; collisions across different (kind,
	// ref_id) pairs are astronomically unlikely with hashtext on the concatenated
	// string.
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1))`,
		kind+"::"+refID.String(),
	); err != nil {
		return nil, fmt.Errorf("advisory lock for ensure thread: %w", err)
	}

	// Upsert thread — no participant_user_ids column any more (P1#11).
	t, err := scanThread(tx.QueryRow(ctx, `
INSERT INTO chat_threads (kind, ref_id)
VALUES ($1, $2)
ON CONFLICT (kind, ref_id) DO UPDATE
    SET updated_at = now()
RETURNING id, kind, ref_id, last_message_id, last_message_at, created_at, updated_at`,
		kind, refID))
	if err != nil {
		return nil, err
	}

	for _, uid := range participantUserIDs {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO chat_thread_participants (thread_id, user_id, role, left_at)
VALUES ($1, $2::uuid, 'participant', NULL)
ON CONFLICT (thread_id, user_id) DO UPDATE
    SET left_at = NULL`, t.ID, uid); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit ensure thread: %w", err)
	}

	// Enrich from participants table (single source of truth).
	pm, err := listParticipantsByThreadIDs(ctx, r.pool, []uuid.UUID{t.ID})
	if err == nil && len(pm[t.ID]) > 0 {
		t.ParticipantUserIDs = pm[t.ID]
	}
	if err := hydrateThreadReads(ctx, r.pool, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (r *Repo) GetThreadByID(ctx context.Context, threadID uuid.UUID) (*domain.Thread, error) {
	// P1#11: no participant_user_ids column — fetched separately below.
	t, err := scanThread(r.pool.QueryRow(ctx, `
SELECT id, kind, ref_id, last_message_id, last_message_at, created_at, updated_at
FROM   chat_threads
WHERE  id = $1`, threadID))
	if err != nil || t == nil {
		return t, err
	}
	pm, err := listParticipantsByThreadIDs(ctx, r.pool, []uuid.UUID{t.ID})
	if err == nil && len(pm[t.ID]) > 0 {
		t.ParticipantUserIDs = pm[t.ID]
	}
	if err := hydrateThreadReads(ctx, r.pool, t); err != nil {
		return nil, err
	}
	return t, nil
}

// ListThreadsForUser returns threads for a user with unread_count read directly
// from chat_thread_participants.unread_count (set by triggers from migration
// 004).  The old correlated subquery over chat_messages is gone (P1#10).
func (r *Repo) ListThreadsForUser(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]domain.Thread, int32, error) {
	uid := userID.String()

	var total int32
	if err := r.pool.QueryRow(ctx, `
SELECT COUNT(DISTINCT thread_id)
FROM   chat_thread_participants
WHERE  user_id = $1::uuid AND left_at IS NULL`, uid).Scan(&total); err != nil {
		return nil, 0, err
	}

	// P1#10: unread_count is now a cheap column read instead of a per-thread
	// correlated subquery.  P1#11: no participant_user_ids column in chat_threads.
	rows, err := r.pool.Query(ctx, `
SELECT t.id, t.kind, t.ref_id,
       t.last_message_id, t.last_message_at,
       t.created_at, t.updated_at,
       p.unread_count
FROM   chat_threads t
JOIN   chat_thread_participants p
       ON p.thread_id = t.id
      AND p.user_id   = $1::uuid
      AND p.left_at IS NULL
ORDER  BY COALESCE(t.last_message_at, t.created_at) DESC
LIMIT  $2 OFFSET $3`, uid, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]domain.Thread, 0)
	for rows.Next() {
		var t domain.Thread
		var lastID *uuid.UUID
		var lastAt *time.Time
		if err := rows.Scan(
			&t.ID, &t.Kind, &t.RefID,
			&lastID, &lastAt,
			&t.CreatedAt, &t.UpdatedAt,
			&t.UnreadCount,
		); err != nil {
			return nil, 0, err
		}
		t.LastMessageID = lastID
		t.LastMessageAt = lastAt
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	ids := make([]uuid.UUID, 0, len(out))
	for i := range out {
		ids = append(ids, out[i].ID)
	}
	pm, err := listParticipantsByThreadIDs(ctx, r.pool, ids)
	if err == nil {
		for i := range out {
			if p := pm[out[i].ID]; len(p) > 0 {
				out[i].ParticipantUserIDs = p
			}
		}
	}
	threadPtrs := make([]*domain.Thread, len(out))
	for i := range out {
		threadPtrs[i] = &out[i]
	}
	if err := hydrateThreadReads(ctx, r.pool, threadPtrs...); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repo) ListMessages(ctx context.Context, threadID uuid.UUID, limit, offset int32) ([]domain.Message, int32, error) {
	var total int32
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM chat_messages WHERE thread_id = $1`, threadID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
SELECT id, thread_id, author_user_id, text, client_msg_id, created_at
FROM   chat_messages
WHERE  thread_id = $1
ORDER  BY created_at ASC
LIMIT  $2 OFFSET $3`, threadID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]domain.Message, 0)
	for rows.Next() {
		var m domain.Message
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.AuthorUserID, &m.Text, &m.ClientMsgID, &m.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// InsertMessage inserts a message, updates chat_threads.last_message_id in the
// same transaction, and returns the fresh thread — all without a second
// round-trip (P1#8, P1#13).
func (r *Repo) InsertMessage(ctx context.Context, threadID, authorUserID uuid.UUID, text, clientMsgID string) (*domain.Message, *domain.Thread, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	text = strings.TrimSpace(text)
	clientMsgID = strings.TrimSpace(clientMsgID)

	// P1#9: generate a server-side UUID when the caller omits client_msg_id so
	// the partial-unique index always fires and prevents accidental duplicates.
	if clientMsgID == "" {
		clientMsgID = uuid.New().String()
	}

	var m domain.Message
	m.ID = uuid.New()
	if err := tx.QueryRow(ctx, `
INSERT INTO chat_messages (id, thread_id, author_user_id, text, client_msg_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (thread_id, author_user_id, client_msg_id)
WHERE client_msg_id <> ''
DO UPDATE SET text = chat_messages.text
RETURNING id, thread_id, author_user_id, text, client_msg_id, created_at`,
		m.ID, threadID, authorUserID, text, clientMsgID,
	).Scan(&m.ID, &m.ThreadID, &m.AuthorUserID, &m.Text, &m.ClientMsgID, &m.CreatedAt); err != nil {
		return nil, nil, err
	}

	// Update thread metadata; only touches last_message_id when this is the
	// winning insert (on conflict the RETURNING id equals the original row's id).
	if _, err := tx.Exec(ctx, `
UPDATE chat_threads
SET    last_message_id = $2, last_message_at = now(), updated_at = now()
WHERE  id = $1`, threadID, m.ID); err != nil {
		return nil, nil, err
	}

	// Fetch the refreshed thread inside the transaction to close the race
	// window (P1#8).  P1#11: no participant_user_ids column.
	t, err := scanThread(tx.QueryRow(ctx, `
SELECT id, kind, ref_id, last_message_id, last_message_at, created_at, updated_at
FROM   chat_threads WHERE id = $1`, threadID))
	if err != nil {
		return nil, nil, err
	}
	if t == nil {
		return nil, nil, fmt.Errorf("thread %s not found after insert", threadID)
	}

	// N10: enrich inside the transaction so the returned thread is consistent
	// with the snapshot that includes the just-inserted message.  A concurrent
	// EnsureThread between Commit and a post-commit enrichment query could change
	// the participant set, making the returned value inconsistent.  READ COMMITTED
	// means we see the committed state of participants at this point in time,
	// which is exactly what we want to fan-out to clients.
	pm, err := listParticipantsByThreadIDs(ctx, tx, []uuid.UUID{t.ID})
	if err != nil {
		return nil, nil, err
	}
	if len(pm[t.ID]) > 0 {
		t.ParticipantUserIDs = pm[t.ID]
	}
	if err := hydrateThreadReads(ctx, tx, t); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("commit insert message: %w", err)
	}

	return &m, t, nil
}

// MarkRead upserts a chat_reads row, setting last_read_message_id to
// chat_threads.last_message_id at the time of the write.
//
// N4 — known watermark race (accepted trade-off):
//
//	Timeline that produces a "future" watermark:
//	  1. Client A calls MarkRead (has seen N messages).
//	  2. Client B inserts message N+1 → fn_increment_unread fires, unread[A]++.
//	  3. Server processes step-1 MarkRead: reads last_message_id = N+1,
//	     writes watermark = N+1, fn_reset_unread fires → unread[A] = 0.
//	  Result: A is marked as having read message N+1 it hasn't seen in the UI.
//
//	Why it's acceptable:
//	  - Message N+1 arrives via WS-fanout milliseconds later; A will see it.
//	  - The only observable artefact is unread_count briefly showing 0.
//	  - No data loss; no security implication.
//
//	Future fix (when needed):
//	  Accept an optional last_seen_message_id from the client and use
//	  LEAST(client_watermark, t.last_message_id) in the SELECT below.
//	  Proto field 3 is reserved for this in MarkReadRequest.
func (r *Repo) MarkRead(ctx context.Context, threadID, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
INSERT INTO chat_reads (thread_id, user_id, last_read_at, last_read_message_id)
SELECT $1, $2, now(), t.last_message_id
FROM   chat_threads t
WHERE  t.id = $1
ON CONFLICT (thread_id, user_id) DO UPDATE
    SET last_read_at          = now(),
        last_read_message_id  = EXCLUDED.last_read_message_id`,
		threadID, userID)
	return err
}
