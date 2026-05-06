package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/postgres"
	"github.com/tienlao/agregator/services/chat-service/config"
	delivery "github.com/tienlao/agregator/services/chat-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/chat-service/internal/repository"
	"github.com/tienlao/agregator/services/chat-service/internal/usecase"
)

func main() {
	log := logger.New("chat-service")
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

	repo := repository.New(pgPool)
	if err := repo.EnsureSchema(ctx); err != nil {
		log.Fatal().Err(err).Msg("chat-service schema check failed")
	}
	uc := usecase.New(repo)
	grpcServer := grpc.NewServer(grpcutil.ServerOptions()...)
	chatv1.RegisterChatServiceServer(grpcServer, delivery.NewServer(uc))

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
	<-quit
	log.Info().Msg("shutting down")
	grpcServer.GracefulStop()
	cancel()
}
