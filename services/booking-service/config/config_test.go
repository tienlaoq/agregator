package config

import "testing"

func TestLoad_Defaults(t *testing.T) {
	for _, k := range []string{
		"GRPC_PORT", "PG_DB", "NATS_URL", "VENUE_SERVICE_ADDR", "PAYMENT_SERVICE_ADDR",
		"CRM_SERVICE_ADDR", "BOOKING_VISIT_TIMEZONE", "CANCEL_DEADLINE_HOURS",
		"CURSOR_HMAC_KEY", "INTERNAL_SERVICE_TOKEN",
	} {
		t.Setenv(k, "")
	}

	cfg := Load()

	if cfg.GRPCPort != "50054" {
		t.Errorf("GRPCPort = %q, want 50054", cfg.GRPCPort)
	}
	if cfg.Postgres.DBName != "booking_db" {
		t.Errorf("Postgres.DBName = %q, want booking_db", cfg.Postgres.DBName)
	}
	if cfg.VisitTimeZone != "Europe/Moscow" {
		t.Errorf("VisitTimeZone = %q, want Europe/Moscow", cfg.VisitTimeZone)
	}
	if cfg.CancelDeadlineHours != 2 {
		t.Errorf("CancelDeadlineHours = %d, want 2", cfg.CancelDeadlineHours)
	}
}

func TestLoad_InvalidTimezoneFallsBack(t *testing.T) {
	t.Setenv("BOOKING_VISIT_TIMEZONE", "Mars/Olympus")
	if got := Load().VisitTimeZone; got != "Europe/Moscow" {
		t.Errorf("invalid TZ should fall back to Europe/Moscow, got %q", got)
	}
}

func TestLoad_CancelDeadlineParsing(t *testing.T) {
	t.Run("valid override", func(t *testing.T) {
		t.Setenv("CANCEL_DEADLINE_HOURS", "6")
		if got := Load().CancelDeadlineHours; got != 6 {
			t.Errorf("CancelDeadlineHours = %d, want 6", got)
		}
	})
	t.Run("zero is allowed", func(t *testing.T) {
		t.Setenv("CANCEL_DEADLINE_HOURS", "0")
		if got := Load().CancelDeadlineHours; got != 0 {
			t.Errorf("CancelDeadlineHours = %d, want 0", got)
		}
	})
	t.Run("negative falls back to default", func(t *testing.T) {
		t.Setenv("CANCEL_DEADLINE_HOURS", "-5")
		if got := Load().CancelDeadlineHours; got != 2 {
			t.Errorf("negative value should keep default 2, got %d", got)
		}
	})
	t.Run("non-numeric falls back to default", func(t *testing.T) {
		t.Setenv("CANCEL_DEADLINE_HOURS", "soon")
		if got := Load().CancelDeadlineHours; got != 2 {
			t.Errorf("non-numeric value should keep default 2, got %d", got)
		}
	})
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv("GRPC_PORT", "9999")
	t.Setenv("PG_DB", "custom_booking")
	t.Setenv("CURSOR_HMAC_KEY", "supersecret")
	t.Setenv("INTERNAL_SERVICE_TOKEN", "tok")

	cfg := Load()

	if cfg.GRPCPort != "9999" || cfg.Postgres.DBName != "custom_booking" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
	if cfg.CursorHMACKey != "supersecret" || cfg.InternalServiceToken != "tok" {
		t.Errorf("secret overrides not applied: %+v", cfg)
	}
}
