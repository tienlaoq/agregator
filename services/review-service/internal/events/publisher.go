package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

// SubjectReviewCreated is the NATS subject for review-created events.
// Both the publisher and all subscribers must reference this constant so that
// a rename is a single-point change with a compile-time error on every stale use.
const SubjectReviewCreated = "review.created"

// TargetType identifies whether a review targets a venue or a master.
// Consumers can branch on this field without inspecting both VenueID and MasterID.
type TargetType string

const (
	TargetTypeVenue  TargetType = "venue"
	TargetTypeMaster TargetType = "master"
)

// ReviewCreatedEvent is the payload published to SubjectReviewCreated.
// Exactly one of VenueID and MasterID is non-empty; TargetType carries the
// same information as a stable discriminator for consumers that don't want to
// inspect both fields.
//
// Idempotency: the outbox worker publishes with at-least-once semantics —
// the same event may be delivered more than once after a crash or a failed
// commit following a successful NATS publish. Subscribers MUST deduplicate on
// ReviewID. The publisher also sets the Nats-Msg-Id header to ReviewID so
// JetStream can suppress duplicates within its configured duplicate window
// (default 2 minutes); this covers crash-loop retries but not retries that
// fall outside that window, which is why consumer-side deduplication is still
// required.
type ReviewCreatedEvent struct {
	ReviewID   string     `json:"review_id"`
	UserID     string     `json:"user_id"`
	TargetType TargetType `json:"target_type"`
	VenueID    string     `json:"venue_id,omitempty"`
	MasterID   string     `json:"master_id,omitempty"`
	Rating     int32      `json:"rating"`
}

type Publisher struct {
	js nats.JetStreamContext
}

func NewPublisher(js nats.JetStreamContext) *Publisher {
	return &Publisher{js: js}
}

func (p *Publisher) PublishReviewCreated(ctx context.Context, event ReviewCreatedEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal review.created event: %w", err)
	}
	// Nats-Msg-Id enables JetStream server-side deduplication within the stream's
	// duplicate window (default 2 min). ReviewID is the natural idempotency key —
	// the same review can only ever be created once, so reusing its ID here is safe.
	msg := &nats.Msg{
		Subject: SubjectReviewCreated,
		Data:    data,
		Header:  nats.Header{nats.MsgIdHdr: []string{event.ReviewID}},
	}
	if _, err = p.js.PublishMsg(msg, nats.Context(ctx)); err != nil {
		return fmt.Errorf("publish %s: %w", SubjectReviewCreated, err)
	}
	return nil
}
