package verifymail

import (
	"context"
	"strings"
	"testing"

	pkgmail "github.com/tienlao/agregator/pkg/mail"
)

func TestEnabled(t *testing.T) {
	if (*Sender)(nil).Enabled() {
		t.Error("nil Sender must report Enabled() == false")
	}
	if New(nil, "https://app").Enabled() {
		t.Error("Sender with nil smtp must report Enabled() == false")
	}

	disabled := New(pkgmail.NewSenderFromEnv(), "https://app")
	if disabled.Enabled() {
		t.Error("Sender backed by an unconfigured smtp must report Enabled() == false")
	}
}

func TestSendVerification_GuardClauses(t *testing.T) {
	ctx := context.Background()

	t.Run("nil smtp returns not-configured error", func(t *testing.T) {
		err := New(nil, "https://app").SendVerification(ctx, "user@example.com", "tok")
		if err == nil || !strings.Contains(err.Error(), "not configured") {
			t.Fatalf("want not-configured error, got %v", err)
		}
	})

	smtp := pkgmail.NewSenderFromEnv()

	t.Run("empty recipient rejected", func(t *testing.T) {
		err := New(smtp, "https://app").SendVerification(ctx, "   ", "tok")
		if err == nil || !strings.Contains(err.Error(), "recipient") {
			t.Fatalf("want recipient error, got %v", err)
		}
	})

	t.Run("empty token rejected", func(t *testing.T) {
		err := New(smtp, "https://app").SendVerification(ctx, "user@example.com", "  ")
		if err == nil || !strings.Contains(err.Error(), "token") {
			t.Fatalf("want token error, got %v", err)
		}
	})
}
