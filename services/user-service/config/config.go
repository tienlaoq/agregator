package config

import (
	"github.com/tienlao/agregator/pkg/config"
)

type Config struct {
	GRPCPort string
	Postgres config.PostgresConfig
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "user_db")
	return Config{
		GRPCPort: config.GetEnv("GRPC_PORT", "50052"),
		Postgres: pg,
	}
}
