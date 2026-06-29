// Package dbmigrate applies forward-only SQL migrations from a directory,
// tracking applied versions in a schema_migrations table.
//
// It is a shared, service-agnostic migrator used by the standalone migrator
// command (services/migrator) that runs as a Helm pre-upgrade hook in k8s.
// venue-service and crm-service keep their own dbmigrate packages because they
// carry a legacy-bootstrap hack tied to a pre-existing venues table; this
// package deliberately omits that hack and is safe for a fresh database.
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

// Up applies *.up.sql from dir in numeric-prefix order, skipping versions that
// are already recorded in schema_migrations. It uses the simple query protocol
// so each file may contain multiple statements. Running it repeatedly is a
// no-op once every file has been applied.
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

// migrationSeq extracts the leading numeric prefix ("012_foo.up.sql" → 12).
// Files without a numeric prefix sort last, preserving the historical ordering
// used by the per-service migrators.
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
