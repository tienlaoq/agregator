package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/postgres"
	pkgredis "github.com/tienlao/agregator/pkg/redis"

	"github.com/tienlao/agregator/services/venue-service/config"
	"github.com/tienlao/agregator/services/venue-service/internal/dbmigrate"
	delivery "github.com/tienlao/agregator/services/venue-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/venue-service/internal/events"
	"github.com/tienlao/agregator/services/venue-service/internal/repository"
	"github.com/tienlao/agregator/services/venue-service/internal/telegram"
	"github.com/tienlao/agregator/services/venue-service/internal/usecase"
)

func main() {
	log := logger.New("venue-service")

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

	migDir := resolveMigrationsDir()
	if err := dbmigrate.Up(ctx, cfg.Postgres.DSN(), migDir, log); err != nil {
		log.Fatal().Err(err).Str("dir", migDir).Msg("postgres migrations failed")
	}

	m := metrics.New("venue-service")
	m.RegisterPgxPool(pgPool)
	go func() {
		addr := metrics.AddrFromEnv()
		log.Info().Str("addr", addr).Msg("metrics listener starting")
		if err := m.Serve(ctx, addr, pgPool.Ping); err != nil {
			log.Error().Err(err).Msg("metrics listener failed")
		}
	}()

	rdb, err := pkgredis.NewClient(ctx, cfg.Redis)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer rdb.Close()
	log.Info().Msg("connected to redis")

	nc, js, err := natsutil.Connect(cfg.NATS.URL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to nats")
	}
	defer nc.Close()
	log.Info().Msg("connected to nats")

	if err := natsutil.EnsureStream(js, "VENUES", []string{"venue.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure VENUES stream")
	}
	// REVIEWS принадлежит review-service; здесь гарантируем его существование на
	// случай, если venue-service стартует раньше — иначе durable-консьюмер не
	// сможет привязаться. EnsureStream идемпотентен.
	if err := natsutil.EnsureStream(js, "REVIEWS", []string{"review.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure REVIEWS stream")
	}

	if os.Getenv("FRONTEND_URL") == "" {
		log.Warn().Msg("FRONTEND_URL not set: generated links (notifications, admin) will be broken")
	}

	repo := repository.NewVenueRepo(pgPool)
	uc := usecase.NewVenueUseCaseWithConfig(repo, rdb, log, usecase.Config{
		VenueCacheTTL:  cfg.VenueCacheTTL,
		SearchCacheTTL: cfg.SearchCacheTTL,
	})
	publisher := events.NewPublisher(js)

	// review-service client + подписчик на review.created: пересчитываем
	// denormalized-рейтинг площадок при появлении новых отзывов.
	dialOpts, err := grpcutil.DialOptions()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC client transport config error")
	}
	reviewConn, err := grpc.NewClient(cfg.ReviewServiceAddr, dialOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial review-service")
	}
	defer reviewConn.Close()
	reviewClient := reviewv1.NewReviewServiceClient(reviewConn)
	log.Info().Str("addr", cfg.ReviewServiceAddr).Msg("review-service client ready")

	reviewSub := events.NewSubscriber(js, reviewClient, uc, log, m)
	if err := reviewSub.SubscribeReviewEvents(); err != nil {
		log.Fatal().Err(err).Msg("failed to subscribe to review events")
	}
	log.Info().Msg("subscribed to review.created events")

	tgNotifier := telegram.NewNotifier(
		os.Getenv("TELEGRAM_BOT_TOKEN"),
		os.Getenv("TELEGRAM_CHAT_ID"),
		os.Getenv("FRONTEND_URL"),
	)
	if !tgNotifier.Enabled() {
		log.Warn().Msg("Telegram notifications disabled: set TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID (e.g. in deploy/.env)")
	}

	srvOpts, err := grpcutil.ServerOptionsFromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC server transport config error")
	}
	srvOpts = append(srvOpts, grpc.ChainUnaryInterceptor(
		m.UnaryServerInterceptor(), // outermost: observes codes after PgError mapping
		grpcutil.SafePgErrorUnaryInterceptor(),
	))
	grpcServer := grpc.NewServer(srvOpts...)
	venuev1.RegisterVenueServiceServer(grpcServer, delivery.NewServer(uc, publisher, tgNotifier, log))

	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	const pingTimeout = 3 * time.Second

	go func() {
		const interval = 10 * time.Second
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
				return
			case <-ticker.C:
				pingCtx, pingCancel := context.WithTimeout(ctx, pingTimeout)
				err := pgPool.Ping(pingCtx)
				pingCancel()
				if err != nil {
					log.Warn().Err(err).Msg("health: postgres ping failed, marking NOT_SERVING")
					healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
				} else {
					healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
				}
			}
		}
	}()

	initPingCtx, initPingCancel := context.WithTimeout(ctx, pingTimeout)
	if err := pgPool.Ping(initPingCtx); err != nil {
		log.Warn().Err(err).Msg("health: initial postgres ping failed, starting NOT_SERVING")
	} else {
		healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	}
	initPingCancel()

	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to listen")
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Info().Str("port", cfg.GRPCPort).Msg("gRPC server starting")
		serveErr <- grpcServer.Serve(lis)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("shutting down")
	case err := <-serveErr:
		log.Error().Err(err).Msg("gRPC server exited")
	}

	grpcServer.GracefulStop()
	cancel()
	log.Info().Msg("venue-service stopped")
}

func resolveMigrationsDir() string {
	if d := os.Getenv("VENUE_MIGRATIONS_DIR"); d != "" {
		return d
	}
	for _, d := range []string{"/migrations", "migrations"} {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return d
		}
	}
	if wd, err := os.Getwd(); err == nil {
		p := filepath.Join(wd, "migrations")
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	return "migrations"
}
