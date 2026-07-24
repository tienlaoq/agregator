package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

// recordingChatNotifier records recipients passed to NotifyChatMessage.
type recordingChatNotifier struct {
	got  []string
	last chatMessageEvent
}

func (r *recordingChatNotifier) NotifyChatMessage(_ context.Context, recipientID, threadID, threadKind, refID, messageID string) {
	r.got = append(r.got, recipientID)
	r.last = chatMessageEvent{
		MessageID: messageID, ThreadID: threadID, ThreadKind: threadKind, RefID: refID,
	}
}

func msgFor(t *testing.T, evt chatMessageEvent) *nats.Msg {
	t.Helper()
	b, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &nats.Msg{Subject: chatMsgSubject, Data: b}
}

func newConsumer(n chatNotifier) *ChatNotifyConsumer {
	return NewChatNotifyConsumer(nil, n, zerolog.Nop())
}

func TestChatNotify_NotifiesParticipantsExceptAuthor(t *testing.T) {
	n := &recordingChatNotifier{}
	c := newConsumer(n)

	c.handle(msgFor(t, chatMessageEvent{
		MessageID:    "m1",
		ThreadID:     "t1",
		AuthorUserID: "author",
		ThreadKind:   "venue",
		RefID:        "v1",
		Participants: []string{"author", "u2", "  u3  ", "", "u2"}, // author + dup + blank + whitespace
	}))

	// author excluded, u2 deduped, blank dropped, u3 trimmed → [u2, u3]
	if len(n.got) != 2 {
		t.Fatalf("recipients = %v, want exactly [u2 u3]", n.got)
	}
	seen := map[string]bool{}
	for _, r := range n.got {
		seen[r] = true
	}
	if !seen["u2"] || !seen["u3"] {
		t.Fatalf("recipients = %v, want u2 and u3", n.got)
	}
	if n.last.ThreadID != "t1" || n.last.RefID != "v1" || n.last.MessageID != "m1" {
		t.Fatalf("payload not forwarded: %+v", n.last)
	}
}

func TestChatNotify_MalformedIsSkipped(t *testing.T) {
	n := &recordingChatNotifier{}
	c := newConsumer(n)

	// Invalid JSON must not panic and must not notify anyone.
	c.handle(&nats.Msg{Subject: chatMsgSubject, Data: []byte("{not json")})
	// Valid JSON but no message_id is treated as malformed.
	c.handle(msgFor(t, chatMessageEvent{ThreadID: "t1", Participants: []string{"u2"}}))

	if len(n.got) != 0 {
		t.Fatalf("expected no notifications, got %v", n.got)
	}
}

func TestChatNotify_AuthorOnlyThreadNotifiesNobody(t *testing.T) {
	n := &recordingChatNotifier{}
	c := newConsumer(n)

	c.handle(msgFor(t, chatMessageEvent{
		MessageID:    "m1",
		AuthorUserID: "author",
		Participants: []string{"author"},
	}))

	if len(n.got) != 0 {
		t.Fatalf("author-only thread should notify nobody, got %v", n.got)
	}
}
