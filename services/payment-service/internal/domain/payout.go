package domain

import "time"

// PayoutStatus is the lifecycle state of an outbound payout.
//
//	pending    — row created, ledger debit written in the same TX, provider
//	             not yet called.
//	processing — provider accepted the request, awaiting async outcome.
//	succeeded  — terminal: provider confirmed funds transferred.
//	failed     — terminal: provider rejected or final failure after retries.
//	             Caller must write a reversal ledger entry to restore balance.
type PayoutStatus string

const (
	PayoutPending    PayoutStatus = "pending"
	PayoutProcessing PayoutStatus = "processing"
	PayoutSucceeded  PayoutStatus = "succeeded"
	PayoutFailed     PayoutStatus = "failed"
)

func (s PayoutStatus) IsTerminal() bool {
	return s == PayoutSucceeded || s == PayoutFailed
}

func (s PayoutStatus) String() string { return string(s) }

// Payout is one outbound money movement attempt for a partner.
//
// MethodKindSnapshot and MethodDisplay are frozen at creation time so the
// historical row reads correctly even after the partner replaces or
// deactivates the underlying PayoutMethod.
type Payout struct {
	ID                 string
	PartnerType        PartnerType
	PartnerID          string
	AmountKopecks      int64
	Currency           string
	MethodID           string
	MethodKindSnapshot PayoutMethodKind
	MethodDisplay      string
	Status             PayoutStatus
	ProviderName       string
	ProviderPayoutID   string
	IdempotencyKey     string
	FailureReason      string
	AttemptCount       int
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CompletedAt        time.Time
}
