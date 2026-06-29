package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/postgres"

	"github.com/tienlao/agregator/services/review-service/config"
	delivery "github.com/tienlao/agregator/services/review-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/review-service/internal/events"
	"github.com/tienlao/agregator/services/review-service/internal/kpi"
	"github.com/tienlao/agregator/services/review-service/internal/outbox"
	"github.com/tienlao/agregator/services/review-service/internal/repository"
	"github.com/tienlao/agregator/services/review-service/internal/usecase"
)

// serviceName is the stable identity string injected into every outbound gRPC
// call via CallerIDClientInterceptor. Downstream services use it to distinguish
// service-to-service traffic from direct client calls.
const serviceName = "review-service"

func main() {
	log := logger.New(serviceName)

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

	m := metrics.New(serviceName)
	m.RegisterPgxPool(pgPool)
	kpi.Register(m.Registry())
	go func() {
		addr := metrics.AddrFromEnv()
		log.Info().Str("addr", addr).Msg("metrics listener starting")
		if err := m.Serve(ctx, addr, pgPool.Ping); err != nil {
			log.Error().Err(err).Msg("metrics listener failed")
		}
	}()

	nc, js, err := natsutil.Connect(cfg.NATSURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to nats")
	}
	defer nc.Close()
	log.Info().Msg("connected to nats")

	if err := natsutil.EnsureStream(js, "REVIEWS", []string{"review.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure REVIEWS stream")
	}

	dialOpts, err := grpcutil.DialOptions()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC transport config error")
	}

	// withCallerID appends the CallerIDClientInterceptor to dialOpts so that every
	// outbound gRPC call carries the "x-caller-id: <serviceName>" header.
	// Downstream services use this header to distinguish service-to-service traffic
	// from direct client calls (e.g. to gate interservice-only RPCs).
	withCallerID := append(dialOpts,
		grpc.WithUnaryInterceptor(grpcutil.CallerIDClientInterceptor(func(_ context.Context) string {
			return serviceName
		})),
	)

	bookingConn, err := grpc.NewClient(cfg.BookingServiceAddr, withCallerID...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial booking-service")
	}
	defer bookingConn.Close()
	bookingClient := bookingv1.NewBookingServiceClient(bookingConn)
	log.Info().Str("addr", cfg.BookingServiceAddr).Msg("booking-service client ready")

	// venue-service connection removed: rating updates are now event-driven
	// (venue-service subscribes to review.created via NATS).

	masterConn, err := grpc.NewClient(cfg.MasterServiceAddr, withCallerID...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial master-service")
	}
	defer masterConn.Close()
	masterClient := masterv1.NewMasterServiceClient(masterConn)
	log.Info().Str("addr", cfg.MasterServiceAddr).Msg("master-service client ready")

	reviewRepo := repository.NewReviewRepo(pgPool)
	outboxRepo := repository.NewOutboxRepo(pgPool)
	publisher := events.NewPublisher(js)

	uc := usecase.NewReviewUseCaseWithOutbox(reviewRepo, outboxRepo, bookingClient, masterClient)

	outboxWorker := outbox.NewWorker(outboxRepo, publisher, cfg.OutboxBatchSize, cfg.OutboxInterval)
	go outboxWorker.Run(ctx)

	srvOpts, err := grpcutil.ServerOptionsFromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC server transport config error")
	}
	srvOpts = append(srvOpts, grpc.ChainUnaryInterceptor(
		m.UnaryServerInterceptor(), // outermost: observes codes after PgError mapping
		grpcutil.PgErrorUnaryInterceptor(),
	))
	grpcServer := grpc.NewServer(srvOpts...)
	reviewv1.RegisterReviewServiceServer(grpcServer, delivery.NewServer(uc))

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
	sig := <-quit
	log.Info().Str("signal", sig.String()).Msg("shutting down")

	// Cancel the root context first so that in-flight upstream gRPC calls
	// (booking-service, master-service) and the outbox worker unblock promptly.
	// GracefulStop then drains any remaining handlers that are already past
	// their upstream calls and just need to flush their responses.
	cancel()
	grpcServer.GracefulStop()
	log.Info().Msg(serviceName + " stopped")
}
