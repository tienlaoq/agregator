package config

import (
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	for _, k := range []string{
		"GRPC_PORT", "PG_DB", "NATS_URL", "BOOKING_SERVICE_ADDR", "VENUE_SERVICE_ADDR",
		"MASTER_SERVICE_ADDR", "OUTBOX_BATCH_SIZE", "OUTBOX_INTERVAL",
	} {
		t.Setenv(k, "")
	}

	cfg := Load()

	if cfg.GRPCPort != "50055" {
		t.Errorf("GRPCPort = %q, want 50055", cfg.GRPCPort)
	}
	if cfg.Postgres.DBName != "review_db" {
		t.Errorf("Postgres.DBName = %q, want review_db", cfg.Postgres.DBName)
	}
	if cfg.OutboxBatchSize != 50 {
		t.Errorf("OutboxBatchSize = %d, want 50", cfg.OutboxBatchSize)
	}
	if cfg.OutboxInterval != 5*time.Second {
		t.Errorf("OutboxInterval = %v, want 5s", cfg.OutboxInterval)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv("GRPC_PORT", "9999")
	t.Setenv("PG_DB", "custom_reviews")
	t.Setenv("OUTBOX_BATCH_SIZE", "100")
	t.Setenv("OUTBOX_INTERVAL", "2s")
	t.Setenv("BOOKING_SERVICE_ADDR", "booking:1")

	cfg := Load()

	if cfg.GRPCPort != "9999" || cfg.Postgres.DBName != "custom_reviews" {
		t.Errorf("port/db overrides not applied: %+v", cfg)
	}
	if cfg.OutboxBatchSize != 100 {
		t.Errorf("OutboxBatchSize = %d, want 100", cfg.OutboxBatchSize)
	}
	if cfg.OutboxInterval != 2*time.Second {
		t.Errorf("OutboxInterval = %v, want 2s", cfg.OutboxInterval)
	}
	if cfg.BookingServiceAddr != "booking:1" {
		t.Errorf("BookingServiceAddr = %q, want booking:1", cfg.BookingServiceAddr)
	}
}
