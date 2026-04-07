package config

import (
	"github.com/tienlao/agregator/pkg/config"
)

type Config struct {
	GRPCPort string
	Postgres config.PostgresConfig
	Redis    config.RedisConfig
	NATS     config.NATSConfig
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "venue_db")

	return Config{
		GRPCPort: config.GetEnv("GRPC_PORT", "50053"),
		Postgres: pg,
		Redis:    config.NewRedisConfig(),
		NATS:     config.NewNATSConfig(),
	}
}
