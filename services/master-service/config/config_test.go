package config

import (
	"strings"
	"testing"

	"github.com/tienlao/agregator/pkg/config"
)

func TestLoad_Defaults(t *testing.T) {
	for _, k := range []string{
		"GRPC_PORT", "PG_DB", "NATS_URL", "PAYMENT_SERVICE_ADDR", "INTERNAL_SERVICE_TOKEN",
	} {
		t.Setenv(k, "")
	}

	cfg := Load()

	if cfg.GRPCPort != "50057" {
		t.Errorf("GRPCPort = %q, want 50057", cfg.GRPCPort)
	}
	if cfg.NATSURL != "nats://localhost:4222" {
		t.Errorf("NATSURL = %q, want default", cfg.NATSURL)
	}
	if cfg.PaymentServiceAddr != "localhost:50056" {
		t.Errorf("PaymentServiceAddr = %q, want default", cfg.PaymentServiceAddr)
	}
	if cfg.Postgres.DBName != "master_db" {
		t.Errorf("Postgres.DBName = %q, want master_db", cfg.Postgres.DBName)
	}
	if cfg.InternalServiceToken != "" {
		t.Errorf("InternalServiceToken = %q, want empty default", cfg.InternalServiceToken)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv("GRPC_PORT", "9999")
	t.Setenv("PG_DB", "custom_db")
	t.Setenv("NATS_URL", "nats://example:4222")
	t.Setenv("PAYMENT_SERVICE_ADDR", "payment:1234")
	t.Setenv("INTERNAL_SERVICE_TOKEN", "secret")

	cfg := Load()

	if cfg.GRPCPort != "9999" {
		t.Errorf("GRPCPort = %q, want 9999", cfg.GRPCPort)
	}
	if cfg.Postgres.DBName != "custom_db" {
		t.Errorf("Postgres.DBName = %q, want custom_db", cfg.Postgres.DBName)
	}
	if cfg.NATSURL != "nats://example:4222" {
		t.Errorf("NATSURL = %q, want override", cfg.NATSURL)
	}
	if cfg.PaymentServiceAddr != "payment:1234" {
		t.Errorf("PaymentServiceAddr = %q, want override", cfg.PaymentServiceAddr)
	}
	if cfg.InternalServiceToken != "secret" {
		t.Errorf("InternalServiceToken = %q, want secret", cfg.InternalServiceToken)
	}
}

// validConfig returns a Config that passes Validate; tests blank one field at a
// time to assert each required-field branch.
func validConfig() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = "master_db"
	pg.Host = "localhost"
	pg.User = "postgres"
	pg.Password = "postgres"
	return Config{
		GRPCPort:           "50057",
		Postgres:           pg,
		NATSURL:            "nats://localhost:4222",
		PaymentServiceAddr: "localhost:50056",
	}
}

func TestValidate_OK(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestValidate_MissingFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *Config)
		want   string
	}{
		{"missing NATS_URL", func(c *Config) { c.NATSURL = "  " }, "NATS_URL is required"},
		{"missing payment addr", func(c *Config) { c.PaymentServiceAddr = "" }, "PAYMENT_SERVICE_ADDR is required"},
		{"missing grpc port", func(c *Config) { c.GRPCPort = "" }, "GRPC_PORT is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(&c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err.Error(), tt.want)
			}
		})
	}
}
