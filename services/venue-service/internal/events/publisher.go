package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

type Publisher struct {
	js nats.JetStreamContext
}

func NewPublisher(js nats.JetStreamContext) *Publisher {
	return &Publisher{js: js}
}

func (p *Publisher) VenueCreated(ctx context.Context, venue *domain.Venue) error {
	return p.publish(ctx, "venue.created", venue)
}

func (p *Publisher) VenueUpdated(ctx context.Context, venue *domain.Venue) error {
	return p.publish(ctx, "venue.updated", venue)
}

// VenueManagementAlert — событие для интеграций (почта, пуши): приостановка и т.п.
type VenueManagementAlert struct {
	Type              string   `json:"type"` // suspended | resumed
	VenueID           string   `json:"venue_id"`
	VenueName         string   `json:"venue_name"`
	OwnerID           string   `json:"owner_id"`
	RecipientUserIDs  []string `json:"recipient_user_ids"`
	ModerationComment string   `json:"moderation_comment"`
}

func (p *Publisher) PublishVenueManagementAlert(ctx context.Context, alert VenueManagementAlert) error {
	return p.publish(ctx, "venue.management.alert", alert)
}

func (p *Publisher) publish(ctx context.Context, subject string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if _, err = p.js.PublishMsg(&nats.Msg{Subject: subject, Data: data}, nats.Context(ctx)); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}
