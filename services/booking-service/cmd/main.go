package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/postgres"

	"github.com/tienlao/agregator/services/booking-service/config"
	delivery "github.com/tienlao/agregator/services/booking-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/booking-service/internal/events"
	"github.com/tienlao/agregator/services/booking-service/internal/repository"
	"github.com/tienlao/agregator/services/booking-service/internal/usecase"
)

func main() {
	log := logger.New("booking-service")

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

	if err := natsutil.EnsureStream(js, "BOOKINGS", []string{"booking.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure BOOKINGS stream")
	}
	if err := natsutil.EnsureStream(js, "PAYMENTS", []string{"payment.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure PAYMENTS stream")
	}

	venueConn, err := grpc.NewClient(cfg.VenueServiceAddr, grpcutil.InsecureDialOptions()...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial venue-service")
	}
	defer venueConn.Close()
	venueClient := venuev1.NewVenueServiceClient(venueConn)
	log.Info().Str("addr", cfg.VenueServiceAddr).Msg("venue-service client ready")

	paymentConn, err := grpc.NewClient(cfg.PaymentServiceAddr, grpcutil.InsecureDialOptions()...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial payment-service")
	}
	defer paymentConn.Close()
	paymentClient := paymentv1.NewPaymentServiceClient(paymentConn)
	log.Info().Str("addr", cfg.PaymentServiceAddr).Msg("payment-service client ready")

	bookingRepo := repository.NewBookingRepo(pgPool)
	publisher := events.NewPublisher(js)

	uc := usecase.NewBookingUseCase(bookingRepo, venueClient, paymentClient, publisher, cfg.VisitTimeZone)

	autoCompleteCtx, autoCompleteCancel := context.WithCancel(ctx)
	defer autoCompleteCancel()
	go func() {
		tick := time.NewTicker(2 * time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-autoCompleteCtx.Done():
				return
			case <-tick.C:
				n, err := uc.AutoCompletePastVisits(autoCompleteCtx)
				if err != nil {
					log.Error().Err(err).Msg("auto-complete past visits")
					continue
				}
				if n > 0 {
					log.Info().Int("count", n).Msg("bookings auto-completed after visit end")
				}
			}
		}
	}()

	sub := events.NewSubscriber(js, uc, log)
	if err := sub.SubscribePaymentEvents(); err != nil {
		log.Fatal().Err(err).Msg("failed to subscribe to payment events")
	}
	log.Info().Msg("subscribed to payment events")

	grpcServer := grpc.NewServer(grpcutil.ServerOptions()...)
	bookingv1.RegisterBookingServiceServer(grpcServer, delivery.NewServer(uc))

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
	log.Info().Msg("booking-service stopped")
}
