package dbmigrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

const (
	depWaitTimeout  = 60 * time.Second
	depWaitInterval = time.Second
)

// dependencyTables must exist before crm migrations run. crm-service shares
// venue_db with venue-service (strangler-fig phase B) and its migrations
// FK-reference venues(id); venue-service owns that table. When CRM moves to its
// own database and drops the cross-service FK, empty this list. See config.go.
var dependencyTables = []string{"venues"}

// Up applies *.up.sql from dir in numeric-prefix order, skipping versions
// already recorded in schema_migrations.
//
// Strangler-fig caveat: crm-service shares its Postgres database with
// venue-service. Because crm migrations FK-reference venues(id), and each
// service migrates itself at startup with no guaranteed ordering (compose
// gates crm on venue's service_started, not service_healthy), Up first waits
// for the dependency tables to appear before applying migrations. The SQL uses
// CREATE TABLE IF NOT EXISTS so re-applying is a safe no-op.
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

	tableExists := func(ctx context.Context, name string) (bool, error) {
		var present bool
		if err := conn.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&present); err != nil {
			return false, err
		}
		return present, nil
	}
	if err := waitForTables(ctx, log, depWaitInterval, depWaitTimeout, dependencyTables, tableExists); err != nil {
		return err
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

// waitForTables blocks until every table reported by exists is present, polling
// at interval until timeout. It returns ctx.Err() on cancellation and a
// descriptive error if a table never appears within the deadline.
func waitForTables(ctx context.Context, log zerolog.Logger, interval, timeout time.Duration, tables []string, exists func(context.Context, string) (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for _, tbl := range tables {
		for {
			ok, err := exists(ctx, tbl)
			if err != nil {
				return fmt.Errorf("check dependency table %q: %w", tbl, err)
			}
			if ok {
				break
			}
			if !time.Now().Before(deadline) {
				return fmt.Errorf("dependency table %q not present after %s: ensure venue-service migrates venue_db first", tbl, timeout)
			}
			log.Info().Str("table", tbl).Msg("waiting for dependency table (owned by venue-service)")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
		}
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
