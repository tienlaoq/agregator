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

func (r *Repo) EnsureSchema(ctx context.Context) error {
	const q = `
SELECT EXISTS (
  SELECT 1
  FROM information_schema.columns
  WHERE table_schema = 'public'
    AND table_name = 'chat_reads'
    AND column_name = 'last_read_message_id'
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

func scanThread(row pgx.Row) (*domain.Thread, error) {
	var t domain.Thread
	var lastID *uuid.UUID
	var lastAt *time.Time
	if err := row.Scan(&t.ID, &t.Kind, &t.RefID, &t.ParticipantUserIDs, &lastID, &lastAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	t.LastMessageID = lastID
	t.LastMessageAt = lastAt
	return &t, nil
}

func (r *Repo) loadParticipantReads(ctx context.Context, threadIDs []uuid.UUID) (map[uuid.UUID][]domain.ParticipantRead, error) {
	out := make(map[uuid.UUID][]domain.ParticipantRead)
	if len(threadIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
SELECT thread_id, user_id::text, last_read_message_id
FROM chat_reads
WHERE thread_id = ANY($1)
ORDER BY thread_id, user_id`, threadIDs)
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

func (r *Repo) hydrateThreadReads(ctx context.Context, threads ...*domain.Thread) error {
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
	m, err := r.loadParticipantReads(ctx, ids)
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

func (r *Repo) listParticipantsByThreadIDs(ctx context.Context, threadIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	if len(threadIDs) == 0 {
		return map[uuid.UUID][]string{}, nil
	}
	rows, err := r.pool.Query(ctx, `
SELECT thread_id, user_id::text
FROM chat_thread_participants
WHERE left_at IS NULL AND thread_id = ANY($1)
ORDER BY thread_id, joined_at ASC`, threadIDs)
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

func (r *Repo) EnsureThread(ctx context.Context, kind string, refID uuid.UUID, participantUserIDs []string) (*domain.Thread, error) {
	const upsert = `
INSERT INTO chat_threads (kind, ref_id, participant_user_ids)
VALUES ($1,$2,$3)
ON CONFLICT (kind, ref_id) DO UPDATE
SET participant_user_ids = (
    SELECT ARRAY(
      SELECT DISTINCT v
      FROM unnest(chat_threads.participant_user_ids || EXCLUDED.participant_user_ids) AS v
      WHERE v IS NOT NULL AND btrim(v) <> ''
      ORDER BY 1
    )
),
    updated_at = now()
RETURNING id, kind, ref_id, participant_user_ids, last_message_id, last_message_at, created_at, updated_at`
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	t, err := scanThread(tx.QueryRow(ctx, upsert, kind, refID, participantUserIDs))
	if err != nil {
		return nil, err
	}
	for _, uid := range participantUserIDs {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		_, err := tx.Exec(ctx, `
INSERT INTO chat_thread_participants (thread_id, user_id, role, left_at)
VALUES ($1, $2::uuid, 'participant', NULL)
ON CONFLICT (thread_id, user_id) DO UPDATE
SET left_at = NULL`, t.ID, uid)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit ensure thread: %w", err)
	}
	m, err := r.listParticipantsByThreadIDs(ctx, []uuid.UUID{t.ID})
	if err == nil && len(m[t.ID]) > 0 {
		t.ParticipantUserIDs = m[t.ID]
	}
	if err := r.hydrateThreadReads(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (r *Repo) GetThreadByID(ctx context.Context, threadID uuid.UUID) (*domain.Thread, error) {
	const q = `SELECT id, kind, ref_id, participant_user_ids, last_message_id, last_message_at, created_at, updated_at
FROM chat_threads WHERE id = $1`
	t, err := scanThread(r.pool.QueryRow(ctx, q, threadID))
	if err != nil || t == nil {
		return t, err
	}
	m, err := r.listParticipantsByThreadIDs(ctx, []uuid.UUID{t.ID})
	if err == nil && len(m[t.ID]) > 0 {
		t.ParticipantUserIDs = m[t.ID]
	}
	if err := r.hydrateThreadReads(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (r *Repo) ListThreadsForUser(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]domain.Thread, int32, error) {
	uid := userID.String()
	const countQ = `SELECT COUNT(DISTINCT thread_id) FROM chat_thread_participants WHERE user_id = $1::uuid AND left_at IS NULL`
	var total int32
	if err := r.pool.QueryRow(ctx, countQ, uid).Scan(&total); err != nil {
		return nil, 0, err
	}

	const q = `
SELECT t.id, t.kind, t.ref_id, t.participant_user_ids, t.last_message_id, t.last_message_at, t.created_at, t.updated_at,
COALESCE((
  SELECT COUNT(*)::int
  FROM chat_messages m
  LEFT JOIN chat_reads r ON r.thread_id = t.id AND r.user_id = $1::uuid
  WHERE m.thread_id = t.id
    AND m.author_user_id <> $1::uuid
    AND (
      r.user_id IS NULL
      OR (
        CASE
          WHEN r.last_read_message_id IS NOT NULL THEN
            EXISTS (
              SELECT 1 FROM chat_messages wm
              WHERE wm.id = r.last_read_message_id
                AND (
                  m.created_at > wm.created_at
                  OR (m.created_at = wm.created_at AND m.id > wm.id)
                )
            )
          WHEN r.last_read_at IS NOT NULL THEN
            m.created_at > r.last_read_at
          ELSE TRUE
        END
      )
    )
), 0) AS unread_count
FROM chat_threads t
JOIN chat_thread_participants p ON p.thread_id = t.id AND p.user_id = $1::uuid AND p.left_at IS NULL
ORDER BY COALESCE(t.last_message_at, t.created_at) DESC
LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, uid, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]domain.Thread, 0)
	for rows.Next() {
		var t domain.Thread
		var lastID *uuid.UUID
		var lastAt *time.Time
		if err := rows.Scan(&t.ID, &t.Kind, &t.RefID, &t.ParticipantUserIDs, &lastID, &lastAt, &t.CreatedAt, &t.UpdatedAt, &t.UnreadCount); err != nil {
			return nil, 0, err
		}
		t.LastMessageID = lastID
		t.LastMessageAt = lastAt
		out = append(out, t)
	}
	ids := make([]uuid.UUID, 0, len(out))
	for i := range out {
		ids = append(ids, out[i].ID)
	}
	pm, err := r.listParticipantsByThreadIDs(ctx, ids)
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
	if err := r.hydrateThreadReads(ctx, threadPtrs...); err != nil {
		return nil, 0, err
	}
	return out, total, rows.Err()
}

func (r *Repo) ListMessages(ctx context.Context, threadID uuid.UUID, limit, offset int32) ([]domain.Message, int32, error) {
	const countQ = `SELECT COUNT(*) FROM chat_messages WHERE thread_id = $1`
	var total int32
	if err := r.pool.QueryRow(ctx, countQ, threadID).Scan(&total); err != nil {
		return nil, 0, err
	}
	const q = `SELECT id, thread_id, author_user_id, text, client_msg_id, created_at
FROM chat_messages
WHERE thread_id = $1
ORDER BY created_at ASC
LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, threadID, limit, offset)
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

func (r *Repo) InsertMessage(ctx context.Context, threadID, authorUserID uuid.UUID, text, clientMsgID string) (*domain.Message, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	text = strings.TrimSpace(text)
	var m domain.Message
	clientMsgID = strings.TrimSpace(clientMsgID)
	m.ID = uuid.New()
	if err := tx.QueryRow(ctx, `INSERT INTO chat_messages (id, thread_id, author_user_id, text, client_msg_id)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (thread_id, author_user_id, client_msg_id)
WHERE client_msg_id <> ''
DO UPDATE SET text = chat_messages.text
RETURNING id, thread_id, author_user_id, text, client_msg_id, created_at`,
		m.ID, threadID, authorUserID, text, clientMsgID).Scan(
		&m.ID, &m.ThreadID, &m.AuthorUserID, &m.Text, &m.ClientMsgID, &m.CreatedAt,
	); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE chat_threads SET last_message_id = $2, last_message_at = now(), updated_at = now() WHERE id = $1`, threadID, m.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit insert message: %w", err)
	}
	return &m, nil
}

func (r *Repo) MarkRead(ctx context.Context, threadID, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
INSERT INTO chat_reads (thread_id, user_id, last_read_at, last_read_message_id)
SELECT $1, $2, now(), t.last_message_id
FROM chat_threads t
WHERE t.id = $1
ON CONFLICT (thread_id, user_id) DO UPDATE
SET last_read_at = now(),
    last_read_message_id = EXCLUDED.last_read_message_id`,
		threadID, userID)
	return err
}
