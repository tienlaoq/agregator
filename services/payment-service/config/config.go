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
	// ProviderMock is the no-network stand-in used until a real bank acquiring
	// gateway is wired up. It is rejected in production by Validate.
	ProviderMock  ProviderName = "mock"
	ProviderTBank ProviderName = "tbank"
	ProviderSber  ProviderName = "sber"
	ProviderAlfa  ProviderName = "alfa"
)

// ProviderConfig holds per-provider credentials loaded from environment.
// Only the fields for the active provider need to be set; unused fields are
// ignored so secrets for inactive providers can be omitted entirely.
//
// The mock provider takes no credentials. Add the chosen acquiring bank's
// fields here when its implementation lands.
type ProviderConfig struct {
	// TBank — TBANK_TERMINAL_KEY / TBANK_SECRET_KEY
	TBankTerminalKey string
	TBankSecretKey   string

	// Sber — SBER_MERCHANT_LOGIN / SBER_SECRET_KEY
	SberMerchantLogin string
	SberSecretKey     string

	// Alfa — ALFA_USERNAME / ALFA_PASSWORD (RBS technical "…-api" user),
	// ALFA_GATEWAY_URL selects the gateway host (prod vs the rbsuat test host).
	AlfaUsername   string
	AlfaPassword   string
	AlfaGatewayURL string
}

type Config struct {
	GRPCPort string
	Postgres config.PostgresConfig
	NATSURL  string

	// Provider selects the active payment gateway at startup.
	// Set via PAYMENT_PROVIDER env var (default: "mock").
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
	// PayoutMinKopecks is the minimum available balance the scheduler will pay
	// out; smaller balances roll over to the next cycle.  Default 100000 (1000 ₽)
	// so partners are not spammed with tiny transfers that cost more in fees than
	// they are worth.
	PayoutMinKopecks int64
	// PayoutWeekday pins weekly payouts to one weekday (0=Sunday … 6=Saturday).
	// Default 3 (Wednesday): a mid-week banking day leaves a full runway to
	// resolve failed transfers before the weekend, and avoids the holiday
	// clustering around Mondays.  Set to -1 to disable weekly batching and pay
	// any ripe balance on every tick.
	PayoutWeekday int
	// PayoutTimezone is the IANA location in which PayoutWeekday is interpreted,
	// so "Wednesday" means the business day, not the server's UTC day. Defaults
	// to Europe/Moscow to match the platform's booking timezone. An unknown
	// location falls back to UTC (logged at startup).
	PayoutTimezone string
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
//   - ActiveProvider must be a known, real gateway. The mock provider is
//     rejected outright: it issues fake payment URLs and fake "succeeded"
//     events, which must never reach real users.
//   - Required credentials for the active provider must be non-empty; a missing
//     credential would otherwise leave the provider unauthenticated against the
//     live gateway.
//   - InternalServiceToken must be set (same rule as before — kept here for
//     symmetry so all production invariants are in one place).
//
// In non-production (dev / CI):
//   - Only the ActiveProvider name is validated; the mock provider is allowed
//     and needs no credentials.
func (c *Config) Validate(env string) error {
	isProduction := strings.EqualFold(strings.TrimSpace(env), "production")

	switch c.ActiveProvider {
	case ProviderMock, ProviderTBank, ProviderSber, ProviderAlfa:
		// known — ok
	default:
		return fmt.Errorf("unknown PAYMENT_PROVIDER %q: must be mock, tbank, sber, or alfa", c.ActiveProvider)
	}

	if !isProduction {
		return nil
	}

	// ── Production-only checks ───────────────────────────────────────────────
	// The mock provider must never run in production: it returns fake payment
	// URLs and emits fake "succeeded" events. Fail fast so a real acquiring
	// gateway must be implemented and selected before go-live.
	if c.ActiveProvider == ProviderMock {
		return fmt.Errorf("production: PAYMENT_PROVIDER=mock is not allowed — select a real acquiring gateway")
	}

	// A missing credential would leave the active provider unauthenticated
	// against the live gateway.  Fail fast so the deploy never starts.
	switch c.ActiveProvider {
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
	case ProviderAlfa:
		if strings.TrimSpace(c.Provider.AlfaUsername) == "" {
			return fmt.Errorf("production: ALFA_USERNAME must be set when PAYMENT_PROVIDER=alfa")
		}
		if strings.TrimSpace(c.Provider.AlfaPassword) == "" {
			return fmt.Errorf("production: ALFA_PASSWORD must be set when PAYMENT_PROVIDER=alfa")
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

		ActiveProvider: ProviderName(config.GetEnv("PAYMENT_PROVIDER", string(ProviderMock))),

		Provider: ProviderConfig{
			// TBank
			TBankTerminalKey: config.GetEnv("TBANK_TERMINAL_KEY", ""),
			TBankSecretKey:   config.GetEnv("TBANK_SECRET_KEY", ""),
			// Sber
			SberMerchantLogin: config.GetEnv("SBER_MERCHANT_LOGIN", ""),
			SberSecretKey:     config.GetEnv("SBER_SECRET_KEY", ""),
			// Alfa
			AlfaUsername:   config.GetEnv("ALFA_USERNAME", ""),
			AlfaPassword:   config.GetEnv("ALFA_PASSWORD", ""),
			AlfaGatewayURL: config.GetEnv("ALFA_GATEWAY_URL", "https://pay.alfabank.ru/payment/rest/"),
		},

		PaymentReturnURL:     config.GetEnv("PAYMENT_RETURN_URL", "http://localhost:3000/bookings"),
		PlatformFeeBPS:       config.GetEnvInt("PLATFORM_FEE_BPS", 1500),
		PayoutHoldHours:      config.GetEnvInt("PAYOUT_HOLD_HOURS", 24),
		PayoutMinKopecks:     int64(config.GetEnvInt("PAYOUT_MIN_KOPECKS", 100000)),
		PayoutWeekday:        config.GetEnvInt("PAYOUT_WEEKDAY", 3), // 3 = Wednesday
		PayoutTimezone:       config.GetEnv("PAYOUT_TIMEZONE", "Europe/Moscow"),
		InternalServiceToken: config.GetEnv("INTERNAL_SERVICE_TOKEN", ""),
	}
}
