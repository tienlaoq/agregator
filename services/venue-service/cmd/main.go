package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/postgres"
	pkgredis "github.com/tienlao/agregator/pkg/redis"

	"github.com/tienlao/agregator/services/venue-service/config"
	delivery "github.com/tienlao/agregator/services/venue-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/venue-service/internal/events"
	"github.com/tienlao/agregator/services/venue-service/internal/repository"
	"github.com/tienlao/agregator/services/venue-service/internal/telegram"
	"github.com/tienlao/agregator/services/venue-service/internal/usecase"
)

func main() {
	log := logger.New("venue-service")

	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pgPool, err := postgres.NewPool(ctx, cfg.Postgres.DSN())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}
	defer pgPool.Close()
	log.Info().Msg("connected to postgres")

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

	grpcServer := grpc.NewServer()
	venuev1.RegisterVenueServiceServer(grpcServer, delivery.NewServer(uc, publisher, tgNotifier))

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
