package grpc

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

// The partner cabinet must never receive back the secrets it submitted: the
// raw payout token, the full bank account number, or the full SBP phone.
// payoutMethodToProto only ever fills masked tails — and the response proto type
// has no field that could carry the raw value.  This test pins both: the masks
// are correct, and the raw secrets appear nowhere in the serialized response.
func TestPayoutMethodToProto_MasksSecrets(t *testing.T) {
	t.Parallel()

	const (
		rawToken   = "tok_live_SECRET_should_never_echo"
		rawAccount = "40817810099910004312"
		rawPhone   = "+79991234567"
	)
	m := &domain.PayoutMethod{
		ID:            "m1",
		PartnerType:   domain.PartnerVenue,
		PartnerID:     "v1",
		Kind:          domain.PayoutMethodBankAccount,
		ProviderName:  "yookassa",
		ProviderToken: rawToken,
		BankBIC:       "044525225",
		BankAccount:   rawAccount,
		BankName:      "Sberbank",
		RecipientName: "OOO Banya",
		SBPPhone:      rawPhone,
		SBPBankID:     "100000000111",
	}

	resp := payoutMethodToProto(m)
	require.NotNil(t, resp)

	// Masked tails are present and correct…
	assert.Equal(t, "•••4312", resp.GetBankAccountMasked(), "only the last 4 account digits may surface")
	assert.Equal(t, "•••67", resp.GetSbpPhoneMasked(), "only the last 2 phone digits may surface")

	// …and the raw secrets appear nowhere in the serialized response.  This is the
	// regression guard: if someone later adds a field that echoes a secret, the
	// JSON dump below will contain it and this assertion fails.
	blob, err := json.Marshal(resp)
	require.NoError(t, err)
	dump := string(blob)
	assert.NotContains(t, dump, rawToken, "payout token must never be echoed")
	assert.NotContains(t, dump, rawAccount, "full bank account must never be echoed")
	assert.NotContains(t, dump, rawPhone, "full SBP phone must never be echoed")
}

func TestMaskAccountTail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{"40817810099910004312", "•••4312"},
		{"1234", "•••1234"},
		{"123", "••••"}, // shorter than 4 → fully masked
		{"", "••••"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, maskAccountTail(tt.in))
		})
	}
}

func TestMaskPhone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{"+79991234567", "•••67"},
		{"  +79991234567  ", "•••67"}, // trimmed first
		{"1", "••"},
		{"", "••"},
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.in), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, maskPhone(tt.in))
		})
	}
}

// Every supported proto kind maps to its domain kind and back unchanged; an
// unspecified kind is rejected with InvalidArgument so a half-built request can
// never be silently treated as a default rail.
func TestPayoutKind_ProtoRoundTrip(t *testing.T) {
	t.Parallel()

	kinds := []paymentv1.PayoutMethodKind{
		paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD,
		paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_BANK_ACCOUNT,
		paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_SBP,
	}
	for _, k := range kinds {
		t.Run(k.String(), func(t *testing.T) {
			t.Parallel()
			dk, err := payoutKindFromProto(k)
			require.NoError(t, err)
			assert.Equal(t, k, payoutKindToProto(dk), "round-trip must be lossless")
		})
	}
}

func TestPayoutKindFromProto_Unspecified_InvalidArgument(t *testing.T) {
	t.Parallel()

	_, err := payoutKindFromProto(paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_UNSPECIFIED)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPayoutKindToProto_Unknown_Unspecified(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_UNSPECIFIED,
		payoutKindToProto(domain.PayoutMethodKind("bogus")),
	)
}

// ── Ledger / payout read views ────────────────────────────────────────────────

// The partner-facing ledger view must reproduce every field faithfully — a
// reversal shown with the wrong sign or a mis-copied entry id would misrepresent
// what the partner is owed.
func TestLedgerEntryToProto(t *testing.T) {
	t.Parallel()

	avail := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	created := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	e := &domain.LedgerEntry{
		ID:              7,
		PartnerType:     domain.PartnerVenue,
		PartnerID:       "v1",
		EntryType:       domain.LedgerReversal,
		AmountKopecks:   -4250,
		PaymentID:       "pay-1",
		PayoutID:        "po-1",
		ReversesEntryID: 3,
		Reason:          "refund",
		AvailableAt:     avail,
		CreatedAt:       created,
	}

	out := ledgerEntryToProto(e)
	assert.Equal(t, int64(7), out.GetId())
	assert.Equal(t, "venue", out.GetPartnerType())
	assert.Equal(t, "v1", out.GetPartnerId())
	assert.Equal(t, "reversal", out.GetEntryType())
	assert.Equal(t, int64(-4250), out.GetAmountKopecks(), "reversal sign must be preserved")
	assert.Equal(t, "pay-1", out.GetPaymentId())
	assert.Equal(t, "po-1", out.GetPayoutId())
	assert.Equal(t, int64(3), out.GetReversesEntryId())
	assert.Equal(t, "refund", out.GetReason())
	assert.True(t, avail.Equal(out.GetAvailableAt().AsTime()))
	assert.True(t, created.Equal(out.GetCreatedAt().AsTime()))
}

// A zero AvailableAt/CreatedAt must not emit a spurious epoch timestamp.
func TestLedgerEntryToProto_ZeroTimes_OmitsTimestamps(t *testing.T) {
	t.Parallel()

	out := ledgerEntryToProto(&domain.LedgerEntry{ID: 1, EntryType: domain.LedgerAccrual})
	assert.Nil(t, out.GetAvailableAt(), "zero available_at must stay nil, not epoch")
	assert.Nil(t, out.GetCreatedAt(), "zero created_at must stay nil, not epoch")
}

func TestPayoutToProto(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	completed := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	p := &domain.Payout{
		ID:                 "po-1",
		PartnerType:        domain.PartnerMaster,
		PartnerID:          "m1",
		AmountKopecks:      50000,
		Currency:           "RUB",
		Status:             domain.PayoutSucceeded,
		MethodKindSnapshot: domain.PayoutMethodCard,
		MethodDisplay:      "Card •••4242",
		ProviderName:       "yookassa",
		ProviderPayoutID:   "pp1",
		FailureReason:      "",
		CreatedAt:          created,
		CompletedAt:        completed,
	}

	out := payoutToProto(p)
	assert.Equal(t, "po-1", out.GetId())
	assert.Equal(t, "master", out.GetPartnerType())
	assert.Equal(t, "m1", out.GetPartnerId())
	assert.Equal(t, int64(50000), out.GetAmountKopecks())
	assert.Equal(t, "RUB", out.GetCurrency())
	assert.Equal(t, domain.PayoutSucceeded.String(), out.GetStatus())
	assert.Equal(t, paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD, out.GetMethodKindSnapshot())
	assert.Equal(t, "Card •••4242", out.GetMethodDisplay())
	assert.Equal(t, "pp1", out.GetProviderPayoutId())
	assert.True(t, created.Equal(out.GetCreatedAt().AsTime()))
	assert.True(t, completed.Equal(out.GetCompletedAt().AsTime()))
}
