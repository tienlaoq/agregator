package grpc

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
	"github.com/tienlao/agregator/services/payment-service/internal/usecase"
)

type deps struct {
	pay    *mockPaymentRepo
	ledger *mockLedgerRepo
	method *mockPayoutMethodRepo
	payout mockPayoutRepo
	prov   *mockProvider
}

func newTestServer(d deps) *Server {
	if d.pay == nil {
		d.pay = &mockPaymentRepo{}
	}
	if d.ledger == nil {
		d.ledger = &mockLedgerRepo{}
	}
	if d.method == nil {
		d.method = &mockPayoutMethodRepo{}
	}
	if d.prov == nil {
		d.prov = &mockProvider{}
	}
	paymentUC := usecase.NewPaymentUseCase(d.pay, mockOutboxRepo{}, d.ledger, d.prov,
		"mock", "https://return", 1500, time.Hour, zerolog.Nop())
	payoutUC := usecase.NewPayoutUseCase(d.payout, d.method, d.ledger, d.prov,
		"mock", usecase.PayoutSchedulerConfig{}, zerolog.Nop())
	return NewServer(paymentUC, payoutUC, zerolog.Nop())
}

func wantCode(t *testing.T, err error, c codes.Code) {
	t.Helper()
	if status.Code(err) != c {
		t.Fatalf("status code = %v, want %v (err: %v)", status.Code(err), c, err)
	}
}

// ── CreatePayment ──────────────────────────────────────────────────────────

func TestCreatePayment_Success(t *testing.T) {
	s := newTestServer(deps{})
	resp, err := s.CreatePayment(context.Background(), &paymentv1.CreatePaymentRequest{
		BookingId: "b1", Amount: 300000, IdempotencyKey: "key-1",
		CounterpartyType: "master", CounterpartyId: "m1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetBookingId() != "b1" || resp.GetAmount() != 300000 {
		t.Errorf("response mismatch: %+v", resp)
	}
	if resp.GetStatus() != string(domain.StatusPending) {
		t.Errorf("status = %q, want pending", resp.GetStatus())
	}
	// 15% platform fee on 300000 = 45000; net = 255000.
	if resp.GetPlatformFeeKopecks() != 45000 || resp.GetCounterpartyNetKopecks() != 255000 {
		t.Errorf("fee/net = %d/%d, want 45000/255000", resp.GetPlatformFeeKopecks(), resp.GetCounterpartyNetKopecks())
	}
	if resp.GetProviderId() == "" {
		t.Error("provider id should be set after the provider call")
	}
}

func TestCreatePayment_InvalidAmount(t *testing.T) {
	s := newTestServer(deps{})
	_, err := s.CreatePayment(context.Background(), &paymentv1.CreatePaymentRequest{
		BookingId: "b1", Amount: 0, IdempotencyKey: "key-1",
	})
	wantCode(t, err, codes.InvalidArgument)
}

// ── GetPayment / GetPaymentByBooking ───────────────────────────────────────

func TestGetPayment_Success(t *testing.T) {
	pay := &mockPaymentRepo{GetByIDFunc: func(_ context.Context, id string) (*domain.Payment, error) {
		return &domain.Payment{ID: id, BookingID: "b1", Amount: 1000, Status: domain.StatusSucceeded, ProviderName: "tbank"}, nil
	}}
	resp, err := newTestServer(deps{pay: pay}).GetPayment(context.Background(), &paymentv1.GetPaymentRequest{Id: "p1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetId() != "p1" || resp.GetStatus() != "succeeded" || resp.GetProviderName() != "tbank" {
		t.Errorf("response mismatch: %+v", resp)
	}
}

func TestGetPayment_ErrorPropagates(t *testing.T) {
	pay := &mockPaymentRepo{GetByIDFunc: func(context.Context, string) (*domain.Payment, error) {
		return nil, status.Error(codes.NotFound, "no such payment")
	}}
	_, err := newTestServer(deps{pay: pay}).GetPayment(context.Background(), &paymentv1.GetPaymentRequest{Id: "p1"})
	wantCode(t, err, codes.NotFound)
}

func TestGetPaymentByBooking_Success(t *testing.T) {
	pay := &mockPaymentRepo{GetByBookingIDFunc: func(_ context.Context, b string) (*domain.Payment, error) {
		return &domain.Payment{ID: "p1", BookingID: b, Status: domain.StatusPending}, nil
	}}
	resp, err := newTestServer(deps{pay: pay}).GetPaymentByBooking(context.Background(), &paymentv1.GetPaymentByBookingRequest{BookingId: "b1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetBookingId() != "b1" {
		t.Errorf("booking id = %q, want b1", resp.GetBookingId())
	}
}

// ── Balance / ledger / payouts reads ───────────────────────────────────────

func TestGetPartnerBalance_Success(t *testing.T) {
	ledger := &mockLedgerRepo{BalanceFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PartnerBalance, error) {
		return &domain.PartnerBalance{PartnerType: pt, PartnerID: pid, TotalKopecks: 5000, AvailableKopecks: 4000, HeldKopecks: 1000}, nil
	}}
	resp, err := newTestServer(deps{ledger: ledger}).GetPartnerBalance(context.Background(),
		&paymentv1.GetPartnerBalanceRequest{PartnerType: "master", PartnerId: "m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetTotalKopecks() != 5000 || resp.GetAvailableKopecks() != 4000 || resp.GetHeldKopecks() != 1000 {
		t.Errorf("balance mismatch: %+v", resp)
	}
}

func TestGetPartnerBalance_InvalidPartnerType(t *testing.T) {
	_, err := newTestServer(deps{}).GetPartnerBalance(context.Background(),
		&paymentv1.GetPartnerBalanceRequest{PartnerType: "user", PartnerId: "x"})
	wantCode(t, err, codes.InvalidArgument)
}

func TestListPartnerLedger_Success(t *testing.T) {
	ledger := &mockLedgerRepo{ListFunc: func(_ context.Context, pt domain.PartnerType, pid string, limit, offset int) ([]domain.LedgerEntry, error) {
		if limit != 25 || offset != 5 {
			t.Errorf("limit/offset = %d/%d, want 25/5", limit, offset)
		}
		return []domain.LedgerEntry{
			{ID: 1, EntryType: domain.LedgerAccrual, AmountKopecks: 1000},
			{ID: 2, EntryType: domain.LedgerPayout, AmountKopecks: -1000},
		}, nil
	}}
	resp, err := newTestServer(deps{ledger: ledger}).ListPartnerLedger(context.Background(),
		&paymentv1.ListPartnerLedgerRequest{PartnerType: "master", PartnerId: "m1", Limit: 25, Offset: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetEntries()) != 2 {
		t.Fatalf("entries = %d, want 2", len(resp.GetEntries()))
	}
}

func TestListPartnerPayouts_Success(t *testing.T) {
	d := deps{payout: mockPayoutRepo{ListFunc: func(_ context.Context, pt domain.PartnerType, pid string, limit, offset int) ([]domain.Payout, error) {
		return []domain.Payout{{ID: "po1", AmountKopecks: 4000, Status: domain.PayoutSucceeded}}, nil
	}}}
	resp, err := newTestServer(d).ListPartnerPayouts(context.Background(),
		&paymentv1.ListPartnerPayoutsRequest{PartnerType: "master", PartnerId: "m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetPayouts()) != 1 || resp.GetPayouts()[0].GetId() != "po1" {
		t.Errorf("payouts mismatch: %+v", resp.GetPayouts())
	}
}

// ── Payout method ──────────────────────────────────────────────────────────

func TestSetPayoutMethod_Success(t *testing.T) {
	var saved *domain.PayoutMethod
	method := &mockPayoutMethodRepo{SaveFunc: func(_ context.Context, m *domain.PayoutMethod) error {
		m.ID = "pm1"
		saved = m
		return nil
	}}
	resp, err := newTestServer(deps{method: method}).SetPayoutMethod(context.Background(), &paymentv1.SetPayoutMethodRequest{
		PartnerType: "master", PartnerId: "m1",
		Kind:      paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD,
		CardLast4: "1234", ProviderToken: "tok", // ProviderName omitted → defaults to active provider
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved == nil || saved.Kind != domain.PayoutMethodCard {
		t.Fatalf("save not called with card kind: %+v", saved)
	}
	if resp.GetId() != "pm1" {
		t.Errorf("response id = %q, want pm1", resp.GetId())
	}
}

func TestSetPayoutMethod_InvalidKind(t *testing.T) {
	_, err := newTestServer(deps{}).SetPayoutMethod(context.Background(), &paymentv1.SetPayoutMethodRequest{
		PartnerType: "master", PartnerId: "m1",
		Kind: paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_UNSPECIFIED,
	})
	wantCode(t, err, codes.InvalidArgument)
}

func TestGetPayoutMethod_NotFound(t *testing.T) {
	// Default GetActive returns NotFound.
	_, err := newTestServer(deps{}).GetPayoutMethod(context.Background(),
		&paymentv1.GetPayoutMethodRequest{PartnerType: "master", PartnerId: "m1"})
	wantCode(t, err, codes.NotFound)
}

func TestGetPayoutMethod_Success(t *testing.T) {
	method := &mockPayoutMethodRepo{GetActiveFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PayoutMethod, error) {
		return &domain.PayoutMethod{ID: "pm1", PartnerType: pt, PartnerID: pid, Kind: domain.PayoutMethodSBP, SBPPhone: "+79991234567"}, nil
	}}
	resp, err := newTestServer(deps{method: method}).GetPayoutMethod(context.Background(),
		&paymentv1.GetPayoutMethodRequest{PartnerType: "master", PartnerId: "m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetId() != "pm1" || resp.GetKind() != paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_SBP {
		t.Errorf("response mismatch: %+v", resp)
	}
}

// ── HandleWebhook routing ──────────────────────────────────────────────────

func TestHandleWebhook_PayoutEventAbsorbed(t *testing.T) {
	// ParsePayoutWebhook yields a payout event; the payout is unknown (default
	// repo → NotFound) so HandlePayoutWebhook absorbs it and the handler returns Ok.
	prov := &mockProvider{ParsePayoutWebhookFunc: func(context.Context, []byte, http.Header) (*provider.PayoutWebhookEvent, error) {
		return &provider.PayoutWebhookEvent{ProviderPayoutID: "po1", Status: provider.PayoutStatusSucceeded, RawEvent: "payout.succeeded"}, nil
	}}
	resp, err := newTestServer(deps{prov: prov}).HandleWebhook(context.Background(), &paymentv1.WebhookRequest{RawBody: []byte("{}")})
	if err != nil {
		t.Fatalf("HandleWebhook returned a gRPC error, expected Ok flag: %v", err)
	}
	if !resp.GetOk() {
		t.Error("expected Ok:true for an absorbed payout webhook")
	}
}

func TestHandleWebhook_PayoutParseErrorReturnsNotOk(t *testing.T) {
	prov := &mockProvider{ParsePayoutWebhookFunc: func(context.Context, []byte, http.Header) (*provider.PayoutWebhookEvent, error) {
		return nil, errors.New("payout parse boom")
	}}
	resp, err := newTestServer(deps{prov: prov}).HandleWebhook(context.Background(), &paymentv1.WebhookRequest{RawBody: []byte("{}")})
	if err != nil {
		t.Fatalf("webhook errors must surface as Ok:false, not gRPC error: %v", err)
	}
	if resp.GetOk() {
		t.Error("expected Ok:false on payout parse error")
	}
}

func TestHandleWebhook_PaymentEventOk(t *testing.T) {
	// Not a payout event → falls through to the payment path; default ParseWebhook
	// returns a non-terminal (pending) event → no-op success → Ok:true.
	resp, err := newTestServer(deps{}).HandleWebhook(context.Background(), &paymentv1.WebhookRequest{RawBody: []byte("{}")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetOk() {
		t.Error("expected Ok:true for a no-op payment webhook")
	}
}

func TestHandleWebhook_PaymentParseErrorReturnsNotOk(t *testing.T) {
	prov := &mockProvider{ParseWebhookFunc: func(context.Context, []byte, http.Header) (*domain.WebhookEvent, error) {
		return nil, errors.New("payment parse boom")
	}}
	resp, err := newTestServer(deps{prov: prov}).HandleWebhook(context.Background(), &paymentv1.WebhookRequest{RawBody: []byte("{}")})
	if err != nil {
		t.Fatalf("webhook errors must surface as Ok:false, not gRPC error: %v", err)
	}
	if resp.GetOk() {
		t.Error("expected Ok:false on payment parse error")
	}
}
