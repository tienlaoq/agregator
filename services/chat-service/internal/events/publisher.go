package events

// Publisher wraps a JetStream context and publishes chat domain events to the
// persistent CHAT_EVENTS stream.
//
// N7: core-NATS nc.Publish is fire-and-forget — messages are lost when NATS is
// temporarily unavailable.  JetStream's js.Publish blocks until the server
// acknowledges persistence, so downstream consumers (push, email) receive every
// event even if they were offline at send time.
//
// Stream provisioning: the CHAT_EVENTS stream must exist before the first
// Publish call.  cmd/main.go calls natsutil.EnsureStream at startup.

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
)

// JetStreamPublisher implements usecase.Publisher using JetStream's at-least-once
// delivery guarantee.
type JetStreamPublisher struct {
	js nats.JetStreamContext
}

// New returns a JetStreamPublisher backed by the provided JetStreamContext.
func New(js nats.JetStreamContext) *JetStreamPublisher {
	return &JetStreamPublisher{js: js}
}

// Publish sends data to subject via JetStream and waits for server-side ack.
// ctx controls the publish deadline — if the broker is stalled, Publish returns
// when ctx is cancelled rather than blocking indefinitely.
// An error means the message was NOT persisted; the caller (usecase.publishMessageCreated)
// logs it but does not fail the SendMessage RPC — the message is already in Postgres.
func (p *JetStreamPublisher) Publish(ctx context.Context, subject string, data []byte) error {
	if _, err := p.js.PublishMsg(&nats.Msg{Subject: subject, Data: data}, nats.Context(ctx)); err != nil {
		return fmt.Errorf("jetstream publish %q: %w", subject, err)
	}
	return nil
}
