package events

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

type ReviewCreatedEvent struct {
	ReviewID string `json:"review_id"`
	UserID   string `json:"user_id"`
	VenueID  string `json:"venue_id"`
	Rating   int32  `json:"rating"`
}

type Publisher struct {
	js nats.JetStreamContext
}

func NewPublisher(js nats.JetStreamContext) *Publisher {
	return &Publisher{js: js}
}

func (p *Publisher) PublishReviewCreated(event ReviewCreatedEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal review.created event: %w", err)
	}
	_, err = p.js.Publish("review.created", data)
	if err != nil {
		return fmt.Errorf("publish review.created: %w", err)
	}
	return nil
}
