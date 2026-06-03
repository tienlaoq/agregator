package config

import (
	"fmt"
	"strings"

	"github.com/tienlao/agregator/pkg/config"
)

// ProviderName identifies the active payment gateway.
// Validated at startup — unknown values cause a fatal error.
type ProviderName string

const (
	ProviderYooKassa ProviderName = "yookassa"
	ProviderTBank    ProviderName = "tbank"
	ProviderSber     ProviderName = "sber"
)

// ProviderConfig holds per-provider credentials loaded from environment.
// Only the fields for the active provider need to be set; unused fields are
// ignored so secrets for inactive providers can be omitted entirely.
type ProviderConfig struct {
	// YooKassa — YOOKASSA_SHOP_ID / YOOKASSA_SECRET_KEY
	YooKassaShopID    string
	YooKassaSecretKey string

	// TBank — TBANK_TERMINAL_KEY / TBANK_SECRET_KEY
	TBankTerminalKey string
	TBankSecretKey   string

	// Sber — SBER_MERCHANT_LOGIN / SBER_SECRET_KEY
	SberMerchantLogin string
	SberSecretKey     string
}

type Config struct {
	GRPCPort string
	Postgres config.PostgresConfig
	NATSURL  string

	// Provider selects the active payment gateway at startup.
	// Set via PAYMENT_PROVIDER env var (default: "yookassa").
	// Switching providers requires only an env change + redeploy — no code change.
	Provider ProviderConfig
	// ActiveProvider is the validated name of the selected gateway.
	ActiveProvider ProviderName

	PaymentReturnURL string
	// PlatformFeeBPS is the marketplace commission in basis points (1500 = 15%).
	PlatformFeeBPS int
	// PayoutHoldHours is how long after the service date a successful payment
	// stays in escrow before becoming available for payout.  24h is the default.
	PayoutHoldHours int
	// InternalServiceToken is the shared secret that booking-service and
	// master-service must present in the "x-service-token" gRPC metadata header
	// on every call to payment-service.  Empty token disables the check in local
	// dev / CI; required in production (ENV=production).
	InternalServiceToken string
}

// Validate returns an error when the config is invalid for the given
// deployment environment.  Pass os.Getenv("ENV") as env; an empty string or
// any value other than "production" (case-insensitive) is treated as non-prod.
//
// In production:
//   - ActiveProvider must be a known gateway.
//   - Required credentials for the active provider must be non-empty; a missing
//     credential would silently enable mock mode, causing fake payment URLs to
//     reach real users and fake "succeeded" events to enter the DB.
//   - InternalServiceToken must be set (same rule as before — kept here for
//     symmetry so all production invariants are in one place).
//
// In non-production (dev / CI):
//   - Only the ActiveProvider name is validated; credentials may be empty to
//     allow mock mode.
func (c *Config) Validate(env string) error {
	isProduction := strings.EqualFold(strings.TrimSpace(env), "production")

	switch c.ActiveProvider {
	case ProviderYooKassa, ProviderTBank, ProviderSber:
		// known — ok
	default:
		return fmt.Errorf("unknown PAYMENT_PROVIDER %q: must be yookassa, tbank, or sber", c.ActiveProvider)
	}

	if !isProduction {
		return nil
	}

	// ── Production-only credential checks ────────────────────────────────────
	// A missing credential silently enables mock mode: CreatePayment returns a
	// fake URL, GetByProviderID looks up a fake UUID, and the outbox publishes
	// a fake "succeeded" event.  Fail fast here so the deploy never starts.
	switch c.ActiveProvider {
	case ProviderYooKassa:
		if strings.TrimSpace(c.Provider.YooKassaShopID) == "" {
			return fmt.Errorf("production: YOOKASSA_SHOP_ID must be set when PAYMENT_PROVIDER=yookassa")
		}
		if strings.TrimSpace(c.Provider.YooKassaSecretKey) == "" {
			return fmt.Errorf("production: YOOKASSA_SECRET_KEY must be set when PAYMENT_PROVIDER=yookassa")
		}
	case ProviderTBank:
		if strings.TrimSpace(c.Provider.TBankTerminalKey) == "" {
			return fmt.Errorf("production: TBANK_TERMINAL_KEY must be set when PAYMENT_PROVIDER=tbank")
		}
		if strings.TrimSpace(c.Provider.TBankSecretKey) == "" {
			return fmt.Errorf("production: TBANK_SECRET_KEY must be set when PAYMENT_PROVIDER=tbank")
		}
	case ProviderSber:
		if strings.TrimSpace(c.Provider.SberMerchantLogin) == "" {
			return fmt.Errorf("production: SBER_MERCHANT_LOGIN must be set when PAYMENT_PROVIDER=sber")
		}
		if strings.TrimSpace(c.Provider.SberSecretKey) == "" {
			return fmt.Errorf("production: SBER_SECRET_KEY must be set when PAYMENT_PROVIDER=sber")
		}
	}

	if strings.TrimSpace(c.InternalServiceToken) == "" {
		return fmt.Errorf("production: INTERNAL_SERVICE_TOKEN must be set (ENV=production)")
	}

	return nil
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "payment_db")

	return Config{
		GRPCPort: config.GetEnv("GRPC_PORT", "50056"),
		Postgres: pg,
		NATSURL:  config.GetEnv("NATS_URL", "nats://localhost:4222"),

		ActiveProvider: ProviderName(config.GetEnv("PAYMENT_PROVIDER", string(ProviderYooKassa))),

		Provider: ProviderConfig{
			// ЮKassa
			YooKassaShopID:    config.GetEnv("YOOKASSA_SHOP_ID", ""),
			YooKassaSecretKey: config.GetEnv("YOOKASSA_SECRET_KEY", ""),
			// TBank
			TBankTerminalKey: config.GetEnv("TBANK_TERMINAL_KEY", ""),
			TBankSecretKey:   config.GetEnv("TBANK_SECRET_KEY", ""),
			// Sber
			SberMerchantLogin: config.GetEnv("SBER_MERCHANT_LOGIN", ""),
			SberSecretKey:     config.GetEnv("SBER_SECRET_KEY", ""),
		},

		PaymentReturnURL:     config.GetEnv("PAYMENT_RETURN_URL", "http://localhost:3000/bookings"),
		PlatformFeeBPS:       config.GetEnvInt("PLATFORM_FEE_BPS", 1500),
		PayoutHoldHours:      config.GetEnvInt("PAYOUT_HOLD_HOURS", 24),
		InternalServiceToken: config.GetEnv("INTERNAL_SERVICE_TOKEN", ""),
	}
}
