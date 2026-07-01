package dbmigrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

// TestUp_EarlyGuards covers the input validation that runs before any database
// connection: a missing/invalid migrations directory and an unparseable DSN.
func TestUp_EarlyGuards(t *testing.T) {
	ctx := context.Background()
	log := zerolog.Nop()

	t.Run("missing dir", func(t *testing.T) {
		err := Up(ctx, "postgres://u:p@localhost:5432/db", filepath.Join(t.TempDir(), "nope"), log)
		if err == nil {
			t.Fatal("expected error for missing migrations dir")
		}
	})

	t.Run("path is a file not a dir", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "f.sql")
		if err := os.WriteFile(file, []byte("SELECT 1;"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Up(ctx, "postgres://u:p@localhost:5432/db", file, log); err == nil {
			t.Fatal("expected error when path is a file")
		}
	})

	t.Run("invalid dsn", func(t *testing.T) {
		if err := Up(ctx, "://not-a-dsn", t.TempDir(), log); err == nil {
			t.Fatal("expected error for invalid dsn")
		}
	})
}
