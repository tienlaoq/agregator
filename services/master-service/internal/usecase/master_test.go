package usecase

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

func Test_normalizeMasterPhotoURL(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	other := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		// ── valid URLs ────────────────────────────────────────────────────────
		{
			name: "disk uploader url",
			url:  "/api/v1/uploads/masters/11111111-1111-1111-1111-111111111111/photo.jpg",
		},
		{
			name: "minio cdn url",
			url:  "https://cdn.example.com/photos/masters/11111111-1111-1111-1111-111111111111/abc123.jpg",
		},
		{
			name: "filename with dash and underscore",
			url:  "/api/v1/uploads/masters/11111111-1111-1111-1111-111111111111/my_photo-1.jpeg",
		},
		// ── empty / missing ───────────────────────────────────────────────────
		{name: "empty url", url: "", wantErr: true},
		{name: "no masters segment", url: "/api/v1/uploads/venues/abc/photo.jpg", wantErr: true},
		// ── wrong master ─────────────────────────────────────────────────────
		{
			name:    "different master id",
			url:     "/api/v1/uploads/masters/22222222-2222-2222-2222-222222222222/photo.jpg",
			wantErr: true,
		},
		// ── classic path traversal ────────────────────────────────────────────
		{
			name:    "dot-dot in path",
			url:     "/api/v1/uploads/masters/11111111-1111-1111-1111-111111111111/../../etc/passwd",
			wantErr: true,
		},
		// ── percent-encoded traversal ─────────────────────────────────────────
		{
			name:    "percent-encoded dot-dot %2e%2e",
			url:     "/api/v1/uploads/masters/11111111-1111-1111-1111-111111111111/%2e%2e/secret",
			wantErr: true,
		},
		{
			name:    "mixed percent encoding ..%2f",
			url:     "/api/v1/uploads/masters/11111111-1111-1111-1111-111111111111/..%2fetc/passwd",
			wantErr: true,
		},
		{
			name:    "uppercase percent encoding %2E%2E",
			url:     "/api/v1/uploads/masters/11111111-1111-1111-1111-111111111111/%2E%2E/secret",
			wantErr: true,
		},
		// ── sub-directory in filename (extra slash) ───────────────────────────
		{
			name:    "extra path segment after filename",
			url:     "/api/v1/uploads/masters/11111111-1111-1111-1111-111111111111/dir/photo.jpg",
			wantErr: true,
		},
		// ── forbidden characters in filename ─────────────────────────────────
		{
			name:    "percent sign in filename",
			url:     "/api/v1/uploads/masters/11111111-1111-1111-1111-111111111111/photo%20name.jpg",
			wantErr: true,
		},
		{
			name:    "backslash in url",
			url:     `/api/v1/uploads/masters/11111111-1111-1111-1111-111111111111/..\..\secret`,
			wantErr: true,
		},
		// ── invalid percent encoding ──────────────────────────────────────────
		{
			name:    "malformed percent encoding",
			url:     "/api/v1/uploads/masters/11111111-1111-1111-1111-111111111111/photo%ZZ.jpg",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeMasterPhotoURL(id, tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeMasterPhotoURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}

	// Sanity-check: other master's ID must always fail regardless of URL shape.
	t.Run("other master id always fails", func(t *testing.T) {
		u := "/api/v1/uploads/masters/22222222-2222-2222-2222-222222222222/photo.jpg"
		if _, err := normalizeMasterPhotoURL(other, u); err != nil {
			t.Fatal("expected no error for own master path, got:", err)
		}
		if _, err := normalizeMasterPhotoURL(id, u); err == nil {
			t.Fatal("expected error for other master path, got nil")
		}
	})
}

func Test_extractMasterPhotoKey(t *testing.T) {
	tests := []struct {
		rawURL  string
		wantKey string
		wantOK  bool
	}{
		{
			rawURL:  "/api/v1/uploads/masters/abc/photo.jpg",
			wantKey: "masters/abc/photo.jpg",
			wantOK:  true,
		},
		{
			rawURL:  "https://cdn.example.com/photos/masters/abc/photo.jpg",
			wantKey: "masters/abc/photo.jpg",
			wantOK:  true,
		},
		{
			rawURL: "/api/v1/uploads/venues/abc/photo.jpg",
			wantOK: false,
		},
		{
			rawURL: "",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.rawURL, func(t *testing.T) {
			got, ok := extractMasterPhotoKey(tt.rawURL)
			if ok != tt.wantOK {
				t.Fatalf("extractMasterPhotoKey(%q) ok = %v, want %v", tt.rawURL, ok, tt.wantOK)
			}
			if ok && got != tt.wantKey {
				t.Fatalf("extractMasterPhotoKey(%q) key = %q, want %q", tt.rawURL, got, tt.wantKey)
			}
		})
	}
}

// validINN12 is a real 12-digit INN with a correct ФНС mod-11 checksum.
// Used in ValidatePayoutProfile tests that must pass the checksum guard.
// Source: publicly known test value from ФНС documentation.
const validINN12 = "500100732259"

// validINN10 is a real 10-digit INN (organisation) with a correct checksum.
// ИНН ФНС России 7707329152, verified: sum=178, 178%11=2, control digit=2.
const validINN10 = "7707329152"

func Test_validateINN(t *testing.T) {
	tests := []struct {
		name  string
		inn   string
		valid bool
	}{
		// ── valid INNs ────────────────────────────────────────────────────────────
		{name: "valid 10-digit org INN", inn: validINN10, valid: true},
		{name: "valid 12-digit individual INN", inn: validINN12, valid: true},
		{name: "valid 12-digit with spaces", inn: "5001 0073 2259", valid: true},
		// ── wrong checksum ────────────────────────────────────────────────────────
		{name: "wrong last digit in 10-digit", inn: "7707329153", valid: false},
		{name: "wrong 11th digit in 12-digit", inn: "500100732169", valid: false},
		{name: "wrong 12th digit in 12-digit", inn: "500100732258", valid: false},
		{name: "all-zeros 10-digit", inn: "0000000000", valid: false},
		{name: "all-zeros 12-digit", inn: "000000000000", valid: false},
		{name: "sequential 10-digit", inn: "1234567890", valid: false},
		{name: "sequential 12-digit", inn: "123456789012", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.ValidateINN(tt.inn); got != tt.valid {
				t.Fatalf("validateINN(%q) = %v, want %v", tt.inn, got, tt.valid)
			}
		})
	}
}

func Test_ValidatePayoutProfile(t *testing.T) {
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
		errField  string
	}{
		// ── valid profiles ────────────────────────────────────────────────────────
		{name: "self employed valid", legalForm: domain.PayoutLegalFormSelfEmployed, inn: validINN12},
		{name: "individual valid", legalForm: domain.PayoutLegalFormIndividual, inn: validINN12},
		{name: "ip valid", legalForm: domain.PayoutLegalFormIP, inn: validINN12, ogrnip: "123456789012345"},
		{name: "ooo valid", legalForm: domain.PayoutLegalFormOOO, inn: validINN10, kpp: "123456789", ogrn: "1234567890123"},
		// ── length errors ─────────────────────────────────────────────────────────
		{name: "ooo missing kpp", legalForm: domain.PayoutLegalFormOOO, inn: validINN10, ogrn: "1234567890123", wantErr: true, errField: "kpp"},
		{name: "ip invalid ogrnip length", legalForm: domain.PayoutLegalFormIP, inn: validINN12, ogrnip: "123", wantErr: true, errField: "ogrnip"},
		{name: "self_employed inn wrong length", legalForm: domain.PayoutLegalFormSelfEmployed, inn: "12345678", wantErr: true, errField: "inn"},
		// ── checksum errors ───────────────────────────────────────────────────────
		{name: "self_employed inn bad checksum", legalForm: domain.PayoutLegalFormSelfEmployed, inn: "123456789012", wantErr: true, errField: "inn"},
		{name: "individual inn bad checksum", legalForm: domain.PayoutLegalFormIndividual, inn: "123456789012", wantErr: true, errField: "inn"},
		{name: "ip inn bad checksum", legalForm: domain.PayoutLegalFormIP, inn: "123456789012", ogrnip: "123456789012345", wantErr: true, errField: "inn"},
		{name: "ooo inn bad checksum", legalForm: domain.PayoutLegalFormOOO, inn: "1234567890", kpp: "123456789", ogrn: "1234567890123", wantErr: true, errField: "inn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := *base
			m.PayoutLegalForm = tt.legalForm
			m.PayoutINN = tt.inn
			m.PayoutKPP = tt.kpp
			m.PayoutOGRN = tt.ogrn
			m.PayoutOGRNIP = tt.ogrnip
			err := m.ValidatePayoutProfile()
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePayoutProfile() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errField != "" {
				var pve *domain.PayoutValidationError
				if !errors.As(err, &pve) {
					t.Fatalf("expected *domain.PayoutValidationError, got %T", err)
				}
				if pve.Field != tt.errField {
					t.Fatalf("PayoutValidationError.Field = %q, want %q", pve.Field, tt.errField)
				}
			}
		})
	}
}

func Test_validateBookingSlot(t *testing.T) {
	// Derive test dates dynamically so the suite does not rot as time passes.
	// Moscow time is UTC+3 (no DST); mirror the same offset used in validateBookingSlot.
	const moscowOffset = 3 * time.Hour
	nowMoscow := time.Now().UTC().Add(moscowOffset)
	today := time.Date(nowMoscow.Year(), nowMoscow.Month(), nowMoscow.Day(), 0, 0, 0, 0, time.UTC)

	fmtDate := func(t time.Time) string { return t.Format(time.DateOnly) }

	todayStr := fmtDate(today)
	tomorrowStr := fmtDate(today.AddDate(0, 0, 1))
	maxStr := fmtDate(today.AddDate(0, 0, bookingMaxAdvanceDays))
	beyondMaxStr := fmtDate(today.AddDate(0, 0, bookingMaxAdvanceDays+1))
	yesterdayStr := fmtDate(today.AddDate(0, 0, -1))
	farPastStr := "2020-01-01"

	tests := []struct {
		name     string
		date     string
		timeFrom string
		timeTo   string
		wantErr  bool
	}{
		// ── valid dates ───────────────────────────────────────────────────────────
		{name: "today", date: todayStr, timeFrom: "10:00", timeTo: "11:30"},
		{name: "tomorrow", date: tomorrowStr, timeFrom: "10:00", timeTo: "11:30"},
		{name: "max advance date", date: maxStr, timeFrom: "10:00", timeTo: "11:30"},
		// ── past / future window errors ───────────────────────────────────────────
		{name: "yesterday", date: yesterdayStr, timeFrom: "10:00", timeTo: "11:00", wantErr: true},
		{name: "far past", date: farPastStr, timeFrom: "10:00", timeTo: "11:00", wantErr: true},
		{name: "beyond max advance", date: beyondMaxStr, timeFrom: "10:00", timeTo: "11:00", wantErr: true},
		// ── format / parse errors ─────────────────────────────────────────────────
		{name: "empty date", date: "", timeFrom: "10:00", timeTo: "11:00", wantErr: true},
		{name: "wrong date format", date: "01/06/2026", timeFrom: "10:00", timeTo: "11:00", wantErr: true},
		{name: "invalid date value", date: "2026-13-01", timeFrom: "10:00", timeTo: "11:00", wantErr: true},
		{name: "empty time_from", date: tomorrowStr, timeFrom: "", timeTo: "11:00", wantErr: true},
		{name: "invalid time_from", date: tomorrowStr, timeFrom: "99:99", timeTo: "11:00", wantErr: true},
		{name: "empty time_to", date: tomorrowStr, timeFrom: "10:00", timeTo: "", wantErr: true},
		{name: "invalid time_to", date: tomorrowStr, timeFrom: "10:00", timeTo: "25:00", wantErr: true},
		// ── time ordering errors ─────────────────────────────────────────────────
		{name: "time_to equals time_from", date: tomorrowStr, timeFrom: "10:00", timeTo: "10:00", wantErr: true},
		{name: "time_to before time_from", date: tomorrowStr, timeFrom: "11:00", timeTo: "10:00", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBookingSlot(tt.date, tt.timeFrom, tt.timeTo)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateBookingSlot(%q, %q, %q) error = %v, wantErr %v",
					tt.date, tt.timeFrom, tt.timeTo, err, tt.wantErr)
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

func TestMasterBookingIdempotencyKey(t *testing.T) {
	t.Parallel()

	bookingA := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	bookingB := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	userA := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	userB := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	t.Run("deterministic — same inputs produce same key", func(t *testing.T) {
		k1 := masterBookingIdempotencyKey(bookingA, userA)
		k2 := masterBookingIdempotencyKey(bookingA, userA)
		if k1 != k2 {
			t.Fatalf("expected same key, got %q vs %q", k1, k2)
		}
	})

	t.Run("different booking IDs produce different keys", func(t *testing.T) {
		k1 := masterBookingIdempotencyKey(bookingA, userA)
		k2 := masterBookingIdempotencyKey(bookingB, userA)
		if k1 == k2 {
			t.Fatal("expected different keys for different booking IDs")
		}
	})

	t.Run("different user IDs produce different keys", func(t *testing.T) {
		k1 := masterBookingIdempotencyKey(bookingA, userA)
		k2 := masterBookingIdempotencyKey(bookingA, userB)
		if k1 == k2 {
			t.Fatal("expected different keys for different user IDs")
		}
	})

	t.Run("key is 64 hex chars (sha256)", func(t *testing.T) {
		k := masterBookingIdempotencyKey(bookingA, userA)
		if len(k) != 64 {
			t.Fatalf("expected 64 hex chars, got %d: %q", len(k), k)
		}
	})

	t.Run("no key collision between swapped IDs", func(t *testing.T) {
		// sha256("booking:user") != sha256("user:booking") — separator preserves ordering
		k1 := masterBookingIdempotencyKey(bookingA, bookingB)
		k2 := masterBookingIdempotencyKey(bookingB, bookingA)
		if k1 == k2 {
			t.Fatal("expected different keys when booking/user IDs are swapped")
		}
	})
}
