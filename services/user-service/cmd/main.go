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
	"google.golang.org/grpc/reflection"

	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/postgres"
	"github.com/tienlao/agregator/services/user-service/config"
	grpcdelivery "github.com/tienlao/agregator/services/user-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/user-service/internal/repository"
	"github.com/tienlao/agregator/services/user-service/internal/usecase"
)

func main() {
	log := logger.New("user-service")

	cfg := config.Load()
	if err := cfg.Postgres.Validate(); err != nil {
		log.Fatal().Err(err).Msg("invalid postgres config")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := postgres.NewPool(ctx, cfg.Postgres.DSN())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}
	defer pool.Close()
	log.Info().Msg("connected to postgres")

	repo := repository.NewPostgresUserRepository(pool)
	uc := usecase.NewUserUseCase(repo)
	userServer := grpcdelivery.NewUserServer(uc, log)

	srvOpts, err := grpcutil.ServerOptionsFromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC server transport config error")
	}
	srvOpts = append(srvOpts, grpc.ChainUnaryInterceptor(
		grpcutil.PgErrorUnaryInterceptor(),
	))
	grpcServer := grpc.NewServer(srvOpts...)
	userv1.RegisterUserServiceServer(grpcServer, userServer)

	// Health service: k8s liveness/readiness probes via grpc_health_v1.
	// Starts NOT_SERVING; transitions to SERVING after the initial Postgres
	// ping succeeds. A background goroutine re-checks every 10 s and flips
	// the status back to NOT_SERVING if the pool becomes unavailable, letting
	// k8s pull the pod from the load-balancer without a restart.
	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	// pingTimeout bounds each individual Postgres check so a slow TCP connect
	// never blocks the health goroutine for longer than this. Must be shorter
	// than the ticker interval (10 s) so ticks don't queue up.
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
				err := pool.Ping(pingCtx)
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

	// Initial ping: short deadline so a slow DB at startup does not delay
	// gRPC from binding and k8s from scheduling readiness probes.
	initPingCtx, initPingCancel := context.WithTimeout(ctx, pingTimeout)
	if err := pool.Ping(initPingCtx); err != nil {
		log.Warn().Err(err).Msg("health: initial postgres ping failed — starting NOT_SERVING")
	} else {
		healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	}
	initPingCancel()

	// Reflection lets grpcurl and other tooling discover the service schema
	// without a separate proto file. Internal network only; no auth bypass.
	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to listen")
	}

	// serveErr carries an unexpected Serve failure back to the main goroutine
	// so deferred cleanup (pool.Close, context cancel) runs before exit.
	// Buffered 1: the goroutine never blocks on send regardless of which
	// select branch wins first.
	serveErr := make(chan error, 1)
	go func() {
		log.Info().Str("port", cfg.GRPCPort).Msg("gRPC server started")
		if err := grpcServer.Serve(lis); err != nil {
			serveErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("shutting down")
	case err := <-serveErr:
		log.Error().Err(err).Msg("gRPC server failed")
	}

	grpcServer.GracefulStop()
	log.Info().Msg("server stopped")
}
