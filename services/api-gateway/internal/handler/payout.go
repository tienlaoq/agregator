package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

// PayoutHandler serves the partner cabinet's escrow endpoints: payout method,
// balance, ledger and payout history. A "partner" is keyed by
// (partner_type, partner_id) where partner_id is a venue UUID or a master UUID.
//
// AUTHORIZATION — the gateway is the only place that knows venue/master
// ownership; payment-service trusts the resolved (partner_type, partner_id) it
// receives over the service-token channel. Every endpoint therefore resolves
// and verifies ownership *before* forwarding:
//   - venue routes: the caller must be the venue's legal owner (OwnerId match).
//     Money rails are owner-only — staff/managers are intentionally excluded.
//   - master routes: partner_id is the caller's own master profile.
//
// PII / FINANCIAL DATA LOGGING POLICY (mirrors the webhook policy in payment.go):
//
//	SetPayoutMethod requests carry payout requisites — a saved-card provider
//	token, a full bank account number, an SBP phone. These MUST NEVER be
//	logged, in whole or in part, on any path. This handler does not log
//	request bodies. payment-service returns only masked fields, so rendering
//	the response is safe.
type PayoutHandler struct {
	payment paymentv1.PaymentServiceClient
	venue   venuev1.VenueServiceClient
	master  masterv1.MasterServiceClient
	log     zerolog.Logger
}

func NewPayoutHandler(
	log zerolog.Logger,
	payment paymentv1.PaymentServiceClient,
	venue venuev1.VenueServiceClient,
	master masterv1.MasterServiceClient,
) *PayoutHandler {
	return &PayoutHandler{payment: payment, venue: venue, master: master, log: log}
}

// ── Partner resolution + authorization ──────────────────────────────────────

// resolveVenuePartner verifies the caller owns the venue named by {venueId} and
// returns ("venue", venueID, true). On any failure it writes the HTTP response
// and returns ok=false.
func (h *PayoutHandler) resolveVenuePartner(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return "", "", false
	}
	venueID := strings.TrimSpace(chi.URLParam(r, "venueId"))
	if venueID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidVenueId)
		return "", "", false
	}
	v, err := h.venue.GetVenue(r.Context(), &venuev1.GetVenueRequest{Id: venueID})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return "", "", false
	}
	if v.GetOwnerId() != actorID {
		httpx.WriteCatalog(w, apicatalog.GatewayPayoutForbidden)
		return "", "", false
	}
	return "venue", venueID, true
}

// resolveMasterPartner resolves the caller's own master profile and returns
// ("master", masterID, true). On any failure it writes the response.
func (h *PayoutHandler) resolveMasterPartner(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	actorID := middleware.UserIDFromCtx(r.Context())
	if actorID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayAuthUnauthorized)
		return "", "", false
	}
	resp, err := h.master.GetMyProfile(r.Context(), &masterv1.GetMyProfileRequest{UserId: actorID})
	if err != nil {
		// No master profile yet → guide the caller to create one first, rather
		// than surfacing a bare upstream 404. Payouts require a profile to key
		// the partner ledger on.
		if status.Code(err) == codes.NotFound {
			httpx.WriteCatalog(w, apicatalog.GatewayMasterNotCreated)
			return "", "", false
		}
		httpx.GRPCErrorToHTTP(w, err)
		return "", "", false
	}
	masterID := strings.TrimSpace(resp.GetMaster().GetId())
	if masterID == "" {
		httpx.WriteCatalog(w, apicatalog.GatewayMasterNotCreated)
		return "", "", false
	}
	return "master", masterID, true
}

// ── Route-bound handlers (venue) ─────────────────────────────────────────────

func (h *PayoutHandler) GetVenuePayoutMethod(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := h.resolveVenuePartner(w, r)
	if !ok {
		return
	}
	h.getPayoutMethod(w, r, pt, pid)
}

func (h *PayoutHandler) SetVenuePayoutMethod(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := h.resolveVenuePartner(w, r)
	if !ok {
		return
	}
	h.setPayoutMethod(w, r, pt, pid)
}

func (h *PayoutHandler) GetVenueBalance(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := h.resolveVenuePartner(w, r)
	if !ok {
		return
	}
	h.getBalance(w, r, pt, pid)
}

func (h *PayoutHandler) ListVenueLedger(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := h.resolveVenuePartner(w, r)
	if !ok {
		return
	}
	h.listLedger(w, r, pt, pid)
}

func (h *PayoutHandler) ListVenuePayouts(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := h.resolveVenuePartner(w, r)
	if !ok {
		return
	}
	h.listPayouts(w, r, pt, pid)
}

// ── Route-bound handlers (master) ────────────────────────────────────────────

func (h *PayoutHandler) GetMasterPayoutMethod(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := h.resolveMasterPartner(w, r)
	if !ok {
		return
	}
	h.getPayoutMethod(w, r, pt, pid)
}

func (h *PayoutHandler) SetMasterPayoutMethod(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := h.resolveMasterPartner(w, r)
	if !ok {
		return
	}
	h.setPayoutMethod(w, r, pt, pid)
}

func (h *PayoutHandler) GetMasterBalance(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := h.resolveMasterPartner(w, r)
	if !ok {
		return
	}
	h.getBalance(w, r, pt, pid)
}

func (h *PayoutHandler) ListMasterLedger(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := h.resolveMasterPartner(w, r)
	if !ok {
		return
	}
	h.listLedger(w, r, pt, pid)
}

func (h *PayoutHandler) ListMasterPayouts(w http.ResponseWriter, r *http.Request) {
	pt, pid, ok := h.resolveMasterPartner(w, r)
	if !ok {
		return
	}
	h.listPayouts(w, r, pt, pid)
}

// ── Shared implementations ───────────────────────────────────────────────────

func (h *PayoutHandler) getPayoutMethod(w http.ResponseWriter, r *http.Request, partnerType, partnerID string) {
	resp, err := h.payment.GetPayoutMethod(r.Context(), &paymentv1.GetPayoutMethodRequest{
		PartnerType: partnerType,
		PartnerId:   partnerID,
	})
	if err != nil {
		// No method configured yet is a normal state, not an error for the UI.
		if status.Code(err) == codes.NotFound {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"payout_method": nil})
			return
		}
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"payout_method": payoutMethodToJSON(resp)})
}

func (h *PayoutHandler) setPayoutMethod(w http.ResponseWriter, r *http.Request, partnerType, partnerID string) {
	// NOTE: req carries payout requisites (provider_token / bank_account /
	// sbp_phone). Do not log req or any of its fields — see the file-level
	// PII policy. payment-service performs the authoritative validation.
	var req struct {
		Kind         string `json:"kind"`
		ProviderName string `json:"provider_name"`

		// Card
		CardLast4     string `json:"card_last4"`
		CardBrand     string `json:"card_brand"`
		ProviderToken string `json:"provider_token"`

		// Bank account
		BankBIC       string `json:"bank_bic"`
		BankAccount   string `json:"bank_account"`
		BankName      string `json:"bank_name"`
		RecipientINN  string `json:"recipient_inn"`
		RecipientKPP  string `json:"recipient_kpp"`
		RecipientName string `json:"recipient_name"`

		// SBP
		SBPPhone  string `json:"sbp_phone"`
		SBPBankID string `json:"sbp_bank_id"`
	}
	if !httpx.ReadJSONOrRespond(w, r, &req) {
		return
	}

	kind, ok := payoutKindFromString(req.Kind)
	if !ok {
		httpx.WriteCatalog(w, apicatalog.GatewayPayoutInvalidKind)
		return
	}

	resp, err := h.payment.SetPayoutMethod(r.Context(), &paymentv1.SetPayoutMethodRequest{
		PartnerType:  partnerType,
		PartnerId:    partnerID,
		Kind:         kind,
		ProviderName: strings.TrimSpace(req.ProviderName),

		CardLast4:     strings.TrimSpace(req.CardLast4),
		CardBrand:     strings.TrimSpace(req.CardBrand),
		ProviderToken: strings.TrimSpace(req.ProviderToken),

		BankBic:       strings.TrimSpace(req.BankBIC),
		BankAccount:   strings.TrimSpace(req.BankAccount),
		BankName:      strings.TrimSpace(req.BankName),
		RecipientInn:  strings.TrimSpace(req.RecipientINN),
		RecipientKpp:  strings.TrimSpace(req.RecipientKPP),
		RecipientName: strings.TrimSpace(req.RecipientName),

		SbpPhone:  strings.TrimSpace(req.SBPPhone),
		SbpBankId: strings.TrimSpace(req.SBPBankID),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}

	// Audit trail: payment-service is the system of record for the requisites,
	// but it only ever sees (partner_type, partner_id) over the service token —
	// it never learns *which* user changed them. The gateway is the only layer
	// that knows the acting user, so this is the sole place that linkage can be
	// recorded. PII-safe: actor/partner are UUIDs and kind is an enum; the
	// requisites themselves (token / account / phone) are never logged.
	h.log.Info().
		Str("event", "payout_method.set").
		Str("actor_id", middleware.UserIDFromCtx(r.Context())).
		Str("partner_type", partnerType).
		Str("partner_id", partnerID).
		Str("kind", payoutKindToString(kind)).
		Msg("partner payout method updated")

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"payout_method": payoutMethodToJSON(resp)})
}

func (h *PayoutHandler) getBalance(w http.ResponseWriter, r *http.Request, partnerType, partnerID string) {
	resp, err := h.payment.GetPartnerBalance(r.Context(), &paymentv1.GetPartnerBalanceRequest{
		PartnerType: partnerType,
		PartnerId:   partnerID,
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	out := map[string]any{
		"partner_type":      resp.GetPartnerType(),
		"partner_id":        resp.GetPartnerId(),
		"total_kopecks":     resp.GetTotalKopecks(),
		"available_kopecks": resp.GetAvailableKopecks(),
		"held_kopecks":      resp.GetHeldKopecks(),
	}
	if ts := resp.GetLastEntryAt(); ts != nil {
		out["last_entry_at"] = ts.AsTime()
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"balance": out})
}

func (h *PayoutHandler) listLedger(w http.ResponseWriter, r *http.Request, partnerType, partnerID string) {
	limit, ok := httpx.QueryInt(w, r, "limit", 50, 1, 200)
	if !ok {
		return
	}
	offset, ok := httpx.QueryInt(w, r, "offset", 0, 0, 1_000_000)
	if !ok {
		return
	}
	resp, err := h.payment.ListPartnerLedger(r.Context(), &paymentv1.ListPartnerLedgerRequest{
		PartnerType: partnerType,
		PartnerId:   partnerID,
		Limit:       int32(limit),
		Offset:      int32(offset),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	entries := make([]map[string]any, 0, len(resp.GetEntries()))
	for _, e := range resp.GetEntries() {
		entries = append(entries, ledgerEntryToJSON(e))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (h *PayoutHandler) listPayouts(w http.ResponseWriter, r *http.Request, partnerType, partnerID string) {
	limit, ok := httpx.QueryInt(w, r, "limit", 50, 1, 200)
	if !ok {
		return
	}
	offset, ok := httpx.QueryInt(w, r, "offset", 0, 0, 1_000_000)
	if !ok {
		return
	}
	resp, err := h.payment.ListPartnerPayouts(r.Context(), &paymentv1.ListPartnerPayoutsRequest{
		PartnerType: partnerType,
		PartnerId:   partnerID,
		Limit:       int32(limit),
		Offset:      int32(offset),
	})
	if err != nil {
		httpx.GRPCErrorToHTTP(w, err)
		return
	}
	payouts := make([]map[string]any, 0, len(resp.GetPayouts()))
	for _, p := range resp.GetPayouts() {
		payouts = append(payouts, payoutToJSON(p))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"payouts": payouts})
}

// ── Converters ───────────────────────────────────────────────────────────────

func payoutKindFromString(s string) (paymentv1.PayoutMethodKind, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "card":
		return paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD, true
	case "bank_account":
		return paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_BANK_ACCOUNT, true
	case "sbp":
		return paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_SBP, true
	default:
		return paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_UNSPECIFIED, false
	}
}

func payoutKindToString(k paymentv1.PayoutMethodKind) string {
	switch k {
	case paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD:
		return "card"
	case paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_BANK_ACCOUNT:
		return "bank_account"
	case paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_SBP:
		return "sbp"
	default:
		return ""
	}
}

// payoutMethodToJSON renders a PayoutMethodResponse. The proto already omits
// write-only secrets (provider_token, full account number) and masks the rest,
// so every field here is safe to return to the partner.
func payoutMethodToJSON(m *paymentv1.PayoutMethodResponse) map[string]any {
	out := map[string]any{
		"id":            m.GetId(),
		"partner_type":  m.GetPartnerType(),
		"partner_id":    m.GetPartnerId(),
		"kind":          payoutKindToString(m.GetKind()),
		"provider_name": m.GetProviderName(),
	}
	switch m.GetKind() {
	case paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD:
		out["card_last4"] = m.GetCardLast4()
		out["card_brand"] = m.GetCardBrand()
	case paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_BANK_ACCOUNT:
		out["bank_bic"] = m.GetBankBic()
		out["bank_account_masked"] = m.GetBankAccountMasked()
		out["bank_name"] = m.GetBankName()
		out["recipient_name"] = m.GetRecipientName()
	case paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_SBP:
		out["sbp_phone_masked"] = m.GetSbpPhoneMasked()
		out["sbp_bank_id"] = m.GetSbpBankId()
	}
	if ts := m.GetCreatedAt(); ts != nil {
		out["created_at"] = ts.AsTime()
	}
	if ts := m.GetUpdatedAt(); ts != nil {
		out["updated_at"] = ts.AsTime()
	}
	return out
}

func ledgerEntryToJSON(e *paymentv1.LedgerEntry) map[string]any {
	out := map[string]any{
		"id":             e.GetId(),
		"partner_type":   e.GetPartnerType(),
		"partner_id":     e.GetPartnerId(),
		"entry_type":     e.GetEntryType(),
		"amount_kopecks": e.GetAmountKopecks(),
	}
	if v := e.GetPaymentId(); v != "" {
		out["payment_id"] = v
	}
	if v := e.GetPayoutId(); v != "" {
		out["payout_id"] = v
	}
	if v := e.GetReversesEntryId(); v != 0 {
		out["reverses_entry_id"] = v
	}
	if v := e.GetReason(); v != "" {
		out["reason"] = v
	}
	if ts := e.GetAvailableAt(); ts != nil {
		out["available_at"] = ts.AsTime()
	}
	if ts := e.GetCreatedAt(); ts != nil {
		out["created_at"] = ts.AsTime()
	}
	return out
}

func payoutToJSON(p *paymentv1.Payout) map[string]any {
	out := map[string]any{
		"id":             p.GetId(),
		"partner_type":   p.GetPartnerType(),
		"partner_id":     p.GetPartnerId(),
		"amount_kopecks": p.GetAmountKopecks(),
		"currency":       p.GetCurrency(),
		"status":         p.GetStatus(),
		"method_kind":    payoutKindToString(p.GetMethodKindSnapshot()),
		"method_display": p.GetMethodDisplay(),
		"provider_name":  p.GetProviderName(),
	}
	if v := p.GetProviderPayoutId(); v != "" {
		out["provider_payout_id"] = v
	}
	if v := p.GetFailureReason(); v != "" {
		out["failure_reason"] = v
	}
	if ts := p.GetCreatedAt(); ts != nil {
		out["created_at"] = ts.AsTime()
	}
	if ts := p.GetCompletedAt(); ts != nil {
		out["completed_at"] = ts.AsTime()
	}
	return out
}
