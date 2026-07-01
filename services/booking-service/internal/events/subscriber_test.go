package events

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/services/booking-service/internal/domain"
	"github.com/tienlao/agregator/services/booking-service/internal/usecase"
)

// --- fakes ---

// stubVenue satisfies venuev1.VenueServiceClient; only ReleaseSlot is called on
// the cancel path under test.
type stubVenue struct{ venuev1.VenueServiceClient }

func (stubVenue) ReleaseSlot(context.Context, *venuev1.ReleaseSlotRequest, ...grpc.CallOption) (*venuev1.ReleaseSlotResponse, error) {
	return &venuev1.ReleaseSlotResponse{}, nil
}

type mockRepo struct {
	domain.BookingRepository
	ConfirmPaymentFunc  func(ctx context.Context, id, paymentID, subject string) (*domain.Booking, bool, error)
	GetByIDFunc         func(ctx context.Context, id string) (*domain.Booking, error)
	CancelWithEventFunc func(ctx context.Context, id string, ev domain.OutboxEvent) error
}

func (m *mockRepo) ConfirmPayment(ctx context.Context, id, paymentID, subject string) (*domain.Booking, bool, error) {
	return m.ConfirmPaymentFunc(ctx, id, paymentID, subject)
}
func (m *mockRepo) GetByID(ctx context.Context, id string) (*domain.Booking, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *mockRepo) CancelWithEvent(ctx context.Context, id string, ev domain.OutboxEvent) error {
	if m.CancelWithEventFunc != nil {
		return m.CancelWithEventFunc(ctx, id, ev)
	}
	return nil
}

type fakeJS struct {
	nats.JetStreamContext
	published []*nats.Msg
	pubErr    error
}

func (f *fakeJS) PublishMsg(m *nats.Msg, _ ...nats.PubOpt) (*nats.PubAck, error) {
	f.published = append(f.published, m)
	if f.pubErr != nil {
		return nil, f.pubErr
	}
	return &nats.PubAck{}, nil
}

func newSub(repo domain.BookingRepository) (*Subscriber, *metrics.Metrics) {
	uc := usecase.NewBookingUseCase(repo, stubVenue{}, nil, nil, nil, zerolog.Nop(), "Europe/Moscow", 0)
	m := metrics.New("booking-service")
	return NewSubscriber(&fakeJS{}, uc, zerolog.Nop(), m), m
}

func natsResult(t *testing.T, m *metrics.Metrics, subject, result string) float64 {
	t.Helper()
	mfs, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "booking_service_nats_messages_total" {
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
			if subj == subject && res == result {
				return mc.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func msg(data string) *nats.Msg { return &nats.Msg{Subject: "x", Data: []byte(data)} }

// --- handlePaymentCompleted ---

func TestHandlePaymentCompleted(t *testing.T) {
	t.Run("malformed json → nak/error", func(t *testing.T) {
		repo := &mockRepo{ConfirmPaymentFunc: func(context.Context, string, string, string) (*domain.Booking, bool, error) {
			t.Fatal("uc must not be called on bad json")
			return nil, false, nil
		}}
		s, m := newSub(repo)
		s.handlePaymentCompleted(msg("{bad"))
		if natsResult(t, m, "payment.completed", metrics.NATSError) != 1 {
			t.Error("expected one error result")
		}
	})

	t.Run("confirmed → ack/ok", func(t *testing.T) {
		repo := &mockRepo{ConfirmPaymentFunc: func(_ context.Context, id, pay, subj string) (*domain.Booking, bool, error) {
			return &domain.Booking{ID: id}, true, nil
		}}
		s, m := newSub(repo)
		s.handlePaymentCompleted(msg(`{"booking_id":"b1","payment_id":"p1"}`))
		if natsResult(t, m, "payment.completed", metrics.NATSOk) != 1 {
			t.Error("expected one ok result")
		}
	})

	t.Run("terminal/invalid skip → ack/ok", func(t *testing.T) {
		repo := &mockRepo{ConfirmPaymentFunc: func(context.Context, string, string, string) (*domain.Booking, bool, error) {
			return nil, false, status.Error(codes.NotFound, "gone")
		}}
		s, m := newSub(repo)
		s.handlePaymentCompleted(msg(`{"booking_id":"b1","payment_id":"p1"}`))
		if natsResult(t, m, "payment.completed", metrics.NATSOk) != 1 {
			t.Error("InvalidArgument/NotFound must be acked as ok (skipped)")
		}
	})

	t.Run("transient error → nak/error", func(t *testing.T) {
		repo := &mockRepo{ConfirmPaymentFunc: func(context.Context, string, string, string) (*domain.Booking, bool, error) {
			return nil, false, errors.New("db down")
		}}
		s, m := newSub(repo)
		s.handlePaymentCompleted(msg(`{"booking_id":"b1","payment_id":"p1"}`))
		if natsResult(t, m, "payment.completed", metrics.NATSError) != 1 {
			t.Error("transient error must nak (error result)")
		}
	})
}

// --- handlePaymentFailed ---

func TestHandlePaymentFailed(t *testing.T) {
	t.Run("malformed json → nak/error", func(t *testing.T) {
		s, m := newSub(&mockRepo{GetByIDFunc: func(context.Context, string) (*domain.Booking, error) {
			t.Fatal("uc must not be called on bad json")
			return nil, nil
		}})
		s.handlePaymentFailed(msg("{bad"))
		if natsResult(t, m, "payment.failed", metrics.NATSError) != 1 {
			t.Error("expected one error result")
		}
	})

	t.Run("cancel succeeds → ack/ok", func(t *testing.T) {
		repo := &mockRepo{GetByIDFunc: func(_ context.Context, id string) (*domain.Booking, error) {
			return &domain.Booking{ID: id, VenueID: "v1", Status: domain.StatusPaymentPending}, nil
		}}
		s, m := newSub(repo)
		s.handlePaymentFailed(msg(`{"booking_id":"b1","payment_id":"p1"}`))
		if natsResult(t, m, "payment.failed", metrics.NATSOk) != 1 {
			t.Error("expected one ok result")
		}
	})

	t.Run("cancel errors → nak/error", func(t *testing.T) {
		repo := &mockRepo{GetByIDFunc: func(context.Context, string) (*domain.Booking, error) {
			return nil, errors.New("db down") // non-NotFound → CancelBookingByPayment returns error
		}}
		s, m := newSub(repo)
		s.handlePaymentFailed(msg(`{"booking_id":"b1","payment_id":"p1"}`))
		if natsResult(t, m, "payment.failed", metrics.NATSError) != 1 {
			t.Error("transient error must nak (error result)")
		}
	})
}

// --- Publisher ---

func TestPublisher_PublishRaw(t *testing.T) {
	js := &fakeJS{}
	p := NewPublisher(js)
	require := func(cond bool, msg string) {
		if !cond {
			t.Fatal(msg)
		}
	}
	err := p.PublishRaw(context.Background(), "booking.created", []byte(`{"x":1}`))
	require(err == nil, "unexpected error")
	require(len(js.published) == 1, "expected one published message")
	require(js.published[0].Subject == "booking.created", "subject mismatch")
	require(string(js.published[0].Data) == `{"x":1}`, "payload mismatch")
}

func TestPublisher_PublishRaw_ErrorPropagates(t *testing.T) {
	p := NewPublisher(&fakeJS{pubErr: errors.New("nats down")})
	if err := p.PublishRaw(context.Background(), "booking.created", []byte("{}")); err == nil {
		t.Fatal("expected publish error to propagate")
	}
}

func TestPublisher_PublishBookingCompleted(t *testing.T) {
	js := &fakeJS{}
	p := NewPublisher(js)
	b := &domain.Booking{ID: "b1", UserID: "u1", VenueID: "v1", Status: domain.StatusCompleted}
	if err := p.PublishBookingCompleted(context.Background(), b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(js.published) != 1 || js.published[0].Subject != "booking.completed" {
		t.Fatalf("expected booking.completed publish, got %+v", js.published)
	}
}
