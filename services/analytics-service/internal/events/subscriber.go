package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/pkg/natsutil"
)

const (
	// SubjectWebEvents — продуктовые события браузера, публикует api-gateway
	// (handler/analytics.go). Стрим ANALYTICS тоже создаёт гейтвей.
	SubjectWebEvents = "analytics.web"

	// StreamName — JetStream-стрим, к которому привязывается durable consumer.
	StreamName = "ANALYTICS"

	handlerTimeout = 10 * time.Second

	// ackWait must exceed handlerTimeout so a slow-but-successful insert is
	// not redelivered mid-flight.
	ackWait = 15 * time.Second

	// maxDeliver bounds retries for a persistently failing insert; the row is
	// then lost from the stream after MaxAge anyway — analytics is best-effort.
	maxDeliver = 5
)

// eventStore — то, что подписчик умеет делать с событием. Интерфейс объявлен
// на месте использования (*repository.EventRepo его реализует).
type eventStore interface {
	InsertEvent(ctx context.Context, streamSeq uint64, event, requestID string, props []byte) error
}

// webEvent — контракт payload'а analytics.web (см. gateway handler/analytics.go).
type webEvent struct {
	Event     string          `json:"event"`
	Props     json.RawMessage `json:"props"`
	RequestID string          `json:"request_id"`
}

type Subscriber struct {
	js    nats.JetStreamContext
	store eventStore
	log   zerolog.Logger
	m     *metrics.Metrics
}

func NewSubscriber(js nats.JetStreamContext, store eventStore, log zerolog.Logger, m *metrics.Metrics) *Subscriber {
	return &Subscriber{js: js, store: store, log: log, m: m}
}

// SubscribeWebEvents привязывает durable push consumer к стриму ANALYTICS.
func (s *Subscriber) SubscribeWebEvents() error {
	_, err := s.js.Subscribe(SubjectWebEvents, s.handleWebEvent,
		nats.Durable("analytics-web-sink"),
		nats.ManualAck(),
		nats.BindStream(StreamName),
		nats.DeliverAll(),
		nats.AckWait(ackWait),
		nats.MaxDeliver(maxDeliver),
	)
	return err
}

func (s *Subscriber) handleWebEvent(msg *nats.Msg) {
	start := time.Now()
	result := metrics.NATSError
	defer func() { s.m.ObserveNATS(SubjectWebEvents, result, time.Since(start)) }()

	meta, err := msg.Metadata()
	if err != nil {
		s.log.Error().Err(err).Msg("analytics: message metadata unavailable")
		_ = msg.Nak()
		return
	}

	var evt webEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil || evt.Event == "" {
		// Битый payload ретраями не починится — Ack, чтобы не зациклить доставку.
		s.log.Error().Err(err).Uint64("stream_seq", meta.Sequence.Stream).
			Int("payload_bytes", len(msg.Data)).Msg("analytics: malformed web event")
		_ = msg.Ack()
		return
	}

	ctx, cancel := natsutil.MsgContext(msg, handlerTimeout)
	defer cancel()

	if err := s.store.InsertEvent(ctx, meta.Sequence.Stream, evt.Event, evt.RequestID, evt.Props); err != nil {
		s.log.Error().Err(err).Str("event", evt.Event).
			Uint64("stream_seq", meta.Sequence.Stream).Msg("analytics: insert failed — will retry")
		_ = msg.Nak()
		return
	}

	result = metrics.NATSOk
	_ = msg.Ack()
}
