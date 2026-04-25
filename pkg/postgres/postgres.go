package postgres

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func envDurationMin(key string, fallback time.Duration) time.Duration {
	v := envInt(key, -1)
	if v <= 0 {
		return fallback
	}
	return time.Duration(v) * time.Minute
}

// NewPool creates a pgx pool. Tune with PG_POOL_MAX_CONNS, PG_POOL_MIN_CONNS,
// PG_POOL_MAX_CONN_LIFETIME_MIN, PG_POOL_MAX_CONN_IDLE_MIN (optional).
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pg config: %w", err)
	}

	maxC := int32(envInt("PG_POOL_MAX_CONNS", 25))
	if maxC < 1 {
		maxC = 1
	}
	cfg.MaxConns = maxC
	minC := int32(envInt("PG_POOL_MIN_CONNS", 2))
	if minC < 0 {
		minC = 0
	}
	if minC > maxC {
		minC = maxC
	}
	cfg.MinConns = minC
	cfg.MaxConnLifetime = envDurationMin("PG_POOL_MAX_CONN_LIFETIME_MIN", 30*time.Minute)
	cfg.MaxConnIdleTime = envDurationMin("PG_POOL_MAX_CONN_IDLE_MIN", 5*time.Minute)

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pg pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping pg: %w", err)
	}

	return pool, nil
}
