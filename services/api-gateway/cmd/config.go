package main

import (
	"crypto/ecdsa"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	pkgauth "github.com/tienlao/agregator/pkg/auth"
	"github.com/tienlao/agregator/pkg/config"
)

// Config holds every env-var the gateway reads, gathered in one place so they
// can be logged at startup and validated before any connection is opened.
type Config struct {
	// HTTP
	HTTPPort   string
	UploadRoot string

	// Auth — ES256 asymmetric JWT.
	// Gateway only needs the public key to verify tokens; it can never issue them.
	JWTECPublicKey *ecdsa.PublicKey // parsed at startup from JWT_EC_PUBLIC_KEY / JWT_EC_PUBLIC_KEY_FILE

	// Service addresses
	AuthAddr    string
	UserAddr    string
	VenueAddr   string
	BookingAddr string
	ReviewAddr  string
	PaymentAddr string
	MasterAddr  string
	ChatAddr    string
	CRMAddr     string

	// Internal service-to-service token (gateway → chat-service)
	InternalServiceToken string

	// OAuth
	GoogleClientID     string
	GoogleClientSecret string
	VKClientID         string
	VKClientSecret     string

	// URLs
	BaseURL     string
	FrontendURL string

	// MinIO / S3-compatible object storage for user-uploaded photos.
	// When MinIOEndpoint is empty the gateway falls back to DiskUploader
	// (single-replica dev mode only — see docs/TECH_DEBT.md [STORAGE-01]).
	MinIOEndpoint      string
	MinIOAccessKey     string
	MinIOSecretKey     string
	MinIOBucket        string
	MinIOPublicBaseURL string // externally-reachable URL, e.g. "https://cdn.example.com/photos"
	MinIOUseSSL        bool

	// Redis
	RedisAddr string

	// NATS
	NATSUrl string

	// Postgres for support tickets
	SupportPGDBName string

	// Support helpdesk webhook
	SupportWebhookURL      string
	SupportWebhookToken    string
	SupportModeratorEmails []string

	// Rate limits
	RateLimitLoginMax       int
	RateLimitLoginWindow    time.Duration
	RateLimitRegisterMax    int
	RateLimitRegisterWindow time.Duration
	RateLimitAnalyticsMax   int
	RateLimitAnalyticsWindow time.Duration
	RateLimitSupportMax     int
	RateLimitSupportWindow  time.Duration
	RateLimitWebhookMax     int
	RateLimitWebhookWindow  time.Duration
	RateLimitAdminMax            int
	RateLimitAdminWindow         time.Duration
	RateLimitResetPasswordMax    int
	RateLimitResetPasswordWindow time.Duration
	RateLimitMasterPublicMax     int
	RateLimitMasterPublicWindow  time.Duration

	// Forgot-password legacy rate-limit
	ForgotPasswordMax       int
	ForgotPasswordWindow    time.Duration
	ForgotPasswordKeyPrefix string
	ForgotPasswordFailOpen  bool

	// REST handler timeout applied via http.TimeoutHandler on all non-WS routes.
	// Must be longer than the slowest legitimate handler (forgot-password ~55s).
	// WebSocket upgrade routes are excluded — see middleware.RequestTimeout.
	RESTHandlerTimeout time.Duration

	// Payment webhook security — CRIT-1.
	//
	// PaymentWebhookSecret is the shared HMAC-SHA256 key used by the new bank
	// acquirer to sign each POST body (X-Signature: sha256=<hex>).
	// Set via PAYMENT_WEBHOOK_SECRET.  Empty disables HMAC verification.
	PaymentWebhookSecret string

	// PaymentWebhookIPAllowlist is a comma-separated list of CIDRs / plain IPs
	// that are allowed to call POST /payments/webhook (e.g. YooKassa IPs).
	// Set via PAYMENT_WEBHOOK_IP_ALLOWLIST.  Empty disables IP verification.
	PaymentWebhookIPAllowlist string
}

// LoadConfig reads all env-vars and returns a Config. No side effects.
func LoadConfig() (Config, error) {
	cfg := Config{
		HTTPPort:   config.GetEnv("HTTP_PORT", "8080"),
		UploadRoot: config.GetEnv("UPLOAD_ROOT", "./data/uploads"),

		AuthAddr:    config.GetEnv("AUTH_SERVICE_ADDR", "localhost:50051"),
		UserAddr:    config.GetEnv("USER_SERVICE_ADDR", "localhost:50052"),
		VenueAddr:   config.GetEnv("VENUE_SERVICE_ADDR", "localhost:50053"),
		BookingAddr: config.GetEnv("BOOKING_SERVICE_ADDR", "localhost:50054"),
		ReviewAddr:  config.GetEnv("REVIEW_SERVICE_ADDR", "localhost:50055"),
		PaymentAddr: config.GetEnv("PAYMENT_SERVICE_ADDR", "localhost:50056"),
		MasterAddr:  config.GetEnv("MASTER_SERVICE_ADDR", "localhost:50057"),
		ChatAddr:    config.GetEnv("CHAT_SERVICE_ADDR", "localhost:50058"),
		CRMAddr:     config.GetEnv("CRM_SERVICE_ADDR", "localhost:50059"),

		InternalServiceToken: strings.TrimSpace(config.GetEnv("INTERNAL_SERVICE_TOKEN", "")),

		GoogleClientID:     config.GetEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: config.GetEnv("GOOGLE_CLIENT_SECRET", ""),
		VKClientID:         config.GetEnv("VK_CLIENT_ID", ""),
		VKClientSecret:     config.GetEnv("VK_CLIENT_SECRET", ""),

		BaseURL:     strings.TrimSpace(config.GetEnv("BASE_URL", "http://localhost:8080")),
		FrontendURL: strings.TrimSpace(config.GetEnv("FRONTEND_URL", "http://localhost:3000")),

		MinIOEndpoint:      strings.TrimSpace(config.GetEnv("MINIO_ENDPOINT", "")),
		MinIOAccessKey:     strings.TrimSpace(config.GetEnv("MINIO_ACCESS_KEY", "")),
		MinIOSecretKey:     strings.TrimSpace(config.GetEnv("MINIO_SECRET_KEY", "")),
		MinIOBucket:        strings.TrimSpace(config.GetEnv("MINIO_BUCKET", "photos")),
		MinIOPublicBaseURL: strings.TrimSpace(config.GetEnv("MINIO_PUBLIC_BASE_URL", "")),
		MinIOUseSSL:        config.GetEnvBool("MINIO_USE_SSL", false),

		RedisAddr: strings.TrimSpace(config.GetEnv("REDIS_ADDR", "")),
		NATSUrl:   strings.TrimSpace(config.GetEnv("NATS_URL", "")),

		SupportPGDBName:        config.GetEnv("PG_DB", "support_db"),
		SupportWebhookURL:      strings.TrimSpace(config.GetEnv("SUPPORT_HELPDESK_WEBHOOK_URL", "")),
		SupportWebhookToken:    strings.TrimSpace(config.GetEnv("SUPPORT_HELPDESK_WEBHOOK_TOKEN", "")),
		SupportModeratorEmails: parseCSV(config.GetEnv("SUPPORT_MODERATOR_EMAILS", "")),

		RateLimitLoginMax:        config.GetEnvInt("RATE_LIMIT_LOGIN_MAX", 10),
		RateLimitLoginWindow:     config.GetEnvDuration("RATE_LIMIT_LOGIN_WINDOW", 5*time.Minute),
		RateLimitRegisterMax:     config.GetEnvInt("RATE_LIMIT_REGISTER_MAX", 5),
		RateLimitRegisterWindow:  config.GetEnvDuration("RATE_LIMIT_REGISTER_WINDOW", 15*time.Minute),
		RateLimitAnalyticsMax:    config.GetEnvInt("RATE_LIMIT_ANALYTICS_MAX", 60),
		RateLimitAnalyticsWindow: config.GetEnvDuration("RATE_LIMIT_ANALYTICS_WINDOW", 1*time.Minute),
		RateLimitSupportMax:      config.GetEnvInt("RATE_LIMIT_SUPPORT_MAX", 5),
		RateLimitSupportWindow:   config.GetEnvDuration("RATE_LIMIT_SUPPORT_WINDOW", 1*time.Hour),
		RateLimitWebhookMax:      config.GetEnvInt("RATE_LIMIT_WEBHOOK_MAX", 30),
		RateLimitWebhookWindow:   config.GetEnvDuration("RATE_LIMIT_WEBHOOK_WINDOW", 1*time.Minute),
		RateLimitAdminMax:            config.GetEnvInt("RATE_LIMIT_ADMIN_MAX", 60),
		RateLimitAdminWindow:         config.GetEnvDuration("RATE_LIMIT_ADMIN_WINDOW", 1*time.Minute),
		RateLimitResetPasswordMax:    config.GetEnvInt("RATE_LIMIT_RESET_PASSWORD_MAX", 10),
		RateLimitResetPasswordWindow: config.GetEnvDuration("RATE_LIMIT_RESET_PASSWORD_WINDOW", 15*time.Minute),
		RateLimitMasterPublicMax:     config.GetEnvInt("RATE_LIMIT_MASTER_PUBLIC_MAX", 120),
		RateLimitMasterPublicWindow:  config.GetEnvDuration("RATE_LIMIT_MASTER_PUBLIC_WINDOW", 1*time.Minute),

		ForgotPasswordMax:       config.GetEnvInt("FORGOT_PASSWORD_RATE_LIMIT_MAX", 10),
		ForgotPasswordWindow:    config.GetEnvDuration("FORGOT_PASSWORD_RATE_LIMIT_WINDOW", 15*time.Minute),
		ForgotPasswordKeyPrefix: config.GetEnv("FORGOT_PASSWORD_RATE_LIMIT_KEY_PREFIX", "auth:forgot:"),
		ForgotPasswordFailOpen:  config.GetEnvBool("FORGOT_PASSWORD_RATE_LIMIT_FAIL_OPEN", true),

		RESTHandlerTimeout: config.GetEnvDuration("REST_HANDLER_TIMEOUT", 60*time.Second),

		PaymentWebhookSecret:      strings.TrimSpace(config.GetEnv("PAYMENT_WEBHOOK_SECRET", "")),
		PaymentWebhookIPAllowlist: strings.TrimSpace(config.GetEnv("PAYMENT_WEBHOOK_IP_ALLOWLIST", "")),
	}
	// Parse the EC public key from env (file path or inline PEM).
	pubPEM := pkgauth.LoadPEMFromEnv("JWT_EC_PUBLIC_KEY_FILE", "JWT_EC_PUBLIC_KEY")
	if len(pubPEM) > 0 {
		pub, err := pkgauth.ParseECPublicKey(pubPEM)
		if err != nil {
			return Config{}, fmt.Errorf("failed to parse JWT_EC_PUBLIC_KEY: %w", err)
		}
		cfg.JWTECPublicKey = pub
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}


// Validate returns an error for any configuration that would cause a
// guaranteed runtime failure or security problem.
func (c Config) Validate() error {
	if c.JWTECPublicKey == nil {
		return fmt.Errorf("JWT_EC_PUBLIC_KEY or JWT_EC_PUBLIC_KEY_FILE is required (ES256 public key PEM)")
	}
	if c.BaseURL != "" {
		u, err := url.Parse(c.BaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("BASE_URL %q must be an absolute URL with scheme and host", c.BaseURL)
		}
		if strings.HasSuffix(u.Path, "/") {
			return fmt.Errorf("BASE_URL %q must not have a trailing slash", c.BaseURL)
		}
	}
	return nil
}

// isProductionEnv returns true when the process appears to be running in a
// production-like environment. Two independent signals are checked:
//
//  1. ENV=production — explicit marker set by the deploy pipeline.
//  2. addr is not a loopback host — a remote service address strongly implies
//     a real deployment even if ENV was omitted.
func isProductionEnv(addr string) bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production") {
		return true
	}
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	host = strings.TrimSpace(strings.ToLower(host))
	return host != "" && host != "localhost" && host != "127.0.0.1" && host != "::1"
}

func parseCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
