package config

import (
	"fmt"
	"strings"
	"time"

	pkgauth "github.com/tienlao/agregator/pkg/auth"
	"github.com/tienlao/agregator/pkg/config"
)

type Config struct {
	GRPCPort string

	// JWT_EC_PRIVATE_KEY_FILE is the path to a PEM-encoded ECDSA private key
	// (PKCS8 or SEC1). Used to sign access tokens. The raw PEM is loaded at
	// startup into JWTECPrivateKeyPEM and parsed by the usecase layer.
	//
	// Alternatively, set JWT_EC_PRIVATE_KEY to the raw PEM content directly
	// (useful in container environments where secrets are injected as env vars).
	// JWT_SECRET (old HS256 shared secret) is no longer accepted.
	JWTECPrivateKeyPEM []byte

	JWTAccessTTL     time.Duration
	JWTRefreshTTL    time.Duration
	PasswordResetTTL time.Duration
	// EmailVerifyTTL is how long an email-verification link stays valid.
	// Longer than the reset TTL because users may check email less urgently.
	EmailVerifyTTL time.Duration

	Postgres        config.PostgresConfig
	UserServiceAddr string

	// FrontendURL is the externally-reachable base URL of the frontend app.
	// Used to construct password-reset links and Telegram admin deep-links.
	FrontendURL string

	// Telegram bot credentials for partner-registration notifications.
	// Both must be set for notifications to be enabled.
	TelegramBotToken string
	TelegramChatID   string

	// LegacyJWTSecret is non-empty when the deprecated JWT_SECRET variable is
	// still present in the environment. Checked at startup to emit a warning;
	// the value itself is never used for signing or verification.
	LegacyJWTSecret string
}

// Load reads all environment variables and returns a validated Config.
// It returns an error instead of silently falling back to defaults when a
// variable is set but malformed — e.g. JWT_ACCESS_TTL=15min is a typo that
// would otherwise produce a 15 m window without any warning.
// Unset variables still use safe defaults (the default value is documented
// next to each GetEnv call).
func Load() (Config, error) {
	accessTTL, err := parseDurationEnv("JWT_ACCESS_TTL", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	if accessTTL <= 0 {
		return Config{}, fmt.Errorf("JWT_ACCESS_TTL must be positive, got %q", config.GetEnv("JWT_ACCESS_TTL", ""))
	}

	refreshTTL, err := parseDurationEnv("JWT_REFRESH_TTL", 720*time.Hour)
	if err != nil {
		return Config{}, err
	}
	if refreshTTL <= 0 {
		return Config{}, fmt.Errorf("JWT_REFRESH_TTL must be positive, got %q", config.GetEnv("JWT_REFRESH_TTL", ""))
	}

	resetTTL, err := parseDurationEnv("PASSWORD_RESET_TTL", time.Hour)
	if err != nil {
		return Config{}, err
	}
	if resetTTL <= 0 {
		return Config{}, fmt.Errorf("PASSWORD_RESET_TTL must be positive, got %q", config.GetEnv("PASSWORD_RESET_TTL", ""))
	}

	verifyTTL, err := parseDurationEnv("EMAIL_VERIFY_TTL", 24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	if verifyTTL <= 0 {
		return Config{}, fmt.Errorf("EMAIL_VERIFY_TTL must be positive, got %q", config.GetEnv("EMAIL_VERIFY_TTL", ""))
	}

	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "auth_db")

	return Config{
		GRPCPort:           config.GetEnv("GRPC_PORT", "50051"),
		JWTECPrivateKeyPEM: pkgauth.LoadPEMFromEnv("JWT_EC_PRIVATE_KEY_FILE", "JWT_EC_PRIVATE_KEY"),
		JWTAccessTTL:       accessTTL,
		JWTRefreshTTL:      refreshTTL,
		PasswordResetTTL:   resetTTL,
		EmailVerifyTTL:     verifyTTL,
		Postgres:           pg,
		UserServiceAddr:    config.GetEnv("USER_SERVICE_ADDR", "localhost:50052"),
		FrontendURL:        normaliseFrontendURL(config.GetEnv("FRONTEND_URL", "")),
		TelegramBotToken:   strings.TrimSpace(config.GetEnv("TELEGRAM_BOT_TOKEN", "")),
		TelegramChatID:     strings.TrimSpace(config.GetEnv("TELEGRAM_CHAT_ID", "")),
		LegacyJWTSecret:    config.GetEnv("JWT_SECRET", ""),
	}, nil
}

// normaliseFrontendURL trims whitespace and a trailing slash from the raw
// FRONTEND_URL value, then falls back to "http://localhost:3000" when the
// result is empty. Callers can concatenate a path directly without re-trimming.
func normaliseFrontendURL(raw string) string {
	u := strings.TrimSuffix(strings.TrimSpace(raw), "/")
	if u == "" {
		return "http://localhost:3000"
	}
	return u
}

// parseDurationEnv reads a duration from the named env var.
// If the variable is unset or empty, defaultVal is returned.
// If the variable is set but cannot be parsed, an error is returned —
// this distinguishes "not configured" (safe default) from "misconfigured"
// (operator typo that must be surfaced immediately at startup).
func parseDurationEnv(key string, defaultVal time.Duration) (time.Duration, error) {
	raw := config.GetEnv(key, "")
	if raw == "" {
		return defaultVal, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q: %w (valid examples: 15m, 1h, 720h)", key, raw, err)
	}
	return d, nil
}
