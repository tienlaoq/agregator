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
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/postgres"

	"github.com/tienlao/agregator/services/review-service/config"
	delivery "github.com/tienlao/agregator/services/review-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/review-service/internal/events"
	"github.com/tienlao/agregator/services/review-service/internal/repository"
	"github.com/tienlao/agregator/services/review-service/internal/usecase"
)

func main() {
	log := logger.New("review-service")

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

	bookingConn, err := grpc.NewClient(cfg.BookingServiceAddr, grpcutil.InsecureDialOptions()...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial booking-service")
	}
	defer bookingConn.Close()
	bookingClient := bookingv1.NewBookingServiceClient(bookingConn)
	log.Info().Str("addr", cfg.BookingServiceAddr).Msg("booking-service client ready")

	venueConn, err := grpc.NewClient(cfg.VenueServiceAddr, grpcutil.InsecureDialOptions()...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial venue-service")
	}
	defer venueConn.Close()
	venueClient := venuev1.NewVenueServiceClient(venueConn)
	log.Info().Str("addr", cfg.VenueServiceAddr).Msg("venue-service client ready")

	masterConn, err := grpc.NewClient(cfg.MasterServiceAddr, grpcutil.InsecureDialOptions()...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial master-service")
	}
	defer masterConn.Close()
	masterClient := masterv1.NewMasterServiceClient(masterConn)
	log.Info().Str("addr", cfg.MasterServiceAddr).Msg("master-service client ready")

	reviewRepo := repository.NewReviewRepo(pgPool)
	publisher := events.NewPublisher(js)

	uc := usecase.NewReviewUseCase(reviewRepo, bookingClient, venueClient, masterClient, publisher)

	grpcServer := grpc.NewServer(grpcutil.ServerOptions()...)
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

	grpcServer.GracefulStop()
	cancel()
	log.Info().Msg("review-service stopped")
}
