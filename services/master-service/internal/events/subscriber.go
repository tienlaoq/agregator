package events

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
	"github.com/tienlao/agregator/services/master-service/internal/usecase"
)

const (
	handlerTimeout = 30 * time.Second

	// maxDeliver is the maximum number of times NATS will deliver a message
	// before giving up and emitting a MaxDeliver advisory (acts as a DLQ signal).
	// 10 attempts × ~5 min max backoff ≈ up to ~30 min of retries total.
	maxDeliver = 10

	// ackWait must be strictly greater than handlerTimeout to avoid a
	// timeout-loop: if ackWait ≤ handlerTimeout NATS redelivers the message
	// while the handler is still running, burning a delivery attempt on every
	// slow-but-eventually-successful processing cycle.
	// The +5 s margin is intentional — it absorbs clock skew between the
	// NATS server and the service pod.
	ackWait = handlerTimeout + 5*time.Second
)

// nakBackoff returns the delay before the next redelivery attempt based on how
// many times the message has been delivered so far. Uses capped exponential
// backoff so transient DB/gRPC errors do not cause a busy-loop on the consumer.
//
// Delivery schedule (approximate):
//
//	attempt 1 → immediate (NATS delivers for the first time)
//	attempt 2 → 5 s
//	attempt 3 → 10 s
//	attempt 4 → 20 s
//	attempt 5 → 40 s
//	attempt 6+ → 5 min (capped)
func nakBackoff(numDelivered uint64) time.Duration {
	const (
		base = 5 * time.Second
		cap  = 5 * time.Minute
	)
	if numDelivered == 0 {
		return base
	}
	d := base
	for i := uint64(1); i < numDelivered; i++ {
		d *= 2
		if d >= cap {
			return cap
		}
	}
	return d
}

// nak sends NakWithDelay using exponential backoff derived from the message
// delivery count. Falls back to plain Nak if metadata is unavailable.
// Always logs the Nak outcome so connection drops are visible.
//
// When NumDelivered approaches maxDeliver the function emits a Warn-level log
// so operators can catch persistent failures before NATS exhausts the delivery
// budget and silently stops retrying (the MaxDeliver advisory requires a
// separate advisory consumer to surface; the log entry is a cheaper signal).
func nak(msg *nats.Msg, log zerolog.Logger) {
	delay := 5 * time.Second
	if meta, err := msg.Metadata(); err == nil {
		delay = nakBackoff(meta.NumDelivered)
		evt := log.Debug().
			Uint64("num_delivered", meta.NumDelivered).
			Int("max_deliver", maxDeliver).
			Dur("backoff", delay)
		// Warn when one attempt remains — the next Nak will hit MaxDeliver and
		// NATS will stop retrying. This gives on-call a chance to intervene
		// before the message lands in the advisory DLQ.
		if meta.NumDelivered >= uint64(maxDeliver)-1 {
			evt = log.Warn().
				Uint64("num_delivered", meta.NumDelivered).
				Int("max_deliver", maxDeliver).
				Dur("backoff", delay)
		}
		evt.Msg("nats: nak with backoff")
	}
	if err := msg.NakWithDelay(delay); err != nil {
		log.Error().Err(err).Msg("nats: NakWithDelay failed")
	}
}

// ack sends Ack and logs any transport error.
func ack(msg *nats.Msg, log zerolog.Logger) {
	if err := msg.Ack(); err != nil {
		log.Error().Err(err).Msg("nats: Ack failed")
	}
}

// truncatePaymentID returns the first 8 characters of a payment ID for log
// fields. The prefix is sufficient for log correlation while limiting exposure
// if the upstream format ever changes from a UUID to a bearer-style token.
//
// Compliance note: before increasing the prefix length, confirm with legal/DPO
// that the truncated value does not constitute personal data under applicable
// regulations (GDPR Art. 4, 152-ФЗ). ЮKassa payment IDs are currently
// UUID v4 — statistically non-identifying — but policy may vary by context.
func truncatePaymentID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "…"
}

type paymentEvent struct {
	PaymentID string `json:"payment_id"`
	BookingID string `json:"booking_id"`
	Status    string `json:"status"`
}

const (
	// paymentStatusCompleted / paymentStatusFailed are the Status values
	// expected in the payload of payment.completed / payment.failed events.
	// The handler asserts that subject and payload agree — a mismatch indicates
	// a publisher bug or a misconfigured routing rule and is Ack'd immediately
	// (retrying will not fix a payload with the wrong status field).
	paymentStatusCompleted = "completed"
	paymentStatusFailed    = "failed"
)

type Subscriber struct {
	js  nats.JetStreamContext
	uc  *usecase.MasterUseCase
	log zerolog.Logger
	m   *metrics.Metrics
}

func NewSubscriber(js nats.JetStreamContext, uc *usecase.MasterUseCase, log zerolog.Logger, m *metrics.Metrics) *Subscriber {
	return &Subscriber{js: js, uc: uc, log: log, m: m}
}

// SubscribePaymentEvents subscribes to payment.completed and payment.failed
// JetStream subjects. It returns the two subscriptions so the caller can
// drain them on shutdown (sub.Drain() waits for in-flight handlers to finish
// before the NATS connection is closed).
func (s *Subscriber) SubscribePaymentEvents() ([]*nats.Subscription, error) {
	// MaxDeliver caps redelivery attempts — after maxDeliver failures NATS emits
	// a $JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES advisory which can be routed
	// to an alerting rule or a dead-letter consumer.
	//
	// AckWait > handlerTimeout ensures a slow-but-successful handler is not
	// redelivered while it is still running.
	subOpts := []nats.SubOpt{
		nats.ManualAck(),
		nats.AckWait(ackWait),
		nats.MaxDeliver(maxDeliver),
	}

	subCompleted, err := s.js.Subscribe("payment.completed", func(msg *nats.Msg) {
		s.handlePaymentCompleted(msg)
	}, append([]nats.SubOpt{nats.Durable("master-payment-completed")}, subOpts...)...)
	if err != nil {
		return nil, err
	}

	subFailed, err := s.js.Subscribe("payment.failed", func(msg *nats.Msg) {
		s.handlePaymentFailed(msg)
	}, append([]nats.SubOpt{nats.Durable("master-payment-failed")}, subOpts...)...)
	if err != nil {
		// Best-effort drain of the first subscription before returning the error
		// so we don't leak it.
		_ = subCompleted.Drain()
		return nil, err
	}

	return []*nats.Subscription{subCompleted, subFailed}, nil
}

func (s *Subscriber) handlePaymentCompleted(msg *nats.Msg) {
	start := time.Now()
	result := metrics.NATSError
	defer func() { s.m.ObserveNATS("payment.completed", result, time.Since(start)) }()

	var evt paymentEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		// Malformed message — Nak so it retries, but it will hit MaxDeliver
		// and stop; a human needs to inspect the DLQ advisory.
		s.log.Error().Err(err).
			Str("subject", msg.Subject).
			Int("payload_bytes", len(msg.Data)).
			Msg("payment.completed: unmarshal failed")
		nak(msg, s.log)
		return
	}

	// Assert that the payload status agrees with the subject. A mismatch means
	// payment-service published a payload with the wrong status field (publisher
	// bug or misconfigured routing). Retrying will not fix it — Ack and alert.
	if evt.Status != paymentStatusCompleted {
		s.log.Error().
			Str("subject", msg.Subject).
			Str("payload_status", evt.Status).
			Str("booking_id", evt.BookingID).
			Str("payment_id_prefix", truncatePaymentID(evt.PaymentID)).
			Msg("payment.completed: payload status mismatch — expected 'completed', got something else; Ack to avoid retry loop")
		ack(msg, s.log)
		return
	}

	// Validate required fields before touching any business logic. An empty
	// booking_id or payment_id is a publisher bug — uuid.Parse("") would
	// eventually return ErrInvalidArgument and Ack anyway, but an explicit
	// check here produces a clearer log message and skips the usecase call.
	if evt.BookingID == "" || evt.PaymentID == "" {
		s.log.Error().
			Str("subject", msg.Subject).
			Bool("booking_id_empty", evt.BookingID == "").
			Bool("payment_id_empty", evt.PaymentID == "").
			Msg("payment.completed: missing required fields in payload; Ack to avoid retry loop")
		ack(msg, s.log)
		return
	}

	ctx, cancel := natsutil.MsgContext(msg, handlerTimeout)
	defer cancel()

	log := s.log.With().
		Str("booking_id", evt.BookingID).
		Str("payment_id_prefix", truncatePaymentID(evt.PaymentID)).
		Logger()

	if err := s.uc.ConfirmBookingByPayment(ctx, evt.BookingID, evt.PaymentID); err != nil {
		switch {
		case errors.Is(err, domain.ErrBookingNotPending), errors.Is(err, domain.ErrPaymentMismatch), errors.Is(err, domain.ErrInvalidArgument):
			// Terminal logic conflict — booking is already in a final state, the
			// payment_id does not match, or the message is malformed. Retrying will
			// never resolve any of these; Ack to prevent MaxDeliver exhaustion.
			//
			// ErrBookingNotPending carries "found status=X" in the error message:
			// that status was read by a post-hoc SELECT after the UPDATE touched 0 rows.
			// A concurrent admin cancel between those two statements can make the SELECT
			// return "cancelled" even if the UPDATE missed for a different reason — both
			// outcomes are correct to Ack (the booking is not in payment_pending either way).
			log.Warn().Err(err).Msg("payment.completed: confirm skipped — logic conflict")
			result = metrics.NATSOk
			ack(msg, log)
		case errors.Is(err, domain.ErrNotFound):
			// Booking not found — possible out-of-order delivery (booking row not
			// yet visible on this replica). Retry with backoff up to maxDeliver times,
			// then let the MaxDeliver DLQ advisory fire for manual inspection.
			log.Warn().Msg("payment.completed: booking not found — will retry")
			nak(msg, log)
		default:
			// Transient error (DB unavailable, network timeout, etc.) — retry.
			log.Error().Err(err).Msg("payment.completed: confirm failed — will retry")
			nak(msg, log)
		}
		return
	}

	log.Info().Msg("payment.completed: master booking confirmed")
	result = metrics.NATSOk
	ack(msg, log)
}

func (s *Subscriber) handlePaymentFailed(msg *nats.Msg) {
	start := time.Now()
	result := metrics.NATSError
	defer func() { s.m.ObserveNATS("payment.failed", result, time.Since(start)) }()

	var evt paymentEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		s.log.Error().Err(err).
			Str("subject", msg.Subject).
			Int("payload_bytes", len(msg.Data)).
			Msg("payment.failed: unmarshal failed")
		nak(msg, s.log)
		return
	}

	// Assert that the payload status agrees with the subject.
	if evt.Status != paymentStatusFailed {
		s.log.Error().
			Str("subject", msg.Subject).
			Str("payload_status", evt.Status).
			Str("booking_id", evt.BookingID).
			Str("payment_id_prefix", truncatePaymentID(evt.PaymentID)).
			Msg("payment.failed: payload status mismatch — expected 'failed', got something else; Ack to avoid retry loop")
		ack(msg, s.log)
		return
	}

	if evt.BookingID == "" || evt.PaymentID == "" {
		s.log.Error().
			Str("subject", msg.Subject).
			Bool("booking_id_empty", evt.BookingID == "").
			Bool("payment_id_empty", evt.PaymentID == "").
			Msg("payment.failed: missing required fields in payload; Ack to avoid retry loop")
		ack(msg, s.log)
		return
	}

	ctx, cancel := natsutil.MsgContext(msg, handlerTimeout)
	defer cancel()

	log := s.log.With().
		Str("booking_id", evt.BookingID).
		Str("payment_id_prefix", truncatePaymentID(evt.PaymentID)).
		Logger()

	if err := s.uc.CancelBookingByPayment(ctx, evt.BookingID, evt.PaymentID); err != nil {
		switch {
		case errors.Is(err, domain.ErrBookingNotPending), errors.Is(err, domain.ErrPaymentMismatch), errors.Is(err, domain.ErrInvalidArgument):
			// Terminal logic conflict — booking already confirmed, payment_id mismatch,
			// or malformed message. Ack; retrying will not change the outcome.
			//
			// ErrBookingNotPending carries "found status=X" in the error message:
			// that status was read by a post-hoc SELECT after the UPDATE touched 0 rows.
			// See handlePaymentCompleted comment for the race window explanation.
			log.Warn().Err(err).Msg("payment.failed: cancel skipped — logic conflict")
			result = metrics.NATSOk
			ack(msg, log)
		case errors.Is(err, domain.ErrNotFound):
			log.Warn().Msg("payment.failed: booking not found — will retry")
			nak(msg, log)
		default:
			log.Error().Err(err).Msg("payment.failed: cancel failed — will retry")
			nak(msg, log)
		}
		return
	}

	log.Info().Msg("payment.failed: master booking cancelled")
	// TODO(refund): publish a "booking.cancelled" event here so payment-service
	// can issue the ЮKassa refund. payment-service already subscribes to
	// "booking.cancelled" and calls RefundByBooking — but master-service never
	// publishes that event, so refunds for master bookings are silently skipped.
	//
	// When implementing:
	//   1. Add js nats.JetStreamContext to Subscriber (already present as s.js).
	//   2. Ensure a BOOKINGS stream covering "booking.>" exists (add to main.go).
	//   3. Publish {"booking_id": evt.BookingID} to "booking.cancelled" after a
	//      successful CancelBookingByPayment — before Ack, so a publish failure
	//      can still Nak and retry the whole handler idempotently.
	//   4. Note: CancelBookingByPayment is idempotent (ErrBookingNotPending on
	//      a second call), so re-delivery after a publish failure is safe.
	result = metrics.NATSOk
	ack(msg, log)
}
