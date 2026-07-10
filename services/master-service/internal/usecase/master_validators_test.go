package usecase

// Focused unit tests for the pure validation / normalisation helpers in
// profile.go. These have no I/O, so they are tested directly without a repo.

import (
	"strings"
	"testing"

	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

func TestNeedsTravelBaseForFormat(t *testing.T) {
	tests := []struct {
		wf   string
		want bool
	}{
		{domain.WorkFormatMobile, true},
		{domain.WorkFormatBoth, true},
		{"  MOBILE  ", true},
		{domain.WorkFormatVenue, false},
		{"", false},
		{"garbage", false},
	}
	for _, tt := range tests {
		if got := needsTravelBaseForFormat(tt.wf); got != tt.want {
			t.Errorf("needsTravelBaseForFormat(%q) = %v, want %v", tt.wf, got, tt.want)
		}
	}
}

func TestClearTravelFieldsForVenue(t *testing.T) {
	lat, lon := 55.75, 37.61
	t.Run("venue clears all travel fields", func(t *testing.T) {
		m := &domain.Master{
			WorkFormat:          domain.WorkFormatVenue,
			TravelBaseLatitude:  &lat,
			TravelBaseLongitude: &lon,
			TravelRadiusKm:      15,
			TravelExcludeZones:  []domain.MasterTravelExcludeZone{{ID: "z1"}},
		}
		clearTravelFieldsForVenue(m)
		if m.TravelBaseLatitude != nil || m.TravelBaseLongitude != nil || m.TravelRadiusKm != 0 || m.TravelExcludeZones != nil {
			t.Fatalf("travel fields not cleared: %+v", m)
		}
	})

	t.Run("mobile keeps travel fields", func(t *testing.T) {
		m := &domain.Master{
			WorkFormat:          domain.WorkFormatMobile,
			TravelBaseLatitude:  &lat,
			TravelBaseLongitude: &lon,
			TravelRadiusKm:      15,
		}
		clearTravelFieldsForVenue(m)
		if m.TravelBaseLatitude == nil || m.TravelRadiusKm != 15 {
			t.Fatal("mobile travel fields must be preserved")
		}
	})
}

func TestValidateTravelBaseForProfile(t *testing.T) {
	lat, lon := 55.75, 37.61
	badLat, badLon := 100.0, 200.0

	tests := []struct {
		name    string
		master  *domain.Master
		wantErr bool
	}{
		{
			name:   "venue format skips validation",
			master: &domain.Master{WorkFormat: domain.WorkFormatVenue},
		},
		{
			name:   "mobile with zero radius is optional",
			master: &domain.Master{WorkFormat: domain.WorkFormatMobile, TravelRadiusKm: 0},
		},
		{
			name:   "mobile with radius and valid coords",
			master: &domain.Master{WorkFormat: domain.WorkFormatMobile, TravelRadiusKm: 10, TravelBaseLatitude: &lat, TravelBaseLongitude: &lon},
		},
		{
			name:    "mobile with radius but missing marker",
			master:  &domain.Master{WorkFormat: domain.WorkFormatMobile, TravelRadiusKm: 10},
			wantErr: true,
		},
		{
			name:    "mobile with out-of-range coords",
			master:  &domain.Master{WorkFormat: domain.WorkFormatBoth, TravelRadiusKm: 10, TravelBaseLatitude: &badLat, TravelBaseLongitude: &badLon},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTravelBaseForProfile(tt.master)
			if (err != nil) != tt.wantErr {
				t.Fatalf("got err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTravelExcludeZones(t *testing.T) {
	tests := []struct {
		name    string
		zones   []domain.MasterTravelExcludeZone
		wantErr bool
	}{
		{name: "empty is valid"},
		{
			name:  "valid zone",
			zones: []domain.MasterTravelExcludeZone{{ID: "z1", Latitude: 55.75, Longitude: 37.61, RadiusKm: 1}},
		},
		{
			name:    "missing id",
			zones:   []domain.MasterTravelExcludeZone{{Latitude: 55.75, Longitude: 37.61}},
			wantErr: true,
		},
		{
			name:    "bad coordinates",
			zones:   []domain.MasterTravelExcludeZone{{ID: "z1", Latitude: 200, Longitude: 500}},
			wantErr: true,
		},
		{
			name:    "radius too small",
			zones:   []domain.MasterTravelExcludeZone{{ID: "z1", Latitude: 55, Longitude: 37, RadiusKm: 0.01}},
			wantErr: true,
		},
		{
			name:    "radius too large",
			zones:   []domain.MasterTravelExcludeZone{{ID: "z1", Latitude: 55, Longitude: 37, RadiusKm: 999}},
			wantErr: true,
		},
		{
			name: "too many zones",
			zones: func() []domain.MasterTravelExcludeZone {
				z := make([]domain.MasterTravelExcludeZone, maxTravelExcludeZones+1)
				for i := range z {
					z[i] = domain.MasterTravelExcludeZone{ID: "z", Latitude: 55, Longitude: 37}
				}
				return z
			}(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTravelExcludeZones(tt.zones)
			if (err != nil) != tt.wantErr {
				t.Fatalf("got err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTravelExcludeZonesInsideTravelRadius(t *testing.T) {
	base := 55.75
	baseLon := 37.61

	t.Run("zone inside radius passes", func(t *testing.T) {
		m := &domain.Master{
			WorkFormat:          domain.WorkFormatMobile,
			TravelBaseLatitude:  &base,
			TravelBaseLongitude: &baseLon,
			TravelRadiusKm:      50,
			TravelExcludeZones:  []domain.MasterTravelExcludeZone{{ID: "z1", Latitude: base, Longitude: baseLon, RadiusKm: 1}},
		}
		if err := validateTravelExcludeZonesInsideTravelRadius(m); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("zone outside radius fails", func(t *testing.T) {
		far := base + 5 // ~555 km north
		m := &domain.Master{
			WorkFormat:          domain.WorkFormatMobile,
			TravelBaseLatitude:  &base,
			TravelBaseLongitude: &baseLon,
			TravelRadiusKm:      10,
			TravelExcludeZones:  []domain.MasterTravelExcludeZone{{ID: "z1", Latitude: far, Longitude: baseLon, RadiusKm: 1}},
		}
		if err := validateTravelExcludeZonesInsideTravelRadius(m); err == nil {
			t.Fatal("expected error for zone outside travel radius")
		}
	})

	t.Run("no marker skips check", func(t *testing.T) {
		m := &domain.Master{
			WorkFormat:         domain.WorkFormatMobile,
			TravelRadiusKm:     10,
			TravelExcludeZones: []domain.MasterTravelExcludeZone{{ID: "z1", Latitude: 80, Longitude: 80, RadiusKm: 1}},
		}
		if err := validateTravelExcludeZonesInsideTravelRadius(m); err != nil {
			t.Fatalf("expected nil when marker missing, got %v", err)
		}
	})
}

func TestValidateReadyForReview(t *testing.T) {
	// base is a fully-valid submittable master.
	base := func() *domain.Master {
		m := activeMaster()
		m.DisplayName = "Иван Банщик"
		m.City = "москва"
		m.Phone = "79991234567"
		m.Bio = "Опытный банщик с большим стажем работы"
		return m
	}

	t.Run("valid profile passes", func(t *testing.T) {
		if err := validateReadyForReview(base()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	mutations := []struct {
		name   string
		mutate func(*domain.Master)
	}{
		{"missing display name", func(m *domain.Master) { m.DisplayName = "  " }},
		{"missing city", func(m *domain.Master) { m.City = "" }},
		{"missing phone", func(m *domain.Master) { m.Phone = "" }},
		{"short bio", func(m *domain.Master) { m.Bio = "коротко" }},
		{"no services", func(m *domain.Master) { m.Services = nil }},
		{"invalid payout", func(m *domain.Master) { m.PayoutINN = "123" }},
	}
	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			m := base()
			tt.mutate(m)
			if err := validateReadyForReview(m); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNormalizeDigits(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"  40702810123456789012 ", "40702810123456789012"},
		{"7 (999) 123-45-67", "79991234567"},
		{"abc", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeDigits(tt.in); got != tt.want {
			t.Errorf("normalizeDigits(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHasAnyPayoutData(t *testing.T) {
	t.Run("empty master has no payout data", func(t *testing.T) {
		if hasAnyPayoutData(&domain.Master{}) {
			t.Fatal("expected false for empty master")
		}
	})

	fields := []func(*domain.Master){
		func(m *domain.Master) { m.PayoutLegalForm = domain.PayoutLegalFormSelfEmployed },
		func(m *domain.Master) { m.PayoutLegalName = "Тест" },
		func(m *domain.Master) { m.PayoutINN = "500100732259" },
		func(m *domain.Master) { m.PayoutBankName = "Т-Банк" },
		func(m *domain.Master) { m.PayoutBIK = "044525974" },
		func(m *domain.Master) { m.PayoutSettlementAccount = "40702810123456789012" },
	}
	for i, set := range fields {
		m := &domain.Master{}
		set(m)
		if !hasAnyPayoutData(m) {
			t.Errorf("field #%d: expected hasAnyPayoutData true", i)
		}
	}

	t.Run("whitespace-only is not data", func(t *testing.T) {
		if hasAnyPayoutData(&domain.Master{PayoutLegalName: strings.Repeat(" ", 5)}) {
			t.Fatal("whitespace-only field should not count as payout data")
		}
	})
}
