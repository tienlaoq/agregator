package config

import (
	"github.com/tienlao/agregator/pkg/config"
)

// Config holds runtime configuration for crm-service.
//
// Strangler-fig phase B: this service shares the venue Postgres database with
// venue-service. PG_DB defaults to "venue_db". When CRM scope grows and the
// data is migrated to its own database, change the default here and add the
// backfill step. See docs/TECH_DEBT.md.
type Config struct {
	GRPCPort string
	Postgres config.PostgresConfig
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "venue_db")

	return Config{
		GRPCPort: config.GetEnv("GRPC_PORT", "50059"),
		Postgres: pg,
	}
}
