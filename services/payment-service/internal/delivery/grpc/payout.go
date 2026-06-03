package grpc

import (
	"context"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/usecase"
)

// ── Payout method ──────────────────────────────────────────────────────────

func (s *Server) SetPayoutMethod(ctx context.Context, req *paymentv1.SetPayoutMethodRequest) (*paymentv1.PayoutMethodResponse, error) {
	if s.payout == nil {
		return nil, pkgerr.Unavailable("payout usecase not configured")
	}
	kind, err := payoutKindFromProto(req.GetKind())
	if err != nil {
		return nil, err
	}
	m, err := s.payout.SetPayoutMethod(ctx, usecase.SetPayoutMethodInput{
		PartnerType:   domain.PartnerType(req.GetPartnerType()),
		PartnerID:     req.GetPartnerId(),
		Kind:          kind,
		ProviderName:  req.GetProviderName(),
		CardLast4:     req.GetCardLast4(),
		CardBrand:     req.GetCardBrand(),
		ProviderToken: req.GetProviderToken(),
		BankBIC:       req.GetBankBic(),
		BankAccount:   req.GetBankAccount(),
		BankName:      req.GetBankName(),
		RecipientINN:  req.GetRecipientInn(),
		RecipientKPP:  req.GetRecipientKpp(),
		RecipientName: req.GetRecipientName(),
		SBPPhone:      req.GetSbpPhone(),
		SBPBankID:     req.GetSbpBankId(),
	})
	if err != nil {
		return nil, err
	}
	return payoutMethodToProto(m), nil
}

func (s *Server) GetPayoutMethod(ctx context.Context, req *paymentv1.GetPayoutMethodRequest) (*paymentv1.PayoutMethodResponse, error) {
	if s.payout == nil {
		return nil, pkgerr.Unavailable("payout usecase not configured")
	}
	m, err := s.payout.GetActivePayoutMethod(ctx, domain.PartnerType(req.GetPartnerType()), req.GetPartnerId())
	if err != nil {
		return nil, err
	}
	return payoutMethodToProto(m), nil
}

// ── Balance & history ──────────────────────────────────────────────────────

func (s *Server) GetPartnerBalance(ctx context.Context, req *paymentv1.GetPartnerBalanceRequest) (*paymentv1.PartnerBalanceResponse, error) {
	if s.payout == nil {
		return nil, pkgerr.Unavailable("payout usecase not configured")
	}
	b, err := s.payout.GetBalance(ctx, domain.PartnerType(req.GetPartnerType()), req.GetPartnerId())
	if err != nil {
		return nil, err
	}
	resp := &paymentv1.PartnerBalanceResponse{
		PartnerType:      string(b.PartnerType),
		PartnerId:        b.PartnerID,
		TotalKopecks:     b.TotalKopecks,
		AvailableKopecks: b.AvailableKopecks,
		HeldKopecks:      b.HeldKopecks,
	}
	if !b.LastEntryAt.IsZero() {
		resp.LastEntryAt = timestamppb.New(b.LastEntryAt)
	}
	return resp, nil
}

func (s *Server) ListPartnerLedger(ctx context.Context, req *paymentv1.ListPartnerLedgerRequest) (*paymentv1.ListPartnerLedgerResponse, error) {
	if s.payout == nil {
		return nil, pkgerr.Unavailable("payout usecase not configured")
	}
	entries, err := s.payout.ListLedger(ctx, domain.PartnerType(req.GetPartnerType()), req.GetPartnerId(),
		int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	out := &paymentv1.ListPartnerLedgerResponse{Entries: make([]*paymentv1.LedgerEntry, 0, len(entries))}
	for i := range entries {
		out.Entries = append(out.Entries, ledgerEntryToProto(&entries[i]))
	}
	return out, nil
}

func (s *Server) ListPartnerPayouts(ctx context.Context, req *paymentv1.ListPartnerPayoutsRequest) (*paymentv1.ListPartnerPayoutsResponse, error) {
	if s.payout == nil {
		return nil, pkgerr.Unavailable("payout usecase not configured")
	}
	payouts, err := s.payout.ListPayouts(ctx, domain.PartnerType(req.GetPartnerType()), req.GetPartnerId(),
		int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	out := &paymentv1.ListPartnerPayoutsResponse{Payouts: make([]*paymentv1.Payout, 0, len(payouts))}
	for i := range payouts {
		out.Payouts = append(out.Payouts, payoutToProto(&payouts[i]))
	}
	return out, nil
}

// ── Conversion helpers ────────────────────────────────────────────────────

func payoutKindFromProto(k paymentv1.PayoutMethodKind) (domain.PayoutMethodKind, error) {
	switch k {
	case paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD:
		return domain.PayoutMethodCard, nil
	case paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_BANK_ACCOUNT:
		return domain.PayoutMethodBankAccount, nil
	case paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_SBP:
		return domain.PayoutMethodSBP, nil
	}
	return "", pkgerr.InvalidArgument("payout method kind unspecified")
}

func payoutKindToProto(k domain.PayoutMethodKind) paymentv1.PayoutMethodKind {
	switch k {
	case domain.PayoutMethodCard:
		return paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_CARD
	case domain.PayoutMethodBankAccount:
		return paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_BANK_ACCOUNT
	case domain.PayoutMethodSBP:
		return paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_SBP
	}
	return paymentv1.PayoutMethodKind_PAYOUT_METHOD_KIND_UNSPECIFIED
}

func payoutMethodToProto(m *domain.PayoutMethod) *paymentv1.PayoutMethodResponse {
	resp := &paymentv1.PayoutMethodResponse{
		Id:            m.ID,
		PartnerType:   string(m.PartnerType),
		PartnerId:     m.PartnerID,
		Kind:          payoutKindToProto(m.Kind),
		ProviderName:  m.ProviderName,
		CardLast4:     m.CardLast4,
		CardBrand:     m.CardBrand,
		BankBic:       m.BankBIC,
		BankName:      m.BankName,
		RecipientName: m.RecipientName,
		SbpBankId:     m.SBPBankID,
	}
	// Write-only fields surface masked in the response only — never echo the
	// full token / account number even back to the partner cabinet, since the
	// gateway log layer would otherwise capture them.
	if m.BankAccount != "" {
		resp.BankAccountMasked = maskAccountTail(m.BankAccount)
	}
	if m.SBPPhone != "" {
		resp.SbpPhoneMasked = maskPhone(m.SBPPhone)
	}
	if !m.CreatedAt.IsZero() {
		resp.CreatedAt = timestamppb.New(m.CreatedAt)
	}
	if !m.UpdatedAt.IsZero() {
		resp.UpdatedAt = timestamppb.New(m.UpdatedAt)
	}
	return resp
}

func ledgerEntryToProto(e *domain.LedgerEntry) *paymentv1.LedgerEntry {
	out := &paymentv1.LedgerEntry{
		Id:              e.ID,
		PartnerType:     string(e.PartnerType),
		PartnerId:       e.PartnerID,
		EntryType:       string(e.EntryType),
		AmountKopecks:   e.AmountKopecks,
		PaymentId:       e.PaymentID,
		PayoutId:        e.PayoutID,
		ReversesEntryId: e.ReversesEntryID,
		Reason:          e.Reason,
	}
	if !e.AvailableAt.IsZero() {
		out.AvailableAt = timestamppb.New(e.AvailableAt)
	}
	if !e.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(e.CreatedAt)
	}
	return out
}

func payoutToProto(p *domain.Payout) *paymentv1.Payout {
	out := &paymentv1.Payout{
		Id:                 p.ID,
		PartnerType:        string(p.PartnerType),
		PartnerId:          p.PartnerID,
		AmountKopecks:      p.AmountKopecks,
		Currency:           p.Currency,
		Status:             p.Status.String(),
		MethodKindSnapshot: payoutKindToProto(p.MethodKindSnapshot),
		MethodDisplay:      p.MethodDisplay,
		ProviderName:       p.ProviderName,
		ProviderPayoutId:   p.ProviderPayoutID,
		FailureReason:      p.FailureReason,
	}
	if !p.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(p.CreatedAt)
	}
	if !p.CompletedAt.IsZero() {
		out.CompletedAt = timestamppb.New(p.CompletedAt)
	}
	return out
}

// maskAccountTail returns the last 4 digits prefixed with "•••" so the
// partner can confirm the account without leaking the rest into logs.
func maskAccountTail(s string) string {
	if len(s) < 4 {
		return "••••"
	}
	return "•••" + s[len(s)-4:]
}

// maskPhone masks all but the last two digits of an E.164 phone.
func maskPhone(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return "••"
	}
	return "•••" + s[len(s)-2:]
}
