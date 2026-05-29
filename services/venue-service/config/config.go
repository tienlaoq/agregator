package config

import (
	"time"

	"github.com/tienlao/agregator/pkg/config"
)

type Config struct {
	GRPCPort string
	Postgres config.PostgresConfig
	Redis    config.RedisConfig
	NATS     config.NATSConfig
	// VenueCacheTTL controls how long individual venue cards are cached in Redis.
	// Env: VENUE_CACHE_TTL (e.g. "10m", "1h"). Default: 10m.
	VenueCacheTTL time.Duration
	// SearchCacheTTL controls how long list/search results are cached in Redis.
	// Env: SEARCH_CACHE_TTL (e.g. "2m", "30s"). Default: 2m.
	SearchCacheTTL time.Duration
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "venue_db")

	return Config{
		GRPCPort:       config.GetEnv("GRPC_PORT", "50053"),
		Postgres:       pg,
		Redis:          config.NewRedisConfig(),
		NATS:           config.NewNATSConfig(),
		VenueCacheTTL:  config.GetEnvDuration("VENUE_CACHE_TTL", 10*time.Minute),
		SearchCacheTTL: config.GetEnvDuration("SEARCH_CACHE_TTL", 2*time.Minute),
	}
}
