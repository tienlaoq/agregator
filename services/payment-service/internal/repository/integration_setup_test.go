//go:build integration

// Postgres integration harness for the repository layer.
//
// Run with:  go test -tags=integration -race -count=1 ./internal/repository/...
//
// A throwaway Postgres container is started once per package run; every up
// migration in ../../migrations is applied as an init script so the schema the
// tests exercise is byte-for-byte the production schema. Tests share the
// container and isolate themselves with random UUID partner ids rather than
// truncating between cases — the only cross-partner query
// (PartnersWithAvailableBalance) asserts membership, never exact counts.
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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

var testPool *pgxpool.Pool

// migrationScripts is the full ordered up-migration chain. The ledger and
// payout tables FK back to payments, so the whole chain must run, not just the
// money-circuit migrations (011-013).
var migrationScripts = []string{
	"001_init.up.sql",
	"002_provider_id_index.up.sql",
	"003_split_counterparty.up.sql",
	"004_status_refunded.up.sql",
	"005_provider_abstraction.up.sql",
	"006_outbox.up.sql",
	"007_provider_id_unique.up.sql",
	"008_seller_account_id_constraints.up.sql",
	"009_drop_marketplace_split.up.sql",
	"010_payment_service_at.up.sql",
	"011_partner_payout_method.up.sql",
	"012_partner_ledger.up.sql",
	"013_payouts.up.sql",
}

func TestMain(m *testing.M) {
	ctx := context.Background()

	_, thisFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
	initScripts := make([]string, len(migrationScripts))
	for i, name := range migrationScripts {
		initScripts[i] = filepath.Join(migrationsDir, name)
	}

	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("payment_test"),
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

// newPartnerID returns a fresh UUID string usable as a partner_id (all three
// money tables type partner_id as UUID).
func newPartnerID() string { return uuid.NewString() }

// statusCode unwraps the gRPC status code the repository attaches via pkg/errors.
func statusCode(err error) codes.Code { return status.Code(err) }

// seedPayment inserts a minimal succeeded payment and returns its id, so a
// ledger accrual can satisfy the partner_ledger.payment_id FK.
func seedPayment(ctx context.Context, t *testing.T, amountKopecks int64) string {
	t.Helper()
	var id string
	err := testPool.QueryRow(ctx, `
		INSERT INTO payments (booking_id, amount, status)
		VALUES (gen_random_uuid(), $1, 'succeeded')
		RETURNING id::text`, amountKopecks).Scan(&id)
	require.NoError(t, err)
	return id
}

// seedAccrual appends one accrual for the partner, creating the backing payment
// row first. availableAt controls the hold window: a past value lands in the
// available balance, a future value is held.
func seedAccrual(ctx context.Context, t *testing.T, repo *LedgerRepo, pt domain.PartnerType, partnerID string, amount int64, availableAt time.Time) {
	t.Helper()
	e := &domain.LedgerEntry{
		PartnerType:   pt,
		PartnerID:     partnerID,
		EntryType:     domain.LedgerAccrual,
		AmountKopecks: amount,
		PaymentID:     seedPayment(ctx, t, amount),
		AvailableAt:   availableAt,
	}
	require.NoError(t, repo.AppendAccrual(ctx, e))
}

// seedCardMethod stores an active card payout method and returns its id for use
// as payouts.method_id (FK -> partner_payout_methods).
func seedCardMethod(ctx context.Context, t *testing.T, pt domain.PartnerType, partnerID string) string {
	t.Helper()
	m := &domain.PayoutMethod{
		PartnerType:   pt,
		PartnerID:     partnerID,
		Kind:          domain.PayoutMethodCard,
		ProviderName:  "mock",
		CardLast4:     "4242",
		CardBrand:     "visa",
		ProviderToken: "tok_" + uuid.NewString(),
	}
	require.NoError(t, NewPayoutMethodRepo(testPool).Save(ctx, m))
	return m.ID
}
