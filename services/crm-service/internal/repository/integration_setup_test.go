//go:build integration

// Postgres integration harness for the crm-service repository layer.
//
// Run with:  go test -tags=integration -race -count=1 ./internal/repository/...
//
// A throwaway Postgres container is started once per package run. crm-service
// does not own the `venues` table (it FKs into venue-service's schema and waits
// for it at deploy time), so a minimal stub of venues is applied first, then
// every up migration in ../../migrations runs as an init script — giving the
// tests the production crm schema. Cases isolate themselves with random venue
// UUIDs rather than truncating between runs.
package repository

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/tienlao/agregator/services/crm-service/internal/domain"
)

var testPool *pgxpool.Pool

// crmMigrations is the ordered up-migration chain crm-service owns.
var crmMigrations = []string{
	"001_venue_staff.up.sql",
	"002_venue_crm_tasks.up.sql",
	"003_crm_tasks_lifecycle.up.sql",
	"004_crm_guest_booking_facts.up.sql",
	"005_crm_guest_profiles.up.sql",
}

// venuesStub is the minimal slice of venue-service's `venues` table the crm
// schema FKs into: only id, owner_id, created_at are referenced by repo queries.
// Named 000_ so the Postgres entrypoint runs it before the crm migrations.
const venuesStub = `CREATE TABLE IF NOT EXISTS venues (
	id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	owner_id   UUID NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`

func TestMain(m *testing.M) {
	ctx := context.Background()

	_, thisFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")

	stubPath := filepath.Join(os.TempDir(), "000_crm_venues_stub.sql")
	if err := os.WriteFile(stubPath, []byte(venuesStub), 0o600); err != nil {
		log.Fatalf("write venues stub: %v", err)
	}
	defer os.Remove(stubPath)

	initScripts := []string{stubPath}
	for _, name := range crmMigrations {
		initScripts = append(initScripts, filepath.Join(migrationsDir, name))
	}

	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("crm_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithInitScripts(initScripts...),
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

	code := m.Run()

	testPool.Close()
	_ = pgContainer.Terminate(ctx)
	os.Exit(code)
}

// seedVenue inserts a venue owned by owner and returns its id. Tests use fresh
// UUIDs per case so rows never collide across the shared container.
func seedVenue(ctx context.Context, t *testing.T, owner uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO venues (id, owner_id) VALUES ($1, $2)`, id, owner)
	require.NoError(t, err)
	return id
}

// newRepo returns the crm repository under test bound to the shared pool.
func newRepo() domain.Repository { return New(testPool) }
