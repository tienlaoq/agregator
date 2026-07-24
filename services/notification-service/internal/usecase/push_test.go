package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
)

type mockPusher struct {
	gotTokens []string
	invalid   []string
	err       error
}

func (m *mockPusher) Push(_ context.Context, tokens []string, _ *domain.Notification) ([]string, error) {
	m.gotTokens = tokens
	return m.invalid, m.err
}

func newPushUC(repo domain.Repository, p Pusher) *NotificationUseCase {
	return New(repo, p, zerolog.Nop())
}

func TestDeliverPush_PrunesInvalidTokens(t *testing.T) {
	var pruned []string
	repo := &mockRepo{
		ListDeviceTokensFunc: func(context.Context, uuid.UUID) ([]string, error) {
			return []string{"good", "dead"}, nil
		},
		DeleteDeviceTokensFunc: func(_ context.Context, tokens []string) error {
			pruned = tokens
			return nil
		},
	}
	pusher := &mockPusher{invalid: []string{"dead"}}
	uc := newPushUC(repo, pusher)

	uc.deliverPush(context.Background(), &domain.Notification{UserID: uuid.New(), Title: "t"})

	if len(pusher.gotTokens) != 2 {
		t.Fatalf("pusher got %v, want 2 tokens", pusher.gotTokens)
	}
	if len(pruned) != 1 || pruned[0] != "dead" {
		t.Fatalf("pruned = %v, want [dead]", pruned)
	}
}

func TestDeliverPush_NoTokensSkipsPush(t *testing.T) {
	repo := &mockRepo{
		ListDeviceTokensFunc: func(context.Context, uuid.UUID) ([]string, error) { return nil, nil },
	}
	pusher := &mockPusher{}
	uc := newPushUC(repo, pusher)

	uc.deliverPush(context.Background(), &domain.Notification{UserID: uuid.New()})

	if pusher.gotTokens != nil {
		t.Fatalf("pusher called with %v, want no call", pusher.gotTokens)
	}
}

func TestRegisterDevice_Validation(t *testing.T) {
	uc := newTestUC(&mockRepo{})
	if err := uc.RegisterDevice(context.Background(), uuid.Nil, "tok", "ios"); err == nil {
		t.Fatal("expected error for nil user_id")
	}
	if err := uc.RegisterDevice(context.Background(), uuid.New(), "   ", "ios"); err == nil {
		t.Fatal("expected error for empty token")
	}

	var saved string
	repo := &mockRepo{SaveDeviceTokenFunc: func(_ context.Context, _ uuid.UUID, token, _ string) error {
		saved = token
		return nil
	}}
	if err := newTestUC(repo).RegisterDevice(context.Background(), uuid.New(), "  abc ", "ios"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved != "abc" {
		t.Fatalf("saved token = %q, want trimmed 'abc'", saved)
	}
}

func TestUnregisterDevice_Validation(t *testing.T) {
	uc := newTestUC(&mockRepo{})
	if err := uc.UnregisterDevice(context.Background(), uuid.New(), ""); err == nil {
		t.Fatal("expected error for empty token")
	}
	repo := &mockRepo{DeleteDeviceTokenFunc: func(context.Context, uuid.UUID, string) error {
		return errors.New("boom")
	}}
	if err := newTestUC(repo).UnregisterDevice(context.Background(), uuid.New(), "tok"); err == nil {
		t.Fatal("expected repo error to propagate")
	}
}
