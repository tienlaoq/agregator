package config

import (
	"fmt"
	"strings"

	"github.com/tienlao/agregator/pkg/config"
)

type Config struct {
	GRPCPort           string
	Postgres           config.PostgresConfig
	NATSURL            string
	PaymentServiceAddr string
	// InternalServiceToken is the shared secret injected into every outbound
	// gRPC call to payment-service as the "x-service-token" metadata header.
	// Must match INTERNAL_SERVICE_TOKEN in payment-service.  Required in production.
	InternalServiceToken string
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "master_db")
	return Config{
		GRPCPort:             config.GetEnv("GRPC_PORT", "50057"),
		Postgres:             pg,
		NATSURL:              config.GetEnv("NATS_URL", "nats://localhost:4222"),
		PaymentServiceAddr:   config.GetEnv("PAYMENT_SERVICE_ADDR", "localhost:50056"),
		InternalServiceToken: config.GetEnv("INTERNAL_SERVICE_TOKEN", ""),
	}
}

// Validate checks all required config fields so the service fails fast at
// startup rather than on the first request that hits a misconfigured dependency.
func (c Config) Validate() error {
	var errs []string
	if err := c.Postgres.Validate(); err != nil {
		errs = append(errs, err.Error())
	}
	if strings.TrimSpace(c.NATSURL) == "" {
		errs = append(errs, "NATS_URL is required")
	}
	if strings.TrimSpace(c.PaymentServiceAddr) == "" {
		errs = append(errs, "PAYMENT_SERVICE_ADDR is required")
	}
	if strings.TrimSpace(c.GRPCPort) == "" {
		errs = append(errs, "GRPC_PORT is required")
	}
	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
