package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

type Publisher struct {
	js nats.JetStreamContext
}

func NewPublisher(js nats.JetStreamContext) *Publisher {
	return &Publisher{js: js}
}

type paymentEvent struct {
	PaymentID string `json:"payment_id"`
	BookingID string `json:"booking_id"`
	Amount    int64  `json:"amount"`
	Status    string `json:"status"`
}

func (p *Publisher) PublishPaymentCompleted(ctx context.Context, payment *domain.Payment) error {
	return p.publish("payment.completed", payment)
}

func (p *Publisher) PublishPaymentFailed(ctx context.Context, payment *domain.Payment) error {
	return p.publish("payment.failed", payment)
}

func (p *Publisher) publish(subject string, payment *domain.Payment) error {
	data, err := json.Marshal(paymentEvent{
		PaymentID: payment.ID,
		BookingID: payment.BookingID,
		Amount:    payment.Amount,
		Status:    payment.Status,
	})
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	_, err = p.js.Publish(subject, data)
	return err
}
