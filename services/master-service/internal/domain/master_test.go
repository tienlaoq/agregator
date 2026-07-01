package domain

import (
	"errors"
	"testing"
)

func TestValidateINN(t *testing.T) {
	tests := []struct {
		name string
		inn  string
		want bool
	}{
		{"valid 10-digit (Sberbank)", "7707083893", true},
		{"valid 10-digit", "7830002293", true},
		{"valid 12-digit individual", "500100732259", true},
		{"valid 10-digit with formatting", "7707-083 893", true},
		{"invalid 10-digit checksum", "7707083894", false},
		{"invalid 12-digit checksum", "771234567890", false},
		{"all zeros rejected", "000000000000", false},
		{"all zeros 10-digit rejected", "0000000000", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateINN(tt.inn); got != tt.want {
				t.Errorf("ValidateINN(%q) = %v, want %v", tt.inn, got, tt.want)
			}
		})
	}
}

func TestValidateINN_PanicsOnWrongLength(t *testing.T) {
	for _, raw := range []string{"123", "123456789", "12345678901"} {
		t.Run(raw, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("ValidateINN(%q) expected panic on invalid length", raw)
				}
			}()
			ValidateINN(raw)
		})
	}
}

func TestNormalizePayoutLegalForm(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"IP", PayoutLegalFormIP},
		{"  ooo ", PayoutLegalFormOOO},
		{"gph", PayoutLegalFormIndividual}, // legacy alias
		{"GPH", PayoutLegalFormIndividual},
		{"self_employed", PayoutLegalFormSelfEmployed},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := NormalizePayoutLegalForm(tt.in); got != tt.want {
				t.Errorf("NormalizePayoutLegalForm(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// validIPMaster returns a Master with a fully valid ИП payout profile. Tests
// mutate one field at a time to exercise each validation branch.
func validIPMaster() *Master {
	return &Master{
		PayoutLegalForm:            PayoutLegalFormIP,
		PayoutLegalName:            "ИП Иванов Иван Иванович",
		PayoutBankName:             "Тинькофф Банк",
		PayoutBIK:                  "044525974",
		PayoutSettlementAccount:    "40802810500000000001",
		PayoutCorrespondentAccount: "30101810145250000974",
		PayoutINN:                  "500100732259", // valid 12-digit
		PayoutOGRNIP:               "304500116000157",
	}
}

func validOOOMaster() *Master {
	return &Master{
		PayoutLegalForm:            PayoutLegalFormOOO,
		PayoutLegalName:            "ООО Ромашка",
		PayoutBankName:             "Сбербанк",
		PayoutBIK:                  "044525225",
		PayoutSettlementAccount:    "40702810400000000123",
		PayoutCorrespondentAccount: "30101810400000000225",
		PayoutINN:                  "7707083893", // valid 10-digit
		PayoutKPP:                  "770701001",
		PayoutOGRN:                 "1027700132195",
	}
}

func TestValidatePayoutProfile_HappyPaths(t *testing.T) {
	if err := validIPMaster().ValidatePayoutProfile(); err != nil {
		t.Errorf("valid ИП profile rejected: %v", err)
	}
	if err := validOOOMaster().ValidatePayoutProfile(); err != nil {
		t.Errorf("valid ООО profile rejected: %v", err)
	}

	// Self-employed: same requirements as ИП minus OGRNIP.
	se := validIPMaster()
	se.PayoutLegalForm = PayoutLegalFormSelfEmployed
	se.PayoutOGRNIP = ""
	if err := se.ValidatePayoutProfile(); err != nil {
		t.Errorf("valid self-employed profile rejected: %v", err)
	}
}

func TestValidatePayoutProfile_FieldErrors(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(m *Master)
		wantField string
	}{
		{"unknown legal form", func(m *Master) { m.PayoutLegalForm = "llc" }, "legal_form"},
		{"missing legal name", func(m *Master) { m.PayoutLegalName = "  " }, "legal_name"},
		// Bank rails (bank name, BIK, settlement / correspondent account) are no
		// longer validated here — they live in payment-service payout methods
		// (the "Финансы" tab), not on the profile. See ValidatePayoutProfile.
		{"IP wrong inn length", func(m *Master) { m.PayoutINN = "7707083893" }, "inn"},
		{"IP bad inn checksum", func(m *Master) { m.PayoutINN = "500100732250" }, "inn"},
		{"IP missing ogrnip", func(m *Master) { m.PayoutOGRNIP = "123" }, "ogrnip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validIPMaster()
			tt.mutate(m)
			err := m.ValidatePayoutProfile()
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
			var pve *PayoutValidationError
			if !errors.As(err, &pve) {
				t.Fatalf("expected *PayoutValidationError, got %T", err)
			}
			if pve.Field != tt.wantField {
				t.Errorf("error field = %q, want %q (msg: %s)", pve.Field, tt.wantField, pve.Message)
			}
		})
	}
}

func TestValidatePayoutProfile_OOOFieldErrors(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(m *Master)
		wantField string
	}{
		{"OOO wrong inn length", func(m *Master) { m.PayoutINN = "500100732259" }, "inn"},
		{"OOO bad inn checksum", func(m *Master) { m.PayoutINN = "7707083894" }, "inn"},
		{"OOO short kpp", func(m *Master) { m.PayoutKPP = "123" }, "kpp"},
		{"OOO short ogrn", func(m *Master) { m.PayoutOGRN = "123" }, "ogrn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validOOOMaster()
			tt.mutate(m)
			err := m.ValidatePayoutProfile()
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
			var pve *PayoutValidationError
			if !errors.As(err, &pve) || pve.Field != tt.wantField {
				t.Fatalf("want field %q, got %v", tt.wantField, err)
			}
		})
	}
}

func TestPayoutProfileReady(t *testing.T) {
	if (*Master)(nil).PayoutProfileReady() {
		t.Error("nil master must not be payout-ready")
	}
	if !validIPMaster().PayoutProfileReady() {
		t.Error("valid ИП master must be payout-ready")
	}
	incomplete := &Master{PayoutLegalForm: PayoutLegalFormIP}
	if incomplete.PayoutProfileReady() {
		t.Error("master missing payout fields must not be payout-ready")
	}
}
