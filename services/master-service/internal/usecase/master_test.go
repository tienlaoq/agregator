package usecase

import (
	"testing"

	"github.com/google/uuid"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

func Test_validatePayoutProfileByLegalForm(t *testing.T) {
	base := &domain.Master{
		PayoutLegalName:            "Иванов И.И.",
		PayoutBankName:             "Т-Банк",
		PayoutBIK:                  "044525974",
		PayoutSettlementAccount:    "40702810123456789012",
		PayoutCorrespondentAccount: "30101810400000000974",
		YookassaSellerAccountID:    "12345",
	}
	tests := []struct {
		name      string
		legalForm string
		inn       string
		kpp       string
		ogrn      string
		ogrnip    string
		wantErr   bool
	}{
		{name: "self employed valid", legalForm: domain.PayoutLegalFormSelfEmployed, inn: "123456789012"},
		{name: "individual valid", legalForm: domain.PayoutLegalFormIndividual, inn: "123456789012"},
		{name: "ip valid", legalForm: domain.PayoutLegalFormIP, inn: "123456789012", ogrnip: "123456789012345"},
		{name: "ooo valid", legalForm: domain.PayoutLegalFormOOO, inn: "1234567890", kpp: "123456789", ogrn: "1234567890123"},
		{name: "ooo missing kpp", legalForm: domain.PayoutLegalFormOOO, inn: "1234567890", ogrn: "1234567890123", wantErr: true},
		{name: "ip invalid ogrnip", legalForm: domain.PayoutLegalFormIP, inn: "123456789012", ogrnip: "123", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := *base
			m.PayoutLegalForm = tt.legalForm
			m.PayoutINN = tt.inn
			m.PayoutKPP = tt.kpp
			m.PayoutOGRN = tt.ogrn
			m.PayoutOGRNIP = tt.ogrnip
			err := validatePayoutProfileByLegalForm(&m)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validatePayoutProfileByLegalForm() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_estimateMasterBookingPriceKopecks(t *testing.T) {
	m := &domain.Master{
		ID:         uuid.New(),
		HourlyRate: 10000,
		Services: []domain.MasterService{
			{ID: uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"), Price: 15000},
		},
	}
	t.Run("by service", func(t *testing.T) {
		id := m.Services[0].ID
		got, err := estimateMasterBookingPriceKopecks(m, &id, "10:00", "11:00")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 15000 {
			t.Fatalf("got %d, want 15000", got)
		}
	})
	t.Run("by hourly duration rounded up", func(t *testing.T) {
		got, err := estimateMasterBookingPriceKopecks(m, nil, "10:00", "11:30")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 15000 {
			t.Fatalf("got %d, want 15000", got)
		}
	})
}
