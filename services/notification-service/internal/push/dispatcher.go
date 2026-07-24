// Package push routes a notification's device tokens to the right push provider
// based on each token's platform, so the service can use different providers per
// platform (e.g. FCM today, a direct APNs client for iOS later) without the
// usecase knowing which is which.
package push

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
)

// Provider delivers to a set of tokens that all belong to one platform. It
// returns the tokens the provider reported as permanently gone (to prune) plus
// the first transport error. fcm.Sender already satisfies this.
type Provider interface {
	Push(ctx context.Context, tokens []string, n *domain.Notification) (invalid []string, err error)
}

// Dispatcher fans a mixed batch of device tokens out to per-platform providers.
// It implements usecase.Pusher. A platform with no specific provider falls back
// to fallback; tokens whose platform has neither are skipped (logged).
type Dispatcher struct {
	byPlatform map[string]Provider
	fallback   Provider
	log        zerolog.Logger
}

// NewDispatcher builds a Dispatcher. byPlatform maps a platform ("ios",
// "android", "web") to its provider; fallback (may be nil) handles any platform
// not in the map, including the empty platform.
func NewDispatcher(byPlatform map[string]Provider, fallback Provider, log zerolog.Logger) *Dispatcher {
	return &Dispatcher{byPlatform: byPlatform, fallback: fallback, log: log}
}

func (d *Dispatcher) providerFor(platform string) Provider {
	if p, ok := d.byPlatform[platform]; ok {
		return p
	}
	return d.fallback
}

// Push groups tokens by platform and sends each group through its provider,
// aggregating invalid tokens and surfacing the first provider error.
func (d *Dispatcher) Push(ctx context.Context, tokens []domain.DeviceToken, n *domain.Notification) ([]string, error) {
	byPlatform := make(map[string][]string)
	for _, t := range tokens {
		byPlatform[t.Platform] = append(byPlatform[t.Platform], t.Token)
	}

	var invalid []string
	var firstErr error
	for platform, toks := range byPlatform {
		p := d.providerFor(platform)
		if p == nil {
			d.log.Warn().Str("platform", platform).Int("count", len(toks)).
				Msg("push: no provider for platform, tokens skipped")
			continue
		}
		inv, err := p.Push(ctx, toks, n)
		invalid = append(invalid, inv...)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return invalid, firstErr
}
