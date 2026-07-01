package events

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
)

type fakeJS struct {
	nats.JetStreamContext
	published []*nats.Msg
	pubErr    error
}

func (f *fakeJS) PublishMsg(m *nats.Msg, _ ...nats.PubOpt) (*nats.PubAck, error) {
	f.published = append(f.published, m)
	if f.pubErr != nil {
		return nil, f.pubErr
	}
	return &nats.PubAck{}, nil
}

func TestPublishReviewCreated(t *testing.T) {
	js := &fakeJS{}
	p := NewPublisher(js)

	ev := ReviewCreatedEvent{
		ReviewID:   "r1",
		UserID:     "u1",
		TargetType: TargetTypeVenue,
		VenueID:    "v1",
		Rating:     5,
	}
	if err := p.PublishReviewCreated(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(js.published) != 1 {
		t.Fatalf("published %d messages, want 1", len(js.published))
	}
	msg := js.published[0]
	if msg.Subject != SubjectReviewCreated {
		t.Errorf("subject = %q, want %q", msg.Subject, SubjectReviewCreated)
	}
	// Nats-Msg-Id must equal the review id for server-side dedup.
	if got := msg.Header.Get(nats.MsgIdHdr); got != "r1" {
		t.Errorf("Nats-Msg-Id = %q, want r1", got)
	}

	var decoded ReviewCreatedEvent
	if err := json.Unmarshal(msg.Data, &decoded); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if decoded.ReviewID != "r1" || decoded.TargetType != TargetTypeVenue || decoded.Rating != 5 {
		t.Errorf("payload mismatch: %+v", decoded)
	}
	// MasterID is omitempty and must be absent for a venue review.
	if decoded.MasterID != "" {
		t.Errorf("master_id should be empty for a venue review, got %q", decoded.MasterID)
	}
}

func TestPublishReviewCreated_ErrorPropagates(t *testing.T) {
	p := NewPublisher(&fakeJS{pubErr: errors.New("nats down")})
	err := p.PublishReviewCreated(context.Background(), ReviewCreatedEvent{ReviewID: "r1"})
	if err == nil {
		t.Fatal("expected publish error to propagate")
	}
}
