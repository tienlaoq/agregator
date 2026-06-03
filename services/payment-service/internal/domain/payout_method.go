package domain

import "time"

// PayoutMethodKind identifies the rail used to deliver money to the partner.
type PayoutMethodKind string

const (
	PayoutMethodCard        PayoutMethodKind = "card"
	PayoutMethodBankAccount PayoutMethodKind = "bank_account"
	PayoutMethodSBP         PayoutMethodKind = "sbp"
)

// PartnerType is the type of the money recipient.  Mirrors Payment.CounterpartyType.
type PartnerType string

const (
	PartnerVenue  PartnerType = "venue"
	PartnerMaster PartnerType = "master"
)

func (p PartnerType) Valid() bool {
	return p == PartnerVenue || p == PartnerMaster
}

func (k PayoutMethodKind) Valid() bool {
	switch k {
	case PayoutMethodCard, PayoutMethodBankAccount, PayoutMethodSBP:
		return true
	}
	return false
}

// PayoutMethod is a partner's bank rails for receiving payouts.
//
// Exactly one PayoutMethod per partner is active at a time (DeactivatedAt is
// zero); deactivated rows are kept for audit so historical payouts remain
// joinable.
//
// PCI:  for card payouts we never store the full PAN.  ProviderToken is an
// opaque, replay-safe handle issued by the provider that the provider
// exchanges back into routable card data at payout time.  CardLast4 is shown
// in the UI only.
type PayoutMethod struct {
	ID            string
	PartnerType   PartnerType
	PartnerID     string
	Kind          PayoutMethodKind
	ProviderName  string // which payout provider issued ProviderToken; required for card
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeactivatedAt time.Time

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

// Display returns a short user-safe label for the method.
func (m *PayoutMethod) Display() string {
	switch m.Kind {
	case PayoutMethodCard:
		return "•••• " + m.CardLast4
	case PayoutMethodBankAccount:
		if len(m.BankAccount) >= 4 {
			return "ACC •••" + m.BankAccount[len(m.BankAccount)-4:]
		}
		return "ACC ••••"
	case PayoutMethodSBP:
		if len(m.SBPPhone) >= 4 {
			return "СБП •••" + m.SBPPhone[len(m.SBPPhone)-4:]
		}
		return "СБП ••••"
	}
	return string(m.Kind)
}
