package repository_test

// Integration tests for the repository layer.  They require a real PostgreSQL
// instance with all chat-service migrations applied.
//
// Run locally:
//
//	TEST_POSTGRES_DSN="postgres://banya:banya_dev_postgres_password@localhost:5432/chat_db?sslmode=disable" \
//	  go test ./internal/repository/... -v -count=1
//
// The tests are skipped automatically when TEST_POSTGRES_DSN is not set, so
// they never block a plain `go test ./...` in CI without a database.

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tienlao/agregator/services/chat-service/internal/repository"
)

var testRepo *repository.Repo

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		// No database available — skip all integration tests gracefully.
		os.Exit(m.Run())
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic("integration test: cannot connect to postgres: " + err.Error())
	}
	if err := pool.Ping(ctx); err != nil {
		panic("integration test: postgres ping failed: " + err.Error())
	}
	testRepo = repository.New(pool)
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// skipIfNoDB skips the calling test when no database connection is available.
func skipIfNoDB(t *testing.T) {
	t.Helper()
	if testRepo == nil {
		t.Skip("TEST_POSTGRES_DSN not set — skipping integration test")
	}
}

// TestIdempotentInsert_UnreadCount verifies the core N3 scenario:
//
//	Sending the same message twice (identical client_msg_id) must result in:
//	  1. exactly one row in chat_messages
//	  2. exactly 1 unread for the recipient (not 2)
//
// This exercises the interaction between the ON CONFLICT DO UPDATE no-op and
// the AFTER INSERT trigger fn_increment_unread — the trigger must fire only
// once because the second call becomes an UPDATE, not an INSERT.
func TestIdempotentInsert_UnreadCount(t *testing.T) {
	skipIfNoDB(t)

	ctx := context.Background()

	// ── set up: create a thread with two participants ──────────────────────────
	senderID := uuid.New()
	recipientID := uuid.New()
	refID := uuid.New()

	thread, err := testRepo.EnsureThread(ctx,
		"venue_booking", refID,
		[]string{senderID.String(), recipientID.String()},
	)
	if err != nil {
		t.Fatalf("EnsureThread: %v", err)
	}

	// ── send with a stable client_msg_id (first time) ─────────────────────────
	clientMsgID := "idempotency-test-" + uuid.NewString()
	msg1, _, err := testRepo.InsertMessage(ctx, thread.ID, senderID, "hello", clientMsgID)
	if err != nil {
		t.Fatalf("InsertMessage (1st): %v", err)
	}

	// ── retry: same client_msg_id (simulates network reconnect) ───────────────
	msg2, _, err := testRepo.InsertMessage(ctx, thread.ID, senderID, "hello", clientMsgID)
	if err != nil {
		t.Fatalf("InsertMessage (2nd / retry): %v", err)
	}

	// ── assert: both calls return the same message ID ─────────────────────────
	if msg1.ID != msg2.ID {
		t.Errorf("idempotency violated: first ID=%s, second ID=%s", msg1.ID, msg2.ID)
	}

	// ── assert: exactly 1 row in chat_messages for this thread + clientMsgID ──
	var msgCount int
	err = testRepo.Pool().QueryRow(ctx,
		`SELECT COUNT(*) FROM chat_messages
		 WHERE thread_id = $1 AND client_msg_id = $2`,
		thread.ID, clientMsgID,
	).Scan(&msgCount)
	if err != nil {
		t.Fatalf("count chat_messages: %v", err)
	}
	if msgCount != 1 {
		t.Errorf("want 1 row in chat_messages, got %d", msgCount)
	}

	// ── assert: recipient has unread_count = 1, not 2 ─────────────────────────
	var unread int
	err = testRepo.Pool().QueryRow(ctx,
		`SELECT unread_count FROM chat_thread_participants
		 WHERE thread_id = $1 AND user_id = $2`,
		thread.ID, recipientID,
	).Scan(&unread)
	if err != nil {
		t.Fatalf("query unread_count: %v", err)
	}
	if unread != 1 {
		t.Errorf("want unread_count=1 for recipient after idempotent send, got %d", unread)
	}

	// ── bonus: sender's own unread_count must stay 0 ──────────────────────────
	var senderUnread int
	err = testRepo.Pool().QueryRow(ctx,
		`SELECT unread_count FROM chat_thread_participants
		 WHERE thread_id = $1 AND user_id = $2`,
		thread.ID, senderID,
	).Scan(&senderUnread)
	if err != nil {
		t.Fatalf("query sender unread_count: %v", err)
	}
	if senderUnread != 0 {
		t.Errorf("want sender unread_count=0, got %d", senderUnread)
	}
}

// TestMarkRead_ResetsUnread verifies that MarkRead zeroes the recipient's
// unread_count and that a subsequent send increments it again from zero.
func TestMarkRead_ResetsUnread(t *testing.T) {
	skipIfNoDB(t)

	ctx := context.Background()

	senderID := uuid.New()
	recipientID := uuid.New()
	refID := uuid.New()

	thread, err := testRepo.EnsureThread(ctx,
		"venue_booking", refID,
		[]string{senderID.String(), recipientID.String()},
	)
	if err != nil {
		t.Fatalf("EnsureThread: %v", err)
	}

	// Send a message → recipient unread = 1.
	if _, _, err := testRepo.InsertMessage(ctx, thread.ID, senderID, "first", uuid.NewString()); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}

	// Recipient reads → unread should reset to 0.
	if err := testRepo.MarkRead(ctx, thread.ID, recipientID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	var unread int
	if err := testRepo.Pool().QueryRow(ctx,
		`SELECT unread_count FROM chat_thread_participants
		 WHERE thread_id = $1 AND user_id = $2`,
		thread.ID, recipientID,
	).Scan(&unread); err != nil {
		t.Fatalf("query unread_count after MarkRead: %v", err)
	}
	if unread != 0 {
		t.Errorf("want unread_count=0 after MarkRead, got %d", unread)
	}

	// Send another message → unread should become 1 again.
	if _, _, err := testRepo.InsertMessage(ctx, thread.ID, senderID, "second", uuid.NewString()); err != nil {
		t.Fatalf("InsertMessage (second): %v", err)
	}
	if err := testRepo.Pool().QueryRow(ctx,
		`SELECT unread_count FROM chat_thread_participants
		 WHERE thread_id = $1 AND user_id = $2`,
		thread.ID, recipientID,
	).Scan(&unread); err != nil {
		t.Fatalf("query unread_count after second send: %v", err)
	}
	if unread != 1 {
		t.Errorf("want unread_count=1 after second send, got %d", unread)
	}
}
