package config

import (
	"github.com/tienlao/agregator/pkg/config"
)

type Config struct {
	GRPCPort          string
	Postgres          config.PostgresConfig
	Redis             config.RedisConfig
	NATSURL           string
	YooKassaShopID    string
	YooKassaSecretKey string
	PaymentReturnURL  string
	// PlatformFeeBPS is the marketplace commission in basis points (1500 = 15%).
	PlatformFeeBPS int
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "payment_db")

	return Config{
		GRPCPort:          config.GetEnv("GRPC_PORT", "50056"),
		Postgres:          pg,
		Redis:             config.NewRedisConfig(),
		NATSURL:           config.GetEnv("NATS_URL", "nats://localhost:4222"),
		YooKassaShopID:    config.GetEnv("YOOKASSA_SHOP_ID", ""),
		YooKassaSecretKey: config.GetEnv("YOOKASSA_SECRET_KEY", ""),
		PaymentReturnURL:  config.GetEnv("PAYMENT_RETURN_URL", "http://localhost:3000/bookings"),
		PlatformFeeBPS:    config.GetEnvInt("PLATFORM_FEE_BPS", 1500),
	}
}
