package config

import (
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// GetEnv treats an empty value as unset, so clearing these keys exercises
	// the default fallbacks regardless of the ambient environment.
	for _, k := range []string{
		"GRPC_PORT", "PG_DB", "VENUE_CACHE_TTL", "SEARCH_CACHE_TTL", "REVIEW_SERVICE_ADDR",
	} {
		t.Setenv(k, "")
	}

	cfg := Load()

	if cfg.GRPCPort != "50053" {
		t.Errorf("GRPCPort = %q, want 50053", cfg.GRPCPort)
	}
	if cfg.Postgres.DBName != "venue_db" {
		t.Errorf("Postgres.DBName = %q, want venue_db", cfg.Postgres.DBName)
	}
	if cfg.VenueCacheTTL != 10*time.Minute {
		t.Errorf("VenueCacheTTL = %s, want 10m", cfg.VenueCacheTTL)
	}
	if cfg.SearchCacheTTL != 2*time.Minute {
		t.Errorf("SearchCacheTTL = %s, want 2m", cfg.SearchCacheTTL)
	}
	if cfg.ReviewServiceAddr != "localhost:50055" {
		t.Errorf("ReviewServiceAddr = %q, want localhost:50055", cfg.ReviewServiceAddr)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("GRPC_PORT", "60053")
	t.Setenv("PG_DB", "custom_db")
	t.Setenv("VENUE_CACHE_TTL", "30m")
	t.Setenv("SEARCH_CACHE_TTL", "45s")
	t.Setenv("REVIEW_SERVICE_ADDR", "review:9999")

	cfg := Load()

	if cfg.GRPCPort != "60053" {
		t.Errorf("GRPCPort = %q, want 60053", cfg.GRPCPort)
	}
	if cfg.Postgres.DBName != "custom_db" {
		t.Errorf("Postgres.DBName = %q, want custom_db", cfg.Postgres.DBName)
	}
	if cfg.VenueCacheTTL != 30*time.Minute {
		t.Errorf("VenueCacheTTL = %s, want 30m", cfg.VenueCacheTTL)
	}
	if cfg.SearchCacheTTL != 45*time.Second {
		t.Errorf("SearchCacheTTL = %s, want 45s", cfg.SearchCacheTTL)
	}
	if cfg.ReviewServiceAddr != "review:9999" {
		t.Errorf("ReviewServiceAddr = %q, want review:9999", cfg.ReviewServiceAddr)
	}
}
