package events

import (
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/services/booking-service/internal/usecase"
)

// handlerTimeout — максимальное время обработки одного NATS сообщения.
// Включает: DB-запросы, gRPC-вызовы к venue/payment, NATS publish.
// При превышении контекст отменяется → gRPC/pgx вернут DeadlineExceeded → Nak → retry.
const handlerTimeout = 30 * time.Second

type paymentEvent struct {
	PaymentID string `json:"payment_id"`
	BookingID string `json:"booking_id"`
	Status    string `json:"status"`
}

type Subscriber struct {
	js  nats.JetStreamContext
	uc  *usecase.BookingUseCase
	log zerolog.Logger
}

func NewSubscriber(js nats.JetStreamContext, uc *usecase.BookingUseCase, log zerolog.Logger) *Subscriber {
	return &Subscriber{js: js, uc: uc, log: log}
}

func (s *Subscriber) SubscribePaymentEvents() error {
	_, err := s.js.Subscribe("payment.completed", func(msg *nats.Msg) {
		s.handlePaymentCompleted(msg)
	}, nats.Durable("booking-payment-completed"), nats.ManualAck())
	if err != nil {
		return err
	}

	_, err = s.js.Subscribe("payment.failed", func(msg *nats.Msg) {
		s.handlePaymentFailed(msg)
	}, nats.Durable("booking-payment-failed"), nats.ManualAck())
	return err
}

func (s *Subscriber) handlePaymentCompleted(msg *nats.Msg) {
	// MsgContext создаётся первым — msg_id доступен во всех лог-сообщениях включая ошибку парсинга.
	ctx, cancel := natsutil.MsgContext(msg, handlerTimeout)
	defer cancel()
	msgID := natsutil.MsgIDFromCtx(ctx)

	var evt paymentEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		s.log.Error().Err(err).Str("msg_id", msgID).Msg("unmarshal payment.completed")
		_ = msg.Nak()
		return
	}

	if err := s.uc.ConfirmBooking(ctx, evt.BookingID, evt.PaymentID); err != nil {
		if st, ok := status.FromError(err); ok && (st.Code() == codes.InvalidArgument || st.Code() == codes.NotFound) {
			s.log.Warn().Err(err).Str("msg_id", msgID).Str("booking_id", evt.BookingID).Msg("confirm booking skipped (terminal or invalid state)")
			_ = msg.Ack()
			return
		}
		s.log.Error().Err(err).Str("msg_id", msgID).Str("booking_id", evt.BookingID).Msg("confirm booking failed")
		_ = msg.Nak()
		return
	}

	s.log.Info().Str("msg_id", msgID).Str("booking_id", evt.BookingID).Msg("booking confirmed via payment")
	_ = msg.Ack()
}

func (s *Subscriber) handlePaymentFailed(msg *nats.Msg) {
	ctx, cancel := natsutil.MsgContext(msg, handlerTimeout)
	defer cancel()
	msgID := natsutil.MsgIDFromCtx(ctx)

	var evt paymentEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		s.log.Error().Err(err).Str("msg_id", msgID).Msg("unmarshal payment.failed")
		_ = msg.Nak()
		return
	}

	if err := s.uc.CancelBookingByPayment(ctx, evt.BookingID); err != nil {
		s.log.Error().Err(err).Str("msg_id", msgID).Str("booking_id", evt.BookingID).Msg("cancel booking on payment failure")
		_ = msg.Nak()
		return
	}

	s.log.Info().Str("msg_id", msgID).Str("booking_id", evt.BookingID).Msg("booking cancelled via payment failure")
	_ = msg.Ack()
}
