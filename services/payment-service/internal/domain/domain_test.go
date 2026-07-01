package domain

import "testing"

func TestPaymentStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		s    PaymentStatus
		want bool
	}{
		{StatusPending, false},
		{StatusSucceeded, true},
		{StatusCancelled, true},
		{StatusRefunded, true},
		{PaymentStatus("bogus"), false},
	}
	for _, tt := range tests {
		if got := tt.s.IsTerminal(); got != tt.want {
			t.Errorf("PaymentStatus(%q).IsTerminal() = %v, want %v", tt.s, got, tt.want)
		}
	}
	if StatusSucceeded.String() != "succeeded" {
		t.Errorf("String() = %q, want succeeded", StatusSucceeded.String())
	}
}

func TestPayoutStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		s    PayoutStatus
		want bool
	}{
		{PayoutPending, false},
		{PayoutProcessing, false},
		{PayoutSucceeded, true},
		{PayoutFailed, true},
	}
	for _, tt := range tests {
		if got := tt.s.IsTerminal(); got != tt.want {
			t.Errorf("PayoutStatus(%q).IsTerminal() = %v, want %v", tt.s, got, tt.want)
		}
	}
	if PayoutProcessing.String() != "processing" {
		t.Errorf("String() = %q, want processing", PayoutProcessing.String())
	}
}

func TestPartnerType_Valid(t *testing.T) {
	for _, tt := range []struct {
		p    PartnerType
		want bool
	}{
		{PartnerVenue, true},
		{PartnerMaster, true},
		{PartnerType(""), false},
		{PartnerType("user"), false},
	} {
		if got := tt.p.Valid(); got != tt.want {
			t.Errorf("PartnerType(%q).Valid() = %v, want %v", tt.p, got, tt.want)
		}
	}
}

func TestPayoutMethodKind_Valid(t *testing.T) {
	for _, tt := range []struct {
		k    PayoutMethodKind
		want bool
	}{
		{PayoutMethodCard, true},
		{PayoutMethodBankAccount, true},
		{PayoutMethodSBP, true},
		{PayoutMethodKind(""), false},
		{PayoutMethodKind("crypto"), false},
	} {
		if got := tt.k.Valid(); got != tt.want {
			t.Errorf("PayoutMethodKind(%q).Valid() = %v, want %v", tt.k, got, tt.want)
		}
	}
}

func TestPayoutMethod_Display(t *testing.T) {
	tests := []struct {
		name string
		m    PayoutMethod
		want string
	}{
		{"card", PayoutMethod{Kind: PayoutMethodCard, CardLast4: "1234"}, "•••• 1234"},
		{"bank with full account", PayoutMethod{Kind: PayoutMethodBankAccount, BankAccount: "40817810099910004312"}, "ACC •••4312"},
		{"bank short account", PayoutMethod{Kind: PayoutMethodBankAccount, BankAccount: "12"}, "ACC ••••"},
		{"sbp with phone", PayoutMethod{Kind: PayoutMethodSBP, SBPPhone: "+79991234567"}, "СБП •••4567"},
		{"sbp short phone", PayoutMethod{Kind: PayoutMethodSBP, SBPPhone: "12"}, "СБП ••••"},
		{"unknown kind falls back to raw", PayoutMethod{Kind: PayoutMethodKind("crypto")}, "crypto"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.Display(); got != tt.want {
				t.Errorf("Display() = %q, want %q", got, tt.want)
			}
		})
	}
}
