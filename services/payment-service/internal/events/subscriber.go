package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/pkg/natsutil"
)

const (
	// handlerTimeout bounds a single delivery's processing time. It must exceed
	// the provider refund retry budget — yookassa's retryAttempts (3) × per-call
	// totalTimeout (10s) plus backoff ≈ 38s. At 30s the context cancelled the
	// retry loop early and NATS redelivered after AckWait (60s) — correct, but
	// wasteful. 45s lets the retry loop run to completion within a single
	// delivery, and still sits comfortably under AckWait so a genuinely stuck
	// handler is still redelivered.
	handlerTimeout = 45 * time.Second

	// bookingCancelledConsumer is the durable name for the push consumer that
	// processes booking.cancelled events.  Changing this name creates a new
	// consumer and orphans the old one — rename only with a coordinated migration.
	bookingCancelledConsumer = "payment-booking-cancelled"

	// DLQSubjectPrefix is the NATS subject prefix for parked (dead-letter)
	// payment messages that exhausted all delivery attempts. main.go wires the
	// PAYMENTS_DLQ stream to DLQSubjectWildcard so it captures every subject
	// below this prefix. Keeping the prefix here (not duplicated as a literal in
	// main.go) means a rename touches exactly one place.
	DLQSubjectPrefix = "dlq.payment."

	// DLQSubjectWildcard is the stream filter that captures all DLQ subjects.
	DLQSubjectWildcard = DLQSubjectPrefix + ">"

	// dlqSubjectBookingCancelled is the concrete subject for parked
	// booking.cancelled refund messages.
	dlqSubjectBookingCancelled = DLQSubjectPrefix + "booking-cancelled"

	// maxDeliver is the maximum number of times NATS will redeliver a message
	// before the consumer considers it exhausted.  After the last attempt the
	// subscriber calls Term() so NATS stops tracking the message, and we publish
	// a copy to the DLQ stream for manual inspection / replay.
	maxDeliver = 10

	// ackWait is how long the server waits for an Ack before redelivering.
	// 60 s gives the payment provider enough room for a slow HTTP response plus
	// our internal retry backoff without triggering a spurious redelivery.
	//
	// MUST equal backOffPolicy[0]: when a consumer is created with both AckWait
	// and BackOff, NATS JetStream silently overrides the consumer's AckWait with
	// BackOff[0]. If the two disagree, the first Subscribe creates the consumer
	// with AckWait=BackOff[0], and every subsequent Subscribe requests this
	// ackWait value, sees the stored BackOff[0], and fails with a config-mismatch
	// error ("ack wait to be ..., but consumer's value is ...") — crashing the
	// service on restart. Keep these in sync.
	ackWait = 60 * time.Second
)

// backOffPolicy is the redelivery schedule passed to NATS as the consumer
// BackOff field.  Durations are server-side — the server waits this long
// before the next redelivery attempt after a Nak.
//
// backOffPolicy[0] also becomes the consumer's effective AckWait (see ackWait
// above), so the first interval is pinned to 60 s to match. The schedule then
// climbs to the 120 s cap.
//
// Attempts: 1→60s, 2→90s, 3–10→120s (capped).
var backOffPolicy = []time.Duration{
	60 * time.Second,
	90 * time.Second,
	120 * time.Second,
	120 * time.Second,
	120 * time.Second,
	120 * time.Second,
	120 * time.Second,
	120 * time.Second,
	120 * time.Second,
}

type refunder interface {
	RefundByBooking(ctx context.Context, bookingID string) error
}

type bookingCancelledEvent struct {
	BookingID string `json:"booking_id"`
	UserID    string `json:"user_id"`
	VenueID   string `json:"venue_id"`
	Status    string `json:"status"`
}

type Subscriber struct {
	js  nats.JetStreamContext
	uc  refunder
	log zerolog.Logger
}

func NewSubscriber(js nats.JetStreamContext, uc refunder, log zerolog.Logger) *Subscriber {
	return &Subscriber{js: js, uc: uc, log: log}
}

// SubscribeBookingEvents подписывается на события из стрима BOOKINGS.
// Сейчас обрабатывается только booking.cancelled — триггер рефанда.
//
// Consumer policy:
//   - MaxDeliver = 10   — after 10 failed attempts the message is Term()'d and
//     moved to the DLQ; prevents infinite retries on broken logic or hard 4xx.
//   - AckWait   = 60 s  — gives the refund provider call enough time before
//     NATS considers the message unacknowledged and redelivers.
//   - BackOff           — exponential, up to 120 s between attempts; avoids
//     hammering a temporarily unavailable provider.
func (s *Subscriber) SubscribeBookingEvents() error {
	// js.Subscribe with Durable creates a push consumer and manages the inbox
	// subject automatically. Passing MaxDeliver/AckWait/BackOff as subscribe
	// options sets them server-side on first creation and updates them on restart.
	_, err := s.js.Subscribe("booking.cancelled", func(msg *nats.Msg) {
		s.handleBookingCancelled(msg)
	},
		nats.Durable(bookingCancelledConsumer),
		nats.ManualAck(),
		nats.BindStream("BOOKINGS"),
		nats.DeliverAll(),
		nats.AckWait(ackWait),
		nats.MaxDeliver(maxDeliver),
		nats.BackOff(backOffPolicy),
	)
	return err
}

func (s *Subscriber) handleBookingCancelled(msg *nats.Msg) {
	meta, err := msg.Metadata()
	if err != nil {
		s.log.Error().Err(err).Msg("payment: failed to read NATS message metadata")
		_ = msg.Nak()
		return
	}

	var evt bookingCancelledEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		s.log.Error().Err(err).Msg("payment: unmarshal booking.cancelled — malformed payload, terminating")
		// Malformed JSON will never recover on retry — term immediately.
		s.termToDLQ(msg, evt.BookingID, "unmarshal error: "+err.Error())
		return
	}

	if evt.BookingID == "" {
		s.log.Warn().Msg("payment: booking.cancelled missing booking_id, skipping")
		_ = msg.Ack()
		return
	}

	ctx, cancel := natsutil.MsgContext(msg, handlerTimeout)
	defer cancel()

	if err := s.uc.RefundByBooking(ctx, evt.BookingID); err != nil {
		// Permanent errors (4xx from the provider, invalid argument, not-found)
		// will never succeed on retry — send straight to DLQ.
		if isPermanent(err) {
			s.log.Error().Err(err).
				Str("booking_id", evt.BookingID).
				Uint64("delivery", meta.NumDelivered).
				Msg("payment: permanent refund error — terminating to DLQ")
			s.termToDLQ(msg, evt.BookingID, err.Error())
			return
		}

		// Transient error.  If this is the last allowed delivery, park in DLQ
		// instead of Nak-ing (which would cause one more useless redelivery after
		// the server already gave up tracking the message).
		if meta.NumDelivered >= uint64(maxDeliver) {
			s.log.Error().Err(err).
				Str("booking_id", evt.BookingID).
				Uint64("delivery", meta.NumDelivered).
				Msg("payment: refund exhausted max retries — terminating to DLQ")
			s.termToDLQ(msg, evt.BookingID, err.Error())
			return
		}

		s.log.Error().Err(err).
			Str("booking_id", evt.BookingID).
			Uint64("delivery", meta.NumDelivered).
			Int("max_deliver", maxDeliver).
			Msg("payment: refund on booking.cancelled failed, will retry")
		_ = msg.Nak()
		return
	}

	s.log.Info().
		Str("booking_id", evt.BookingID).
		Uint64("delivery", meta.NumDelivered).
		Msg("payment: refund issued on booking.cancelled")
	_ = msg.Ack()
}

// termToDLQ calls msg.Term() to stop NATS redeliveries and publishes the
// original message body to the DLQ stream for manual inspection and replay.
// Errors here are logged but never returned — the primary goal is to Term()
// the message even if the DLQ publish fails.
func (s *Subscriber) termToDLQ(msg *nats.Msg, bookingID, reason string) {
	if err := msg.Term(); err != nil {
		s.log.Error().Err(err).Str("booking_id", bookingID).Msg("payment: msg.Term() failed")
	}

	dlqPayload, _ := json.Marshal(map[string]string{
		"booking_id":       bookingID,
		"original_subject": msg.Subject,
		"reason":           reason,
	})
	if _, err := s.js.Publish(dlqSubjectBookingCancelled, dlqPayload); err != nil {
		s.log.Error().Err(err).
			Str("booking_id", bookingID).
			Msg("payment: failed to publish to DLQ — message parked only by Term()")
	}
}

// isPermanent returns true for errors that will never succeed on retry:
// provider-level 4xx surfaces as gRPC InvalidArgument or NotFound codes.
func isPermanent(err error) bool {
	switch status.Code(err) {
	case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists, codes.PermissionDenied:
		return true
	default:
		return false
	}
}
