package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── Stubs ────────────────────────────────────────────────────────────────────

type payoutVenueStub struct {
	venuev1.VenueServiceClient
	getVenue func(*venuev1.GetVenueRequest) (*venuev1.VenueResponse, error)
}

func (s *payoutVenueStub) GetVenue(_ context.Context, in *venuev1.GetVenueRequest, _ ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return s.getVenue(in)
}

type payoutMasterStub struct {
	masterv1.MasterServiceClient
	getMyProfile func(*masterv1.GetMyProfileRequest) (*masterv1.MasterResponse, error)
}

func (s *payoutMasterStub) GetMyProfile(_ context.Context, in *masterv1.GetMyProfileRequest, _ ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	return s.getMyProfile(in)
}

// payoutPaymentStub records what the gateway forwarded so tests can assert that
// the partner identity was resolved correctly — and, crucially, that the
// payment RPC was NOT reached at all on an authorization failure.
type payoutPaymentStub struct {
	paymentv1.PaymentServiceClient

	getReq *paymentv1.GetPayoutMethodRequest
	getErr error
	getOut *paymentv1.PayoutMethodResponse

	setReq *paymentv1.SetPayoutMethodRequest
	setErr error
	setOut *paymentv1.PayoutMethodResponse

	balReq *paymentv1.GetPartnerBalanceRequest
}

func (s *payoutPaymentStub) GetPayoutMethod(_ context.Context, in *paymentv1.GetPayoutMethodRequest, _ ...grpc.CallOption) (*paymentv1.PayoutMethodResponse, error) {
	s.getReq = in
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.getOut != nil {
		return s.getOut, nil
	}
	return &paymentv1.PayoutMethodResponse{Id: "pm1", PartnerType: in.GetPartnerType(), PartnerId: in.GetPartnerId()}, nil
}

func (s *payoutPaymentStub) SetPayoutMethod(_ context.Context, in *paymentv1.SetPayoutMethodRequest, _ ...grpc.CallOption) (*paymentv1.PayoutMethodResponse, error) {
	s.setReq = in
	if s.setErr != nil {
		return nil, s.setErr
	}
	if s.setOut != nil {
		return s.setOut, nil
	}
	return &paymentv1.PayoutMethodResponse{
		Id: "pm1", PartnerType: in.GetPartnerType(), PartnerId: in.GetPartnerId(), Kind: in.GetKind(),
	}, nil
}

func (s *payoutPaymentStub) GetPartnerBalance(_ context.Context, in *paymentv1.GetPartnerBalanceRequest, _ ...grpc.CallOption) (*paymentv1.PartnerBalanceResponse, error) {
	s.balReq = in
	return &paymentv1.PartnerBalanceResponse{
		PartnerType: in.GetPartnerType(), PartnerId: in.GetPartnerId(),
		TotalKopecks: 1000, AvailableKopecks: 600, HeldKopecks: 400,
	}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// venueReq builds a request whose chi route var {venueId} and ctx user match the
// arguments, mirroring what the router + Auth middleware produce at runtime.
func venueReq(t *testing.T, method, venueID, actorID, body string) *http.Request {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, "/owner/venues/"+venueID+"/payout-method", nil)
	} else {
		r = httptest.NewRequest(method, "/owner/venues/"+venueID+"/payout-method", strings.NewReader(body))
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("venueId", venueID)
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	if actorID != "" {
		ctx = middleware.WithUserID(ctx, actorID)
	}
	return r.WithContext(ctx)
}

func newPayoutHandler(p paymentv1.PaymentServiceClient, v venuev1.VenueServiceClient, m masterv1.MasterServiceClient) *PayoutHandler {
	return NewPayoutHandler(zerolog.Nop(), p, v, m)
}

// ── Venue authorization ────────────────────────────────────────────────────────

func TestPayout_Venue_OwnerMatch_Allows(t *testing.T) {
	pay := &payoutPaymentStub{}
	ven := &payoutVenueStub{getVenue: func(in *venuev1.GetVenueRequest) (*venuev1.VenueResponse, error) {
		return &venuev1.VenueResponse{Id: in.GetId(), OwnerId: "owner-1"}, nil
	}}
	h := newPayoutHandler(pay, ven, nil)

	rr := httptest.NewRecorder()
	h.GetVenuePayoutMethod(rr, venueReq(t, http.MethodGet, "venue-1", "owner-1", ""))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if pay.getReq == nil {
		t.Fatal("expected payment.GetPayoutMethod to be called")
	}
	if pay.getReq.GetPartnerType() != "venue" || pay.getReq.GetPartnerId() != "venue-1" {
		t.Fatalf("wrong partner forwarded: %s/%s", pay.getReq.GetPartnerType(), pay.getReq.GetPartnerId())
	}
}

// The single most important property of this money circuit: a caller who is not
// the venue's legal owner must be rejected BEFORE any payment RPC is reached, so
// another partner's financial data can never leak.
func TestPayout_Venue_OwnerMismatch_Forbidden_NoPaymentCall(t *testing.T) {
	pay := &payoutPaymentStub{}
	ven := &payoutVenueStub{getVenue: func(in *venuev1.GetVenueRequest) (*venuev1.VenueResponse, error) {
		return &venuev1.VenueResponse{Id: in.GetId(), OwnerId: "the-real-owner"}, nil
	}}
	h := newPayoutHandler(pay, ven, nil)

	rr := httptest.NewRecorder()
	h.GetVenueBalance(rr, venueReq(t, http.MethodGet, "venue-1", "attacker", ""))

	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d (%s)", rr.Code, rr.Body.String())
	}
	if pay.balReq != nil {
		t.Fatal("SECURITY: payment RPC was reached for a non-owner")
	}
	assertCode(t, rr, "GATEWAY.PAYOUT.FORBIDDEN")
}

func TestPayout_Venue_Unauthenticated_401_NoVenueCall(t *testing.T) {
	pay := &payoutPaymentStub{}
	venCalled := false
	ven := &payoutVenueStub{getVenue: func(in *venuev1.GetVenueRequest) (*venuev1.VenueResponse, error) {
		venCalled = true
		return &venuev1.VenueResponse{Id: in.GetId(), OwnerId: "owner-1"}, nil
	}}
	h := newPayoutHandler(pay, ven, nil)

	rr := httptest.NewRecorder()
	h.GetVenuePayoutMethod(rr, venueReq(t, http.MethodGet, "venue-1", "", ""))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
	if venCalled {
		t.Fatal("venue RPC should not be reached without an authenticated actor")
	}
	if pay.getReq != nil {
		t.Fatal("payment RPC should not be reached without an authenticated actor")
	}
}

func TestPayout_Venue_NotFound_404_NoPaymentCall(t *testing.T) {
	pay := &payoutPaymentStub{}
	ven := &payoutVenueStub{getVenue: func(*venuev1.GetVenueRequest) (*venuev1.VenueResponse, error) {
		return nil, status.Error(codes.NotFound, "venue not found")
	}}
	h := newPayoutHandler(pay, ven, nil)

	rr := httptest.NewRecorder()
	h.GetVenuePayoutMethod(rr, venueReq(t, http.MethodGet, "ghost", "owner-1", ""))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d (%s)", rr.Code, rr.Body.String())
	}
	if pay.getReq != nil {
		t.Fatal("payment RPC should not be reached when the venue does not exist")
	}
}

func TestPayout_Venue_MissingVenueID_400(t *testing.T) {
	h := newPayoutHandler(&payoutPaymentStub{}, &payoutVenueStub{}, nil)
	rr := httptest.NewRecorder()
	// Empty venueId route var.
	h.GetVenuePayoutMethod(rr, venueReq(t, http.MethodGet, "", "owner-1", ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

// ── Payout-method empty state ───────────────────────────────────────────────────

// A partner who has not configured a payout method yet is a normal UI state, not
// an error: the endpoint returns 200 with payout_method:null.
func TestPayout_GetMethod_NotFound_ReturnsNull(t *testing.T) {
	pay := &payoutPaymentStub{getErr: status.Error(codes.NotFound, "no method")}
	ven := &payoutVenueStub{getVenue: func(in *venuev1.GetVenueRequest) (*venuev1.VenueResponse, error) {
		return &venuev1.VenueResponse{Id: in.GetId(), OwnerId: "owner-1"}, nil
	}}
	h := newPayoutHandler(pay, ven, nil)

	rr := httptest.NewRecorder()
	h.GetVenuePayoutMethod(rr, venueReq(t, http.MethodGet, "venue-1", "owner-1", ""))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if v, ok := out["payout_method"]; !ok || v != nil {
		t.Fatalf("want payout_method:null, got %v", out)
	}
}

// ── SetPayoutMethod ──────────────────────────────────────────────────────────────

func TestPayout_SetMethod_InvalidKind_400_NoPaymentCall(t *testing.T) {
	pay := &payoutPaymentStub{}
	ven := &payoutVenueStub{getVenue: func(in *venuev1.GetVenueRequest) (*venuev1.VenueResponse, error) {
		return &venuev1.VenueResponse{Id: in.GetId(), OwnerId: "owner-1"}, nil
	}}
	h := newPayoutHandler(pay, ven, nil)

	rr := httptest.NewRecorder()
	h.SetVenuePayoutMethod(rr, venueReq(t, http.MethodPut, "venue-1", "owner-1", `{"kind":"crypto"}`))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rr.Code, rr.Body.String())
	}
	if pay.setReq != nil {
		t.Fatal("payment SetPayoutMethod should not be called for an invalid kind")
	}
	assertCode(t, rr, "GATEWAY.PAYOUT.INVALID_KIND")
}

func TestPayout_SetMethod_Card_ForwardsPartnerAndKind(t *testing.T) {
	pay := &payoutPaymentStub{}
	ven := &payoutVenueStub{getVenue: func(in *venuev1.GetVenueRequest) (*venuev1.VenueResponse, error) {
		return &venuev1.VenueResponse{Id: in.GetId(), OwnerId: "owner-1"}, nil
	}}
	h := newPayoutHandler(pay, ven, nil)

	body := `{"kind":"card","card_last4":"4242","provider_token":"tok_secret"}`
	rr := httptest.NewRecorder()
	h.SetVenuePayoutMethod(rr, venueReq(t, http.MethodPut, "venue-1", "owner-1", body))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if pay.setReq == nil {
		t.Fatal("expected SetPayoutMethod to be called")
	}
	if pay.setReq.GetPartnerType() != "venue" || pay.setReq.GetPartnerId() != "venue-1" {
		t.Fatalf("wrong partner: %s/%s", pay.setReq.GetPartnerType(), pay.setReq.GetPartnerId())
	}
	if pay.setReq.GetKind() != paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD {
		t.Fatalf("wrong kind: %v", pay.setReq.GetKind())
	}
	if pay.setReq.GetProviderToken() != "tok_secret" {
		t.Fatalf("provider token not forwarded to payment-service")
	}
}

// ── Master resolution ────────────────────────────────────────────────────────────

// A master can only ever address their OWN profile: partner_id is resolved from
// the caller's master profile, never from a client-supplied id.
func TestPayout_Master_ResolvesOwnProfile(t *testing.T) {
	pay := &payoutPaymentStub{}
	mas := &payoutMasterStub{getMyProfile: func(in *masterv1.GetMyProfileRequest) (*masterv1.MasterResponse, error) {
		if in.GetUserId() != "user-9" {
			t.Fatalf("GetMyProfile must be keyed by the caller, got %q", in.GetUserId())
		}
		return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "master-77"}}, nil
	}}
	h := newPayoutHandler(pay, nil, mas)

	r := httptest.NewRequest(http.MethodGet, "/owner/master/balance", nil)
	r = r.WithContext(middleware.WithUserID(r.Context(), "user-9"))
	rr := httptest.NewRecorder()
	h.GetMasterBalance(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if pay.balReq.GetPartnerType() != "master" || pay.balReq.GetPartnerId() != "master-77" {
		t.Fatalf("partner must be the resolved master profile, got %s/%s",
			pay.balReq.GetPartnerType(), pay.balReq.GetPartnerId())
	}
}

func TestPayout_Master_NoProfile_NotCreated(t *testing.T) {
	pay := &payoutPaymentStub{}
	mas := &payoutMasterStub{getMyProfile: func(*masterv1.GetMyProfileRequest) (*masterv1.MasterResponse, error) {
		return nil, status.Error(codes.NotFound, "no profile")
	}}
	h := newPayoutHandler(pay, nil, mas)

	r := httptest.NewRequest(http.MethodGet, "/owner/master/payout-method", nil)
	r = r.WithContext(middleware.WithUserID(r.Context(), "user-9"))
	rr := httptest.NewRecorder()
	h.GetMasterPayoutMethod(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (%s)", rr.Code, rr.Body.String())
	}
	if pay.getReq != nil {
		t.Fatal("payment RPC should not be reached when the master has no profile")
	}
	assertCode(t, rr, "GATEWAY.MASTER.NOT_CREATED")
}

// ── shared assertion ─────────────────────────────────────────────────────────────

func assertCode(t *testing.T, rr *httptest.ResponseRecorder, want string) {
	t.Helper()
	// apicatalog.Entry.Write emits a flat {"code":"...","error":"..."} body.
	var out struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad error json %q: %v", rr.Body.String(), err)
	}
	if out.Code != want {
		t.Fatalf("want error code %s, got %q (%s)", want, out.Code, rr.Body.String())
	}
}
