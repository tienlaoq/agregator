package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"google.golang.org/grpc"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	pkgmail "github.com/tienlao/agregator/pkg/mail"
	"github.com/tienlao/agregator/pkg/postgres"
	pkgredis "github.com/tienlao/agregator/pkg/redis"
	pkgtelegram "github.com/tienlao/agregator/pkg/telegram"

	"github.com/tienlao/agregator/services/auth-service/config"
	delivery "github.com/tienlao/agregator/services/auth-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/auth-service/internal/passwordmail"
	"github.com/tienlao/agregator/services/auth-service/internal/repository"
	"github.com/tienlao/agregator/services/auth-service/internal/usecase"
)

func main() {
	log := logger.New("auth-service")

	cfg := config.Load()
	if err := cfg.Postgres.Validate(); err != nil {
		log.Fatal().Err(err).Msg("invalid postgres config")
	}
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

	userConn, err := grpc.NewClient(cfg.UserServiceAddr, grpcutil.InsecureDialOptions()...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial user-service")
	}
	defer userConn.Close()
	userClient := userv1.NewUserServiceClient(userConn)
	log.Info().Str("addr", cfg.UserServiceAddr).Msg("user-service client ready")

	credRepo := repository.NewCredentialRepo(pgPool)
	tokenRepo := repository.NewRefreshTokenRepo(pgPool)
	resetRepo := repository.NewPasswordResetRepo(pgPool)

	smtpSender := pkgmail.NewSenderFromEnv()
	frontendURL := strings.TrimSpace(os.Getenv("FRONTEND_URL"))
	resetMail := passwordmail.New(smtpSender, frontendURL)
	if !smtpSender.Enabled() {
		log.Warn().Msg("SMTP not configured: password reset emails will not be sent (set SMTP_HOST, SMTP_FROM, SMTP_USER, SMTP_PASSWORD)")
	}

	tgClient := pkgtelegram.NewClient(os.Getenv("TELEGRAM_BOT_TOKEN"), os.Getenv("TELEGRAM_CHAT_ID"))
	if !tgClient.Enabled() {
		log.Warn().Msg("Telegram registration notify disabled: set TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID (e.g. deploy/.env)")
	} else {
		log.Info().Msg("Telegram registration notify enabled for roles master, venue_owner")
	}

	uc := usecase.NewAuthUseCase(
		credRepo, tokenRepo,
		resetRepo, resetMail, cfg.PasswordResetTTL,
		userClient,
		cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL,
		tgClient, frontendURL, log,
	)

	grpcServer := grpc.NewServer(grpcutil.ServerOptions()...)
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
