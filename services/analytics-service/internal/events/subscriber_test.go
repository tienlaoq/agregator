package events

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/pkg/metrics"
)

// fakeStore records the last InsertEvent call.
type fakeStore struct {
	err       error
	callCount int
	gotSeq    uint64
	gotEvent  string
	gotReqID  string
	gotProps  []byte
}

func (f *fakeStore) InsertEvent(_ context.Context, seq uint64, event, requestID string, props []byte) error {
	f.callCount++
	f.gotSeq = seq
	f.gotEvent = event
	f.gotReqID = requestID
	f.gotProps = props
	return f.err
}

// jsMsg builds a message whose Reply subject parses as JetStream metadata
// with the given stream sequence ($JS.ACK.<stream>.<consumer>.<delivered>.<sseq>.<cseq>.<ts>.<pending>).
func jsMsg(data string, streamSeq string) *nats.Msg {
	return &nats.Msg{
		Subject: SubjectWebEvents,
		Reply:   "$JS.ACK.ANALYTICS.analytics-web-sink.1." + streamSeq + ".1.1700000000000000000.0",
		Data:    []byte(data),
		// Metadata() требует msg, привязанный к подписке (checkReply);
		// пустая Subscription этого достаточно для парсинга Reply-токенов.
		Sub: &nats.Subscription{},
	}
}

func Test_handleWebEvent(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		storeErr  error
		wantCalls int
		wantEvent string
		wantSeq   uint64
		wantReqID string
	}{
		{
			name:      "valid event is stored with stream seq",
			data:      `{"event":"page_view","props":{"page":"/venues"},"request_id":"req-1"}`,
			wantCalls: 1,
			wantEvent: "page_view",
			wantSeq:   42,
			wantReqID: "req-1",
		},
		{
			name:      "malformed json is acked without insert",
			data:      `{not-json`,
			wantCalls: 0,
		},
		{
			name:      "empty event name is dropped",
			data:      `{"event":"","props":{}}`,
			wantCalls: 0,
		},
		{
			name:      "insert error still calls store once",
			data:      `{"event":"page_view"}`,
			storeErr:  errors.New("db down"),
			wantCalls: 1,
			wantEvent: "page_view",
			wantSeq:   42,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{err: tt.storeErr}
			s := NewSubscriber(nil, store, zerolog.Nop(), metrics.New("analytics-service-test"))
			s.handleWebEvent(jsMsg(tt.data, "42"))

			if store.callCount != tt.wantCalls {
				t.Fatalf("insert calls = %d, want %d", store.callCount, tt.wantCalls)
			}
			if tt.wantCalls == 0 {
				return
			}
			if store.gotEvent != tt.wantEvent {
				t.Errorf("event = %q, want %q", store.gotEvent, tt.wantEvent)
			}
			if store.gotSeq != tt.wantSeq {
				t.Errorf("stream_seq = %d, want %d", store.gotSeq, tt.wantSeq)
			}
			if store.gotReqID != tt.wantReqID {
				t.Errorf("request_id = %q, want %q", store.gotReqID, tt.wantReqID)
			}
		})
	}
}
