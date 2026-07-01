package events

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
)

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

func TestPublish(t *testing.T) {
	js := &fakeJS{}
	p := New(js)
	if err := p.Publish(context.Background(), "chat.message.created", []byte(`{"x":1}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(js.published) != 1 {
		t.Fatalf("published %d messages, want 1", len(js.published))
	}
	if js.published[0].Subject != "chat.message.created" {
		t.Errorf("subject = %q", js.published[0].Subject)
	}
	if string(js.published[0].Data) != `{"x":1}` {
		t.Errorf("data = %q", js.published[0].Data)
	}
}

func TestPublish_ErrorWrapped(t *testing.T) {
	p := New(&fakeJS{pubErr: errors.New("broker down")})
	err := p.Publish(context.Background(), "chat.message.created", []byte("{}"))
	if err == nil {
		t.Fatal("expected error")
	}
	// Error must mention the subject for diagnosability.
	if got := err.Error(); got == "" || !contains(got, "chat.message.created") {
		t.Errorf("error %q should reference the subject", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
