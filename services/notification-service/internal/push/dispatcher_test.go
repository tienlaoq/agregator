package push

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
)

// recordingProvider records the tokens it received and returns canned results.
type recordingProvider struct {
	got     []string
	invalid []string
	err     error
}

func (p *recordingProvider) Push(_ context.Context, tokens []string, _ *domain.Notification) ([]string, error) {
	p.got = append(p.got, tokens...)
	return p.invalid, p.err
}

func dt(token, platform string) domain.DeviceToken {
	return domain.DeviceToken{Token: token, Platform: platform}
}

func TestDispatcher_RoutesByPlatform(t *testing.T) {
	ios := &recordingProvider{invalid: []string{"ios-dead"}}
	fallback := &recordingProvider{}
	d := NewDispatcher(map[string]Provider{"ios": ios}, fallback, zerolog.Nop())

	invalid, err := d.Push(context.Background(), []domain.DeviceToken{
		dt("ios-1", "ios"),
		dt("ios-dead", "ios"),
		dt("and-1", "android"), // no specific provider -> fallback
		dt("web-1", "web"),     // fallback
		dt("empty", ""),        // fallback
	}, &domain.Notification{})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	if len(ios.got) != 2 {
		t.Fatalf("ios provider got %v, want 2 tokens", ios.got)
	}
	if len(fallback.got) != 3 {
		t.Fatalf("fallback got %v, want 3 tokens", fallback.got)
	}
	if len(invalid) != 1 || invalid[0] != "ios-dead" {
		t.Fatalf("invalid = %v, want [ios-dead]", invalid)
	}
}

func TestDispatcher_NoProviderSkips(t *testing.T) {
	// No fallback: android tokens have nowhere to go and must be skipped, not
	// crash. ios still delivered.
	ios := &recordingProvider{}
	d := NewDispatcher(map[string]Provider{"ios": ios}, nil, zerolog.Nop())

	invalid, err := d.Push(context.Background(), []domain.DeviceToken{
		dt("ios-1", "ios"),
		dt("and-1", "android"),
	}, &domain.Notification{})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if len(ios.got) != 1 || ios.got[0] != "ios-1" {
		t.Fatalf("ios got %v, want [ios-1]", ios.got)
	}
	if len(invalid) != 0 {
		t.Fatalf("invalid = %v, want none", invalid)
	}
}

func TestDispatcher_SurfacesFirstError(t *testing.T) {
	boom := errors.New("boom")
	d := NewDispatcher(map[string]Provider{
		"ios": &recordingProvider{err: boom},
	}, &recordingProvider{}, zerolog.Nop())

	_, err := d.Push(context.Background(), []domain.DeviceToken{dt("ios-1", "ios")}, &domain.Notification{})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}
