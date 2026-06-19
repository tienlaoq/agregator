package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/postgres"

	"github.com/tienlao/agregator/services/crm-service/config"
	"github.com/tienlao/agregator/services/crm-service/internal/dbmigrate"
	delivery "github.com/tienlao/agregator/services/crm-service/internal/delivery/grpc"
	crmevents "github.com/tienlao/agregator/services/crm-service/internal/events"
	"github.com/tienlao/agregator/services/crm-service/internal/repository"
	"github.com/tienlao/agregator/services/crm-service/internal/usecase"
)

func main() {
	log := logger.New("crm-service")

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

	migDir := resolveMigrationsDir()
	if err := dbmigrate.Up(ctx, cfg.Postgres.DSN(), migDir, log); err != nil {
		log.Fatal().Err(err).Str("dir", migDir).Msg("postgres migrations failed")
	}

	m := metrics.New("crm-service")
	m.RegisterPgxPool(pgPool)
	go func() {
		addr := metrics.AddrFromEnv()
		log.Info().Str("addr", addr).Msg("metrics listener starting")
		if err := m.Serve(ctx, addr, pgPool.Ping); err != nil {
			log.Error().Err(err).Msg("metrics listener failed")
		}
	}()

	repo := repository.New(pgPool)
	uc := usecase.New(repo, usecase.WithVIPThreshold(cfg.VIPSpentThreshold))

	// Guest-projection consumer: subscribe to booking.* on the BOOKINGS stream
	// and maintain the Customer 360 read model. The stream is owned by
	// booking-service; EnsureStream is a no-op when it already exists.
	nc, js, err := natsutil.Connect(cfg.NATSURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to nats")
	}
	log.Info().Msg("connected to nats")
	if err := natsutil.EnsureStream(js, crmevents.StreamName, []string{"booking.>"}); err != nil {
		log.Fatal().Err(err).Msg("ensure BOOKINGS stream failed")
	}
	guestSub := crmevents.NewSubscriber(js, repo, log, m)
	if err := guestSub.Subscribe(); err != nil {
		log.Fatal().Err(err).Msg("subscribe to booking events failed")
	}
	log.Info().Msg("guest-projection consumer started")

	srvOpts, err := grpcutil.ServerOptionsFromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC server transport config error")
	}
	srvOpts = append(srvOpts, grpc.ChainUnaryInterceptor(
		m.UnaryServerInterceptor(), // outermost: observes codes after PgError mapping
		grpcutil.SafePgErrorUnaryInterceptor(),
	))
	grpcServer := grpc.NewServer(srvOpts...)
	crmv1.RegisterCRMServiceServer(grpcServer, delivery.NewServer(uc, log))

	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

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

	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to listen")
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Info().Str("port", cfg.GRPCPort).Msg("gRPC server starting")
		serveErr <- grpcServer.Serve(lis)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("shutting down")
	case err := <-serveErr:
		log.Error().Err(err).Msg("gRPC server exited")
	}

	grpcServer.GracefulStop()
	// Drain waits for in-flight booking-event handlers to finish (DrainTimeout).
	if err := nc.Drain(); err != nil {
		log.Warn().Err(err).Msg("nats drain error")
	}
	cancel()
	log.Info().Msg("crm-service stopped")
}

func resolveMigrationsDir() string {
	if d := os.Getenv("CRM_MIGRATIONS_DIR"); d != "" {
		return d
	}
	for _, d := range []string{"/migrations", "migrations"} {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return d
		}
	}
	if wd, err := os.Getwd(); err == nil {
		p := filepath.Join(wd, "migrations")
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	return "migrations"
}
