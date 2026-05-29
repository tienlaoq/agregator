package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"github.com/tienlao/agregator/pkg/natsutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/postgres"

	"github.com/tienlao/agregator/services/master-service/config"
	delivery "github.com/tienlao/agregator/services/master-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/master-service/internal/events"
	"github.com/tienlao/agregator/services/master-service/internal/repository"
	"github.com/tienlao/agregator/services/master-service/internal/usecase"
)

func main() {
	log := logger.New("master-service")
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatal().Err(err).Msg("invalid config")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pgPool, err := postgres.NewPool(ctx, cfg.Postgres.DSN())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}
	defer pgPool.Close()
	pCfg := pgPool.Config()
	log.Info().
		Int32("max_conns", pCfg.MaxConns).
		Int32("min_conns", pCfg.MinConns).
		Dur("max_conn_lifetime", pCfg.MaxConnLifetime).
		Dur("max_conn_idle_time", pCfg.MaxConnIdleTime).
		Msg("connected to postgres")

	nc, js, err := natsutil.Connect(cfg.NATSURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to nats")
	}
	defer nc.Close()
	if err := natsutil.EnsureStream(js, "PAYMENTS", []string{"payment.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure PAYMENTS stream")
	}

	dialOpts, err := grpcutil.DialOptions()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC transport config error")
	}

	repo := repository.NewMasterRepo(pgPool)
	if cfg.InternalServiceToken == "" {
		log.Warn().Msg("INTERNAL_SERVICE_TOKEN not set — outbound auth to payment-service disabled (dev/CI only)")
	}
	paymentDialOpts := append(dialOpts,
		grpc.WithUnaryInterceptor(grpcutil.ServiceTokenClientInterceptor(cfg.InternalServiceToken)),
	)
	paymentConn, err := grpc.NewClient(cfg.PaymentServiceAddr, paymentDialOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial payment-service")
	}
	defer paymentConn.Close()
	paymentClient := paymentv1.NewPaymentServiceClient(paymentConn)

	uc := usecase.NewMasterUseCase(repo, paymentClient, log)
	sub := events.NewSubscriber(js, uc, log)
	// Subscriptions are drained via nc.Drain() on shutdown; we don't need
	// to hold the individual *nats.Subscription handles after this point.
	if _, err := sub.SubscribePaymentEvents(); err != nil {
		log.Fatal().Err(err).Msg("failed to subscribe to payment events")
	}

	srvOpts, err := grpcutil.ServerOptionsFromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC server transport config error")
	}
	srvOpts = append(srvOpts, grpc.ChainUnaryInterceptor(
		grpcutil.CallerIDServerInterceptor(),
		grpcutil.PgErrorUnaryInterceptor(),
	))
	grpcServer := grpc.NewServer(srvOpts...)
	masterv1.RegisterMasterServiceServer(grpcServer, delivery.NewServer(uc))

	// Health service: k8s liveness/readiness probes via grpc_health_v1.
	// The service starts NOT_SERVING and transitions to SERVING only after the
	// Postgres ping succeeds. The background goroutine below re-checks every 10 s
	// and flips status back to NOT_SERVING if the pool becomes unavailable,
	// allowing k8s to pull the pod from the load-balancer without a restart.
	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	// pingTimeout bounds each individual Postgres liveness check so a slow TCP
	// connect never blocks the health goroutine (or main) for longer than this.
	// Must be shorter than the ticker interval (10 s) so ticks don't queue up.
	const pingTimeout = 3 * time.Second

	go func() {
		const interval = 10 * time.Second
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
				return
			case <-t.C:
				pingCtx, pingCancel := context.WithTimeout(ctx, pingTimeout)
				err := pgPool.Ping(pingCtx)
				pingCancel()
				if err != nil {
					log.Warn().Err(err).Msg("health: postgres ping failed — marking NOT_SERVING")
					healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
				} else {
					healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
				}
			}
		}
	}()
	// Initial ping: use a short deadline so a slow DB at startup does not
	// delay gRPC from binding and k8s from scheduling readiness probes.
	initPingCtx, initPingCancel := context.WithTimeout(ctx, pingTimeout)
	if err := pgPool.Ping(initPingCtx); err != nil {
		log.Warn().Err(err).Msg("health: initial postgres ping failed — starting NOT_SERVING")
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
	// SIGINT / SIGTERM: standard k8s pod-termination signals.
	// SIGHUP: sent by init systems and supervisors on bare-metal / VM hosts when
	// the controlling terminal or parent process exits.  Without it a SIGHUP
	// would terminate the process immediately (default disposition) instead of
	// going through the graceful-shutdown path above.
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	<-quit
	log.Info().Msg("shutting down")

	// shutdownTimeout must fit within k8s terminationGracePeriodSeconds (default
	// 30 s). We use 20 s so the kubelet still has 10 s of margin to SIGKILL the
	// process cleanly. Both drains run concurrently so the budget is shared, not
	// additive: even if NATS handlers take 10 s and gRPC RPCs take 10 s, the
	// total wall-clock time is ~10 s, not 20 s.
	const shutdownTimeout = 20 * time.Second
	shutCtx, shutCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutCancel()

	var wg sync.WaitGroup

	// nc.Drain() drains all subscriptions on the connection at once and
	// respects the DrainTimeout set in natsutil.Connect (10 s). This is
	// preferable to calling s.Drain() per-subscription sequentially, which
	// would stack timeouts and could block for N×timeout. Runs concurrently
	// with GracefulStop so the budgets overlap rather than stack.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := nc.Drain(); err != nil {
			log.Warn().Err(err).Msg("nats: drain error")
		}
		log.Info().Msg("nats: connection drained")
	}()

	// Stop accepting new gRPC requests and wait for active RPCs to complete.
	wg.Add(1)
	go func() {
		defer wg.Done()
		grpcServer.GracefulStop()
		log.Info().Msg("grpc: server stopped")
	}()

	// Wait for both drains to finish or for the hard deadline to expire.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Info().Msg("graceful shutdown complete")
	case <-shutCtx.Done():
		log.Warn().Dur("timeout", shutdownTimeout).Msg("shutdown timeout exceeded — forcing stop")
		grpcServer.Stop() // hard stop; NATS handlers are abandoned by nc.Close() in defer
	}

	cancel()
	log.Info().Msg("master-service stopped")
}
