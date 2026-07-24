package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	notificationv1 "github.com/tienlao/agregator/gen/go/notification/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/pkg/postgres"
	"github.com/tienlao/agregator/services/notification-service/config"
	delivery "github.com/tienlao/agregator/services/notification-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/notification-service/internal/fcm"
	"github.com/tienlao/agregator/services/notification-service/internal/repository"
	"github.com/tienlao/agregator/services/notification-service/internal/usecase"
)

// isProductionEnv mirrors the other services: ENV=production, or a non-loopback
// PG_HOST, is treated as a real deployment where service auth is mandatory.
func isProductionEnv(pgHost string) bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production") {
		return true
	}
	h := strings.TrimSpace(strings.ToLower(pgHost))
	return h != "" && h != "localhost" && h != "127.0.0.1" && h != "::1"
}

// loadFCMCredentials resolves the Firebase service-account JSON from either a
// file path or an inline env var (file wins). Returns nil when neither is set;
// a file that is configured but unreadable is fatal so the misconfig surfaces.
func loadFCMCredentials(cfg config.Config, log zerolog.Logger) []byte {
	if p := strings.TrimSpace(cfg.FCMCredentialsFile); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			log.Fatal().Err(err).Str("path", p).Msg("cannot read FCM_CREDENTIALS_FILE")
		}
		return b
	}
	if j := strings.TrimSpace(cfg.FCMCredentialsJSON); j != "" {
		return []byte(j)
	}
	return nil
}

func main() {
	log := logger.New("notification-service")
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

	m := metrics.New("notification-service")
	m.RegisterPgxPool(pgPool)
	go func() {
		addr := metrics.AddrFromEnv()
		log.Info().Str("addr", addr).Msg("metrics listener starting")
		if err := m.Serve(ctx, addr, pgPool.Ping); err != nil {
			log.Error().Err(err).Msg("metrics listener failed")
		}
	}()

	repo := repository.New(pgPool)
	// Advisory schema check (not a startup gate): a missing table is logged so
	// ops can act, but queries fail loudly anyway if the schema is truly broken.
	if err := repo.EnsureSchema(ctx); err != nil {
		log.Warn().Err(err).Msg("notification-service schema check: migrations may be pending — service will start anyway")
	}

	// Mobile push (FCM): optional. A nil Pusher disables it — the in-app bell
	// still works. Bad credentials are a hard failure so a misconfigured deploy
	// is visible instead of silently dropping push.
	var pusher usecase.Pusher
	if sender, err := fcm.New(ctx, loadFCMCredentials(cfg, log)); err != nil {
		log.Fatal().Err(err).Msg("invalid FCM credentials")
	} else if sender != nil {
		pusher = sender // only assign when non-nil, else the interface holds a typed-nil
		log.Info().Msg("mobile push enabled (FCM)")
	} else {
		log.Warn().Msg("FCM credentials not set — mobile push disabled (in-app bell still works)")
	}

	uc := usecase.New(repo, pusher, log)

	// Service-to-service token auth: mandatory in production, optional (no-op
	// interceptor) for local dev / CI so the service can run standalone.
	serviceToken := strings.TrimSpace(os.Getenv("INTERNAL_SERVICE_TOKEN"))
	if serviceToken == "" {
		if isProductionEnv(cfg.Postgres.Host) {
			log.Fatal().Msg("notification-service: INTERNAL_SERVICE_TOKEN must be set in production (ENV=production or remote PG_HOST)")
		}
		log.Warn().Msg("notification-service: INTERNAL_SERVICE_TOKEN not set — service-to-service auth disabled (dev/CI only)")
	}
	srvOpts, err := grpcutil.ServerOptionsFromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC server transport config error")
	}
	srvOpts = append(srvOpts, grpc.ChainUnaryInterceptor(
		m.UnaryServerInterceptor(), // outermost: observes codes after PgError mapping
		grpcutil.PgErrorUnaryInterceptor(),
		grpcutil.ServiceTokenServerInterceptor(serviceToken),
	))
	grpcServer := grpc.NewServer(srvOpts...)
	notificationv1.RegisterNotificationServiceServer(grpcServer, delivery.NewServer(uc, log))

	// Health service: k8s readiness/liveness probes via grpc_health_v1.
	// Starts NOT_SERVING and flips to SERVING only after a Postgres ping
	// succeeds; a background goroutine re-checks every 10 s and flips back to
	// NOT_SERVING if the pool becomes unavailable, so k8s pulls the pod from the
	// load-balancer without a restart.
	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	// pingTimeout bounds each Postgres liveness check; keep it shorter than the
	// ticker interval so ticks never queue up.
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
