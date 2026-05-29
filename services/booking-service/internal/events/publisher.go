package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"

	"github.com/tienlao/agregator/services/booking-service/internal/domain"
)

type Publisher struct {
	js nats.JetStreamContext
}

func NewPublisher(js nats.JetStreamContext) *Publisher {
	return &Publisher{js: js}
}

type bookingEvent struct {
	BookingID string `json:"booking_id"`
	UserID    string `json:"user_id"`
	VenueID   string `json:"venue_id"`
	Status    string `json:"status"`
}

func (p *Publisher) PublishBookingCreated(ctx context.Context, b *domain.Booking) error {
	return p.publish(ctx, "booking.created", b)
}

func (p *Publisher) PublishBookingConfirmed(ctx context.Context, b *domain.Booking) error {
	return p.publish(ctx, "booking.confirmed", b)
}

func (p *Publisher) PublishBookingCancelled(ctx context.Context, b *domain.Booking) error {
	return p.publish(ctx, "booking.cancelled", b)
}

func (p *Publisher) PublishBookingCompleted(ctx context.Context, b *domain.Booking) error {
	return p.publish(ctx, "booking.completed", b)
}

// publish отправляет событие в JetStream синхронно, уважая дедлайн/отмену ctx.
// nats.Context(ctx) позволяет js.PublishMsg вернуть ошибку немедленно если
// контекст отменён или истёк — без зависания на залипшей шине.
func (p *Publisher) publish(ctx context.Context, subject string, b *domain.Booking) error {
	data, err := json.Marshal(bookingEvent{
		BookingID: b.ID,
		UserID:    b.UserID,
		VenueID:   b.VenueID,
		Status:    string(b.Status),
	})
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	_, err = p.js.PublishMsg(&nats.Msg{Subject: subject, Data: data}, nats.Context(ctx))
	return err
}
