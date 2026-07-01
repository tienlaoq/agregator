//go:build integration

// Postgres integration harness for the review-service repository layer.
//
// Run with:  go test -tags=integration -race -count=1 ./internal/repository/...
//
// A throwaway Postgres container is started once per package run; the full chain
// of up-migrations in ../../migrations is applied as init scripts. The schema is
// self-contained. Cases isolate themselves with fresh UUIDs.
package repository

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/tienlao/agregator/services/review-service/internal/domain"
)

var testPool *pgxpool.Pool

func upMigrations(dir string) ([]string, error) {
	all, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
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

func TestMain(m *testing.M) {
	ctx := context.Background()

	_, thisFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")

	initScripts, err := upMigrations(migrationsDir)
	if err != nil {
		log.Fatalf("collect migrations: %v", err)
	}
	if len(initScripts) == 0 {
		log.Fatalf("no up-migrations found in %s", migrationsDir)
	}

	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("review_test"),
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

func newRepo() *ReviewRepo { return NewReviewRepo(testPool) }

// createVenueReview runs the full review+rating-cache transaction the usecase
// performs and returns the inserted review.
func createVenueReview(ctx context.Context, t *testing.T, venueID, userID string, rating int32) *domain.Review {
	t.Helper()
	repo := newRepo()
	rev := &domain.Review{
		UserID: userID, UserName: "Гость", VenueID: venueID, Rating: rating, Text: "ок",
	}
	tx, err := repo.BeginTx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	require.NoError(t, repo.CreateTx(ctx, tx, rev))
	require.NoError(t, repo.UpdateVenueRatingTx(ctx, tx, venueID, rating))
	require.NoError(t, tx.Commit(ctx))
	require.NotEmpty(t, rev.ID)
	return rev
}

func createMasterReview(ctx context.Context, t *testing.T, masterID, userID string, rating int32) *domain.Review {
	t.Helper()
	repo := newRepo()
	rev := &domain.Review{
		UserID: userID, UserName: "Гость", MasterID: masterID, Rating: rating, Text: "ок",
	}
	tx, err := repo.BeginTx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	require.NoError(t, repo.CreateTx(ctx, tx, rev))
	require.NoError(t, repo.UpdateMasterRatingTx(ctx, tx, masterID, rating))
	require.NoError(t, tx.Commit(ctx))
	require.NotEmpty(t, rev.ID)
	return rev
}
