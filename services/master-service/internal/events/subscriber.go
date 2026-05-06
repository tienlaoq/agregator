package events

import (
	"context"
	"encoding/json"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/master-service/internal/usecase"
)

type paymentEvent struct {
	PaymentID string `json:"payment_id"`
	BookingID string `json:"booking_id"`
	Status    string `json:"status"`
}

type Subscriber struct {
	js  nats.JetStreamContext
	uc  *usecase.MasterUseCase
	log zerolog.Logger
}

func NewSubscriber(js nats.JetStreamContext, uc *usecase.MasterUseCase, log zerolog.Logger) *Subscriber {
	return &Subscriber{js: js, uc: uc, log: log}
}

func (s *Subscriber) SubscribePaymentEvents() error {
	_, err := s.js.Subscribe("payment.completed", func(msg *nats.Msg) {
		s.handlePaymentCompleted(msg)
	}, nats.Durable("master-payment-completed"), nats.ManualAck())
	if err != nil {
		return err
	}
	_, err = s.js.Subscribe("payment.failed", func(msg *nats.Msg) {
		s.handlePaymentFailed(msg)
	}, nats.Durable("master-payment-failed"), nats.ManualAck())
	return err
}

func (s *Subscriber) handlePaymentCompleted(msg *nats.Msg) {
	var evt paymentEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		s.log.Error().Err(err).Msg("unmarshal payment.completed")
		_ = msg.Nak()
		return
	}
	if err := s.uc.ConfirmBookingByPayment(context.Background(), evt.BookingID, evt.PaymentID); err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.InvalidArgument {
			_ = msg.Ack()
			return
		}
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			_ = msg.Nak()
			return
		}
		s.log.Error().Err(err).Str("payment_id", evt.PaymentID).Msg("confirm master booking failed")
		_ = msg.Nak()
		return
	}
	_ = msg.Ack()
}

func (s *Subscriber) handlePaymentFailed(msg *nats.Msg) {
	var evt paymentEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		s.log.Error().Err(err).Msg("unmarshal payment.failed")
		_ = msg.Nak()
		return
	}
	if err := s.uc.CancelBookingByPayment(context.Background(), evt.BookingID, evt.PaymentID); err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			_ = msg.Nak()
			return
		}
		s.log.Error().Err(err).Str("payment_id", evt.PaymentID).Msg("cancel master booking failed")
		_ = msg.Nak()
		return
	}
	_ = msg.Ack()
}
