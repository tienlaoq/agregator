package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

// PayoutMethodRepo persists partner payout rails.
//
// Save semantics: one transaction marks any prior active method as
// deactivated_at=now() and inserts the new row. The partial unique index
// (partner_payout_methods_active_uniq) is the authoritative serialization
// boundary — two concurrent Save calls for the same partner will fight at
// commit and the loser will retry; we never observe two active rows.
type PayoutMethodRepo struct {
	pool *pgxpool.Pool
}

func NewPayoutMethodRepo(pool *pgxpool.Pool) *PayoutMethodRepo {
	return &PayoutMethodRepo{pool: pool}
}

const payoutMethodCols = `
	id, partner_type, partner_id, method_kind,
	COALESCE(provider_name, ''),
	COALESCE(card_last4, ''), COALESCE(card_brand, ''), COALESCE(provider_token, ''),
	COALESCE(bank_account_bic, ''), COALESCE(bank_account_number, ''),
	COALESCE(bank_name, ''), COALESCE(recipient_inn, ''),
	COALESCE(recipient_kpp, ''), COALESCE(recipient_name, ''),
	COALESCE(sbp_phone, ''), COALESCE(sbp_bank_id, ''),
	created_at, updated_at, deactivated_at`

func (r *PayoutMethodRepo) Save(ctx context.Context, m *domain.PayoutMethod) error {
	if !m.PartnerType.Valid() {
		return pkgerr.InvalidArgument("invalid partner_type")
	}
	if !m.Kind.Valid() {
		return pkgerr.InvalidArgument("invalid method_kind")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("payout method save begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Deactivate any current active method for the partner. NOOP if there is none.
	if _, err = tx.Exec(ctx, `
		UPDATE partner_payout_methods
		   SET deactivated_at = now()
		 WHERE partner_type = $1 AND partner_id = $2 AND deactivated_at IS NULL
	`, string(m.PartnerType), m.PartnerID); err != nil {
		return fmt.Errorf("payout method deactivate prev: %w", err)
	}

	// Empty-string columns become NULL so the per-kind CHECK constraints
	// see them as "not provided" rather than "empty". Card constraint, for
	// example, fails on empty provider_token; storing NULL keeps the row
	// semantically clean and lets the CHECK do its job.
	row := tx.QueryRow(ctx, `
		INSERT INTO partner_payout_methods (
			partner_type, partner_id, method_kind, provider_name,
			card_last4, card_brand, provider_token,
			bank_account_bic, bank_account_number, bank_name,
			recipient_inn, recipient_kpp, recipient_name,
			sbp_phone, sbp_bank_id
		) VALUES (
			$1, $2, $3, NULLIF($4, ''),
			NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''),
			NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''),
			NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''),
			NULLIF($14, ''), NULLIF($15, '')
		)
		RETURNING id, created_at, updated_at`,
		string(m.PartnerType), m.PartnerID, string(m.Kind), m.ProviderName,
		m.CardLast4, m.CardBrand, m.ProviderToken,
		m.BankBIC, m.BankAccount, m.BankName,
		m.RecipientINN, m.RecipientKPP, m.RecipientName,
		m.SBPPhone, m.SBPBankID,
	)
	if err = row.Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return fmt.Errorf("payout method insert: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("payout method commit: %w", err)
	}
	return nil
}

func (r *PayoutMethodRepo) GetActive(ctx context.Context, partnerType domain.PartnerType, partnerID string) (*domain.PayoutMethod, error) {
	q := `SELECT ` + payoutMethodCols + `
		FROM partner_payout_methods
		WHERE partner_type = $1 AND partner_id = $2 AND deactivated_at IS NULL`
	return r.scanOne(ctx, q, string(partnerType), partnerID)
}

func (r *PayoutMethodRepo) GetByID(ctx context.Context, id string) (*domain.PayoutMethod, error) {
	q := `SELECT ` + payoutMethodCols + ` FROM partner_payout_methods WHERE id = $1`
	return r.scanOne(ctx, q, id)
}

func (r *PayoutMethodRepo) scanOne(ctx context.Context, q string, args ...any) (*domain.PayoutMethod, error) {
	m := &domain.PayoutMethod{}
	var partnerType, kind string
	var deactivatedAt *time.Time
	err := r.pool.QueryRow(ctx, q, args...).Scan(
		&m.ID, &partnerType, &m.PartnerID, &kind,
		&m.ProviderName,
		&m.CardLast4, &m.CardBrand, &m.ProviderToken,
		&m.BankBIC, &m.BankAccount, &m.BankName,
		&m.RecipientINN, &m.RecipientKPP, &m.RecipientName,
		&m.SBPPhone, &m.SBPBankID,
		&m.CreatedAt, &m.UpdatedAt, &deactivatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("payout method not found")
	}
	if err != nil {
		return nil, err
	}
	m.PartnerType = domain.PartnerType(partnerType)
	m.Kind = domain.PayoutMethodKind(kind)
	if deactivatedAt != nil {
		m.DeactivatedAt = *deactivatedAt
	}
	return m, nil
}
