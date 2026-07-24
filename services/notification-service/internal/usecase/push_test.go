package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
)

type mockPusher struct {
	gotTokens []domain.DeviceToken
	invalid   []string
	err       error
}

func (m *mockPusher) Push(_ context.Context, tokens []domain.DeviceToken, _ *domain.Notification) ([]string, error) {
	m.gotTokens = tokens
	return m.invalid, m.err
}

func newPushUC(repo domain.Repository, p Pusher) *NotificationUseCase {
	return New(repo, p, zerolog.Nop())
}

func TestDeliverPush_PrunesInvalidTokens(t *testing.T) {
	var pruned []string
	repo := &mockRepo{
		ListDeviceTokensFunc: func(context.Context, uuid.UUID) ([]domain.DeviceToken, error) {
			return []domain.DeviceToken{{Token: "good", Platform: "android"}, {Token: "dead", Platform: "android"}}, nil
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
		ListDeviceTokensFunc: func(context.Context, uuid.UUID) ([]domain.DeviceToken, error) { return nil, nil },
	}
	pusher := &mockPusher{}
	uc := newPushUC(repo, pusher)

	uc.deliverPush(context.Background(), &domain.Notification{UserID: uuid.New()})

	if pusher.gotTokens != nil {
		t.Fatalf("pusher called with %v, want no call", pusher.gotTokens)
	}
}

// blockingPusher blocks in Push until release is closed, to exercise WaitPush's
// timeout path.
type blockingPusher struct{ release chan struct{} }

func (b *blockingPusher) Push(_ context.Context, _ []domain.DeviceToken, _ *domain.Notification) ([]string, error) {
	<-b.release
	return nil, nil
}

func TestWaitPush_DrainsThenTimesOut(t *testing.T) {
	repo := &mockRepo{
		ListDeviceTokensFunc: func(context.Context, uuid.UUID) ([]domain.DeviceToken, error) {
			return []domain.DeviceToken{{Token: "tok", Platform: "android"}}, nil
		},
	}

	// Clean drain: a fast pusher finishes well within the wait.
	uc := newPushUC(repo, &mockPusher{})
	if _, err := uc.Create(context.Background(), uuid.New(), "t", "hi", "", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !uc.WaitPush(2 * time.Second) {
		t.Fatal("WaitPush should drain the fast push cleanly")
	}

	// Timeout: a blocking pusher keeps the goroutine in flight past the deadline.
	bp := &blockingPusher{release: make(chan struct{})}
	uc = newPushUC(repo, bp)
	if _, err := uc.Create(context.Background(), uuid.New(), "t", "hi", "", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if uc.WaitPush(50 * time.Millisecond) {
		t.Fatal("WaitPush should time out while a push is blocked")
	}
	close(bp.release) // let the goroutine finish so it doesn't outlive the test
	uc.WaitPush(2 * time.Second)
}

func TestRegisterDevice_Validation(t *testing.T) {
	uc := newTestUC(&mockRepo{})
	if err := uc.RegisterDevice(context.Background(), uuid.Nil, "tok", "ios"); err == nil {
		t.Fatal("expected error for nil user_id")
	}
	if err := uc.RegisterDevice(context.Background(), uuid.New(), "   ", "ios"); err == nil {
		t.Fatal("expected error for empty token")
	}

	if err := newTestUC(&mockRepo{}).RegisterDevice(context.Background(), uuid.New(), "tok", "windows"); err == nil {
		t.Fatal("expected error for unknown platform")
	}

	var saved, savedPlatform string
	repo := &mockRepo{SaveDeviceTokenFunc: func(_ context.Context, _ uuid.UUID, token, platform string) error {
		saved, savedPlatform = token, platform
		return nil
	}}
	if err := newTestUC(repo).RegisterDevice(context.Background(), uuid.New(), "  abc ", " IOS "); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved != "abc" {
		t.Fatalf("saved token = %q, want trimmed 'abc'", saved)
	}
	if savedPlatform != "ios" {
		t.Fatalf("saved platform = %q, want normalized 'ios'", savedPlatform)
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
