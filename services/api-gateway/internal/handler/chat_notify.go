package handler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/pkg/natsutil"
)

const (
	chatEventsStream     = "CHAT_EVENTS" // owned by chat-service
	chatMsgSubject       = "chat.message.created"
	chatNotifyDurable    = "gateway-chat-notify"
	chatNotifyQueue      = "gateway-chat-notify" // queue group → one replica handles each msg
	chatNotifyAckWait    = 30 * time.Second
	chatNotifyMaxDeliver = 5
	chatNotifyTimeout    = 5 * time.Second
)

// chatMessageEvent mirrors the payload published by chat-service
// (internal/usecase/chat.go publishMessageCreated). Only the fields the
// notification needs are decoded.
type chatMessageEvent struct {
	MessageID    string   `json:"message_id"`
	ThreadID     string   `json:"thread_id"`
	AuthorUserID string   `json:"author_user_id"`
	ThreadKind   string   `json:"thread_kind"`
	RefID        string   `json:"ref_id"`
	Participants []string `json:"participants"`
}

// chatNotifier is the notification dependency, defined at the use site.
// *NotificationHandler implements it.
type chatNotifier interface {
	NotifyChatMessage(ctx context.Context, recipientID, threadID, threadKind, refID, messageID string)
}

// ChatNotifyConsumer turns each chat.message.created into a bell+push for every
// thread participant except the author. It binds a durable queue consumer to the
// CHAT_EVENTS stream, so across gateway replicas exactly one handles each event.
type ChatNotifyConsumer struct {
	js       nats.JetStreamContext
	notifier chatNotifier
	log      zerolog.Logger
}

func NewChatNotifyConsumer(js nats.JetStreamContext, n chatNotifier, log zerolog.Logger) *ChatNotifyConsumer {
	return &ChatNotifyConsumer{js: js, notifier: n, log: log}
}

// Subscribe binds the durable queue consumer. DeliverNew avoids replaying the
// whole backlog into everyone's inbox on first deploy.
func (c *ChatNotifyConsumer) Subscribe() error {
	_, err := c.js.QueueSubscribe(chatMsgSubject, chatNotifyQueue, c.handle,
		nats.Durable(chatNotifyDurable),
		nats.ManualAck(),
		nats.BindStream(chatEventsStream),
		nats.DeliverNew(),
		nats.AckWait(chatNotifyAckWait),
		nats.MaxDeliver(chatNotifyMaxDeliver),
	)
	return err
}

func (c *ChatNotifyConsumer) handle(msg *nats.Msg) {
	var evt chatMessageEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil || evt.MessageID == "" {
		// A malformed payload will never parse on retry — Ack to avoid a poison loop.
		c.log.Error().Err(err).Int("payload_bytes", len(msg.Data)).
			Msg("chat-notify: malformed event")
		_ = msg.Ack()
		return
	}

	ctx, cancel := natsutil.MsgContext(msg, chatNotifyTimeout)
	defer cancel()

	seen := make(map[string]struct{}, len(evt.Participants))
	for _, uid := range evt.Participants {
		uid = strings.TrimSpace(uid)
		if uid == "" || uid == evt.AuthorUserID {
			continue // never notify the author of their own message
		}
		if _, dup := seen[uid]; dup {
			continue // one bell per recipient even if the list repeats a uid
		}
		seen[uid] = struct{}{}
		// Notify is best-effort: it logs and swallows failures internally, so a
		// single recipient's inbox hiccup can't block the others or the Ack.
		c.notifier.NotifyChatMessage(ctx, uid, evt.ThreadID, evt.ThreadKind, evt.RefID, evt.MessageID)
	}
	_ = msg.Ack()
}
