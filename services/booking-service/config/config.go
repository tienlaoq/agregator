package config

import (
	"github.com/tienlao/agregator/pkg/config"
)

type Config struct {
	GRPCPort           string
	Postgres           config.PostgresConfig
	Redis              config.RedisConfig
	NATSURL            string
	VenueServiceAddr   string
	PaymentServiceAddr string
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "booking_db")

	return Config{
		GRPCPort:           config.GetEnv("GRPC_PORT", "50054"),
		Postgres:           pg,
		Redis:              config.NewRedisConfig(),
		NATSURL:            config.GetEnv("NATS_URL", "nats://localhost:4222"),
		VenueServiceAddr:   config.GetEnv("VENUE_SERVICE_ADDR", "localhost:50053"),
		PaymentServiceAddr: config.GetEnv("PAYMENT_SERVICE_ADDR", "localhost:50056"),
	}
}
