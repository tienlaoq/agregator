// Package limits centralises every business-level constraint for the
// api-gateway in one place so they are easy to review, compare, and override
// for load testing via environment variables.
//
// # Overrides
//
// Set GATEWAY_LIMIT_<NAME>=<value> to override a limit at startup.
// Numeric limits accept plain integers; time limits accept Go duration strings
// (e.g. "30s", "2m").  An unparseable value is silently ignored and the
// compiled-in default is used instead.
//
// Example (load test with relaxed chat rate-limiting):
//
//	GATEWAY_LIMIT_CHAT_SEND_RATE_MAX=1000 ./api-gateway
//
// Note: overrides are resolved once at package init time. Changing env vars
// after the process has started has no effect.
package limits

import (
	"os"
	"strconv"
	"time"
)

// ── Chat ─────────────────────────────────────────────────────────────────────

var (
	// ChatSendRateMax is the maximum number of messages a user may send per
	// ChatSendRateWindow on a single thread.
	ChatSendRateMax = intLimit("CHAT_SEND_RATE_MAX", 20)

	// ChatSendRateWindow is the rolling window for the chat send-rate limit.
	ChatSendRateWindow = durLimit("CHAT_SEND_RATE_WINDOW", time.Minute)

	// ChatWSTicketTTL is how long a one-time WebSocket auth ticket remains valid
	// after issuance.
	ChatWSTicketTTL = durLimit("CHAT_WS_TICKET_TTL", 90*time.Second)

	// ChatWSReadLimit caps incoming WebSocket JSON frames to guard against memory
	// exhaustion.  8 KiB is generous for any legitimate chat payload.
	ChatWSReadLimit = int64(intLimit("CHAT_WS_READ_LIMIT", 8*1024))

	// ChatWSPingInterval is how often the server sends a ping frame to detect
	// dead connections.
	ChatWSPingInterval = durLimit("CHAT_WS_PING_INTERVAL", 30*time.Second)

	// ChatWSPongWait is how long the server waits for a pong before closing the
	// connection.  Must satisfy PongWait > PingInterval to allow at least one
	// full round-trip.  The 10 s slack on top of PingInterval absorbs network
	// jitter, TLS re-negotiations, and client-side GC pauses that could otherwise
	// cause spurious disconnects even when the client is still alive.
	//
	// Invariant (enforced at init by chat.go): PongWait > PingInterval.
	ChatWSPongWait = durLimit("CHAT_WS_PONG_WAIT", 30*time.Second+10*time.Second)

	// ChatWSWriteWait is the deadline for a single WebSocket write operation.
	ChatWSWriteWait = durLimit("CHAT_WS_WRITE_WAIT", 10*time.Second)

	// ChatInMemoryPruneInterval is how often the in-memory rate-limiter prunes
	// stale keys to prevent unbounded map growth during long QA sessions.
	ChatInMemoryPruneInterval = durLimit("CHAT_IN_MEMORY_PRUNE_INTERVAL", 5*time.Minute)
)

// ── JSON request bodies ───────────────────────────────────────────────────────

var (
	// JSONMaxBodyBytes is the maximum size of a JSON request body accepted by
	// readJSON. Requests that exceed this limit are rejected with 413 before the
	// body is decoded. The default (64 KiB) is generous for any legitimate JSON
	// API payload; override with JSON_MAX_BODY_BYTES if a specific endpoint
	// requires a larger limit (e.g. bulk-import).
	JSONMaxBodyBytes = int64(intLimit("JSON_MAX_BODY_BYTES", 64<<10)) // 64 KiB
)

// ── Photo uploads ─────────────────────────────────────────────────────────────

var (
	// PhotoMaxBytes is the maximum size of a single uploaded photo (venue or
	// master).  The multipart reader is given PhotoMaxBytes+1024 to account for
	// boundary overhead.
	PhotoMaxBytes = int64(intLimit("PHOTO_MAX_BYTES", 5<<20)) // 5 MiB
)

// ── Support tickets ───────────────────────────────────────────────────────────

var (
	// SupportWebhookTimeout is the HTTP client timeout for outbound helpdesk
	// webhook calls.
	SupportWebhookTimeout = durLimit("SUPPORT_WEBHOOK_TIMEOUT", 6*time.Second)

	// SupportMaxBodyBytes is the response-body read limit for helpdesk webhook
	// responses.  Declared as int64 so it can be passed directly to
	// io.LimitReader without a cast at every call site.
	SupportMaxBodyBytes = int64(intLimit("SUPPORT_MAX_BODY_BYTES", 8192))

	// SupportMaxTopicLen is the maximum character length of a ticket topic.
	SupportMaxTopicLen = intLimit("SUPPORT_MAX_TOPIC_LEN", 180)

	// SupportMaxMessageLen is the maximum character length of a ticket message.
	SupportMaxMessageLen = intLimit("SUPPORT_MAX_MESSAGE_LEN", 4000)

	// SupportMaxEmailLen is the maximum character length of the email field.
	SupportMaxEmailLen = intLimit("SUPPORT_MAX_EMAIL_LEN", 320)

	// SupportMaxRefFieldLen is the maximum length of reference fields (BookingID,
	// PaymentID).
	SupportMaxRefFieldLen = intLimit("SUPPORT_MAX_REF_FIELD_LEN", 128)

	// SupportMaxSourcePageLen is the maximum length of the source-page URL field.
	SupportMaxSourcePageLen = intLimit("SUPPORT_MAX_SOURCE_PAGE_LEN", 1024)
)

// ── Payment ───────────────────────────────────────────────────────────────────

var (
	// PaymentWebhookMaxBodyBytes is the maximum size of an inbound payment
	// webhook body.  256 KiB is sufficient for any YooKassa payload.
	PaymentWebhookMaxBodyBytes = int64(intLimit("PAYMENT_WEBHOOK_MAX_BODY_BYTES", 256<<10))
)

// ── Master ────────────────────────────────────────────────────────────────────

var (
	// MasterImportMaxBodyBytes is the maximum size of a master-profile import
	// JSON body.
	MasterImportMaxBodyBytes = int64(intLimit("MASTER_IMPORT_MAX_BODY_BYTES", 1<<20)) // 1 MiB

	// MasterPublicRateMax / MasterPublicRateWindow guard the public catalogue
	// endpoints (GET /masters and GET /masters/{slug}) against scraping and
	// enumeration. 120 req / min per IP allows a normal browser session with
	// paginated search while blocking tight scraping loops.
	MasterPublicRateMax    = intLimit("MASTER_PUBLIC_RATE_MAX", 120)
	MasterPublicRateWindow = durLimit("MASTER_PUBLIC_RATE_WINDOW", time.Minute)
)

// ── WebSocket hub ─────────────────────────────────────────────────────────────

var (
	// ChatHubQueueSize is the per-connection send-queue capacity.  A full queue
	// causes the connection to be closed rather than blocking the broadcaster.
	ChatHubQueueSize = intLimit("CHAT_HUB_QUEUE_SIZE", 64)

	// ChatHubWriteTimeout is the write deadline for a single hub broadcast write.
	ChatHubWriteTimeout = durLimit("CHAT_HUB_WRITE_TIMEOUT", 5*time.Second)
)

// ── helpers ───────────────────────────────────────────────────────────────────

// intLimit returns the compiled-in default unless GATEWAY_LIMIT_<name> is set
// to a valid integer, in which case that value is used instead.
func intLimit(name string, def int) int {
	if raw := os.Getenv("GATEWAY_LIMIT_" + name); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			return v
		}
	}
	return def
}

// durLimit returns the compiled-in default unless GATEWAY_LIMIT_<name> is set
// to a valid Go duration string (e.g. "30s", "2m"), in which case that value
// is used instead.
func durLimit(name string, def time.Duration) time.Duration {
	if raw := os.Getenv("GATEWAY_LIMIT_" + name); raw != "" {
		if v, err := time.ParseDuration(raw); err == nil {
			return v
		}
	}
	return def
}
