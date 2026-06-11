package config

import "github.com/tienlao/agregator/pkg/config"

type Config struct {
	Postgres config.PostgresConfig
	NATSURL  string
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "analytics_db")
	return Config{
		Postgres: pg,
		NATSURL:  config.GetEnv("NATS_URL", "nats://localhost:4222"),
	}
}
