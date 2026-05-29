package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"google.golang.org/grpc"

	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
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
		grpcutil.PgErrorUnaryInterceptor(),
		grpcutil.ServiceTokenServerInterceptor(serviceToken),
	))
	grpcServer := grpc.NewServer(srvOpts...)
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
