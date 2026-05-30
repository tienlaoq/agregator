package dbmigrate

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestWaitForTables_NoDependencies(t *testing.T) {
	called := false
	exists := func(context.Context, string) (bool, error) { called = true; return false, nil }
	if err := waitForTables(context.Background(), zerolog.Nop(), time.Millisecond, time.Second, nil, exists); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("exists must not be called when there are no dependency tables")
	}
}

func TestWaitForTables_AppearsAfterRetries(t *testing.T) {
	var calls int32
	exists := func(context.Context, string) (bool, error) {
		return atomic.AddInt32(&calls, 1) >= 3, nil
	}
	err := waitForTables(context.Background(), zerolog.Nop(), time.Millisecond, time.Second, []string{"venues"}, exists)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected 3 polls, got %d", got)
	}
}

func TestWaitForTables_Timeout(t *testing.T) {
	exists := func(context.Context, string) (bool, error) { return false, nil }
	err := waitForTables(context.Background(), zerolog.Nop(), time.Millisecond, 15*time.Millisecond, []string{"venues"}, exists)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "not present after") {
		t.Fatalf("expected timeout message, got: %v", err)
	}
}

func TestWaitForTables_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	exists := func(context.Context, string) (bool, error) { return false, nil }
	err := waitForTables(ctx, zerolog.Nop(), time.Second, time.Minute, []string{"venues"}, exists)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestWaitForTables_ExistsError(t *testing.T) {
	boom := errors.New("boom")
	exists := func(context.Context, string) (bool, error) { return false, boom }
	err := waitForTables(context.Background(), zerolog.Nop(), time.Millisecond, time.Second, []string{"venues"}, exists)
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped boom, got: %v", err)
	}
}
