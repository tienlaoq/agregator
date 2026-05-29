package config

import "github.com/tienlao/agregator/pkg/config"

type Config struct {
	GRPCPort string
	Postgres config.PostgresConfig
	// NATSUrl is optional; when empty, NATS publishing is skipped and the
	// service operates without async events (useful for tests / local dev).
	NATSUrl string
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "chat_db")
	return Config{
		GRPCPort: config.GetEnv("GRPC_PORT", "50058"),
		Postgres: pg,
		NATSUrl:  config.GetEnv("NATS_URL", ""),
	}
}
