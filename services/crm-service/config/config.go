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
	// NATSURL — JetStream connection for the guest-projection consumer
	// (subscribes to booking.* on the BOOKINGS stream). See docs/CRM_DESIGN.md §4.
	NATSURL string
	// VIPSpentThreshold — guest total_spent (same unit as booking total_price)
	// at/above which the VIP segment applies. 0 disables the VIP segment.
	VIPSpentThreshold int64
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "venue_db")

	return Config{
		GRPCPort:          config.GetEnv("GRPC_PORT", "50059"),
		Postgres:          pg,
		NATSURL:           config.GetEnv("NATS_URL", "nats://localhost:4222"),
		VIPSpentThreshold: int64(config.GetEnvInt("CRM_VIP_SPENT_THRESHOLD", 30000)),
	}
}
