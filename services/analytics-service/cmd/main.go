// analytics-service — sink продуктовой аналитики: durable consumer стрима
// ANALYTICS (analytics.web с фронтенда через api-gateway) → Postgres
// analytics_db.events. Grafana читает витрину через read-only датасорс.
// gRPC-сервера нет — только NATS-консьюмер и метрики.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/postgres"

	"github.com/tienlao/agregator/services/analytics-service/config"
	"github.com/tienlao/agregator/services/analytics-service/internal/events"
	"github.com/tienlao/agregator/services/analytics-service/internal/repository"
)

func main() {
	log := logger.New("analytics-service")

	cfg := config.Load()
	if err := cfg.Postgres.Validate(); err != nil {
		log.Fatal().Err(err).Msg("invalid postgres config")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pgPool, err := postgres.NewPool(ctx, cfg.Postgres.DSN())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}
	defer pgPool.Close()
	log.Info().Msg("connected to postgres")

	m := metrics.New("analytics-service")
	m.RegisterPgxPool(pgPool)
	go func() {
		addr := metrics.AddrFromEnv()
		log.Info().Str("addr", addr).Msg("metrics listener starting")
		if err := m.Serve(ctx, addr, pgPool.Ping); err != nil {
			log.Error().Err(err).Msg("metrics listener failed")
		}
	}()

	nc, js, err := natsutil.Connect(cfg.NATSURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to nats")
	}
	defer nc.Close()
	log.Info().Msg("connected to nats")

	// Стрим создаёт и гейтвей; дублирование идемпотентно и спасает при
	// старте analytics-service раньше гейтвея.
	if err := natsutil.EnsureStreamMaxAge(js, events.StreamName, []string{"analytics.>"}, 30*24*time.Hour); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure ANALYTICS stream")
	}

	repo := repository.NewEventRepo(pgPool)
	sub := events.NewSubscriber(js, repo, log, m)
	if err := sub.SubscribeWebEvents(); err != nil {
		log.Fatal().Err(err).Msg("failed to subscribe to analytics.web")
	}
	log.Info().Msg("subscribed to analytics.web (durable analytics-web-sink)")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Info().Str("signal", sig.String()).Msg("shutting down")

	// Drain дожидается in-flight хендлеров (DrainTimeout в natsutil.Connect).
	if err := nc.Drain(); err != nil {
		log.Warn().Err(err).Msg("nats drain error")
	}
	cancel()
	log.Info().Msg("analytics-service stopped")
}
