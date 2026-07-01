package mock

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

func TestIsMockMode(t *testing.T) {
	if !NewClient().IsMockMode() {
		t.Error("mock provider must report IsMockMode() == true")
	}
}

func TestCreatePayment_SyntheticIDAndURL(t *testing.T) {
	res, err := NewClient().CreatePayment(context.Background(), provider.CreateRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProviderPaymentID == "" {
		t.Error("ProviderPaymentID must be non-empty")
	}
	if !strings.Contains(res.ConfirmationURL, res.ProviderPaymentID) {
		t.Errorf("ConfirmationURL %q must embed the payment id", res.ConfirmationURL)
	}
}

func TestNoOpMethods(t *testing.T) {
	c := NewClient()
	ctx := context.Background()
	if err := c.Capture(ctx, "p", "k"); err != nil {
		t.Errorf("Capture should be a no-op, got %v", err)
	}
	if err := c.Refund(ctx, "p", 100, "reason"); err != nil {
		t.Errorf("Refund should be a no-op, got %v", err)
	}
	if err := c.VerifySignature(ctx, []byte("{}"), http.Header{}); err != nil {
		t.Errorf("VerifySignature should accept everything, got %v", err)
	}
}

func TestParseWebhook(t *testing.T) {
	c := NewClient()
	ctx := context.Background()

	t.Run("invalid json", func(t *testing.T) {
		if _, err := c.ParseWebhook(ctx, []byte("{bad"), nil); err == nil {
			t.Error("expected error for malformed body")
		}
	})
	t.Run("missing id", func(t *testing.T) {
		if _, err := c.ParseWebhook(ctx, []byte(`{"object":{"status":"succeeded"}}`), nil); err == nil {
			t.Error("expected error when payment id is absent")
		}
	})

	tests := []struct {
		name        string
		body        string
		wantStatus  domain.PaymentStatus
		wantCapture bool
	}{
		{"succeeded", `{"event":"payment.succeeded","object":{"id":"p1","status":"succeeded"}}`, domain.StatusSucceeded, false},
		{"canceled spelling", `{"object":{"id":"p1","status":"canceled"}}`, domain.StatusCancelled, false},
		{"cancelled spelling", `{"object":{"id":"p1","status":"cancelled"}}`, domain.StatusCancelled, false},
		{"unknown status is pending", `{"object":{"id":"p1","status":"weird"}}`, domain.StatusPending, false},
		{"waiting_for_capture", `{"object":{"id":"p1","status":"waiting_for_capture"}}`, domain.StatusPending, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, err := c.ParseWebhook(ctx, []byte(tt.body), nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ev.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", ev.Status, tt.wantStatus)
			}
			if ev.RequiresCapture != tt.wantCapture {
				t.Errorf("RequiresCapture = %v, want %v", ev.RequiresCapture, tt.wantCapture)
			}
			if tt.wantCapture && ev.CaptureKey == "" {
				t.Error("CaptureKey must be set when RequiresCapture is true")
			}
		})
	}
}

func TestParsePayoutWebhook(t *testing.T) {
	c := NewClient()
	ctx := context.Background()

	t.Run("non-payout event falls back to nil,nil", func(t *testing.T) {
		ev, err := c.ParsePayoutWebhook(ctx, []byte(`{"event":"payment.succeeded","object":{"id":"p1"}}`), nil)
		if err != nil || ev != nil {
			t.Fatalf("want (nil,nil) for a non-payout event, got (%v,%v)", ev, err)
		}
	})
	t.Run("invalid json", func(t *testing.T) {
		if _, err := c.ParsePayoutWebhook(ctx, []byte("{bad"), nil); err == nil {
			t.Error("expected error for malformed body")
		}
	})
	t.Run("payout missing id", func(t *testing.T) {
		if _, err := c.ParsePayoutWebhook(ctx, []byte(`{"event":"payout.succeeded","object":{}}`), nil); err == nil {
			t.Error("expected error when payout id is absent")
		}
	})

	tests := []struct {
		name string
		body string
		want provider.PayoutStatus
	}{
		{"succeeded", `{"event":"payout.succeeded","object":{"id":"po1","status":"succeeded"}}`, provider.PayoutStatusSucceeded},
		{"canceled maps to failed", `{"event":"payout.failed","object":{"id":"po1","status":"canceled"}}`, provider.PayoutStatusFailed},
		{"unknown maps to pending", `{"event":"payout.update","object":{"id":"po1","status":"weird"}}`, provider.PayoutStatusPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, err := c.ParsePayoutWebhook(ctx, []byte(tt.body), nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ev == nil {
				t.Fatal("expected a payout event")
			}
			if ev.Status != tt.want {
				t.Errorf("Status = %q, want %q", ev.Status, tt.want)
			}
		})
	}
}
