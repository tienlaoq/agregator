package telegram

import (
	"context"
	"testing"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

// A notifier built without bot credentials must be disabled and must turn every
// Notify* call into a no-op (returning nil) so the gRPC layer never blocks on a
// missing Telegram integration.
func TestNotifier_DisabledNoop(t *testing.T) {
	n := NewNotifier("", "", "https://admin.example.com/")

	if n.Enabled() {
		t.Fatal("Enabled() = true for empty credentials, want false")
	}

	venue := &domain.Venue{
		Name:            "Тестовая баня",
		Type:            domain.VenueTypeBanya,
		Address:         "ул. Пушкина, 1",
		Phone:           "+70000000000",
		Description:     "описание",
		LegalEntityName: "ИП Тестов",
		INN:             "7707083893",
		Status:          domain.StatusActive,
	}

	if err := n.NotifyNewVenue(context.Background(), venue); err != nil {
		t.Errorf("NotifyNewVenue() on disabled notifier = %v, want nil", err)
	}
	if err := n.NotifyModerated(context.Background(), venue); err != nil {
		t.Errorf("NotifyModerated() on disabled notifier = %v, want nil", err)
	}
}

func TestNotifier_EnabledWithCredentials(t *testing.T) {
	n := NewNotifier("bot-token", "chat-id", "https://admin.example.com")
	if !n.Enabled() {
		t.Fatal("Enabled() = false with credentials set, want true")
	}
}
