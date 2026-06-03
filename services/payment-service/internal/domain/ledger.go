package domain

import "time"

// LedgerEntryType classifies a partner_ledger row.
//
// See the migration comments for the full state-machine; in short:
//
//	accrual   — +amount, owed to partner on payment success.
//	reversal  — −amount, undoes an accrual after a refund.
//	payout    — −amount, money sent to partner via a payout.
//	adjustment — any sign, manual correction; requires reason.
type LedgerEntryType string

const (
	LedgerAccrual    LedgerEntryType = "accrual"
	LedgerReversal   LedgerEntryType = "reversal"
	LedgerPayout     LedgerEntryType = "payout"
	LedgerAdjustment LedgerEntryType = "adjustment"
)

// LedgerEntry is one row in the append-only partner ledger.
//
// The ledger is the single source of truth for "how much do we owe partner X".
// Balance = SUM(amount_kopecks) WHERE partner_* = X.
// Available balance excludes accruals whose available_at is in the future.
type LedgerEntry struct {
	ID              int64
	PartnerType     PartnerType
	PartnerID       string
	EntryType       LedgerEntryType
	AmountKopecks   int64
	PaymentID       string
	PayoutID        string
	ReversesEntryID int64
	AvailableAt     time.Time
	Reason          string
	CreatedAt       time.Time
}

// PartnerBalance is the result of the SUM-based balance lookup.
//
// Total = SUM(amount) across every ledger row for the partner. Includes held
// accruals; pending-payout debits are already netted out because they are
// stored as negative ledger entries the moment a payout is created.
//
// Available = SUM(amount) excluding accruals whose available_at > now().
// This is the only number the payout scheduler looks at.
//
// Held = Total − Available. It is exactly the sum of accruals still in their
// hold window — money owed but not yet released for payout.
type PartnerBalance struct {
	PartnerType      PartnerType
	PartnerID        string
	TotalKopecks     int64
	AvailableKopecks int64
	HeldKopecks      int64
	LastEntryAt      time.Time
}
