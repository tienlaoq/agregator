package yookassa

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// roundTripperFunc stands in for the network: it captures the outbound request
// and returns a canned response so CreatePayout can be exercised end to end
// without touching ЮKassa.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// ── splitRussianName ──────────────────────────────────────────────────────────

func TestSplitRussianName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		in                  string
		first, middle, last string
	}{
		{"empty", "", "", "", ""},
		{"single", "Иванов", "Иванов", "", ""},
		{"last first", "Иванов Иван", "Иван", "", "Иванов"},
		{"full triple", "Иванов Иван Иванович", "Иван", "Иванович", "Иванов"},
		{"extra parts fold into middle", "Иванов Иван Иванович Сергеевич", "Иван", "Иванович Сергеевич", "Иванов"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			first, middle, last := splitRussianName(tt.in)
			if first != tt.first || middle != tt.middle || last != tt.last {
				t.Errorf("splitRussianName(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.in, first, middle, last, tt.first, tt.middle, tt.last)
			}
		})
	}
}

// ── buildPayoutDestination ────────────────────────────────────────────────────

// The bank-account rail must place each requisite in the field ЮKassa expects —
// a mis-mapping here sends money to the wrong account.
func TestBuildPayoutDestination_BankAccount(t *testing.T) {
	t.Parallel()

	dest, err := buildPayoutDestination(provider.PayoutRequest{
		Kind:          provider.PayoutDestBankAccount,
		BankBIC:       "044525225",
		BankAccount:   "40817810099910004312",
		RecipientName: "Иванов Иван Иванович",
		RecipientINN:  "7707083893",
		RecipientKPP:  "770701001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dest.Type != "bank_account" {
		t.Errorf("Type = %q, want bank_account", dest.Type)
	}
	if dest.BankID != "044525225" {
		t.Errorf("BankID = %q, want the BIC", dest.BankID)
	}
	if dest.AccountNumber != "40817810099910004312" {
		t.Errorf("AccountNumber = %q, want the account", dest.AccountNumber)
	}
	if dest.RecipientFirstName != "Иван" || dest.RecipientMiddleName != "Иванович" || dest.RecipientLastName != "Иванов" {
		t.Errorf("name split = (%q, %q, %q), want (Иван, Иванович, Иванов)",
			dest.RecipientFirstName, dest.RecipientMiddleName, dest.RecipientLastName)
	}
	if dest.RecipientINN != "7707083893" || dest.RecipientKPP != "770701001" {
		t.Errorf("INN/KPP = (%q, %q), want (7707083893, 770701001)", dest.RecipientINN, dest.RecipientKPP)
	}
}

func TestBuildPayoutDestination_SBP(t *testing.T) {
	t.Parallel()

	dest, err := buildPayoutDestination(provider.PayoutRequest{
		Kind:      provider.PayoutDestSBP,
		SBPPhone:  "+79991234567",
		SBPBankID: "100000000111",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dest.Type != "sbp" {
		t.Errorf("Type = %q, want sbp", dest.Type)
	}
	if dest.Phone != "+79991234567" {
		t.Errorf("Phone = %q, want the SBP phone", dest.Phone)
	}
	if dest.BankID != "100000000111" {
		t.Errorf("BankID = %q, want the SBP bank id", dest.BankID)
	}
}

func TestBuildPayoutDestination_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  provider.PayoutRequest
	}{
		{"bank missing bic", provider.PayoutRequest{Kind: provider.PayoutDestBankAccount, BankAccount: "40817810099910004312", RecipientName: "Иванов Иван"}},
		{"bank missing account", provider.PayoutRequest{Kind: provider.PayoutDestBankAccount, BankBIC: "044525225", RecipientName: "Иванов Иван"}},
		{"bank missing name", provider.PayoutRequest{Kind: provider.PayoutDestBankAccount, BankBIC: "044525225", BankAccount: "40817810099910004312"}},
		{"sbp missing phone", provider.PayoutRequest{Kind: provider.PayoutDestSBP, SBPBankID: "100000000111"}},
		{"card not handled here", provider.PayoutRequest{Kind: provider.PayoutDestCard, ProviderToken: "tok"}},
		{"unknown kind", provider.PayoutRequest{Kind: provider.PayoutDestinationKind("bogus")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := buildPayoutDestination(tt.req); err == nil {
				t.Errorf("expected an error for %s, got nil", tt.name)
			}
		})
	}
}

// ── ParsePayoutWebhook ────────────────────────────────────────────────────────

func TestParsePayoutWebhook_Succeeded(t *testing.T) {
	t.Parallel()

	c := NewClient("shop", "key")
	body := []byte(`{"event":"payout.succeeded","object":{"id":"po_1","status":"succeeded"}}`)
	evt, err := c.ParsePayoutWebhook(context.Background(), body, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt == nil {
		t.Fatal("expected an event, got nil")
	}
	if evt.ProviderPayoutID != "po_1" {
		t.Errorf("ProviderPayoutID = %q, want po_1", evt.ProviderPayoutID)
	}
	if evt.Status != provider.PayoutStatusSucceeded {
		t.Errorf("Status = %v, want succeeded", evt.Status)
	}
}

func TestParsePayoutWebhook_Canceled_MapsToFailedWithReason(t *testing.T) {
	t.Parallel()

	c := NewClient("shop", "key")
	body := []byte(`{"event":"payout.canceled","object":{"id":"po_2","status":"canceled","cancellation_details":{"party":"payout_network","reason":"account_closed"}}}`)
	evt, err := c.ParsePayoutWebhook(context.Background(), body, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Status != provider.PayoutStatusFailed {
		t.Errorf("Status = %v, want failed", evt.Status)
	}
	if evt.FailureReason != "payout_network:account_closed" {
		t.Errorf("FailureReason = %q, want payout_network:account_closed", evt.FailureReason)
	}
}

// A non-payout event signals fall-through with (nil, nil) so the caller routes
// it to the payment webhook parser instead.
func TestParsePayoutWebhook_NonPayoutEvent_FallsThrough(t *testing.T) {
	t.Parallel()

	c := NewClient("shop", "key")
	body := []byte(`{"event":"payment.succeeded","object":{"id":"pay_1","status":"succeeded"}}`)
	evt, err := c.ParsePayoutWebhook(context.Background(), body, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt != nil {
		t.Errorf("expected nil event for a non-payout notification, got %+v", evt)
	}
}

func TestParsePayoutWebhook_MissingID_Errors(t *testing.T) {
	t.Parallel()

	c := NewClient("shop", "key")
	body := []byte(`{"event":"payout.succeeded","object":{"status":"succeeded"}}`)
	if _, err := c.ParsePayoutWebhook(context.Background(), body, nil); err == nil {
		t.Error("expected an error when the payout id is missing, got nil")
	}
}

func TestParsePayoutWebhook_UnknownStatus_DefaultsPending(t *testing.T) {
	t.Parallel()

	c := NewClient("shop", "key")
	body := []byte(`{"event":"payout.updated","object":{"id":"po_3","status":"weird_new_status"}}`)
	evt, err := c.ParsePayoutWebhook(context.Background(), body, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Status != provider.PayoutStatusPending {
		t.Errorf("Status = %v, want pending for an unknown status", evt.Status)
	}
}

func TestParsePayoutWebhook_InvalidJSON_Errors(t *testing.T) {
	t.Parallel()

	c := NewClient("shop", "key")
	if _, err := c.ParsePayoutWebhook(context.Background(), []byte(`{not json`), nil); err == nil {
		t.Error("expected a parse error for malformed JSON, got nil")
	}
}

// ── CreatePayout ──────────────────────────────────────────────────────────────

func TestCreatePayout_MockMode_Succeeds(t *testing.T) {
	t.Parallel()

	c := NewClient("", "") // empty shopID → mock mode
	res, err := c.CreatePayout(context.Background(), provider.PayoutRequest{
		AmountKopecks: 50000, IdempotencyKey: "payout:abc", Kind: provider.PayoutDestCard, ProviderToken: "tok",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != provider.PayoutStatusSucceeded {
		t.Errorf("Status = %v, want succeeded in mock mode", res.Status)
	}
	if res.ProviderPayoutID == "" {
		t.Error("mock mode must still return a synthetic provider payout id")
	}
}

func TestCreatePayout_Validation(t *testing.T) {
	t.Parallel()

	c := NewClient("shop", "key") // real mode so validation runs before any HTTP
	tests := []struct {
		name string
		req  provider.PayoutRequest
	}{
		{"non-positive amount", provider.PayoutRequest{AmountKopecks: 0, IdempotencyKey: "k", Kind: provider.PayoutDestCard, ProviderToken: "tok"}},
		{"missing idempotency key", provider.PayoutRequest{AmountKopecks: 100, IdempotencyKey: "", Kind: provider.PayoutDestCard, ProviderToken: "tok"}},
		{"card missing token", provider.PayoutRequest{AmountKopecks: 100, IdempotencyKey: "k", Kind: provider.PayoutDestCard}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := c.CreatePayout(context.Background(), tt.req); err == nil {
				t.Errorf("expected an error for %s, got nil", tt.name)
			}
		})
	}
}

// The bank-account rail must serialize to payout_destination_data (not a
// payout_token) with the requisites in the right fields, and carry an
// Idempotence-Key header.  This is the one place that proves the outbound wire
// body actually routes money correctly.
func TestCreatePayout_BankAccount_WireBody(t *testing.T) {
	t.Parallel()

	var captured payoutRequest
	var gotPath, gotIdempotency string
	c := NewClient("shop", "key")
	c.httpClient = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotIdempotency = r.Header.Get("Idempotence-Key")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		return jsonResponse(http.StatusOK, `{"id":"po_77","status":"pending"}`), nil
	})}

	res, err := c.CreatePayout(context.Background(), provider.PayoutRequest{
		AmountKopecks:  120000,
		Currency:       "RUB",
		IdempotencyKey: "payout:11111111-1111-1111-1111-111111111111",
		Kind:           provider.PayoutDestBankAccount,
		BankBIC:        "044525225",
		BankAccount:    "40817810099910004312",
		RecipientName:  "Иванов Иван Иванович",
		PartnerType:    "venue",
		PartnerID:      "v1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v3/payouts" {
		t.Errorf("request path = %q, want /v3/payouts", gotPath)
	}
	if gotIdempotency == "" {
		t.Error("Idempotence-Key header must be set")
	}
	if captured.PayoutToken != "" {
		t.Errorf("bank-account payout must not carry a payout_token, got %q", captured.PayoutToken)
	}
	if captured.PayoutDestination == nil {
		t.Fatal("bank-account payout must carry payout_destination_data")
	}
	if captured.PayoutDestination.Type != "bank_account" {
		t.Errorf("destination type = %q, want bank_account", captured.PayoutDestination.Type)
	}
	if captured.PayoutDestination.BankID != "044525225" || captured.PayoutDestination.AccountNumber != "40817810099910004312" {
		t.Errorf("bank routing = (bic %q, acct %q), want (044525225, 40817810099910004312)",
			captured.PayoutDestination.BankID, captured.PayoutDestination.AccountNumber)
	}
	if res.ProviderPayoutID != "po_77" || res.Status != provider.PayoutStatusPending {
		t.Errorf("result = (%q, %v), want (po_77, pending)", res.ProviderPayoutID, res.Status)
	}
}

func TestCreatePayout_Card_WireBody(t *testing.T) {
	t.Parallel()

	var captured payoutRequest
	c := NewClient("shop", "key")
	c.httpClient = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		return jsonResponse(http.StatusOK, `{"id":"po_88","status":"succeeded"}`), nil
	})}

	res, err := c.CreatePayout(context.Background(), provider.PayoutRequest{
		AmountKopecks:  90000,
		IdempotencyKey: "payout:22222222-2222-2222-2222-222222222222",
		Kind:           provider.PayoutDestCard,
		ProviderToken:  "tok_live_card",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.PayoutToken != "tok_live_card" {
		t.Errorf("card payout must carry the payout_token, got %q", captured.PayoutToken)
	}
	if captured.PayoutDestination != nil {
		t.Errorf("card payout must not carry payout_destination_data, got %+v", captured.PayoutDestination)
	}
	if res.Status != provider.PayoutStatusSucceeded {
		t.Errorf("Status = %v, want succeeded", res.Status)
	}
}

// An upstream error (non-2xx) surfaces as an error rather than a phantom success.
func TestCreatePayout_UpstreamError_Surfaces(t *testing.T) {
	t.Parallel()

	c := NewClient("shop", "key")
	c.httpClient = &http.Client{Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, `{"type":"error","code":"invalid_request"}`), nil
	})}

	_, err := c.CreatePayout(context.Background(), provider.PayoutRequest{
		AmountKopecks:  10000,
		IdempotencyKey: "payout:33333333-3333-3333-3333-333333333333",
		Kind:           provider.PayoutDestCard,
		ProviderToken:  "tok",
	})
	if err == nil {
		t.Error("expected an error for a 400 response, got nil")
	}
}
