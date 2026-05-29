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

// Up applies *.up.sql from dir in numeric-prefix order, skipping already recorded versions.
// Uses simple query protocol so each file may contain multiple statements.
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

	if err := bootstrapExistingDB(ctx, conn, dir, log); err != nil {
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

// firstPostLegacyMigration is the cutoff for the "legacy DB bootstrap" hack:
// when the database already has `venues` but `schema_migrations` was empty,
// every migration with a numeric prefix < this value is recorded as
// already-applied without being executed.
//
// Historically this was named firstCRMStaffMigration (= 12, the first staff
// schema). After the CRM extraction (migrations 012/013 moved to crm-service),
// 11 is still the highest legacy-applied prefix in venue-service, so the
// cutoff stays at 12 — only the name was generalised.
const firstPostLegacyMigration = 12

func bootstrapExistingDB(ctx context.Context, conn *pgx.Conn, dir string, log zerolog.Logger) error {
	var n int64
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		return fmt.Errorf("count schema_migrations: %w", err)
	}
	if n > 0 {
		return nil
	}
	var venuesExists bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'venues'
		)`).Scan(&venuesExists); err != nil {
		return fmt.Errorf("check venues table: %w", err)
	}
	if !venuesExists {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("bootstrap readdir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".up.sql") && migrationSeq(name) < firstPostLegacyMigration {
			files = append(files, name)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return migrationSeq(files[i]) < migrationSeq(files[j])
	})
	for _, name := range files {
		if _, err := conn.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING`, name); err != nil {
			return fmt.Errorf("bootstrap record %s: %w", name, err)
		}
	}
	log.Info().Int("count", len(files)).Msg("bootstrapped schema_migrations for pre-existing venue DB")
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
