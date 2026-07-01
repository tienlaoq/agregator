package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
)

// ── payoutKind round-trip ────────────────────────────────────────────────────

func TestPayoutKindFromString(t *testing.T) {
	tests := []struct {
		in     string
		want   paymentv1.PayoutMethodKind
		wantOK bool
	}{
		{"card", paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD, true},
		{"CARD", paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD, true},
		{"  card  ", paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD, true},
		{"bank_account", paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_BANK_ACCOUNT, true},
		{"sbp", paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_SBP, true},
		{"", paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_UNSPECIFIED, false},
		{"crypto", paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_UNSPECIFIED, false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := payoutKindFromString(tc.in)
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("payoutKindFromString(%q) = (%v,%v), want (%v,%v)", tc.in, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestPayoutKindToString(t *testing.T) {
	tests := []struct {
		in   paymentv1.PayoutMethodKind
		want string
	}{
		{paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD, "card"},
		{paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_BANK_ACCOUNT, "bank_account"},
		{paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_SBP, "sbp"},
		{paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_UNSPECIFIED, ""},
	}
	for _, tc := range tests {
		if got := payoutKindToString(tc.in); got != tc.want {
			t.Fatalf("payoutKindToString(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// kindRoundTrip ensures every string accepted by FromString is reproduced by
// ToString, guarding the two switches against drifting apart.
func TestPayoutKind_RoundTrip(t *testing.T) {
	for _, s := range []string{"card", "bank_account", "sbp"} {
		k, ok := payoutKindFromString(s)
		if !ok {
			t.Fatalf("%q should be a valid kind", s)
		}
		if back := payoutKindToString(k); back != s {
			t.Fatalf("round-trip mismatch: %q -> %v -> %q", s, k, back)
		}
	}
}

// ── payoutMethodToJSON ───────────────────────────────────────────────────────

func TestPayoutMethodToJSON_CardOnlyExposesCardFields(t *testing.T) {
	now := timestamppb.Now()
	m := &paymentv1.PayoutMethodResponse{
		Id:          "pm1",
		PartnerType: "venue",
		PartnerId:   "v1",
		Kind:        paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD,
		CardLast4:   "4242",
		CardBrand:   "visa",
		// Bank/SBP fields set but MUST NOT leak for a card method.
		BankAccountMasked: "****0001",
		SbpPhoneMasked:    "+7***0002",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	out := payoutMethodToJSON(m)

	if out["kind"] != "card" {
		t.Fatalf("kind = %v, want card", out["kind"])
	}
	if out["card_last4"] != "4242" || out["card_brand"] != "visa" {
		t.Fatalf("card fields missing: %v", out)
	}
	for _, leaked := range []string{"bank_account_masked", "sbp_phone_masked", "bank_bic"} {
		if _, ok := out[leaked]; ok {
			t.Fatalf("card method must not include %q, got %v", leaked, out)
		}
	}
	if _, ok := out["created_at"]; !ok {
		t.Fatal("created_at should be present when timestamp set")
	}
}

func TestPayoutMethodToJSON_BankAndSBP(t *testing.T) {
	bank := payoutMethodToJSON(&paymentv1.PayoutMethodResponse{
		Kind:              paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_BANK_ACCOUNT,
		BankBic:           "044525225",
		BankAccountMasked: "****0001",
		BankName:          "Bank",
		RecipientName:     "ООО Ромашка",
	})
	if bank["bank_account_masked"] != "****0001" || bank["bank_bic"] != "044525225" {
		t.Fatalf("bank fields missing: %v", bank)
	}
	if _, ok := bank["card_last4"]; ok {
		t.Fatalf("bank method must not include card fields: %v", bank)
	}

	sbp := payoutMethodToJSON(&paymentv1.PayoutMethodResponse{
		Kind:           paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_SBP,
		SbpPhoneMasked: "+7***0002",
		SbpBankId:      "100000000111",
	})
	if sbp["sbp_phone_masked"] != "+7***0002" || sbp["sbp_bank_id"] != "100000000111" {
		t.Fatalf("sbp fields missing: %v", sbp)
	}
}

func TestPayoutMethodToJSON_OmitsNilTimestamps(t *testing.T) {
	out := payoutMethodToJSON(&paymentv1.PayoutMethodResponse{
		Kind: paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD,
	})
	if _, ok := out["created_at"]; ok {
		t.Fatal("created_at must be omitted when timestamp is nil")
	}
	if _, ok := out["updated_at"]; ok {
		t.Fatal("updated_at must be omitted when timestamp is nil")
	}
}

// ── ledgerEntryToJSON ────────────────────────────────────────────────────────

func TestLedgerEntryToJSON_OmitsEmptyOptionalFields(t *testing.T) {
	out := ledgerEntryToJSON(&paymentv1.LedgerEntry{
		Id:            7,
		PartnerType:   "master",
		PartnerId:     "m1",
		EntryType:     "credit",
		AmountKopecks: 5000,
	})
	for _, k := range []string{"id", "partner_type", "partner_id", "entry_type", "amount_kopecks"} {
		if _, ok := out[k]; !ok {
			t.Fatalf("required key %q missing: %v", k, out)
		}
	}
	for _, k := range []string{"payment_id", "payout_id", "reverses_entry_id", "reason", "available_at", "created_at"} {
		if _, ok := out[k]; ok {
			t.Fatalf("optional key %q should be omitted when empty: %v", k, out)
		}
	}
}

func TestLedgerEntryToJSON_IncludesPopulatedOptionalFields(t *testing.T) {
	out := ledgerEntryToJSON(&paymentv1.LedgerEntry{
		Id:              7,
		PaymentId:       "pay-1",
		PayoutId:        "po-1",
		ReversesEntryId: 3,
		Reason:          "refund",
		AvailableAt:     timestamppb.Now(),
		CreatedAt:       timestamppb.Now(),
	})
	for _, k := range []string{"payment_id", "payout_id", "reverses_entry_id", "reason", "available_at", "created_at"} {
		if _, ok := out[k]; !ok {
			t.Fatalf("populated key %q should be present: %v", k, out)
		}
	}
}

// ── payoutToJSON ─────────────────────────────────────────────────────────────

func TestPayoutToJSON(t *testing.T) {
	full := payoutToJSON(&paymentv1.Payout{
		Id:                 "po-1",
		PartnerType:        "venue",
		PartnerId:          "v1",
		AmountKopecks:      9000,
		Currency:           "RUB",
		Status:             "completed",
		MethodKindSnapshot: paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_SBP,
		MethodDisplay:      "СБП ****0002",
		ProviderName:       "tinkoff",
		ProviderPayoutId:   "ext-1",
		FailureReason:      "",
		CreatedAt:          timestamppb.Now(),
		CompletedAt:        timestamppb.Now(),
	})
	if full["method_kind"] != "sbp" {
		t.Fatalf("method_kind = %v, want sbp", full["method_kind"])
	}
	if full["provider_payout_id"] != "ext-1" {
		t.Fatalf("provider_payout_id missing: %v", full)
	}
	if _, ok := full["failure_reason"]; ok {
		t.Fatalf("empty failure_reason must be omitted: %v", full)
	}

	minimal := payoutToJSON(&paymentv1.Payout{Id: "po-2", Status: "pending"})
	if _, ok := minimal["completed_at"]; ok {
		t.Fatal("completed_at must be omitted when nil")
	}
	if _, ok := minimal["provider_payout_id"]; ok {
		t.Fatal("provider_payout_id must be omitted when empty")
	}
}

// ── List handlers (exercise listLedger / listPayouts / getBalance) ───────────

// payoutListStub serves the read endpoints and records the forwarded paging.
type payoutListStub struct {
	paymentv1.PaymentServiceClient
	ledgerReq  *paymentv1.ListPartnerLedgerRequest
	payoutsReq *paymentv1.ListPartnerPayoutsRequest
}

func (s *payoutListStub) GetPartnerBalance(_ context.Context, in *paymentv1.GetPartnerBalanceRequest, _ ...grpc.CallOption) (*paymentv1.PartnerBalanceResponse, error) {
	return &paymentv1.PartnerBalanceResponse{
		PartnerType: in.GetPartnerType(), PartnerId: in.GetPartnerId(),
		TotalKopecks: 1000, AvailableKopecks: 600, HeldKopecks: 400,
		LastEntryAt: timestamppb.Now(),
	}, nil
}

func (s *payoutListStub) ListPartnerLedger(_ context.Context, in *paymentv1.ListPartnerLedgerRequest, _ ...grpc.CallOption) (*paymentv1.ListPartnerLedgerResponse, error) {
	s.ledgerReq = in
	return &paymentv1.ListPartnerLedgerResponse{
		Entries: []*paymentv1.LedgerEntry{{Id: 1, EntryType: "credit", AmountKopecks: 100}},
	}, nil
}

func (s *payoutListStub) ListPartnerPayouts(_ context.Context, in *paymentv1.ListPartnerPayoutsRequest, _ ...grpc.CallOption) (*paymentv1.ListPartnerPayoutsResponse, error) {
	s.payoutsReq = in
	return &paymentv1.ListPartnerPayoutsResponse{
		Payouts: []*paymentv1.Payout{{Id: "po-1", Status: "completed"}},
	}, nil
}

func ownerVenueStub() *payoutVenueStub {
	return &payoutVenueStub{getVenue: func(in *venuev1.GetVenueRequest) (*venuev1.VenueResponse, error) {
		return &venuev1.VenueResponse{Id: in.GetId(), OwnerId: "owner-1"}, nil
	}}
}

func TestPayout_ListVenueLedger_ForwardsPagingAndRenders(t *testing.T) {
	pay := &payoutListStub{}
	h := newPayoutHandler(pay, ownerVenueStub(), nil)

	r := venueReq(t, http.MethodGet, "venue-1", "owner-1", "")
	// override URL with paging query
	r.URL.RawQuery = "limit=10&offset=20"
	rr := httptest.NewRecorder()
	h.ListVenueLedger(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if pay.ledgerReq == nil {
		t.Fatal("ListPartnerLedger not called")
	}
	if pay.ledgerReq.GetLimit() != 10 || pay.ledgerReq.GetOffset() != 20 {
		t.Fatalf("paging not forwarded: limit=%d offset=%d", pay.ledgerReq.GetLimit(), pay.ledgerReq.GetOffset())
	}
	if pay.ledgerReq.GetPartnerType() != "venue" || pay.ledgerReq.GetPartnerId() != "venue-1" {
		t.Fatalf("partner not forwarded: %s/%s", pay.ledgerReq.GetPartnerType(), pay.ledgerReq.GetPartnerId())
	}
	var body struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(body.Entries))
	}
}

func TestPayout_ListVenuePayouts_RendersList(t *testing.T) {
	pay := &payoutListStub{}
	h := newPayoutHandler(pay, ownerVenueStub(), nil)

	rr := httptest.NewRecorder()
	h.ListVenuePayouts(rr, venueReq(t, http.MethodGet, "venue-1", "owner-1", ""))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if pay.payoutsReq.GetLimit() != 50 || pay.payoutsReq.GetOffset() != 0 {
		t.Fatalf("default paging not applied: limit=%d offset=%d", pay.payoutsReq.GetLimit(), pay.payoutsReq.GetOffset())
	}
	var body struct {
		Payouts []map[string]any `json:"payouts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Payouts) != 1 {
		t.Fatalf("want 1 payout, got %d", len(body.Payouts))
	}
}

func TestPayout_GetVenueBalance_RendersBalanceWithLastEntry(t *testing.T) {
	pay := &payoutListStub{}
	h := newPayoutHandler(pay, ownerVenueStub(), nil)

	rr := httptest.NewRecorder()
	h.GetVenueBalance(rr, venueReq(t, http.MethodGet, "venue-1", "owner-1", ""))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	var body struct {
		Balance map[string]any `json:"balance"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Balance["available_kopecks"] != float64(600) {
		t.Fatalf("available_kopecks = %v, want 600", body.Balance["available_kopecks"])
	}
	if _, ok := body.Balance["last_entry_at"]; !ok {
		t.Fatal("last_entry_at should be present when set")
	}
}

func TestPayout_ListLedger_InvalidLimit_400_NoPaymentCall(t *testing.T) {
	pay := &payoutListStub{}
	h := newPayoutHandler(pay, ownerVenueStub(), nil)

	r := venueReq(t, http.MethodGet, "venue-1", "owner-1", "")
	r.URL.RawQuery = "limit=99999" // exceeds the 200 max
	rr := httptest.NewRecorder()
	h.ListVenueLedger(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for out-of-range limit, got %d (%s)", rr.Code, rr.Body.String())
	}
	if pay.ledgerReq != nil {
		t.Fatal("payment RPC must not be reached on invalid paging")
	}
}
