package config

import (
	"time"

	"github.com/tienlao/agregator/pkg/config"
)

type Config struct {
	GRPCPort         string
	JWTSecret        string
	JWTAccessTTL     time.Duration
	JWTRefreshTTL    time.Duration
	PasswordResetTTL time.Duration
	Postgres         config.PostgresConfig
	Redis            config.RedisConfig
	NATSURL          string
	UserServiceAddr  string
}

func Load() Config {
	accessTTL, err := time.ParseDuration(config.GetEnv("JWT_ACCESS_TTL", "15m"))
	if err != nil {
		accessTTL = 15 * time.Minute
	}
	refreshTTL, err := time.ParseDuration(config.GetEnv("JWT_REFRESH_TTL", "720h"))
	if err != nil {
		refreshTTL = 720 * time.Hour
	}

	resetTTL, err := time.ParseDuration(config.GetEnv("PASSWORD_RESET_TTL", "1h"))
	if err != nil || resetTTL <= 0 {
		resetTTL = time.Hour
	}

	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "auth_db")

	return Config{
		GRPCPort:         config.GetEnv("GRPC_PORT", "50051"),
		JWTSecret:        config.GetEnv("JWT_SECRET", ""),
		JWTAccessTTL:     accessTTL,
		JWTRefreshTTL:    refreshTTL,
		PasswordResetTTL: resetTTL,
		Postgres:         pg,
		Redis:            config.NewRedisConfig(),
		NATSURL:          config.GetEnv("NATS_URL", "nats://localhost:4222"),
		UserServiceAddr:  config.GetEnv("USER_SERVICE_ADDR", "localhost:50052"),
	}
}
