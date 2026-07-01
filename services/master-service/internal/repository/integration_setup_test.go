//go:build integration

// Postgres integration harness for the master-service repository layer.
//
// Run with:  go test -tags=integration -race -count=1 ./internal/repository/...
//
// A throwaway Postgres container is started once per package run. master-service
// owns a self-contained schema (every FK references its own tables), so the full
// chain of up-migrations in ../../migrations is applied programmatically after
// the container is ready — giving the tests the production master schema. Cases
// isolate themselves with random UUIDs / slugs rather than truncating between runs.
//
// Migrations are applied from Go (not via WithInitScripts) so the harness can
// skip skippedMigrations — see that var for the rationale.
package repository

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

var testPool *pgxpool.Pool

// skippedMigrations lists up-migrations the harness does NOT apply, keyed by
// file name with the reason. Empty: the full chain now applies cleanly from
// scratch on PostgreSQL 16. (Migrations 011/017/019 previously failed a
// from-scratch apply — see their headers for the fixes.) Kept as an explicit
// hook so a future known-broken migration can be quarantined with a documented
// reason instead of silently failing the whole package.
var skippedMigrations = map[string]string{}

// upMigrations returns every ".up.sql" file in migrationsDir sorted by name.
// The numeric prefixes are zero-padded (001..022), so lexical order equals the
// intended migration order.
func upMigrations(migrationsDir string) ([]string, error) {
	all, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
	if err != nil {
		return nil, err
	}
	ups := make([]string, 0, len(all))
	for _, p := range all {
		if strings.HasSuffix(p, ".up.sql") {
			ups = append(ups, p)
		}
	}
	sort.Strings(ups)
	return ups, nil
}

// applyMigrations runs each up-migration in order via the simple query protocol
// (PgConn().Exec), which — unlike the extended protocol pool.Exec uses — accepts
// the multi-statement SQL the migration files contain.
func applyMigrations(ctx context.Context, pool *pgxpool.Pool, ups []string) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	for _, path := range ups {
		name := filepath.Base(path)
		if reason := skippedMigrations[name]; reason != "" {
			log.Printf("integration: skipping migration %s (%s)", name, reason)
			continue
		}
		sql, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := conn.Conn().PgConn().Exec(ctx, string(sql)).ReadAll(); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

func TestMain(m *testing.M) {
	ctx := context.Background()

	_, thisFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")

	ups, err := upMigrations(migrationsDir)
	if err != nil {
		log.Fatalf("collect migrations: %v", err)
	}
	if len(ups) == 0 {
		log.Fatalf("no up-migrations found in %s", migrationsDir)
	}

	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("master_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForAll(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(300*time.Second),
				wait.ForListeningPort("5432/tcp").
					WithStartupTimeout(300*time.Second),
			),
		),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("connection string: %v", err)
	}

	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatalf("create pool: %v", err)
	}

	if err := applyMigrations(ctx, testPool, ups); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}

	code := m.Run()

	testPool.Close()
	_ = pgContainer.Terminate(ctx)
	os.Exit(code)
}

// newRepo returns the master repository under test bound to the shared pool.
func newRepo() *MasterRepo { return NewMasterRepo(testPool) }

// seedMaster inserts a minimal valid master profile in the given status and
// returns it. Each call uses fresh UUIDs and a unique slug so rows never
// collide across the shared container.
func seedMaster(ctx context.Context, t *testing.T, status string) *domain.Master {
	t.Helper()
	m := &domain.Master{
		ID:                       uuid.New(),
		UserID:                   uuid.New(),
		Slug:                     "master-" + uuid.NewString()[:12],
		DisplayName:              "Тест Мастер",
		WorkFormat:               domain.WorkFormatBoth,
		Specializations:          []string{},
		AvailabilityJSON:         "{}", // column is JSONB (migration 017); "" is invalid JSON
		PayoutVerificationStatus: domain.PayoutVerificationUnverified,
		Status:                   status,
	}
	require.NoError(t, newRepo().Insert(ctx, m))
	return m
}
