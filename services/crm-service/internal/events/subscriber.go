// Package events contains the crm-service NATS consumer that projects
// booking.* events into the guest read model (Customer 360). See
// docs/CRM_DESIGN.md §4.
package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/services/crm-service/internal/domain"
)

const (
	// StreamName is the JetStream stream that carries booking events (owned by
	// booking-service). The consumer binds to it; it does not create it.
	StreamName = "BOOKINGS"

	// subjectFilter matches booking.created/confirmed/cancelled/completed — all
	// of which carry the enriched payload built by domain.NewBookingEvent.
	subjectFilter = "booking.*"

	// durableName makes this a durable consumer so redeploys resume where they
	// left off rather than replaying the whole stream.
	durableName = "crm-guest-projection"

	handlerTimeout = 10 * time.Second
	// ackWait must exceed handlerTimeout so a slow-but-successful apply is not
	// redelivered mid-flight.
	ackWait    = 15 * time.Second
	maxDeliver = 5
)

// factApplier is the projection store dependency, defined at the use site.
// *repository.crmRepo (via domain.Repository) implements it.
type factApplier interface {
	ApplyBookingFact(ctx context.Context, f *domain.BookingFact) error
}

// bookingEvent mirrors the canonical booking.* wire format
// (booking-service/internal/domain/outbox.go). Only the fields the projection
// needs are decoded.
type bookingEvent struct {
	BookingID  string `json:"booking_id"`
	UserID     string `json:"user_id"`
	VenueID    string `json:"venue_id"`
	Status     string `json:"status"`
	TotalPrice int64  `json:"total_price"`
	Date       string `json:"date"`
	Guests     int32  `json:"guests"`
}

// Subscriber consumes booking.* and applies each event to the guest projection.
type Subscriber struct {
	js    nats.JetStreamContext
	store factApplier
	log   zerolog.Logger
	m     *metrics.Metrics
}

func NewSubscriber(js nats.JetStreamContext, store factApplier, log zerolog.Logger, m *metrics.Metrics) *Subscriber {
	return &Subscriber{js: js, store: store, log: log, m: m}
}

// Subscribe binds a durable push consumer to the BOOKINGS stream.
func (s *Subscriber) Subscribe() error {
	_, err := s.js.Subscribe(subjectFilter, s.handle,
		nats.Durable(durableName),
		nats.ManualAck(),
		nats.BindStream(StreamName),
		nats.DeliverAll(),
		nats.AckWait(ackWait),
		nats.MaxDeliver(maxDeliver),
	)
	return err
}

func (s *Subscriber) handle(msg *nats.Msg) {
	start := time.Now()
	result := metrics.NATSError
	defer func() { s.m.ObserveNATS(msg.Subject, result, time.Since(start)) }()

	var evt bookingEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil || evt.BookingID == "" {
		// A malformed payload will never parse on retry — Ack to avoid a poison loop.
		s.log.Error().Err(err).Str("subject", msg.Subject).
			Int("payload_bytes", len(msg.Data)).Msg("crm: malformed booking event")
		_ = msg.Ack()
		return
	}

	fact, err := bookingEventToFact(&evt)
	if err != nil {
		s.log.Error().Err(err).Str("subject", msg.Subject).
			Str("booking_id", evt.BookingID).Msg("crm: unusable booking event — dropping")
		_ = msg.Ack()
		return
	}

	ctx, cancel := natsutil.MsgContext(msg, handlerTimeout)
	defer cancel()

	if err := s.store.ApplyBookingFact(ctx, fact); err != nil {
		s.log.Error().Err(err).Str("booking_id", evt.BookingID).
			Msg("crm: apply booking fact failed — will retry")
		_ = msg.Nak()
		return
	}

	result = metrics.NATSOk
	_ = msg.Ack()
}

// bookingEventToFact converts a decoded event into a projection fact. The status
// (and its rank) come straight from the payload, so the same booking advancing
// pending → completed never regresses the stored fact.
func bookingEventToFact(evt *bookingEvent) (*domain.BookingFact, error) {
	bookingID, err := uuid.Parse(evt.BookingID)
	if err != nil {
		return nil, err
	}
	venueID, err := uuid.Parse(evt.VenueID)
	if err != nil {
		return nil, err
	}
	userID, err := uuid.Parse(evt.UserID)
	if err != nil {
		return nil, err
	}
	var visit *time.Time
	if evt.Date != "" {
		if d, derr := time.Parse("2006-01-02", evt.Date); derr == nil {
			visit = &d
		}
	}
	return &domain.BookingFact{
		BookingID:  bookingID,
		VenueID:    venueID,
		UserID:     userID,
		Status:     evt.Status,
		StatusRank: domain.BookingStatusRank(evt.Status),
		TotalPrice: evt.TotalPrice,
		VisitDate:  visit,
		Guests:     evt.Guests,
	}, nil
}
