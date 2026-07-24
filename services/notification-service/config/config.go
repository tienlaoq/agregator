package config

import "github.com/tienlao/agregator/pkg/config"

type Config struct {
	GRPCPort string
	Postgres config.PostgresConfig

	// Firebase service-account JSON for FCM push. Provide EITHER a file path
	// (FCM_CREDENTIALS_FILE, preferred for secret mounts) OR the raw JSON inline
	// (FCM_CREDENTIALS_JSON). Both empty ⇒ mobile push disabled (bell still works).
	FCMCredentialsFile string
	FCMCredentialsJSON string
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "notification_db")
	return Config{
		GRPCPort:           config.GetEnv("GRPC_PORT", "50060"),
		Postgres:           pg,
		FCMCredentialsFile: config.GetEnv("FCM_CREDENTIALS_FILE", ""),
		FCMCredentialsJSON: config.GetEnv("FCM_CREDENTIALS_JSON", ""),
	}
}
