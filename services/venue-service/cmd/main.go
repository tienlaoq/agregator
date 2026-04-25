package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"google.golang.org/grpc"

	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/postgres"
	pkgredis "github.com/tienlao/agregator/pkg/redis"

	"github.com/tienlao/agregator/services/venue-service/config"
	delivery "github.com/tienlao/agregator/services/venue-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/venue-service/internal/dbmigrate"
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

	repo := repository.NewVenueRepo(pgPool)
	uc := usecase.NewVenueUseCase(repo, rdb)
	publisher := events.NewPublisher(js)
	tgNotifier := telegram.NewNotifier(
		os.Getenv("TELEGRAM_BOT_TOKEN"),
		os.Getenv("TELEGRAM_CHAT_ID"),
		os.Getenv("FRONTEND_URL"),
	)
	if !tgNotifier.Enabled() {
		log.Warn().Msg("Telegram notifications disabled: set TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID (e.g. in deploy/.env)")
	}

	grpcServer := grpc.NewServer(grpcutil.ServerOptions()...)
	venuev1.RegisterVenueServiceServer(grpcServer, delivery.NewServer(uc, publisher, tgNotifier, log))

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to listen")
	}

	go func() {
		log.Info().Str("port", cfg.GRPCPort).Msg("gRPC server starting")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatal().Err(err).Msg("gRPC server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Info().Str("signal", sig.String()).Msg("shutting down")

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
