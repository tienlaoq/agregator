package config

import (
	"testing"
	"time"
)

func TestNormaliseFrontendURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty falls back to localhost", "", "http://localhost:3000"},
		{"whitespace only falls back", "   ", "http://localhost:3000"},
		{"trailing slash stripped", "https://app.example.com/", "https://app.example.com"},
		{"surrounding whitespace trimmed", "  https://app.example.com  ", "https://app.example.com"},
		{"whitespace and trailing slash", "  https://app.example.com/  ", "https://app.example.com"},
		{"no trailing slash left intact", "https://app.example.com", "https://app.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normaliseFrontendURL(tt.raw); got != tt.want {
				t.Errorf("normaliseFrontendURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseDurationEnv(t *testing.T) {
	const key = "TEST_DURATION_ENV"

	t.Run("unset returns default", func(t *testing.T) {
		t.Setenv(key, "")
		got, err := parseDurationEnv(key, 42*time.Second)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 42*time.Second {
			t.Errorf("got %v, want default 42s", got)
		}
	})

	t.Run("valid value parsed", func(t *testing.T) {
		t.Setenv(key, "90m")
		got, err := parseDurationEnv(key, time.Second)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 90*time.Minute {
			t.Errorf("got %v, want 90m", got)
		}
	})

	t.Run("malformed value errors", func(t *testing.T) {
		t.Setenv(key, "15min") // common typo: Go wants "15m"
		_, err := parseDurationEnv(key, time.Second)
		if err == nil {
			t.Fatal("expected error for malformed duration, got nil")
		}
	})
}

func TestLoad_Defaults(t *testing.T) {
	// Clear every variable Load reads so we exercise the default branch.
	for _, k := range []string{
		"JWT_ACCESS_TTL", "JWT_REFRESH_TTL", "PASSWORD_RESET_TTL", "EMAIL_VERIFY_TTL",
		"GRPC_PORT", "USER_SERVICE_ADDR", "FRONTEND_URL",
		"TELEGRAM_BOT_TOKEN", "TELEGRAM_CHAT_ID", "JWT_SECRET",
		"JWT_EC_PRIVATE_KEY_FILE", "JWT_EC_PRIVATE_KEY", "PG_DB",
	} {
		t.Setenv(k, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.GRPCPort != "50051" {
		t.Errorf("GRPCPort = %q, want 50051", cfg.GRPCPort)
	}
	if cfg.JWTAccessTTL != 15*time.Minute {
		t.Errorf("JWTAccessTTL = %v, want 15m", cfg.JWTAccessTTL)
	}
	if cfg.JWTRefreshTTL != 720*time.Hour {
		t.Errorf("JWTRefreshTTL = %v, want 720h", cfg.JWTRefreshTTL)
	}
	if cfg.PasswordResetTTL != time.Hour {
		t.Errorf("PasswordResetTTL = %v, want 1h", cfg.PasswordResetTTL)
	}
	if cfg.EmailVerifyTTL != 24*time.Hour {
		t.Errorf("EmailVerifyTTL = %v, want 24h", cfg.EmailVerifyTTL)
	}
	if cfg.UserServiceAddr != "localhost:50052" {
		t.Errorf("UserServiceAddr = %q, want localhost:50052", cfg.UserServiceAddr)
	}
	if cfg.FrontendURL != "http://localhost:3000" {
		t.Errorf("FrontendURL = %q, want http://localhost:3000", cfg.FrontendURL)
	}
	if cfg.Postgres.DBName != "auth_db" {
		t.Errorf("Postgres.DBName = %q, want auth_db", cfg.Postgres.DBName)
	}
}

func TestLoad_OverridesAndTrimming(t *testing.T) {
	t.Setenv("JWT_ACCESS_TTL", "30m")
	t.Setenv("JWT_REFRESH_TTL", "1h")
	t.Setenv("PASSWORD_RESET_TTL", "2h")
	t.Setenv("EMAIL_VERIFY_TTL", "48h")
	t.Setenv("GRPC_PORT", "60000")
	t.Setenv("USER_SERVICE_ADDR", "user:9000")
	t.Setenv("FRONTEND_URL", "  https://banya.example/  ")
	t.Setenv("TELEGRAM_BOT_TOKEN", "  token  ")
	t.Setenv("TELEGRAM_CHAT_ID", "  chat  ")
	t.Setenv("PG_DB", "custom_db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.JWTAccessTTL != 30*time.Minute {
		t.Errorf("JWTAccessTTL = %v, want 30m", cfg.JWTAccessTTL)
	}
	if cfg.GRPCPort != "60000" {
		t.Errorf("GRPCPort = %q, want 60000", cfg.GRPCPort)
	}
	if cfg.FrontendURL != "https://banya.example" {
		t.Errorf("FrontendURL = %q, want trimmed https://banya.example", cfg.FrontendURL)
	}
	if cfg.TelegramBotToken != "token" {
		t.Errorf("TelegramBotToken = %q, want trimmed token", cfg.TelegramBotToken)
	}
	if cfg.TelegramChatID != "chat" {
		t.Errorf("TelegramChatID = %q, want trimmed chat", cfg.TelegramChatID)
	}
	if cfg.Postgres.DBName != "custom_db" {
		t.Errorf("Postgres.DBName = %q, want custom_db", cfg.Postgres.DBName)
	}
}

func TestLoad_InvalidDurations(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
	}{
		{"malformed access ttl", "JWT_ACCESS_TTL", "nonsense"},
		{"non-positive access ttl", "JWT_ACCESS_TTL", "0s"},
		{"negative refresh ttl", "JWT_REFRESH_TTL", "-1h"},
		{"malformed reset ttl", "PASSWORD_RESET_TTL", "abc"},
		{"non-positive verify ttl", "EMAIL_VERIFY_TTL", "0h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Keep the other TTLs valid so only the field under test fails.
			t.Setenv("JWT_ACCESS_TTL", "15m")
			t.Setenv("JWT_REFRESH_TTL", "720h")
			t.Setenv("PASSWORD_RESET_TTL", "1h")
			t.Setenv("EMAIL_VERIFY_TTL", "24h")
			t.Setenv(tt.key, tt.val)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with %s=%q expected error, got nil", tt.key, tt.val)
			}
		})
	}
}
