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
	return Config{
		GRPCPort: config.GetEnv("GRPC_PORT", "50052"),
		Postgres: config.PostgresConfig{
			Host:     config.GetEnv("PG_HOST", "localhost"),
			Port:     config.GetEnvInt("PG_PORT", 5432),
			User:     config.GetEnv("PG_USER", "banya"),
			Password: config.GetEnv("PG_PASSWORD", "banya_secret"),
			DBName:   config.GetEnv("PG_DB", "user_db"),
			SSLMode:  config.GetEnv("PG_SSLMODE", "disable"),
		},
		Redis: config.NewRedisConfig(),
		NATS:  config.NewNATSConfig(),
	}
}
