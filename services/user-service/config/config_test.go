package config

import "testing"

func TestLoad_Defaults(t *testing.T) {
	for _, k := range []string{"GRPC_PORT", "PG_DB"} {
		t.Setenv(k, "")
	}

	cfg := Load()

	if cfg.GRPCPort != "50052" {
		t.Errorf("GRPCPort = %q, want 50052", cfg.GRPCPort)
	}
	if cfg.Postgres.DBName != "user_db" {
		t.Errorf("Postgres.DBName = %q, want user_db", cfg.Postgres.DBName)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv("GRPC_PORT", "60052")
	t.Setenv("PG_DB", "custom_users")

	cfg := Load()

	if cfg.GRPCPort != "60052" {
		t.Errorf("GRPCPort = %q, want 60052", cfg.GRPCPort)
	}
	if cfg.Postgres.DBName != "custom_users" {
		t.Errorf("Postgres.DBName = %q, want custom_users", cfg.Postgres.DBName)
	}
}
