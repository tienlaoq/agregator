// Package notify provides an asynchronous partner-registration notification
// worker that sends Telegram messages without blocking the gRPC Register call.
package notify

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/rs/zerolog"

	pkgtelegram "github.com/tienlao/agregator/pkg/telegram"
	"github.com/tienlao/agregator/services/auth-service/internal/domain"
)

const (
	// defaultBufSize is the number of events that can be queued before Enqueue
	// starts dropping. At ~1 partner registration per second (very generous),
	// this covers a 64-second burst with zero worker capacity — sufficient for
	// any realistic Telegram API degradation window.
	defaultBufSize = 64

	// workerSendTimeout caps a single Telegram HTTP POST. The Telegram Bot API
	// is typically <500 ms; 5 s is a generous ceiling that still lets the worker
	// drain the queue quickly on shutdown.
	workerSendTimeout = 5 * time.Second

	// drainTimeout is the extra time the worker waits after ctx cancellation to
	// flush buffered events before exiting. Keeps shutdown clean without
	// indefinitely blocking os.Exit.
	drainTimeout = 10 * time.Second
)

// TelegramWorker buffers PartnerRegisteredEvents and delivers them to Telegram
// in a single background goroutine. It implements domain.PartnerNotifier.
//
// Start the worker with Run(ctx) in a goroutine before use. The worker exits
// when ctx is cancelled and drains remaining buffered events (up to drainTimeout)
// so that notifications queued just before shutdown are not silently dropped.
type TelegramWorker struct {
	tg          *pkgtelegram.Client
	frontendURL string
	ch          chan domain.PartnerRegisteredEvent
	log         zerolog.Logger
}

// NewTelegramWorker creates a worker but does not start it. Call Run in a
// goroutine.
func NewTelegramWorker(
	tg *pkgtelegram.Client,
	frontendURL string,
	log zerolog.Logger,
) *TelegramWorker {
	return &TelegramWorker{
		tg:          tg,
		frontendURL: frontendURL,
		ch:          make(chan domain.PartnerRegisteredEvent, defaultBufSize),
		log:         log,
	}
}

// Enqueue adds ev to the internal buffer and returns immediately. If the
// buffer is full (worker falling behind) the event is dropped with a Warn log
// so that the caller — the Register RPC handler — is never blocked.
func (w *TelegramWorker) Enqueue(ev domain.PartnerRegisteredEvent) {
	select {
	case w.ch <- ev:
	default:
		w.log.Warn().
			Str("role", ev.Role).Str("user_id", ev.UserID).
			Msg("notify: partner-registration event dropped — buffer full")
	}
}

// Run processes events until ctx is cancelled, then drains the channel for up
// to drainTimeout before returning. Call in a goroutine:
//
//	go worker.Run(ctx)
func (w *TelegramWorker) Run(ctx context.Context) {
	w.log.Info().Int("buf_size", defaultBufSize).Msg("notify: partner-registration worker started")
	for {
		select {
		case ev := <-w.ch:
			w.send(ev)
		case <-ctx.Done():
			w.drain()
			w.log.Info().Msg("notify: partner-registration worker stopped")
			return
		}
	}
}

// drain flushes buffered events after ctx cancellation, respecting drainTimeout.
func (w *TelegramWorker) drain() {
	deadline := time.After(drainTimeout)
	for {
		select {
		case ev := <-w.ch:
			w.send(ev)
		case <-deadline:
			remaining := len(w.ch)
			if remaining > 0 {
				w.log.Warn().Int("dropped", remaining).
					Msg("notify: drain timeout — some partner-registration events dropped on shutdown")
			}
			return
		default:
			return // channel empty
		}
	}
}

// send formats and delivers a single event to Telegram.
func (w *TelegramWorker) send(ev domain.PartnerRegisteredEvent) {
	if w.tg == nil || !w.tg.Enabled() {
		return
	}

	var roleRu, adminPath, emoji string
	switch ev.Role {
	case "master":
		roleRu, adminPath, emoji = "Пар-мастер", "/admin/masters", "🧖"
	case "venue_owner":
		roleRu, adminPath, emoji = "Партнёр бани", "/admin/venues", "🏛"
	default:
		w.log.Warn().Str("role", ev.Role).Msg("notify: unknown partner role — skipping")
		return
	}

	// w.frontendURL is already normalised by config.normaliseFrontendURL:
	// trimmed, no trailing slash, non-empty (falls back to localhost:3000).
	link := w.frontendURL + adminPath

	phoneDisp := strings.TrimSpace(ev.Phone)
	if phoneDisp == "" {
		phoneDisp = "—"
	}

	text := fmt.Sprintf(
		"%s <b>Новая регистрация</b>\n\n"+
			"Тип аккаунта: <b>%s</b>\n"+
			"Имя: %s\n"+
			"Email: <code>%s</code>\n"+
			"Телефон: %s\n"+
			"User ID: <code>%s</code>\n\n"+
			`<a href="%s">Открыть в админке</a>`,
		emoji,
		html.EscapeString(roleRu),
		html.EscapeString(strings.TrimSpace(ev.Name)),
		html.EscapeString(strings.TrimSpace(ev.Email)),
		html.EscapeString(phoneDisp),
		ev.UserID, // UUID: charset [0-9a-f-], no escaping needed
		link,      // constructed from trusted frontendURL + const path
	)

	sendCtx, cancel := context.WithTimeout(context.Background(), workerSendTimeout)
	defer cancel()
	if err := w.tg.SendHTML(sendCtx, text); err != nil {
		w.log.Warn().Err(err).Str("role", ev.Role).Str("user_id", ev.UserID).
			Msg("notify: telegram partner registration failed")
	}
}
