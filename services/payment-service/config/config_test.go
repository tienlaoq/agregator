package config

import (
	"strings"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	for _, k := range []string{
		"GRPC_PORT", "PG_DB", "NATS_URL", "PAYMENT_PROVIDER",
		"TBANK_TERMINAL_KEY", "TBANK_SECRET_KEY", "SBER_MERCHANT_LOGIN", "SBER_SECRET_KEY",
		"PAYMENT_RETURN_URL", "PLATFORM_FEE_BPS", "PAYOUT_HOLD_HOURS", "INTERNAL_SERVICE_TOKEN",
	} {
		t.Setenv(k, "")
	}

	cfg := Load()

	if cfg.GRPCPort != "50056" {
		t.Errorf("GRPCPort = %q, want 50056", cfg.GRPCPort)
	}
	if cfg.Postgres.DBName != "payment_db" {
		t.Errorf("Postgres.DBName = %q, want payment_db", cfg.Postgres.DBName)
	}
	if cfg.ActiveProvider != ProviderMock {
		t.Errorf("ActiveProvider = %q, want mock", cfg.ActiveProvider)
	}
	if cfg.PlatformFeeBPS != 1500 {
		t.Errorf("PlatformFeeBPS = %d, want 1500", cfg.PlatformFeeBPS)
	}
	if cfg.PayoutHoldHours != 24 {
		t.Errorf("PayoutHoldHours = %d, want 24", cfg.PayoutHoldHours)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Setenv("PAYMENT_PROVIDER", "tbank")
	t.Setenv("PLATFORM_FEE_BPS", "1000")
	t.Setenv("PAYOUT_HOLD_HOURS", "48")
	t.Setenv("INTERNAL_SERVICE_TOKEN", "secret")

	cfg := Load()

	if cfg.ActiveProvider != ProviderTBank {
		t.Errorf("ActiveProvider = %q, want tbank", cfg.ActiveProvider)
	}
	if cfg.PlatformFeeBPS != 1000 || cfg.PayoutHoldHours != 48 {
		t.Errorf("fee/hold = %d/%d, want 1000/48", cfg.PlatformFeeBPS, cfg.PayoutHoldHours)
	}
	if cfg.InternalServiceToken != "secret" {
		t.Errorf("InternalServiceToken = %q, want secret", cfg.InternalServiceToken)
	}
}

func TestValidate_NonProduction(t *testing.T) {
	// Mock is allowed and needs no credentials outside production.
	c := &Config{ActiveProvider: ProviderMock}
	if err := c.Validate(""); err != nil {
		t.Errorf("non-prod mock rejected: %v", err)
	}
	if err := c.Validate("development"); err != nil {
		t.Errorf("dev mock rejected: %v", err)
	}

	// Unknown provider is rejected regardless of environment.
	bad := &Config{ActiveProvider: ProviderName("paypal")}
	if err := bad.Validate(""); err == nil {
		t.Error("unknown provider must be rejected even in non-prod")
	}
}

func TestValidate_Production(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string // substring; "" means expect success
	}{
		{
			name:    "mock rejected in prod",
			cfg:     Config{ActiveProvider: ProviderMock, InternalServiceToken: "t"},
			wantErr: "mock is not allowed",
		},
		{
			name:    "tbank missing terminal key",
			cfg:     Config{ActiveProvider: ProviderTBank, InternalServiceToken: "t"},
			wantErr: "TBANK_TERMINAL_KEY",
		},
		{
			name: "tbank missing secret key",
			cfg: Config{ActiveProvider: ProviderTBank, InternalServiceToken: "t",
				Provider: ProviderConfig{TBankTerminalKey: "k"}},
			wantErr: "TBANK_SECRET_KEY",
		},
		{
			name: "tbank missing service token",
			cfg: Config{ActiveProvider: ProviderTBank,
				Provider: ProviderConfig{TBankTerminalKey: "k", TBankSecretKey: "s"}},
			wantErr: "INTERNAL_SERVICE_TOKEN",
		},
		{
			name: "tbank fully configured ok",
			cfg: Config{ActiveProvider: ProviderTBank, InternalServiceToken: "t",
				Provider: ProviderConfig{TBankTerminalKey: "k", TBankSecretKey: "s"}},
			wantErr: "",
		},
		{
			name:    "sber missing login",
			cfg:     Config{ActiveProvider: ProviderSber, InternalServiceToken: "t"},
			wantErr: "SBER_MERCHANT_LOGIN",
		},
		{
			name: "sber fully configured ok",
			cfg: Config{ActiveProvider: ProviderSber, InternalServiceToken: "t",
				Provider: ProviderConfig{SberMerchantLogin: "l", SberSecretKey: "s"}},
			wantErr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate("production")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_ProductionCaseInsensitiveEnv(t *testing.T) {
	c := &Config{ActiveProvider: ProviderMock}
	if err := c.Validate("PRODUCTION"); err == nil {
		t.Error("ENV=PRODUCTION (any case) must enforce production rules and reject mock")
	}
}
