package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

type fakeJS struct {
	nats.JetStreamContext
	msgs []*nats.Msg
	err  error
}

func (f *fakeJS) PublishMsg(m *nats.Msg, _ ...nats.PubOpt) (*nats.PubAck, error) {
	f.msgs = append(f.msgs, m)
	if f.err != nil {
		return nil, f.err
	}
	return &nats.PubAck{}, nil
}

// mockOutbox lets a test drive the publish callback RelayBatch would invoke.
type mockOutbox struct {
	relay func(ctx context.Context, limit int, publish func(*domain.OutboxEvent) error) (int, error)
}

func (m *mockOutbox) RelayBatch(ctx context.Context, limit int, publish func(*domain.OutboxEvent) error) (int, error) {
	return m.relay(ctx, limit, publish)
}
func (m *mockOutbox) MarkFailed(context.Context, int64, string) error { return nil }

func TestNewWorker_DefaultPollInterval(t *testing.T) {
	w := NewWorker(&mockOutbox{}, &fakeJS{}, zerolog.Nop(), 0)
	if w.pollInterval != defaultPollInterval {
		t.Errorf("pollInterval = %v, want default %v", w.pollInterval, defaultPollInterval)
	}
	w2 := NewWorker(&mockOutbox{}, &fakeJS{}, zerolog.Nop(), 5*time.Second)
	if w2.pollInterval != 5*time.Second {
		t.Errorf("pollInterval = %v, want 5s", w2.pollInterval)
	}
}

func TestRelay_PublishesEventToNATS(t *testing.T) {
	js := &fakeJS{}
	var publishErr error
	outbox := &mockOutbox{relay: func(_ context.Context, limit int, publish func(*domain.OutboxEvent) error) (int, error) {
		if limit != fetchBatchSize {
			t.Errorf("limit = %d, want %d", limit, fetchBatchSize)
		}
		publishErr = publish(&domain.OutboxEvent{
			ID: 1, Subject: domain.SubjectPaymentCompleted,
			Payload: []byte(`{"payment_id":"p1"}`), PaymentID: "p1",
		})
		return 1, nil
	}}

	NewWorker(outbox, js, zerolog.Nop(), time.Second).relay(context.Background())

	if publishErr != nil {
		t.Fatalf("publish returned error: %v", publishErr)
	}
	if len(js.msgs) != 1 {
		t.Fatalf("published %d msgs, want 1", len(js.msgs))
	}
	if js.msgs[0].Subject != string(domain.SubjectPaymentCompleted) {
		t.Errorf("subject = %q, want %q", js.msgs[0].Subject, domain.SubjectPaymentCompleted)
	}
	if string(js.msgs[0].Data) != `{"payment_id":"p1"}` {
		t.Errorf("payload = %q", js.msgs[0].Data)
	}
}

func TestRelay_PublishErrorSurfacesToRelayBatch(t *testing.T) {
	js := &fakeJS{err: errors.New("nats unavailable")}
	var publishErr error
	outbox := &mockOutbox{relay: func(_ context.Context, _ int, publish func(*domain.OutboxEvent) error) (int, error) {
		publishErr = publish(&domain.OutboxEvent{ID: 1, Subject: domain.SubjectPaymentFailed, Payload: []byte("{}")})
		return 1, nil
	}}

	NewWorker(outbox, js, zerolog.Nop(), time.Second).relay(context.Background())

	if publishErr == nil {
		t.Fatal("publish must return the NATS error so RelayBatch marks the row failed")
	}
}

func TestRelay_BatchInfraErrorIsHandled(t *testing.T) {
	// RelayBatch infra failure must not panic; relay logs and returns.
	outbox := &mockOutbox{relay: func(context.Context, int, func(*domain.OutboxEvent) error) (int, error) {
		return 0, errors.New("tx open failed")
	}}
	NewWorker(outbox, &fakeJS{}, zerolog.Nop(), time.Second).relay(context.Background())
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	var ticks int
	outbox := &mockOutbox{relay: func(context.Context, int, func(*domain.OutboxEvent) error) (int, error) {
		ticks++
		return 0, nil
	}}
	w := NewWorker(outbox, &fakeJS{}, zerolog.Nop(), 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after context cancellation")
	}
	if ticks == 0 {
		t.Error("expected at least one relay tick before cancellation")
	}
}
