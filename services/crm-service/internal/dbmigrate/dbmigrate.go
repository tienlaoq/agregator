package dbmigrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

// Up applies *.up.sql from dir in numeric-prefix order, skipping versions
// already recorded in schema_migrations.
//
// Strangler-fig caveat: crm-service shares its Postgres database with
// venue-service, so the venue_staff / venue_crm_tasks tables may already
// exist (created by venue-service migrations 012/013). The migration SQL
// uses CREATE TABLE IF NOT EXISTS so re-applying is a safe no-op; this
// function records the migration as applied without aborting.
func Up(ctx context.Context, dsn, dir string, log zerolog.Logger) error {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("migrations dir %q: %w", dir, err)
	}

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connect for migrate: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("schema_migrations: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".up.sql") {
			files = append(files, name)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return migrationSeq(files[i]) < migrationSeq(files[j])
	})

	for _, name := range files {
		var dummy int
		err := conn.QueryRow(ctx,
			`SELECT 1 FROM schema_migrations WHERE version = $1`, name).Scan(&dummy)
		if err == nil {
			continue
		}
		if err != pgx.ErrNoRows {
			return fmt.Errorf("check migration %s: %w", name, err)
		}

		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		sql := strings.TrimSpace(string(body))
		if sql == "" {
			continue
		}

		if _, err := conn.Exec(ctx, sql); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := conn.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		log.Info().Str("migration", name).Msg("applied migration")
	}
	return nil
}

func migrationSeq(filename string) int {
	i := strings.IndexByte(filename, '_')
	if i <= 0 {
		return 1 << 30
	}
	n, err := strconv.Atoi(filename[:i])
	if err != nil {
		return 1 << 30
	}
	return n
}
