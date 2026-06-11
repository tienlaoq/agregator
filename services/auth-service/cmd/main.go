package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	pkgauth "github.com/tienlao/agregator/pkg/auth"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	pkgmail "github.com/tienlao/agregator/pkg/mail"
	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/pkg/postgres"
	pkgtelegram "github.com/tienlao/agregator/pkg/telegram"

	"github.com/tienlao/agregator/services/auth-service/config"
	"github.com/tienlao/agregator/services/auth-service/internal/adapter"
	delivery "github.com/tienlao/agregator/services/auth-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/auth-service/internal/kpi"
	"github.com/tienlao/agregator/services/auth-service/internal/notify"
	"github.com/tienlao/agregator/services/auth-service/internal/passwordmail"
	"github.com/tienlao/agregator/services/auth-service/internal/repository"
	"github.com/tienlao/agregator/services/auth-service/internal/usecase"
	"github.com/tienlao/agregator/services/auth-service/internal/verifymail"
)

func main() {
	log := logger.New("auth-service")

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("invalid config")
	}
	if err := cfg.Postgres.Validate(); err != nil {
		log.Fatal().Err(err).Msg("invalid postgres config")
	}
	if len(cfg.JWTECPrivateKeyPEM) == 0 {
		log.Fatal().Msg("JWT_EC_PRIVATE_KEY or JWT_EC_PRIVATE_KEY_FILE is required (ES256 private key PEM)")
	}
	if cfg.LegacyJWTSecret != "" {
		log.Warn().Msg("JWT_SECRET is set but no longer used — auth-service now uses ES256 (JWT_EC_PRIVATE_KEY). Remove JWT_SECRET from your environment.")
	}

	jwtPrivKey, err := pkgauth.ParseECPrivateKey(cfg.JWTECPrivateKeyPEM)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to parse JWT_EC_PRIVATE_KEY")
	}
	log.Info().Msg("JWT ES256 private key loaded")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Rate-limiting for auth endpoints (login, register, forgot-password,
	// reset-password) is enforced entirely by api-gateway using a Redis-backed
	// sliding-window limiter (see services/api-gateway/internal/middleware/ratelimit.go
	// and cmd/router.go). auth-service itself is an internal gRPC service that
	// is never exposed directly to the internet, so a second rate-limit layer
	// here would only add operational complexity without improving security.
	// Redis is therefore not required by auth-service; if you need per-method
	// gRPC rate-limiting in the future, add it as a gRPC interceptor and pass
	// the Redis client at that point.

	pgPool, err := postgres.NewPool(ctx, cfg.Postgres.DSN())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}
	defer pgPool.Close()
	log.Info().Msg("connected to postgres")

	m := metrics.New("auth-service")
	m.RegisterPgxPool(pgPool)
	kpi.Register(m.Registry())
	go func() {
		addr := metrics.AddrFromEnv()
		log.Info().Str("addr", addr).Msg("metrics listener starting")
		if err := m.Serve(ctx, addr, pgPool.Ping); err != nil {
			log.Error().Err(err).Msg("metrics listener failed")
		}
	}()

	dialOpts, err := grpcutil.DialOptions()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC transport config error")
	}

	userConn, err := grpc.NewClient(cfg.UserServiceAddr, dialOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial user-service")
	}
	defer userConn.Close()
	userClient := adapter.NewUserClientAdapter(userv1.NewUserServiceClient(userConn))
	log.Info().Str("addr", cfg.UserServiceAddr).Msg("user-service client ready")

	credRepo := repository.NewCredentialRepo(pgPool)
	tokenRepo := repository.NewRefreshTokenRepo(pgPool)
	resetRepo := repository.NewPasswordResetRepo(pgPool)
	verifyRepo := repository.NewEmailVerificationRepo(pgPool)

	smtpSender := pkgmail.NewSenderFromEnv()
	resetMail := passwordmail.New(smtpSender, cfg.FrontendURL)
	verifyMail := verifymail.New(smtpSender, cfg.FrontendURL)
	if !smtpSender.Enabled() {
		log.Warn().Msg("SMTP not configured: password reset and email verification emails will not be sent (set SMTP_HOST, SMTP_FROM, SMTP_USER, SMTP_PASSWORD)")
	}

	tgClient := pkgtelegram.NewClient(cfg.TelegramBotToken, cfg.TelegramChatID)
	if !tgClient.Enabled() {
		log.Warn().Msg("Telegram registration notify disabled: set TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID (e.g. deploy/.env)")
	} else {
		log.Info().Msg("Telegram registration notify enabled for roles master, venue_owner")
	}

	// Partner-registration notifications are delivered asynchronously by a
	// dedicated worker so that the Telegram HTTP round-trip (~RTT up to 5 s)
	// does not block the Register gRPC response path.
	notifyWorker := notify.NewTelegramWorker(tgClient, cfg.FrontendURL, log)
	go notifyWorker.Run(ctx)

	uc := usecase.NewAuthUseCase(
		credRepo, tokenRepo,
		resetRepo, resetMail, cfg.PasswordResetTTL,
		verifyRepo, verifyMail, cfg.EmailVerifyTTL,
		userClient,
		jwtPrivKey, cfg.JWTAccessTTL, cfg.JWTRefreshTTL,
		notifyWorker, log,
	)

	srvOpts, err := grpcutil.ServerOptionsFromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC server transport config error")
	}
	srvOpts = append(srvOpts, grpc.ChainUnaryInterceptor(
		m.UnaryServerInterceptor(), // outermost: observes codes after PgError mapping
		grpcutil.PgErrorUnaryInterceptor(),
	))
	grpcServer := grpc.NewServer(srvOpts...)
	authv1.RegisterAuthServiceServer(grpcServer, delivery.NewServer(uc, log))

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

	go runTokenCleanup(ctx, credRepo, tokenRepo, resetRepo, verifyRepo, log)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Info().Str("signal", sig.String()).Msg("shutting down")

	grpcServer.GracefulStop()
	cancel()
	log.Info().Msg("auth-service stopped")
}

// runTokenCleanup periodically deletes expired refresh tokens, consumed/expired
// password-reset tokens, and orphan OAuth credentials (created with no email and
// no password when a provider returned EmailVerified=false, and never promoted).
//
// Interval: 1 h — generous enough that a single replica is never hammering the
// DB, short enough that tables don't accumulate more than ~1 h of stale rows.
// Set TOKEN_CLEANUP_INTERVAL (e.g. "6h") to override without a redeploy.
//
// Orphan min-age: 30 days by default. Set OAUTH_ORPHAN_MIN_AGE (e.g. "72h")
// to override. Orphans younger than this are kept — the user may still return
// and get their email promoted in branch 1 of OAuthLogin.
//
// Each DELETE is covered by indexes added in migrations 004 and 005.
func runTokenCleanup(
	ctx context.Context,
	creds *repository.CredentialRepo,
	tokens *repository.RefreshTokenRepo,
	resets *repository.PasswordResetRepo,
	verifies *repository.EmailVerificationRepo,
	appLog zerolog.Logger,
) {
	interval := time.Hour
	if raw := os.Getenv("TOKEN_CLEANUP_INTERVAL"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			interval = d
		}
	}

	orphanMinAge := 30 * 24 * time.Hour // 30 days
	if raw := os.Getenv("OAUTH_ORPHAN_MIN_AGE"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			orphanMinAge = d
		}
	}

	appLog.Info().Dur("interval", interval).Dur("orphan_min_age", orphanMinAge).
		Msg("token cleanup worker started")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			n1, err1 := tokens.DeleteExpired(cleanupCtx)
			n2, err2 := resets.DeleteExpired(cleanupCtx)
			n3, err3 := creds.DeleteOrphanOAuthAccounts(cleanupCtx, orphanMinAge)
			n4, err4 := verifies.DeleteExpired(cleanupCtx)
			cancel()

			// Log each failure independently so that a double-failure on the same
			// tick is not silently hidden by an else-if chain.
			if err1 != nil {
				appLog.Warn().Err(err1).Msg("token cleanup: DeleteExpired(refresh_tokens) failed")
			}
			if err2 != nil {
				appLog.Warn().Err(err2).Msg("token cleanup: DeleteExpired(password_resets) failed")
			}
			if err3 != nil {
				appLog.Warn().Err(err3).Msg("token cleanup: DeleteOrphanOAuthAccounts failed")
			}
			if err4 != nil {
				appLog.Warn().Err(err4).Msg("token cleanup: DeleteExpired(email_verification_tokens) failed")
			}
			appLog.Debug().
				Int64("refresh_tokens", n1).
				Int64("reset_tokens", n2).
				Int64("orphan_oauth_creds", n3).
				Int64("verification_tokens", n4).
				Msg("token cleanup complete")
		}
	}
}
