package events

import (
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

func (p *Publisher) VenueCreated(venue *domain.Venue) error {
	return p.publish("venue.created", venue)
}

func (p *Publisher) VenueUpdated(venue *domain.Venue) error {
	return p.publish("venue.updated", venue)
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

func (p *Publisher) VenueManagementAlert(alert VenueManagementAlert) error {
	return p.publish("venue.management.alert", alert)
}

func (p *Publisher) publish(subject string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	_, err = p.js.Publish(subject, data)
	if err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}
