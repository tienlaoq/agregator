package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type EventRepo struct {
	pool *pgxpool.Pool
}

func NewEventRepo(pool *pgxpool.Pool) *EventRepo {
	return &EventRepo{pool: pool}
}

// InsertEvent persists one product event. streamSeq is the JetStream stream
// sequence: the UNIQUE constraint turns an at-least-once redelivery into a
// no-op instead of a duplicate row.
func (r *EventRepo) InsertEvent(ctx context.Context, streamSeq uint64, event, requestID string, props []byte) error {
	if len(props) == 0 {
		props = []byte("{}")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO events (stream_seq, event, request_id, props)
		VALUES ($1, $2, $3, $4::jsonb)
		ON CONFLICT (stream_seq) DO NOTHING`,
		int64(streamSeq), event, requestID, props,
	)
	return err
}
