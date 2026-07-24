package repository

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
)

// TestRepo_Integration exercises every query in queries.go against a real
// Postgres. It is skipped unless NOTIFICATION_TEST_DSN points at a migrated
// notification_db, e.g.:
//
//	NOTIFICATION_TEST_DSN='postgres://banya:...@localhost:5432/notification_db' \
//	  go test ./internal/repository/ -run Integration -v
//
// Each run uses a fresh random user_id and cleans up after itself, so it is
// safe to run repeatedly against a shared dev database.
func TestRepo_Integration(t *testing.T) {
	dsn := os.Getenv("NOTIFICATION_TEST_DSN")
	if dsn == "" {
		t.Skip("NOTIFICATION_TEST_DSN not set; skipping DB integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	r := New(pool)
	userID := uuid.New()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM notifications WHERE user_id = $1", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM device_tokens WHERE user_id = $1", userID)
	})

	if err := r.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	// Create round-trips all columns, including the optional data payload.
	created, err := r.Create(ctx, &domain.Notification{
		UserID: userID, Type: "booking", Title: "Привет", Body: "body", Data: `{"k":"v"}`,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created == nil || created.ID == uuid.Nil {
		t.Fatal("Create returned no populated row")
	}
	if created.Type != "booking" || created.Title != "Привет" || created.Data != `{"k":"v"}` || created.ReadAt != nil {
		t.Fatalf("Create round-trip mismatch: %+v", created)
	}

	if c, err := r.UnreadCount(ctx, userID); err != nil || c != 1 {
		t.Fatalf("UnreadCount after create = %d, err %v; want 1", c, err)
	}

	// List: all + unread-only both return the row; total is correct.
	list, total, err := r.List(ctx, userID, 10, 0, false)
	if err != nil || total != 1 || len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("List(all) = %d rows total=%d err=%v", len(list), total, err)
	}
	unreadList, unreadTotal, err := r.List(ctx, userID, 10, 0, true)
	if err != nil || unreadTotal != 1 || len(unreadList) != 1 {
		t.Fatalf("List(unread) = %d rows total=%d err=%v", len(unreadList), unreadTotal, err)
	}

	// MarkRead flips the row and is reflected in the count + unread filter.
	ok, err := r.MarkRead(ctx, userID, created.ID)
	if err != nil || !ok {
		t.Fatalf("MarkRead = %v, err %v; want true", ok, err)
	}
	if c, err := r.UnreadCount(ctx, userID); err != nil || c != 0 {
		t.Fatalf("UnreadCount after mark = %d, err %v; want 0", c, err)
	}
	if l, _, err := r.List(ctx, userID, 10, 0, true); err != nil || len(l) != 0 {
		t.Fatalf("List(unread) after mark = %d rows err %v; want 0", len(l), err)
	}
	// MarkRead again is a no-op (already read).
	if ok, err := r.MarkRead(ctx, userID, created.ID); err != nil || ok {
		t.Fatalf("MarkRead(second) = %v, err %v; want false", ok, err)
	}

	// MarkAllRead clears a fresh unread notification.
	if _, err := r.Create(ctx, &domain.Notification{UserID: userID, Type: "t", Title: "hi"}); err != nil {
		t.Fatalf("Create #2: %v", err)
	}
	if err := r.MarkAllRead(ctx, userID); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	if c, err := r.UnreadCount(ctx, userID); err != nil || c != 0 {
		t.Fatalf("UnreadCount after mark-all = %d, err %v; want 0", c, err)
	}

	// Device tokens: upsert, list, single delete, batch delete.
	if err := r.SaveDeviceToken(ctx, userID, "tokA", "ios"); err != nil {
		t.Fatalf("SaveDeviceToken A: %v", err)
	}
	if err := r.SaveDeviceToken(ctx, userID, "tokB", "android"); err != nil {
		t.Fatalf("SaveDeviceToken B: %v", err)
	}
	// Re-save A (ON CONFLICT upsert path) must not create a duplicate.
	if err := r.SaveDeviceToken(ctx, userID, "tokA", "web"); err != nil {
		t.Fatalf("SaveDeviceToken A upsert: %v", err)
	}
	toks, err := r.ListDeviceTokens(ctx, userID)
	if err != nil || len(toks) != 2 {
		t.Fatalf("ListDeviceTokens = %v err %v; want 2", toks, err)
	}
	if err := r.DeleteDeviceToken(ctx, userID, "tokA"); err != nil {
		t.Fatalf("DeleteDeviceToken: %v", err)
	}
	if toks, _ := r.ListDeviceTokens(ctx, userID); len(toks) != 1 || toks[0].Token != "tokB" || toks[0].Platform != "android" {
		t.Fatalf("after delete = %v; want [{tokB android}]", toks)
	}
	if err := r.DeleteDeviceTokens(ctx, []string{"tokB"}); err != nil {
		t.Fatalf("DeleteDeviceTokens: %v", err)
	}
	if toks, _ := r.ListDeviceTokens(ctx, userID); len(toks) != 0 {
		t.Fatalf("after batch delete = %v; want empty", toks)
	}
}
