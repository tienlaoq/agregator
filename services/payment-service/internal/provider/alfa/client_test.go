package alfa

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// newTestClient points a Client at an httptest server so the RBS wire format can
// be exercised without touching the real gateway.
func newTestClient(srv *httptest.Server) *Client {
	return NewClient("u", "p", srv.URL)
}

func TestCreatePayment_buildsRequestAndParsesForm(t *testing.T) {
	var gotMethod, gotAmount, gotCurrency, gotOrderNumber, gotUser string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = strings.TrimPrefix(r.URL.Path, "/")
		_ = r.ParseForm()
		gotAmount = r.PostForm.Get("amount")
		gotCurrency = r.PostForm.Get("currency")
		gotOrderNumber = r.PostForm.Get("orderNumber")
		gotUser = r.PostForm.Get("userName")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"orderId":"abc123","formUrl":"https://pay/redir"}`))
	}))
	defer srv.Close()

	res, err := newTestClient(srv).CreatePayment(context.Background(), provider.CreateRequest{
		AmountKopecks:  150000,
		ReturnURL:      "https://app/return",
		IdempotencyKey: "booking-42",
		BookingID:      "42",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if gotMethod != "register.do" {
		t.Errorf("method = %q, want register.do", gotMethod)
	}
	if gotAmount != "150000" {
		t.Errorf("amount = %q, want 150000 (kopecks)", gotAmount)
	}
	if gotCurrency != currencyRUB {
		t.Errorf("currency = %q, want %q", gotCurrency, currencyRUB)
	}
	if gotUser != "u" {
		t.Errorf("userName = %q, want u", gotUser)
	}
	if len(gotOrderNumber) != 30 {
		t.Errorf("orderNumber len = %d, want 30", len(gotOrderNumber))
	}
	if res.ProviderPaymentID != "abc123" || res.ConfirmationURL != "https://pay/redir" {
		t.Errorf("result = %+v", res)
	}
}

func TestCreatePayment_businessRejectionIsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errorCode":"1","errorMessage":"duplicate order"}`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv).CreatePayment(context.Background(), provider.CreateRequest{IdempotencyKey: "x"})
	if err == nil {
		t.Fatal("want error on non-zero errorCode")
	}
	if errors.Is(err, provider.ErrTransient) {
		t.Error("business rejection must NOT be transient (would trigger unsafe retry)")
	}
}

func TestCall_5xxIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	err := newTestClient(srv).Refund(context.Background(), "oid", 100, "k")
	if !errors.Is(err, provider.ErrTransient) {
		t.Errorf("5xx must be transient (reconcile, not compensate); got %v", err)
	}
}

func TestParseWebhook_pullConfirmsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("orderId") != "ord-9" {
			t.Errorf("status pull orderId = %q", r.PostForm.Get("orderId"))
		}
		_, _ = w.Write([]byte(`{"errorCode":"0","orderStatus":2}`))
	}))
	defer srv.Close()

	// Native RBS callback is form-encoded with mdOrder.
	body := []byte(url.Values{"mdOrder": {"ord-9"}, "operation": {"deposited"}, "status": {"1"}}.Encode())
	ev, err := newTestClient(srv).ParseWebhook(context.Background(), body, nil)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if ev.ProviderPaymentID != "ord-9" || ev.Status != domain.StatusSucceeded {
		t.Errorf("event = %+v, want ord-9/succeeded", ev)
	}
}

func TestMapStatus(t *testing.T) {
	cases := map[int]domain.PaymentStatus{
		rbsDeposited: domain.StatusSucceeded,
		rbsRefunded:  domain.StatusRefunded,
		rbsReversed:  domain.StatusCancelled,
		rbsDeclined:  domain.StatusCancelled,
		0:            domain.StatusPending,
		99:           domain.StatusPending,
	}
	for in, want := range cases {
		if got := mapStatus(in); got != want {
			t.Errorf("mapStatus(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestDeriveOrderNumber_stableAndBounded(t *testing.T) {
	a := deriveOrderNumber("booking-42")
	b := deriveOrderNumber("booking-42")
	c := deriveOrderNumber("booking-43")
	if a != b {
		t.Error("same key must yield same order number (idempotency)")
	}
	if a == c {
		t.Error("different keys must yield different order numbers")
	}
	if len(a) != 30 {
		t.Errorf("order number len = %d, want 30", len(a))
	}
}

func TestCreatePayout_notSupported(t *testing.T) {
	_, err := NewClient("u", "p", "https://x/").CreatePayout(context.Background(), provider.PayoutRequest{})
	if err == nil {
		t.Fatal("payout must fail loudly on acquiring-only Alfa")
	}
}
