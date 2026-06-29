package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/postgres"
	"github.com/tienlao/agregator/services/chat-service/config"
	delivery "github.com/tienlao/agregator/services/chat-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/chat-service/internal/events"
	"github.com/tienlao/agregator/services/chat-service/internal/repository"
	"github.com/tienlao/agregator/services/chat-service/internal/usecase"
)

// isProductionEnv returns true when the process is running in a production-like
// environment.  Two independent signals are checked so that neither alone is a
// single point of misconfiguration:
//
//  1. ENV=production  — explicit opt-in used by our deploy pipeline.
//  2. PG_HOST is not a loopback address — a remote database strongly implies
//     a real deployment, even if ENV was forgotten.
func isProductionEnv(pgHost string) bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production") {
		return true
	}
	h := strings.TrimSpace(strings.ToLower(pgHost))
	return h != "" && h != "localhost" && h != "127.0.0.1" && h != "::1"
}

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

	m := metrics.New("chat-service")
	m.RegisterPgxPool(pgPool)
	go func() {
		addr := metrics.AddrFromEnv()
		log.Info().Str("addr", addr).Msg("metrics listener starting")
		if err := m.Serve(ctx, addr, pgPool.Ping); err != nil {
			log.Error().Err(err).Msg("metrics listener failed")
		}
	}()

	repo := repository.New(pgPool)
	// P2#17: schema check is advisory, not a gate.  Killing the pod on a missing
	// column blocks rolling updates when a migration hasn't run yet.  Log a warning
	// so ops can act, but let the service start — actual queries will fail loudly
	// if the schema really is broken, which is the right signal.
	if err := repo.EnsureSchema(ctx); err != nil {
		log.Warn().Err(err).Msg("chat-service schema check: migrations may be pending — service will start anyway")
	}

	// N7 / P1#16: connect to NATS JetStream for durable domain event publishing.
	//
	// JetStream's js.Publish blocks until the server acknowledges persistence,
	// so downstream consumers (push, email) receive every chat.message.created
	// event even if they were temporarily offline.  core-NATS nc.Publish was
	// fire-and-forget and silently dropped messages during NATS restarts.
	//
	// The CHAT_EVENTS stream is provisioned idempotently at startup via
	// natsutil.EnsureStream; subjects "chat.>" are stored with FileStorage so
	// events survive NATS server restarts.
	//
	// The service starts successfully without NATS_URL — events are simply not
	// published (useful for local dev / unit tests).
	var uc *usecase.ChatUseCase
	if natsURL := strings.TrimSpace(cfg.NATSUrl); natsURL != "" {
		nc, js, natsErr := natsutil.Connect(natsURL)
		if natsErr != nil {
			log.Warn().Err(natsErr).Msg("chat-service: NATS unavailable, domain events will not be published")
			uc = usecase.New(repo)
		} else {
			defer nc.Close()
			if streamErr := natsutil.EnsureStream(js, "CHAT_EVENTS", []string{"chat.>"}); streamErr != nil {
				log.Warn().Err(streamErr).Msg("chat-service: failed to ensure CHAT_EVENTS JetStream stream — events may be lost")
			}
			log.Info().Str("nats_url", natsURL).Msg("chat-service: connected to NATS JetStream (CHAT_EVENTS stream)")
			uc = usecase.NewWithPublisher(repo, events.New(js))
		}
	} else {
		log.Info().Msg("chat-service: NATS_URL not set, skipping event publishing")
		uc = usecase.New(repo)
	}

	// P0#6 / N2: service-to-service token auth.
	//
	// In production (ENV=production or PG_HOST ≠ localhost/127.0.0.1) an empty
	// token is a misconfiguration — warn-logs are easy to miss in prod alerting,
	// so we hard-fail to surface the gap at deploy time rather than silently
	// running without auth.
	//
	// In local dev / CI the token is optional: the interceptor becomes a no-op
	// so services can be run individually without coordinating a shared secret.
	serviceToken := strings.TrimSpace(os.Getenv("INTERNAL_SERVICE_TOKEN"))
	if serviceToken == "" {
		if isProductionEnv(cfg.Postgres.Host) {
			log.Fatal().Msg("chat-service: INTERNAL_SERVICE_TOKEN must be set in production (ENV=production or remote PG_HOST)")
		}
		log.Warn().Msg("chat-service: INTERNAL_SERVICE_TOKEN not set — service-to-service auth disabled (dev/CI only)")
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
	chatv1.RegisterChatServiceServer(grpcServer, delivery.NewServer(uc))

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
