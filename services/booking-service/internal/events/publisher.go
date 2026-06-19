package events

import (
	"context"

	"github.com/nats-io/nats.go"

	"github.com/tienlao/agregator/services/booking-service/internal/domain"
)

type Publisher struct {
	js nats.JetStreamContext
}

func NewPublisher(js nats.JetStreamContext) *Publisher {
	return &Publisher{js: js}
}

// PublishBookingCompleted публикует booking.completed напрямую. Этот путь не
// идёт через outbox — он защищён собственным механизмом completed_event_sent_at
// (см. AutoCompletePastVisits, migration 007).
func (p *Publisher) PublishBookingCompleted(ctx context.Context, b *domain.Booking) error {
	ev, err := domain.NewBookingEvent("booking.completed", b)
	if err != nil {
		return err
	}
	return p.PublishRaw(ctx, ev.Subject, ev.Payload)
}

// PublishRaw отправляет уже сериализованное тело события в subject синхронно,
// уважая дедлайн/отмену ctx. nats.Context(ctx) позволяет js.PublishMsg вернуть
// ошибку немедленно если контекст отменён — без зависания на залипшей шине.
// Используется поллером outbox (ProcessOutbox) для доставки staged-событий
// booking.created/confirmed/cancelled. См. TECH_DEBT [BOOKING-PUBLISH-LOSS].
func (p *Publisher) PublishRaw(ctx context.Context, subject string, payload []byte) error {
	_, err := p.js.PublishMsg(&nats.Msg{Subject: subject, Data: payload}, nats.Context(ctx))
	return err
}
