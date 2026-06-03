package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// Reconciliation tunables.
//
// reconcileMinAge — a payout must be older than this before reconciliation
// re-calls the provider. Short transient blips usually resolve on the next
// scheduler tick; reconciling too eagerly risks racing with an in-flight
// provider response.
//
// reconcileInterval — how often the reconciliation loop runs. Faster than the
// main scheduler tick so a stuck payout is unblocked quickly (the
// payouts_partner_active_uniq index blocks all new payouts for a partner while
// one is stuck in pending).
//
// reconcileBatchLimit — max payouts processed per reconciliation tick.
const (
	reconcileMinAge     = 2 * time.Minute
	reconcileInterval   = 1 * time.Minute
	reconcileBatchLimit = 100
)

// PayoutUseCase orchestrates the partner-payout lifecycle.
//
// Responsibilities:
//   - manage partner payout methods (CRUD),
//   - expose balance and ledger reads,
//   - schedule outbound payouts: pick partners with ripe balance,
//     create the payout row + ledger debit atomically, call the provider,
//     and react to the resulting webhook by marking succeeded or failed
//     (the latter writes a compensating ledger credit so the balance is
//     restored).
//
// The scheduler runs as a goroutine started by RunScheduler.  All scheduler
// work happens in batches read from PartnersWithAvailableBalance — there is
// no persistent in-memory queue, so a process restart simply means the next
// tick picks up the same partners.
type PayoutUseCase struct {
	payouts        domain.PayoutRepository
	methods        domain.PayoutMethodRepository
	ledger         domain.LedgerRepository
	provider       provider.PaymentProvider
	activeProvider string

	minPayoutKopecks int64
	tickInterval     time.Duration
	log              zerolog.Logger
}

// PayoutSchedulerConfig holds the scheduler tunables.
type PayoutSchedulerConfig struct {
	// MinPayoutKopecks is the smallest balance the scheduler will attempt to pay
	// out in one batch.  Smaller balances accumulate until the threshold is met.
	MinPayoutKopecks int64
	// TickInterval is how often the scheduler scans for ripe partners.
	// Defaults to 1 hour when zero — ledger accruals naturally become available
	// at the per-second granularity of available_at, so anything more frequent
	// than a few minutes is just churn.
	TickInterval time.Duration
}

func NewPayoutUseCase(
	payouts domain.PayoutRepository,
	methods domain.PayoutMethodRepository,
	ledger domain.LedgerRepository,
	prov provider.PaymentProvider,
	activeProvider string,
	cfg PayoutSchedulerConfig,
	lg zerolog.Logger,
) *PayoutUseCase {
	if cfg.MinPayoutKopecks <= 0 {
		cfg.MinPayoutKopecks = 10000 // 100 rubles
	}
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = time.Hour
	}
	return &PayoutUseCase{
		payouts:          payouts,
		methods:          methods,
		ledger:           ledger,
		provider:         prov,
		activeProvider:   activeProvider,
		minPayoutKopecks: cfg.MinPayoutKopecks,
		tickInterval:     cfg.TickInterval,
		log:              lg.With().Str("component", "payout-usecase").Logger(),
	}
}

// ── Method CRUD ────────────────────────────────────────────────────────────

// SetPayoutMethodInput is the provider-agnostic input for saving a partner's
// bank rails.  The caller fills only the fields matching Kind; others are
// ignored.  The repository enforces the per-kind CHECK constraints.
type SetPayoutMethodInput struct {
	PartnerType  domain.PartnerType
	PartnerID    string
	Kind         domain.PayoutMethodKind
	ProviderName string

	// Card
	CardLast4     string
	CardBrand     string
	ProviderToken string

	// Bank account
	BankBIC       string
	BankAccount   string
	BankName      string
	RecipientINN  string
	RecipientKPP  string
	RecipientName string

	// SBP
	SBPPhone  string
	SBPBankID string
}

func (uc *PayoutUseCase) SetPayoutMethod(ctx context.Context, in SetPayoutMethodInput) (*domain.PayoutMethod, error) {
	if !in.PartnerType.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid partner_type")
	}
	if strings.TrimSpace(in.PartnerID) == "" {
		return nil, status.Error(codes.InvalidArgument, "partner_id required")
	}
	if !in.Kind.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid method_kind")
	}
	// Provider name for card-rail methods must match the active payout provider —
	// stored tokens are provider-scoped and not exchangeable.
	if in.Kind == domain.PayoutMethodCard {
		if strings.TrimSpace(in.ProviderName) == "" {
			in.ProviderName = uc.activeProvider
		}
		if uc.activeProvider != "" && in.ProviderName != uc.activeProvider {
			return nil, status.Errorf(codes.InvalidArgument,
				"card payout method provider %q does not match active payout provider %q",
				in.ProviderName, uc.activeProvider)
		}
	}

	m := &domain.PayoutMethod{
		PartnerType:   in.PartnerType,
		PartnerID:     strings.TrimSpace(in.PartnerID),
		Kind:          in.Kind,
		ProviderName:  strings.TrimSpace(in.ProviderName),
		CardLast4:     strings.TrimSpace(in.CardLast4),
		CardBrand:     strings.TrimSpace(in.CardBrand),
		ProviderToken: strings.TrimSpace(in.ProviderToken),
		BankBIC:       strings.TrimSpace(in.BankBIC),
		BankAccount:   strings.TrimSpace(in.BankAccount),
		BankName:      strings.TrimSpace(in.BankName),
		RecipientINN:  strings.TrimSpace(in.RecipientINN),
		RecipientKPP:  strings.TrimSpace(in.RecipientKPP),
		RecipientName: strings.TrimSpace(in.RecipientName),
		SBPPhone:      strings.TrimSpace(in.SBPPhone),
		SBPBankID:     strings.TrimSpace(in.SBPBankID),
	}
	if err := uc.methods.Save(ctx, m); err != nil {
		return nil, fmt.Errorf("save payout method: %w", err)
	}
	return m, nil
}

func (uc *PayoutUseCase) GetActivePayoutMethod(ctx context.Context, partnerType domain.PartnerType, partnerID string) (*domain.PayoutMethod, error) {
	return uc.methods.GetActive(ctx, partnerType, partnerID)
}

// ── Balance & history reads ────────────────────────────────────────────────

func (uc *PayoutUseCase) GetBalance(ctx context.Context, partnerType domain.PartnerType, partnerID string) (*domain.PartnerBalance, error) {
	if !partnerType.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid partner_type")
	}
	return uc.ledger.Balance(ctx, partnerType, partnerID)
}

func (uc *PayoutUseCase) ListLedger(ctx context.Context, partnerType domain.PartnerType, partnerID string, limit, offset int) ([]domain.LedgerEntry, error) {
	return uc.ledger.List(ctx, partnerType, partnerID, limit, offset)
}

func (uc *PayoutUseCase) ListPayouts(ctx context.Context, partnerType domain.PartnerType, partnerID string, limit, offset int) ([]domain.Payout, error) {
	return uc.payouts.List(ctx, partnerType, partnerID, limit, offset)
}

// ── Scheduler ──────────────────────────────────────────────────────────────

// RunScheduler runs the payout loop until ctx is cancelled.
// One tick happens immediately on start so a freshly-restarted service does
// not wait the full interval to drain accumulated balances.
//
// Concurrency:
//   - safe to call once per process; multiple parallel calls would compete
//     for the same partners but the payouts_partner_active_uniq partial
//     unique index ensures no double-payout (one wins, others get
//     AlreadyExists and skip).
//   - scheduler.tick is non-reentrant: a single tick processes all ripe
//     partners sequentially.  If batch is large enough to take longer than
//     the tick interval, the next tick simply waits.
//
// The reconciliation loop runs on a separate, faster ticker so payouts stuck
// in pending (provider call returned a transient error) are unblocked within
// minutes rather than waiting a full scheduler tick.
func (uc *PayoutUseCase) RunScheduler(ctx context.Context) error {
	// One immediate tick on start, then on the regular ticker cadence.
	uc.tick(ctx)
	uc.reconcile(ctx)

	t := time.NewTicker(uc.tickInterval)
	defer t.Stop()
	rt := time.NewTicker(reconcileInterval)
	defer rt.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			uc.tick(ctx)
		case <-rt.C:
			uc.reconcile(ctx)
		}
	}
}

// reconcile re-drives payouts stuck in 'pending' state by re-calling the
// provider with the SAME idempotency key.  The provider deduplicates on this
// key, so:
//   - if the original request actually landed at the provider, the response
//     returns the existing payout — we record provider_payout_id and move to
//     'processing'.
//   - if the original request never landed, the provider creates the payout
//     now — same recovery path.
//
// This is the second half of the transient-error fix: payoutPartner leaves
// the row in pending instead of reversing, and reconcile pulls it forward
// the moment the provider becomes reachable again.
func (uc *PayoutUseCase) reconcile(ctx context.Context) {
	stuck, err := uc.payouts.ListPendingOlderThan(ctx, reconcileMinAge, reconcileBatchLimit)
	if err != nil {
		uc.log.Error().Err(err).Msg("payout reconcile: list pending failed")
		return
	}
	for i := range stuck {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := uc.reconcileOne(ctx, &stuck[i]); err != nil {
			uc.log.Error().
				Str("payout_id", stuck[i].ID).
				Str("idempotency_key", stuck[i].IdempotencyKey).
				Err(err).
				Msg("payout reconcile: one failed")
		}
	}
}

// reconcileOne re-calls provider.CreatePayout for a single stuck pending
// payout.  The provider returns the existing payout on idempotent-key match,
// so this is safe to invoke repeatedly.
//
// Transient errors are logged and left for the next reconciliation tick.
// Permanent errors (4xx) reverse the ledger — at this point the provider has
// definitively rejected the request, no payout was issued.
func (uc *PayoutUseCase) reconcileOne(ctx context.Context, p *domain.Payout) error {
	method, err := uc.methods.GetByID(ctx, p.MethodID)
	if err != nil {
		// Snapshot method missing is a hard inconsistency — surface as terminal
		// failure so the partner is unblocked.  This should never happen given
		// the FK from payouts.method_id, but keeps the loop from spinning forever.
		_, markErr := uc.payouts.MarkFailedWithReversal(ctx, p.ID,
			"reconcile: payout method missing: "+err.Error(), time.Now())
		if markErr != nil {
			return fmt.Errorf("reconcile method missing AND mark-failed failed: %v / %w", err, markErr)
		}
		return nil
	}

	result, err := uc.provider.CreatePayout(ctx, provider.PayoutRequest{
		AmountKopecks:  p.AmountKopecks,
		Currency:       p.Currency,
		IdempotencyKey: p.IdempotencyKey,
		Kind:           toProviderDestKind(method.Kind),
		ProviderToken:  method.ProviderToken,
		BankBIC:        method.BankBIC,
		BankAccount:    method.BankAccount,
		RecipientINN:   method.RecipientINN,
		RecipientKPP:   method.RecipientKPP,
		RecipientName:  method.RecipientName,
		SBPPhone:       method.SBPPhone,
		SBPBankID:      method.SBPBankID,
		PartnerType:    string(p.PartnerType),
		PartnerID:      p.PartnerID,
		Description:    "Платёж партнёру",
	})
	if err != nil {
		if errors.Is(err, provider.ErrTransient) {
			uc.log.Warn().
				Str("payout_id", p.ID).
				Err(err).
				Msg("payout reconcile: still transient — will retry")
			return nil
		}
		_, markErr := uc.payouts.MarkFailedWithReversal(ctx, p.ID, err.Error(), time.Now())
		if markErr != nil {
			return fmt.Errorf("reconcile permanent error AND mark-failed failed: %v / %w", err, markErr)
		}
		return nil
	}

	if err := uc.payouts.MarkProcessing(ctx, p.ID, result.ProviderPayoutID); err != nil {
		return fmt.Errorf("reconcile mark processing: %w", err)
	}

	switch result.Status {
	case provider.PayoutStatusSucceeded:
		if _, err := uc.payouts.MarkSucceeded(ctx, p.ID, time.Now()); err != nil {
			return fmt.Errorf("reconcile mark succeeded: %w", err)
		}
	case provider.PayoutStatusFailed:
		if _, err := uc.payouts.MarkFailedWithReversal(ctx, p.ID, result.FailureReason, time.Now()); err != nil {
			return fmt.Errorf("reconcile mark failed: %w", err)
		}
	}
	uc.log.Info().
		Str("payout_id", p.ID).
		Str("provider_payout_id", result.ProviderPayoutID).
		Str("status", string(result.Status)).
		Msg("payout reconcile: recovered")
	return nil
}

// tick processes one batch of ripe partners.  Errors from individual partner
// payouts are logged and swallowed — one bad partner must not stop progress
// for the rest.
func (uc *PayoutUseCase) tick(ctx context.Context) {
	const batchLimit = 100

	partners, err := uc.ledger.PartnersWithAvailableBalance(ctx, uc.minPayoutKopecks, batchLimit)
	if err != nil {
		uc.log.Error().Err(err).Msg("payout scheduler: enumerate partners failed")
		return
	}
	for _, p := range partners {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := uc.payoutPartner(ctx, p); err != nil {
			uc.log.Error().
				Str("partner_type", string(p.PartnerType)).
				Str("partner_id", p.PartnerID).
				Err(err).
				Msg("payout scheduler: partner payout failed")
		}
	}
}

// payoutPartner attempts to issue one payout for a single partner.
//
// Flow:
//  1. Re-read available balance inside this call — between the enumeration
//     query and now the balance may have moved (refund, another scheduler).
//  2. Resolve the active payout method.
//  3. CreatePendingWithLedger writes the payout row + negative ledger debit
//     in one TX.  The partial unique index serialises concurrent attempts.
//  4. Call provider.CreatePayout; on terminal-fast-success or terminal-fast-
//     failure update the payout row right away.  On Pending leave it as
//     processing — the webhook will move it terminal.
func (uc *PayoutUseCase) payoutPartner(ctx context.Context, ref domain.PartnerRef) error {
	balance, err := uc.ledger.Balance(ctx, ref.PartnerType, ref.PartnerID)
	if err != nil {
		return fmt.Errorf("balance: %w", err)
	}
	if balance.AvailableKopecks < uc.minPayoutKopecks {
		return nil
	}

	method, err := uc.methods.GetActive(ctx, ref.PartnerType, ref.PartnerID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			uc.log.Info().
				Str("partner_type", string(ref.PartnerType)).
				Str("partner_id", ref.PartnerID).
				Int64("available_kopecks", balance.AvailableKopecks).
				Msg("payout scheduler: partner has balance but no active method")
			return nil
		}
		return fmt.Errorf("get method: %w", err)
	}

	amount := balance.AvailableKopecks

	// Derive idempotency key from the payout's own UUID, which we pre-generate
	// so the same UUID lands in the DB row and in the provider call.  Earlier
	// drafts derived the key from (partner, amount); that wrongly collapsed two
	// legitimate payouts of the same amount (one issued, then refunded, then a
	// fresh accrual restored the balance) into one provider call.  Per-row UUID
	// ensures every new payout row gets its own provider-side identity.
	payoutID := uuid.NewString()
	idempotencyKey := "payout:" + payoutID

	payout := &domain.Payout{
		ID:                 payoutID,
		PartnerType:        ref.PartnerType,
		PartnerID:          ref.PartnerID,
		AmountKopecks:      amount,
		MethodID:           method.ID,
		MethodKindSnapshot: method.Kind,
		MethodDisplay:      method.Display(),
		ProviderName:       uc.activeProvider,
		IdempotencyKey:     idempotencyKey,
	}
	if err := uc.payouts.CreatePendingWithLedger(ctx, payout); err != nil {
		// AlreadyExists means another scheduler tick beat us to it OR the
		// partner already has an in-flight payout — leave for next time.
		if status.Code(err) == codes.AlreadyExists {
			return nil
		}
		// FailedPrecondition means the live balance inside the repo TX was
		// lower than the planned amount (a refund-reversal landed between our
		// Balance read and the CreatePendingWithLedger call).  The advisory
		// lock inside the repo refused the over-large payout — exactly the
		// safety net we want.  Skip the partner; next tick re-reads balance.
		if status.Code(err) == codes.FailedPrecondition {
			uc.log.Info().
				Str("partner_type", string(ref.PartnerType)).
				Str("partner_id", ref.PartnerID).
				Int64("planned_amount_kopecks", amount).
				Err(err).
				Msg("payout scheduler: balance dropped before insert — skipping")
			return nil
		}
		return fmt.Errorf("create pending payout: %w", err)
	}

	result, err := uc.provider.CreatePayout(ctx, provider.PayoutRequest{
		AmountKopecks:  amount,
		Currency:       "RUB",
		IdempotencyKey: idempotencyKey,
		Kind:           toProviderDestKind(method.Kind),
		ProviderToken:  method.ProviderToken,
		BankBIC:        method.BankBIC,
		BankAccount:    method.BankAccount,
		RecipientINN:   method.RecipientINN,
		RecipientKPP:   method.RecipientKPP,
		RecipientName:  method.RecipientName,
		SBPPhone:       method.SBPPhone,
		SBPBankID:      method.SBPBankID,
		PartnerType:    string(ref.PartnerType),
		PartnerID:      ref.PartnerID,
		Description:    "Платёж партнёру",
	})
	if err != nil {
		// CRITICAL: do not reverse the ledger on transient errors.  The provider
		// may have accepted the request even though we never got a response (TCP
		// timeout, 5xx).  Reversing here and creating a new payout on the next
		// tick (with a fresh idempotency key) would double-pay the partner once
		// the original request lands.  Leave the row in pending; the
		// reconciliation loop will re-call CreatePayout with the SAME idempotency
		// key — ЮKassa deduplicates and returns the existing payout if any.
		if errors.Is(err, provider.ErrTransient) {
			uc.log.Warn().
				Str("payout_id", payout.ID).
				Str("idempotency_key", idempotencyKey).
				Err(err).
				Msg("payout: transient provider error — leaving pending for reconciliation")
			return nil
		}
		// Permanent error (4xx, validation) — provider definitely rejected the
		// request, no payout was issued.  Safe to reverse.
		_, markErr := uc.payouts.MarkFailedWithReversal(ctx, payout.ID, err.Error(), time.Now())
		if markErr != nil {
			return fmt.Errorf("provider create payout failed AND mark-failed failed: provider=%v markErr=%w", err, markErr)
		}
		return fmt.Errorf("provider create payout: %w", err)
	}

	if err := uc.payouts.MarkProcessing(ctx, payout.ID, result.ProviderPayoutID); err != nil {
		return fmt.Errorf("mark processing: %w", err)
	}

	switch result.Status {
	case provider.PayoutStatusSucceeded:
		if _, err := uc.payouts.MarkSucceeded(ctx, payout.ID, time.Now()); err != nil {
			return fmt.Errorf("mark succeeded: %w", err)
		}
	case provider.PayoutStatusFailed:
		if _, err := uc.payouts.MarkFailedWithReversal(ctx, payout.ID, result.FailureReason, time.Now()); err != nil {
			return fmt.Errorf("mark failed: %w", err)
		}
	}
	// PayoutStatusPending — leave for webhook.
	return nil
}

func toProviderDestKind(k domain.PayoutMethodKind) provider.PayoutDestinationKind {
	switch k {
	case domain.PayoutMethodCard:
		return provider.PayoutDestCard
	case domain.PayoutMethodBankAccount:
		return provider.PayoutDestBankAccount
	case domain.PayoutMethodSBP:
		return provider.PayoutDestSBP
	}
	return ""
}

// ── Payout webhook ────────────────────────────────────────────────────────

// HandlePayoutWebhook processes an inbound payout notification, transitioning
// the local payout row to its terminal state and writing a compensating
// ledger entry on failure.
//
// Returns (nil, nil) when the body is not a payout notification — the caller
// should then fall back to the payment HandleWebhook path.
func (uc *PayoutUseCase) HandlePayoutWebhook(ctx context.Context, rawBody []byte) (*provider.PayoutWebhookEvent, error) {
	event, err := uc.provider.ParsePayoutWebhook(ctx, rawBody, nil)
	if err != nil {
		return nil, fmt.Errorf("parse payout webhook: %w", err)
	}
	if event == nil {
		return nil, nil
	}

	p, err := uc.payouts.GetByProviderPayoutID(ctx, event.ProviderPayoutID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Stale webhook for a payout we don't know about (cross-environment
			// replay, manual provider-side payout).  Absorb silently.
			uc.log.Warn().
				Str("object_id", event.ProviderPayoutID).
				Str("event", event.RawEvent).
				Msg("payout webhook: unknown provider payout id")
			return event, nil
		}
		return event, fmt.Errorf("get payout: %w", err)
	}

	// Provider-mismatch guard: reject a webhook whose payout row was created by
	// a different provider than the one currently active.  Mirrors the same
	// guard in HandleWebhook (payment.go) — without it, a stale ЮKassa
	// notification arriving after a T-Bank migration would mutate a T-Bank
	// payout row with ЮKassa data (status flip, failure reason, ledger
	// reversal) because both providers may share structurally-similar payload
	// formats.
	//
	// In mock mode activeProvider is empty — skip the check to keep tests simple.
	if uc.activeProvider != "" && p.ProviderName != uc.activeProvider {
		uc.log.Warn().
			Str("object_id", event.ProviderPayoutID).
			Str("payout_provider", p.ProviderName).
			Str("active_provider", uc.activeProvider).
			Str("event", event.RawEvent).
			Msg("payout webhook: provider mismatch — ignoring")
		return event, status.Errorf(codes.FailedPrecondition,
			"payout webhook provider mismatch: payout created by %q, active provider is %q",
			p.ProviderName, uc.activeProvider,
		)
	}

	switch event.Status {
	case provider.PayoutStatusSucceeded:
		if _, err := uc.payouts.MarkSucceeded(ctx, p.ID, time.Now()); err != nil {
			return event, fmt.Errorf("mark succeeded: %w", err)
		}
	case provider.PayoutStatusFailed:
		if _, err := uc.payouts.MarkFailedWithReversal(ctx, p.ID, event.FailureReason, time.Now()); err != nil {
			return event, fmt.Errorf("mark failed: %w", err)
		}
	}
	return event, nil
}

// schedulerHandle is a small helper that owns the goroutine launched by
// StartSchedulerInBackground so the caller can Stop it cleanly.
//
// The struct lives at the package level so the main wiring can pass it
// through dependency injection without leaking the goroutine boilerplate
// into business code.
type schedulerHandle struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// StartSchedulerInBackground launches RunScheduler in a goroutine and returns
// a stop function.  The stop function cancels the context and waits for the
// scheduler goroutine to exit, guaranteeing no leaked goroutines on shutdown.
//
// Each call to StartSchedulerInBackground spawns a separate goroutine — this
// is intentionally not guarded; the caller (cmd/main.go) is responsible for
// invoking it exactly once.
func (uc *PayoutUseCase) StartSchedulerInBackground(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	h := &schedulerHandle{cancel: cancel, done: make(chan struct{})}

	go func() {
		defer close(h.done)
		if err := uc.RunScheduler(ctx); err != nil && !errors.Is(err, context.Canceled) {
			uc.log.Error().Err(err).Msg("payout scheduler exited with error")
		}
	}()

	return func() {
		h.cancel()
		<-h.done
	}
}
