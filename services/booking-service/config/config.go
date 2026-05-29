package config

import (
	"strconv"
	"strings"
	"time"

	"github.com/tienlao/agregator/pkg/config"
)

type Config struct {
	GRPCPort           string
	Postgres           config.PostgresConfig
	Redis              config.RedisConfig
	NATSURL            string
	VenueServiceAddr   string
	PaymentServiceAddr string
	CRMServiceAddr     string
	// VisitTimeZone — IANA TZ для date+time_to брони (окончание визита → completed).
	VisitTimeZone string
	// CancelDeadlineHours — за сколько часов до начала визита пользователь
	// может отменить бронь самостоятельно. 0 отключает ограничение (не рекомендуется).
	// Читается из CANCEL_DEADLINE_HOURS, по умолчанию 2.
	CancelDeadlineHours int
	// CursorHMACKey — секрет для подписи keyset-курсоров (HMAC-SHA256).
	// Читается из CURSOR_HMAC_KEY. Если не задан — используется небезопасный
	// fallback-ключ и при старте пишется предупреждение в лог.
	// В production обязательно задать непустое значение.
	CursorHMACKey string
	// InternalServiceToken is the shared secret injected into every outbound
	// gRPC call to payment-service as the "x-service-token" metadata header.
	// Must match INTERNAL_SERVICE_TOKEN in payment-service.  Required in production.
	InternalServiceToken string
}

func Load() Config {
	pg := config.NewPostgresConfig("PG")
	pg.DBName = config.GetEnv("PG_DB", "booking_db")

	tz := strings.TrimSpace(config.GetEnv("BOOKING_VISIT_TIMEZONE", "Europe/Moscow"))
	if _, err := time.LoadLocation(tz); err != nil {
		tz = "Europe/Moscow"
	}

	cancelDeadlineHours := 2
	if v, err := strconv.Atoi(config.GetEnv("CANCEL_DEADLINE_HOURS", "2")); err == nil && v >= 0 {
		cancelDeadlineHours = v
	}

	return Config{
		GRPCPort:            config.GetEnv("GRPC_PORT", "50054"),
		Postgres:            pg,
		Redis:               config.NewRedisConfig(),
		NATSURL:             config.GetEnv("NATS_URL", "nats://localhost:4222"),
		VenueServiceAddr:    config.GetEnv("VENUE_SERVICE_ADDR", "localhost:50053"),
		PaymentServiceAddr:  config.GetEnv("PAYMENT_SERVICE_ADDR", "localhost:50056"),
		CRMServiceAddr:      config.GetEnv("CRM_SERVICE_ADDR", "localhost:50059"),
		VisitTimeZone:       tz,
		CancelDeadlineHours: cancelDeadlineHours,
		CursorHMACKey:        config.GetEnv("CURSOR_HMAC_KEY", ""),
		InternalServiceToken: config.GetEnv("INTERNAL_SERVICE_TOKEN", ""),
	}
}
