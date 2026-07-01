package events

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/pkg/metrics"
)

func TestIsPermanent(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"invalid argument", status.Error(codes.InvalidArgument, "x"), true},
		{"not found", status.Error(codes.NotFound, "x"), true},
		{"already exists", status.Error(codes.AlreadyExists, "x"), true},
		{"permission denied", status.Error(codes.PermissionDenied, "x"), true},
		{"unavailable is transient", status.Error(codes.Unavailable, "x"), false},
		{"deadline is transient", status.Error(codes.DeadlineExceeded, "x"), false},
		{"plain error is transient", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPermanent(tt.err); got != tt.want {
				t.Errorf("isPermanent(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// --- fakes ---

type fakeJS struct {
	nats.JetStreamContext
	publishedSubjects []string
	subscribeErr      error
}

func (f *fakeJS) Publish(subj string, _ []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	f.publishedSubjects = append(f.publishedSubjects, subj)
	return &nats.PubAck{}, nil
}

func (f *fakeJS) Subscribe(_ string, _ nats.MsgHandler, _ ...nats.SubOpt) (*nats.Subscription, error) {
	if f.subscribeErr != nil {
		return nil, f.subscribeErr
	}
	return &nats.Subscription{}, nil
}

type mockRefunder struct {
	calls       int
	lastBooking string
	err         error
}

func (m *mockRefunder) RefundByBooking(_ context.Context, id string) error {
	m.calls++
	m.lastBooking = id
	return m.err
}

// jsMsg crafts a *nats.Msg carrying a valid JetStream ACK reply so Metadata()
// parses (delivered = NumDelivered). Ack/Nak/Term no-op (no connection) — the
// handler ignores their errors, so the routing logic is fully exercised and the
// chosen outcome is observable via the metrics result label.
func jsMsg(delivered int, data []byte) *nats.Msg {
	return &nats.Msg{
		Subject: "booking.cancelled",
		Reply:   fmt.Sprintf("$JS.ACK.BOOKINGS.payment-booking-cancelled.%d.10.5.1700000000000000000.0", delivered),
		Sub:     &nats.Subscription{},
		Data:    data,
	}
}

// natsResult returns the count recorded for ("booking.cancelled", result).
func natsResult(t *testing.T, m *metrics.Metrics, result string) float64 {
	t.Helper()
	mfs, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "payment_service_nats_messages_total" {
			continue
		}
		for _, mc := range mf.GetMetric() {
			var subj, res string
			for _, l := range mc.GetLabel() {
				switch l.GetName() {
				case "subject":
					subj = l.GetValue()
				case "result":
					res = l.GetValue()
				}
			}
			if subj == "booking.cancelled" && res == result {
				return mc.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func newSub(uc refunder) (*Subscriber, *fakeJS, *metrics.Metrics) {
	js := &fakeJS{}
	m := metrics.New("payment-service")
	return NewSubscriber(js, uc, zerolog.Nop(), m), js, m
}

func TestHandleBookingCancelled(t *testing.T) {
	t.Run("metadata error → nak, no refund", func(t *testing.T) {
		ref := &mockRefunder{}
		s, js, m := newSub(ref)
		// No Reply / Sub → Metadata() fails.
		s.handleBookingCancelled(&nats.Msg{Subject: "booking.cancelled", Data: []byte("{}")})
		if ref.calls != 0 {
			t.Error("refund must not be called when metadata is unreadable")
		}
		if len(js.publishedSubjects) != 0 {
			t.Error("no DLQ publish expected")
		}
		if natsResult(t, m, metrics.NATSError) != 1 {
			t.Error("expected one 'error' result")
		}
	})

	t.Run("malformed json → DLQ", func(t *testing.T) {
		ref := &mockRefunder{}
		s, js, m := newSub(ref)
		s.handleBookingCancelled(jsMsg(1, []byte("{not json")))
		if ref.calls != 0 {
			t.Error("refund must not run on malformed payload")
		}
		if len(js.publishedSubjects) != 1 || js.publishedSubjects[0] != dlqSubjectBookingCancelled {
			t.Errorf("expected DLQ publish to %q, got %v", dlqSubjectBookingCancelled, js.publishedSubjects)
		}
		if natsResult(t, m, metrics.NATSDLQ) != 1 {
			t.Error("expected one 'dlq' result")
		}
	})

	t.Run("missing booking_id → ack, no refund", func(t *testing.T) {
		ref := &mockRefunder{}
		s, js, m := newSub(ref)
		s.handleBookingCancelled(jsMsg(1, []byte(`{"booking_id":""}`)))
		if ref.calls != 0 || len(js.publishedSubjects) != 0 {
			t.Error("empty booking_id must be acked without refund or DLQ")
		}
		if natsResult(t, m, metrics.NATSOk) != 1 {
			t.Error("expected one 'ok' result")
		}
	})

	t.Run("success → ack, refund issued", func(t *testing.T) {
		ref := &mockRefunder{}
		s, js, m := newSub(ref)
		s.handleBookingCancelled(jsMsg(1, []byte(`{"booking_id":"b1"}`)))
		if ref.calls != 1 || ref.lastBooking != "b1" {
			t.Errorf("refund calls=%d booking=%q, want 1/b1", ref.calls, ref.lastBooking)
		}
		if len(js.publishedSubjects) != 0 {
			t.Error("no DLQ on success")
		}
		if natsResult(t, m, metrics.NATSOk) != 1 {
			t.Error("expected one 'ok' result")
		}
	})

	t.Run("permanent error → DLQ", func(t *testing.T) {
		ref := &mockRefunder{err: status.Error(codes.InvalidArgument, "bad booking")}
		s, js, m := newSub(ref)
		s.handleBookingCancelled(jsMsg(1, []byte(`{"booking_id":"b1"}`)))
		if ref.calls != 1 {
			t.Error("refund attempted once")
		}
		if len(js.publishedSubjects) != 1 {
			t.Error("permanent error must go straight to DLQ")
		}
		if natsResult(t, m, metrics.NATSDLQ) != 1 {
			t.Error("expected one 'dlq' result")
		}
	})

	t.Run("transient error below max → nak retry", func(t *testing.T) {
		ref := &mockRefunder{err: errors.New("provider timeout")}
		s, js, m := newSub(ref)
		s.handleBookingCancelled(jsMsg(1, []byte(`{"booking_id":"b1"}`)))
		if ref.calls != 1 {
			t.Error("refund attempted once")
		}
		if len(js.publishedSubjects) != 0 {
			t.Error("transient error below max must NOT go to DLQ (it retries)")
		}
		if natsResult(t, m, metrics.NATSError) != 1 {
			t.Error("expected one 'error' result (will retry)")
		}
	})

	t.Run("transient error at max delivery → DLQ", func(t *testing.T) {
		ref := &mockRefunder{err: errors.New("provider timeout")}
		s, js, m := newSub(ref)
		s.handleBookingCancelled(jsMsg(maxDeliver, []byte(`{"booking_id":"b1"}`)))
		if len(js.publishedSubjects) != 1 {
			t.Error("transient error at max retries must be parked in DLQ")
		}
		if natsResult(t, m, metrics.NATSDLQ) != 1 {
			t.Error("expected one 'dlq' result")
		}
	})
}

func TestSubscribeBookingEvents(t *testing.T) {
	t.Run("propagates subscribe error", func(t *testing.T) {
		js := &fakeJS{subscribeErr: errors.New("stream missing")}
		s := NewSubscriber(js, &mockRefunder{}, zerolog.Nop(), metrics.New("payment-service"))
		if err := s.SubscribeBookingEvents(); err == nil {
			t.Fatal("expected subscribe error to propagate")
		}
	})
	t.Run("success", func(t *testing.T) {
		s := NewSubscriber(&fakeJS{}, &mockRefunder{}, zerolog.Nop(), metrics.New("payment-service"))
		if err := s.SubscribeBookingEvents(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
