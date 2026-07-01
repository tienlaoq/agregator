package events

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/services/crm-service/internal/domain"
)

// fakeStore is a func-field stub of factApplier; it records the applied fact.
type fakeStore struct {
	fn      func(ctx context.Context, f *domain.BookingFact) error
	calls   int
	lastArg *domain.BookingFact
}

func (s *fakeStore) ApplyBookingFact(ctx context.Context, f *domain.BookingFact) error {
	s.calls++
	s.lastArg = f
	if s.fn != nil {
		return s.fn(ctx, f)
	}
	return nil
}

func newTestSubscriber(store factApplier) *Subscriber {
	// js stays nil: handle() never touches it, only Subscribe() does.
	return NewSubscriber(nil, store, zerolog.Nop(), metrics.New("crm-test"))
}

func msg(subject, data string) *nats.Msg {
	return &nats.Msg{Subject: subject, Data: []byte(data)}
}

func TestHandle(t *testing.T) {
	validPayload := func() string {
		return `{"booking_id":"` + uuid.NewString() + `","venue_id":"` + uuid.NewString() +
			`","user_id":"` + uuid.NewString() + `","status":"completed","total_price":3000,"date":"2026-05-01","guests":2}`
	}

	t.Run("valid event applies fact", func(t *testing.T) {
		store := &fakeStore{}
		newTestSubscriber(store).handle(msg("booking.completed", validPayload()))
		if store.calls != 1 {
			t.Fatalf("ApplyBookingFact calls = %d, want 1", store.calls)
		}
		if store.lastArg == nil || store.lastArg.Status != "completed" || store.lastArg.TotalPrice != 3000 {
			t.Fatalf("unexpected fact: %+v", store.lastArg)
		}
	})

	t.Run("malformed json is dropped without applying", func(t *testing.T) {
		store := &fakeStore{}
		newTestSubscriber(store).handle(msg("booking.created", `{not json`))
		if store.calls != 0 {
			t.Fatalf("ApplyBookingFact calls = %d, want 0", store.calls)
		}
	})

	t.Run("empty booking id is dropped without applying", func(t *testing.T) {
		store := &fakeStore{}
		newTestSubscriber(store).handle(msg("booking.created", `{"booking_id":"","status":"pending"}`))
		if store.calls != 0 {
			t.Fatalf("ApplyBookingFact calls = %d, want 0", store.calls)
		}
	})

	t.Run("unparseable ids are dropped without applying", func(t *testing.T) {
		store := &fakeStore{}
		// Non-empty booking_id passes the poison check but fails uuid.Parse.
		newTestSubscriber(store).handle(msg("booking.created", `{"booking_id":"not-a-uuid","status":"pending"}`))
		if store.calls != 0 {
			t.Fatalf("ApplyBookingFact calls = %d, want 0", store.calls)
		}
	})

	t.Run("apply error does not panic and reports failure", func(t *testing.T) {
		store := &fakeStore{fn: func(context.Context, *domain.BookingFact) error { return errors.New("db down") }}
		// Nak() on an unbound msg returns an error that handle ignores; this must not panic.
		newTestSubscriber(store).handle(msg("booking.completed", validPayload()))
		if store.calls != 1 {
			t.Fatalf("ApplyBookingFact calls = %d, want 1", store.calls)
		}
	})
}
