package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/postgres"

	"github.com/tienlao/agregator/services/review-service/config"
	delivery "github.com/tienlao/agregator/services/review-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/review-service/internal/events"
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
		grpcutil.PgErrorUnaryInterceptor(),
	))
	grpcServer := grpc.NewServer(srvOpts...)
	reviewv1.RegisterReviewServiceServer(grpcServer, delivery.NewServer(uc))

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
