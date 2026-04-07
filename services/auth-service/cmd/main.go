package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/postgres"
	pkgredis "github.com/tienlao/agregator/pkg/redis"

	"github.com/tienlao/agregator/services/auth-service/config"
	delivery "github.com/tienlao/agregator/services/auth-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/auth-service/internal/repository"
	"github.com/tienlao/agregator/services/auth-service/internal/usecase"
)

func main() {
	log := logger.New("auth-service")

	cfg := config.Load()
	if cfg.JWTSecret == "" {
		log.Fatal().Msg("JWT_SECRET is required")
	}

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

	userConn, err := grpc.NewClient(cfg.UserServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial user-service")
	}
	defer userConn.Close()
	userClient := userv1.NewUserServiceClient(userConn)
	log.Info().Str("addr", cfg.UserServiceAddr).Msg("user-service client ready")

	credRepo := repository.NewCredentialRepo(pgPool)
	tokenRepo := repository.NewRefreshTokenRepo(pgPool)

	uc := usecase.NewAuthUseCase(
		credRepo, tokenRepo, userClient,
		cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL,
	)

	grpcServer := grpc.NewServer()
	authv1.RegisterAuthServiceServer(grpcServer, delivery.NewServer(uc))

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
	log.Info().Msg("auth-service stopped")
}
