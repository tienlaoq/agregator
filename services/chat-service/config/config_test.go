package config

import "testing"

func TestLoad_Defaults(t *testing.T) {
	for _, k := range []string{"GRPC_PORT", "PG_DB", "NATS_URL"} {
		t.Setenv(k, "")
	}
	cfg := Load()
	if cfg.GRPCPort != "50058" {
		t.Errorf("GRPCPort = %q, want 50058", cfg.GRPCPort)
	}
	if cfg.Postgres.DBName != "chat_db" {
		t.Errorf("Postgres.DBName = %q, want chat_db", cfg.Postgres.DBName)
	}
	if cfg.NATSUrl != "" {
		t.Errorf("NATSUrl default should be empty (NATS optional), got %q", cfg.NATSUrl)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv("GRPC_PORT", "9999")
	t.Setenv("PG_DB", "custom_chat")
	t.Setenv("NATS_URL", "nats://example:4222")
	cfg := Load()
	if cfg.GRPCPort != "9999" || cfg.Postgres.DBName != "custom_chat" || cfg.NATSUrl != "nats://example:4222" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}
