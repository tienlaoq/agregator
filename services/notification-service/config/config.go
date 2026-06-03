package config

import "github.com/tienlao/agregator/pkg/config"

type Config struct {
	GRPCPort string
	Postgres config.PostgresConfig
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "notification_db")
	return Config{
		GRPCPort: config.GetEnv("GRPC_PORT", "50060"),
		Postgres: pg,
	}
}
