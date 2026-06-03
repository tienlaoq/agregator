// Package yookassa implements the provider.PaymentProvider interface for ЮKassa
// (https://yookassa.ru).  All ЮKassa-specific wire formats, status strings, and
// two-phase capture mechanics are encapsulated here; nothing leaks into the
// usecase or domain layers.
package yookassa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// Timeouts and retry policy.
const (
	// totalTimeout is the end-to-end deadline for a single HTTP call,
	// including connection, TLS handshake, request write, and response read.
	// If ЮKassa hangs, the call unblocks after this duration so the gRPC
	// connection pool is not exhausted.
	totalTimeout = 10 * time.Second

	// retryAttempts is the number of attempts for idempotent operations
	// (Capture, Refund, getPaymentStatus).  CreatePayment is also idempotent
	// by Idempotence-Key but a failed create is intentionally not retried here
	// — the caller (usecase crash-recovery) handles that at a higher level.
	retryAttempts = 3

	// retryBase is the initial backoff interval; each attempt doubles it
	// (capped at retryMax) with ±25 % full-jitter to spread thundering herds.
	retryBase = 200 * time.Millisecond
	retryMax  = 4 * time.Second

	// pullConfirmTimeout is the maximum wall-clock budget for the pull-confirm
	// GET inside ParseWebhook (3 attempts × 10 s HTTP timeout + backoff ≈ 38 s
	// worst case).  We cap it at 15 s regardless of how generous the upstream
	// gRPC deadline is: if ЮKassa is degraded the webhook handler must return
	// quickly so that ЮKassa's own retry does not create a thundering herd of
	// in-flight connections.  The ctx is narrowed with context.WithTimeout so
	// that even a Background ctx (e.g. in tests) gets this ceiling applied.
	pullConfirmTimeout = 15 * time.Second
)

// Client implements provider.PaymentProvider for ЮKassa.
// Use NewClient to construct; the zero value is not valid.
// All methods are safe for concurrent use.
type Client struct {
	shopID     string
	secretKey  string
	httpClient *http.Client
	mockMode   bool
}

// NewClient creates a new ЮKassa client with a production-ready HTTP transport.
// When shopID is empty the client runs in mock mode: all methods succeed
// without making real HTTP calls.
func NewClient(shopID, secretKey string) *Client {
	return &Client{
		shopID:    shopID,
		secretKey: secretKey,
		// Transport is shared across all requests for connection reuse (keep-alive).
		// MaxIdleConns / MaxIdleConnsPerHost prevent socket exhaustion under burst load.
		httpClient: &http.Client{
			Timeout: totalTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        64,
				MaxIdleConnsPerHost: 16,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		mockMode: shopID == "",
	}
}

// IsMockMode reports whether the client runs without real network calls.
func (c *Client) IsMockMode() bool { return c.mockMode }

// ── Wire types ───────────────────────────────────────────────────────────────

type amount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type confirmation struct {
	Type      string `json:"type"`
	ReturnURL string `json:"return_url"`
}

type paymentRequest struct {
	Amount       amount       `json:"amount"`
	Confirmation confirmation `json:"confirmation"`
	Description  string       `json:"description"`
	Capture      bool         `json:"capture"`
}

type paymentResponse struct {
	ID           string `json:"id"`
	Confirmation struct {
		ConfirmationURL string `json:"confirmation_url"`
	} `json:"confirmation"`
	Status string `json:"status"`
}

// webhookPayload is the wire format of a ЮKassa event notification.
// Only the fields required for routing and permitted structured logging are
// declared; all other fields (amount, metadata, payer, etc.) must not be read
// or logged per the webhook logging policy in CLAUDE.md.
type webhookPayload struct {
	// Event is the notification type, e.g. "payment.succeeded".
	// Permitted log field per CLAUDE.md.
	Event  string `json:"event"`
	Object struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"object"`
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// kopecksToStr converts an integer kopeck amount to the decimal string format
// expected by the ЮKassa API (e.g. 150050 → "1500.50").
//
// float64 is intentionally avoided: for amounts above ~2^53 kopecks
// (≈90 trillion roubles) float64 loses integer precision and would silently
// produce a wrong value.  Integer arithmetic is exact for all representable
// int64 values.
func kopecksToStr(k int64) string {
	return fmt.Sprintf("%d.%02d", k/100, k%100)
}

// yookassaNamespace is the UUID v5 namespace for deriving the Idempotence-Key
// header sent to ЮKassa.  Fixed so that the derived key is stable across
// restarts and namespaced away from other UUID v5 usages in the system.
var yookassaNamespace = uuid.MustParse("7d3f4e1a-92b0-4c8d-bf3a-1e5f6a2c9d0e")

// providerKey converts an internal idempotency key to a UUID v5 string suitable
// for ЮKassa's Idempotence-Key header (≤64 chars, UUID format required).
//
// Deterministic: crash-recovery retries with the same internalKey always
// produce the same UUID, so ЮKassa returns the same payment rather than
// creating a duplicate.
func providerKey(internalKey string) string {
	return uuid.NewSHA1(yookassaNamespace, []byte(internalKey)).String()
}

// statusToDomain maps ЮKassa pull-confirmed status strings to domain values.
// Statuses absent from this map are treated as non-terminal (no-op).
var statusToDomain = map[string]domain.PaymentStatus{
	"succeeded": domain.StatusSucceeded,
	"canceled":  domain.StatusCancelled, // ЮKassa spells it with one 'l'
}

// ── Retry helpers ─────────────────────────────────────────────────────────────

// retryDo executes fn up to retryAttempts times.  It retries only on transient
// network errors or 5xx responses.  4xx responses are permanent failures and
// are returned immediately without retry.
//
// Backoff: base * 2^attempt with ±25% full jitter, capped at retryMax.
// The caller must pass a context so the whole retry loop respects upstream
// deadlines (e.g. the gRPC server deadline).
func retryDo(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := range retryAttempts {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled before attempt %d: %w", attempt+1, err)
		}
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if isPermanent(lastErr) {
			return lastErr
		}
		if attempt < retryAttempts-1 {
			sleep := backoff(attempt)
			select {
			case <-time.After(sleep):
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during backoff: %w", ctx.Err())
			}
		}
	}
	return fmt.Errorf("all %d attempts failed: %w", retryAttempts, lastErr)
}

// isPermanent returns true for errors that must not be retried.
// Currently: HTTP 4xx client errors (bad request, auth failure, not found).
func isPermanent(err error) bool {
	var apiErr *apiError
	if asAPIError(err, &apiErr) {
		return apiErr.statusCode >= 400 && apiErr.statusCode < 500
	}
	return false
}

// backoff returns the sleep duration for a given attempt index (0-based).
// Formula: base * 2^attempt, capped at retryMax, with ±25% uniform jitter.
func backoff(attempt int) time.Duration {
	exp := retryBase * (1 << attempt) // base, 2×base, 4×base, …
	if exp > retryMax {
		exp = retryMax
	}
	// Full jitter in [0.75×exp, 1.25×exp].
	jitter := time.Duration(rand.Float64()*float64(exp)/2) - exp/4
	d := exp + jitter
	if d < 0 {
		d = 0
	}
	return d
}

// ── apiError ──────────────────────────────────────────────────────────────────

// apiError wraps a non-2xx ЮKassa response so isPermanent can inspect the code.
type apiError struct {
	statusCode int
	body       string
	op         string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("yookassa %s: HTTP %d: %s", e.op, e.statusCode, e.body)
}

// asAPIError is a poor-man's errors.As for *apiError without importing errors.
// We use a simple type assertion because apiError is always wrapped as a pointer
// at the first level; the retry helper never double-wraps it.
func asAPIError(err error, target **apiError) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*apiError); ok {
		*target = e
		return true
	}
	return false
}

// ── PaymentProvider implementation ──────────────────────────────────────────

// CreatePayment creates a ЮKassa payment.
//
// All charges land on the single platform shop account (escrow model).
// We always use capture=true (immediate capture) — there is no marketplace
// split at the provider, and partner payouts are issued separately through
// CreatePayout.
//
// CreatePayment is idempotent by Idempotence-Key but is NOT retried here —
// the usecase crash-recovery loop handles retries at a higher level to avoid
// ambiguity about whether a payment was created.
func (c *Client) CreatePayment(ctx context.Context, req provider.CreateRequest) (*provider.Result, error) {
	if c.mockMode {
		mockID := uuid.New().String()
		return &provider.Result{
			ProviderPaymentID: mockID,
			ConfirmationURL:   fmt.Sprintf("https://mock.yookassa.ru/pay/%s", mockID),
		}, nil
	}
	if req.AmountKopecks <= 0 {
		return nil, &apiError{statusCode: 400, op: "CreatePayment", body: "non-positive gross amount"}
	}

	body := paymentRequest{
		Amount:       amount{Value: kopecksToStr(req.AmountKopecks), Currency: "RUB"},
		Confirmation: confirmation{Type: "redirect", ReturnURL: req.ReturnURL},
		Description:  req.Description,
		Capture:      true,
	}

	// CreatePayment is called once; ctx deadline enforces the total call budget.
	resp, err := c.postPayment(ctx, body, providerKey(req.IdempotencyKey))
	if err != nil {
		return nil, err
	}
	return &provider.Result{
		ProviderPaymentID: resp.PaymentID,
		ConfirmationURL:   resp.ConfirmationURL,
	}, nil
}

// Capture confirms a previously authorized ЮKassa payment (two-phase capture).
// Retried up to retryAttempts times on transient errors; idempotent by captureKey.
// ctx is propagated to the retry loop and each HTTP request — if the upstream
// gRPC deadline fires or shutdown is requested the loop stops immediately.
func (c *Client) Capture(ctx context.Context, providerPaymentID, idempotencyKey string) error {
	if c.mockMode {
		return nil
	}
	// Body must be re-created per attempt: bytes.NewReader is single-use.
	return retryDo(ctx, func() error {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"https://api.yookassa.ru/v3/payments/"+providerPaymentID+"/capture",
			bytes.NewReader([]byte("{}")),
		)
		if err != nil {
			return fmt.Errorf("create capture request: %w", err)
		}
		req.SetBasicAuth(c.shopID, c.secretKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotence-Key", idempotencyKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("capture do request: %w", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("capture read response: %w", err)
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			return &apiError{statusCode: resp.StatusCode, body: string(body), op: "Capture"}
		}
		return nil
	})
}

// Refund issues a full refund via POST /v3/refunds.
// Retried up to retryAttempts times on transient errors; idempotent by idempotencyKey.
// ctx is propagated to the retry loop and each HTTP request.
func (c *Client) Refund(ctx context.Context, providerPaymentID string, amountKopecks int64, idempotencyKey string) error {
	if c.mockMode {
		return nil
	}
	reqBody := struct {
		PaymentID string `json:"payment_id"`
		Amount    amount `json:"amount"`
	}{
		PaymentID: providerPaymentID,
		Amount:    amount{Value: kopecksToStr(amountKopecks), Currency: "RUB"},
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal refund request: %w", err)
	}
	return retryDo(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.yookassa.ru/v3/refunds", bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("create refund request: %w", err)
		}
		req.SetBasicAuth(c.shopID, c.secretKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotence-Key", idempotencyKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("refund do request: %w", err)
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("refund read response: %w", err)
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			return &apiError{statusCode: resp.StatusCode, body: string(respBody), op: "Refund"}
		}
		return nil
	})
}

// VerifySignature for ЮKassa relies on IP allowlisting configured at the
// infrastructure level.  Returns nil unconditionally; if ЮKassa adds HMAC
// signing in future, implement the check here.
func (c *Client) VerifySignature(_ context.Context, _ []byte, _ http.Header) error {
	return nil
}

// ParseWebhook parses a raw ЮKassa notification, applies pull-confirm for
// terminal-state events, and returns a provider-agnostic WebhookEvent.
//
// Protocol:
//
//   - "waiting_for_capture": authorized but not yet captured.
//     Returns RequiresCapture=true; pull-confirm skipped (status is
//     unambiguous and the authorization window may expire before a round-trip).
//
//   - Any other status: pull-confirm via GET /v3/payments/{id} to obtain the
//     authoritative state.  The payload status is ignored for terminal
//     transitions to prevent replay/tampering.
//
//   - Non-terminal confirmed status (e.g. "pending"): returns StatusPending
//     with RequiresCapture=false — usecase treats this as a no-op.
func (c *Client) ParseWebhook(ctx context.Context, rawBody []byte, _ http.Header) (*domain.WebhookEvent, error) {
	var payload webhookPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return nil, fmt.Errorf("parse webhook body: %w", err)
	}

	providerID := payload.Object.ID
	if providerID == "" {
		return nil, fmt.Errorf("webhook: missing payment id in payload")
	}

	if payload.Object.Status == "waiting_for_capture" {
		return &domain.WebhookEvent{
			ProviderPaymentID: providerID,
			Status:            domain.StatusPending,
			RequiresCapture:   true,
			CaptureKey:        providerID + "-capture",
			RawEvent:          payload.Event,
			RawProviderStatus: payload.Object.Status,
		}, nil
	}

	rawStatus, err := c.getPaymentStatus(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("pull-confirm: %w", err)
	}

	domainStatus, ok := statusToDomain[rawStatus]
	if !ok {
		// rawStatus came back from ЮKassa but is not a status we map to a
		// terminal domain state — treat as non-terminal (no-op for the usecase).
		// RawProviderStatus is surfaced to the caller so the delivery layer can
		// emit a warning log instead of silently absorbing an unrecognised status.
		domainStatus = domain.StatusPending
	}

	return &domain.WebhookEvent{
		ProviderPaymentID: providerID,
		Status:            domainStatus,
		RawEvent:          payload.Event,
		RawProviderStatus: rawStatus, // pull-confirmed status, not payload hint
	}, nil
}

// ── Internal helpers ─────────────────────────────────────────────────────────

type yooKassaPaymentResult struct {
	PaymentID       string
	ConfirmationURL string
}

func (c *Client) postPayment(ctx context.Context, body paymentRequest, idempotencyKey string) (*yooKassaPaymentResult, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.yookassa.ru/v3/payments", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(c.shopID, c.secretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotence-Key", idempotencyKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, &apiError{statusCode: resp.StatusCode, body: string(respBody), op: "CreatePayment"}
	}

	var result paymentResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &yooKassaPaymentResult{
		PaymentID:       result.ID,
		ConfirmationURL: result.Confirmation.ConfirmationURL,
	}, nil
}

// getPaymentStatus fetches the authoritative payment status via GET /v3/payments/{id}.
// Retried up to retryAttempts times on transient errors.
//
// A pullConfirmTimeout ceiling is applied regardless of what the caller's ctx
// allows: with 3 attempts × 10 s HTTP timeout + backoff the worst case is ~38 s,
// which keeps an in-flight webhook connection alive long enough for ЮKassa to
// fire a second retry, compounding load.  The ceiling keeps worst-case latency
// predictable.  If the caller's ctx is already shorter, its deadline wins (the
// narrowing is a min, not an override).
//
// In mock mode always returns "succeeded".
func (c *Client) getPaymentStatus(ctx context.Context, providerPaymentID string) (string, error) {
	if c.mockMode {
		return "succeeded", nil
	}

	ctx, cancel := context.WithTimeout(ctx, pullConfirmTimeout)
	defer cancel()

	var result string
	err := retryDo(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.yookassa.ru/v3/payments/"+providerPaymentID, nil)
		if err != nil {
			return fmt.Errorf("create get-payment request: %w", err)
		}
		req.SetBasicAuth(c.shopID, c.secretKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("get-payment do request: %w", err)
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("get-payment read response: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return &apiError{statusCode: resp.StatusCode, body: string(respBody), op: "getPaymentStatus"}
		}

		var r struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(respBody, &r); err != nil {
			return fmt.Errorf("unmarshal get-payment response: %w", err)
		}
		if r.Status == "" {
			return fmt.Errorf("yookassa get-payment: empty status in response")
		}
		result = r.Status
		return nil
	})
	return result, err
}
