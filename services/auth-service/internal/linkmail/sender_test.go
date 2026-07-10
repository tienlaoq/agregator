package linkmail

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

	// NewSenderFromEnv with no SMTP_* set yields a non-nil but disabled sender.
	disabled := New(pkgmail.NewSenderFromEnv(), "https://app")
	if disabled.Enabled() {
		t.Error("Sender backed by an unconfigured smtp must report Enabled() == false")
	}
}

// sendFunc lets the guard-clause table exercise both link emails through one
// path: they share the same validation, so the same cases apply to each.
func TestSend_GuardClauses(t *testing.T) {
	ctx := context.Background()

	sends := map[string]func(s *Sender, ctx context.Context, to, tok string) error{
		"SendPasswordReset": (*Sender).SendPasswordReset,
		"SendVerification":  (*Sender).SendVerification,
	}

	for name, send := range sends {
		t.Run(name, func(t *testing.T) {
			t.Run("nil smtp returns not-configured error", func(t *testing.T) {
				err := send(New(nil, "https://app"), ctx, "user@example.com", "tok")
				if err == nil || !strings.Contains(err.Error(), "not configured") {
					t.Fatalf("want not-configured error, got %v", err)
				}
			})

			// A non-nil (but unconfigured) smtp reaches the input-validation
			// branches, which return before any network is touched.
			smtp := pkgmail.NewSenderFromEnv()

			t.Run("empty recipient rejected", func(t *testing.T) {
				err := send(New(smtp, "https://app"), ctx, "   ", "tok")
				if err == nil || !strings.Contains(err.Error(), "recipient") {
					t.Fatalf("want recipient error, got %v", err)
				}
			})

			t.Run("empty token rejected", func(t *testing.T) {
				err := send(New(smtp, "https://app"), ctx, "user@example.com", "  ")
				if err == nil || !strings.Contains(err.Error(), "token") {
					t.Fatalf("want token error, got %v", err)
				}
			})
		})
	}
}
