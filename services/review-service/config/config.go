package config

import (
	"time"

	"github.com/tienlao/agregator/pkg/config"
)

type Config struct {
	GRPCPort           string
	Postgres           config.PostgresConfig
	Redis              config.RedisConfig
	NATSURL            string
	BookingServiceAddr string
	VenueServiceAddr   string
	MasterServiceAddr  string
	// Outbox worker tuning — override via OUTBOX_BATCH_SIZE / OUTBOX_INTERVAL.
	OutboxBatchSize int
	OutboxInterval  time.Duration
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "review_db")

	return Config{
		GRPCPort:           config.GetEnv("GRPC_PORT", "50055"),
		Postgres:           pg,
		Redis:              config.NewRedisConfig(),
		NATSURL:            config.GetEnv("NATS_URL", "nats://localhost:4222"),
		BookingServiceAddr: config.GetEnv("BOOKING_SERVICE_ADDR", "localhost:50054"),
		VenueServiceAddr:   config.GetEnv("VENUE_SERVICE_ADDR", "localhost:50053"),
		MasterServiceAddr:  config.GetEnv("MASTER_SERVICE_ADDR", "localhost:50057"),
		OutboxBatchSize:    config.GetEnvInt("OUTBOX_BATCH_SIZE", 50),
		OutboxInterval:     config.GetEnvDuration("OUTBOX_INTERVAL", 5*time.Second),
	}
}
