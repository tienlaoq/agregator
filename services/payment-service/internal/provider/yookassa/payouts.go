// ЮKassa Payouts API (POST /v3/payouts).  Separate from the charge flow:
// payouts are issued from the platform's "agent" account to a partner's
// card/bank-account/SBP destination.  All requests are idempotent on
// Idempotence-Key and observable via the payout.* webhook events.

package yookassa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// ── Wire types ───────────────────────────────────────────────────────────────

// payoutDestination is the bank_account / sbp variant of the request body.
// Card payouts use a separate top-level PayoutToken field rather than this
// struct, per the ЮKassa Payouts API.
type payoutDestination struct {
	Type string `json:"type"`

	// Bank account
	BankID              string `json:"bank_id,omitempty"`
	AccountNumber       string `json:"account_number,omitempty"`
	RecipientFirstName  string `json:"recipient_first_name,omitempty"`
	RecipientLastName   string `json:"recipient_last_name,omitempty"`
	RecipientMiddleName string `json:"recipient_middle_name,omitempty"`
	RecipientINN        string `json:"recipient_inn,omitempty"`
	RecipientKPP        string `json:"recipient_kpp,omitempty"`

	// SBP
	Phone string `json:"phone,omitempty"`
}

// payoutRequest covers both rail variants ЮKassa exposes:
//
//   - Card: top-level payout_token; payout_destination_data omitted.
//   - Bank account / SBP: payout_destination_data populated; payout_token omitted.
//
// Both branches are marshalled by the same struct via omitempty so we never
// build two parallel request types.
type payoutRequest struct {
	Amount            amount             `json:"amount"`
	PayoutToken       string             `json:"payout_token,omitempty"`
	PayoutDestination *payoutDestination `json:"payout_destination_data,omitempty"`
	Description       string             `json:"description,omitempty"`
	Metadata          map[string]string  `json:"metadata,omitempty"`
}

type payoutResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	// CancellationDetails is populated only when status == "canceled".
	CancellationDetails *struct {
		Party  string `json:"party"`
		Reason string `json:"reason"`
	} `json:"cancellation_details,omitempty"`
}

// ── Status mapping ──────────────────────────────────────────────────────────

// payoutStatusToDomain maps ЮKassa payout statuses to provider-agnostic values.
// ЮKassa spells cancellation with one 'l' (canceled), matching the payment flow.
var payoutStatusToDomain = map[string]provider.PayoutStatus{
	"pending":   provider.PayoutStatusPending,
	"succeeded": provider.PayoutStatusSucceeded,
	"canceled":  provider.PayoutStatusFailed,
}

// ── PaymentProvider.CreatePayout implementation ─────────────────────────────

// CreatePayout posts a new payout request to ЮKassa.  Idempotent by
// Idempotence-Key.  Not retried at this level — like CreatePayment, the
// scheduler handles retries at a higher level using the same key so the
// provider deduplicates server-side.
func (c *Client) CreatePayout(ctx context.Context, req provider.PayoutRequest) (*provider.PayoutResult, error) {
	if c.mockMode {
		// Mirror CreatePayment mock semantics: terminal success right away,
		// no need for a webhook round-trip during local development.
		return &provider.PayoutResult{
			ProviderPayoutID: uuid.New().String(),
			Status:           provider.PayoutStatusSucceeded,
		}, nil
	}
	if req.AmountKopecks <= 0 {
		return nil, &apiError{statusCode: 400, op: "CreatePayout", body: "non-positive amount"}
	}
	if req.IdempotencyKey == "" {
		return nil, &apiError{statusCode: 400, op: "CreatePayout", body: "missing idempotency key"}
	}

	currency := req.Currency
	if currency == "" {
		currency = "RUB"
	}

	body := payoutRequest{
		Amount:      amount{Value: kopecksToStr(req.AmountKopecks), Currency: currency},
		Description: req.Description,
		Metadata: map[string]string{
			"partner_type": req.PartnerType,
			"partner_id":   req.PartnerID,
		},
	}

	// Card payouts go through the top-level payout_token; other rails use the
	// payout_destination_data nested object.  Setting both is rejected by ЮKassa.
	if req.Kind == provider.PayoutDestCard {
		if req.ProviderToken == "" {
			return nil, &apiError{statusCode: 400, op: "CreatePayout", body: "missing provider_token for card payout"}
		}
		body.PayoutToken = req.ProviderToken
	} else {
		dest, err := buildPayoutDestination(req)
		if err != nil {
			return nil, err
		}
		body.PayoutDestination = dest
	}

	resp, err := c.postPayout(ctx, body, providerKey(req.IdempotencyKey))
	if err != nil {
		return nil, classifyPayoutError(err)
	}

	result := &provider.PayoutResult{
		ProviderPayoutID: resp.ID,
		Status:           payoutStatusToDomain[resp.Status],
	}
	if result.Status == "" {
		// Unknown status — surface as pending; webhook will deliver the final state.
		result.Status = provider.PayoutStatusPending
	}
	if resp.CancellationDetails != nil {
		result.FailureReason = resp.CancellationDetails.Party + ":" + resp.CancellationDetails.Reason
	}
	return result, nil
}

// buildPayoutDestination handles bank_account and SBP rails (card payouts use
// the top-level payout_token in CreatePayout, not this struct).
func buildPayoutDestination(req provider.PayoutRequest) (*payoutDestination, error) {
	switch req.Kind {
	case provider.PayoutDestBankAccount:
		if req.BankBIC == "" || req.BankAccount == "" || req.RecipientName == "" {
			return nil, &apiError{statusCode: 400, op: "CreatePayout", body: "incomplete bank account destination"}
		}
		first, middle, last := splitRussianName(req.RecipientName)
		return &payoutDestination{
			Type:                "bank_account",
			BankID:              req.BankBIC,
			AccountNumber:       req.BankAccount,
			RecipientFirstName:  first,
			RecipientMiddleName: middle,
			RecipientLastName:   last,
			RecipientINN:        req.RecipientINN,
			RecipientKPP:        req.RecipientKPP,
		}, nil

	case provider.PayoutDestSBP:
		if req.SBPPhone == "" {
			return nil, &apiError{statusCode: 400, op: "CreatePayout", body: "missing SBP phone"}
		}
		return &payoutDestination{Type: "sbp", Phone: req.SBPPhone, BankID: req.SBPBankID}, nil
	}
	return nil, &apiError{statusCode: 400, op: "CreatePayout", body: "unknown destination kind: " + string(req.Kind)}
}

// splitRussianName splits "Фамилия Имя Отчество" into the three components
// ЮKassa expects.  Anything missing is returned as empty string.
//
// We deliberately keep this dumb: callers store the trimmed display name as
// the partner entered it, and the split happens only at the outbound boundary
// where ЮKassa demands the three fields separately.
func splitRussianName(fullName string) (first, middle, last string) {
	parts := strings.Fields(fullName)
	switch len(parts) {
	case 0:
		return "", "", ""
	case 1:
		return parts[0], "", ""
	case 2:
		// Russian convention: last_name first_name when two parts.
		return parts[1], "", parts[0]
	default:
		return parts[1], strings.Join(parts[2:], " "), parts[0]
	}
}

// ── PaymentProvider.ParsePayoutWebhook implementation ───────────────────────

// ParsePayoutWebhook parses payout.succeeded / payout.canceled notifications.
// Returns (nil, nil) when the event is not a payout event so the caller can
// fall back to the regular payment ParseWebhook.
//
// Unlike ParseWebhook for payments, no pull-confirm is needed: payouts have no
// two-phase capture flow, so the payload status is authoritative.  We still
// validate that the event name begins with "payout." to defend against
// misrouted notifications.
func (c *Client) ParsePayoutWebhook(_ context.Context, rawBody []byte, _ http.Header) (*provider.PayoutWebhookEvent, error) {
	var payload struct {
		Event  string `json:"event"`
		Object struct {
			ID                  string `json:"id"`
			Status              string `json:"status"`
			CancellationDetails *struct {
				Party  string `json:"party"`
				Reason string `json:"reason"`
			} `json:"cancellation_details,omitempty"`
		} `json:"object"`
	}
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return nil, fmt.Errorf("parse payout webhook body: %w", err)
	}
	if !strings.HasPrefix(payload.Event, "payout.") {
		return nil, nil // not a payout event; caller falls back to payment parsing
	}
	if payload.Object.ID == "" {
		return nil, fmt.Errorf("payout webhook: missing id in payload")
	}

	status, ok := payoutStatusToDomain[payload.Object.Status]
	if !ok {
		status = provider.PayoutStatusPending
	}

	evt := &provider.PayoutWebhookEvent{
		ProviderPayoutID: payload.Object.ID,
		Status:           status,
		RawEvent:         payload.Event,
	}
	if payload.Object.CancellationDetails != nil {
		evt.FailureReason = payload.Object.CancellationDetails.Party + ":" + payload.Object.CancellationDetails.Reason
	}
	return evt, nil
}

// ── Internal ────────────────────────────────────────────────────────────────

func (c *Client) postPayout(ctx context.Context, body payoutRequest, idempotencyKey string) (*payoutResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal payout request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.yookassa.ru/v3/payouts", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create payout request: %w", err)
	}
	req.SetBasicAuth(c.shopID, c.secretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotence-Key", idempotencyKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Network / TLS / TCP-level failure — request may or may not have reached the
		// provider. Mark transient so the caller does NOT compensate (would double-pay
		// if the request actually landed).
		return nil, fmt.Errorf("do payout request: %w: %w", provider.ErrTransient, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		// Response body read failed — provider state is unknown (status line was read
		// but body wasn't). Conservatively treat as transient.
		return nil, fmt.Errorf("read payout response: %w: %w", provider.ErrTransient, err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, &apiError{statusCode: resp.StatusCode, body: string(respBody), op: "CreatePayout"}
	}

	var out payoutResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("unmarshal payout response: %w", err)
	}
	return &out, nil
}

// classifyPayoutError tags an error from postPayout as transient (when the
// provider state is unknown after a 5xx) so the usecase keeps the payout in
// pending for reconciliation rather than reversing the ledger debit.
//
// 4xx responses are permanent (validation, auth, malformed destination) — the
// provider rejected the request and never issued the payout, so reversal is
// safe.  Network errors are already wrapped with ErrTransient inside postPayout.
//
// Errors that are neither *apiError nor already-wrapped (e.g. JSON marshal
// failures) are returned unchanged: the request never went out, so they are
// permanent by construction.
func classifyPayoutError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, provider.ErrTransient) {
		return err
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		if apiErr.statusCode >= 500 || apiErr.statusCode == 0 {
			return fmt.Errorf("%w: %w", provider.ErrTransient, err)
		}
		// 4xx — permanent. Safe to reverse.
	}
	return err
}
