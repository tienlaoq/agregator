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
	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/postgres"

	"github.com/tienlao/agregator/services/booking-service/config"
	delivery "github.com/tienlao/agregator/services/booking-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/booking-service/internal/events"
	"github.com/tienlao/agregator/services/booking-service/internal/kpi"
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

	m := metrics.New("booking-service")
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

	if err := natsutil.EnsureStream(js, "BOOKINGS", []string{"booking.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure BOOKINGS stream")
	}
	if err := natsutil.EnsureStream(js, "PAYMENTS", []string{"payment.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure PAYMENTS stream")
	}

	dialOpts, err := grpcutil.DialOptions()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC transport config error")
	}

	venueConn, err := grpc.NewClient(cfg.VenueServiceAddr, dialOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial venue-service")
	}
	defer venueConn.Close()
	venueClient := venuev1.NewVenueServiceClient(venueConn)
	log.Info().Str("addr", cfg.VenueServiceAddr).Msg("venue-service client ready")

	// crm-service owns staff & management-access (extracted from venue-service).
	// booking-service consults it on staff-side endpoints (ListVenueBookings,
	// staff notes) before serving private data to a non-owner.
	crmConn, err := grpc.NewClient(cfg.CRMServiceAddr, dialOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial crm-service")
	}
	defer crmConn.Close()
	crmClient := crmv1.NewCRMServiceClient(crmConn)
	log.Info().Str("addr", cfg.CRMServiceAddr).Msg("crm-service client ready")

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
	log.Info().Str("addr", cfg.PaymentServiceAddr).Msg("payment-service client ready")

	if cfg.CursorHMACKey == "" {
		log.Warn().Msg("CURSOR_HMAC_KEY not set — using insecure fallback key for cursor signing; set in production")
	}
	bookingRepo := repository.NewBookingRepo(pgPool, cfg.CursorHMACKey)
	publisher := events.NewPublisher(js)

	uc := usecase.NewBookingUseCase(bookingRepo, venueClient, crmClient, paymentClient, publisher, log, cfg.VisitTimeZone, cfg.CancelDeadlineHours)

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

	// Outbox poller: публикует staged-события booking.created/confirmed/cancelled
	// из транзакционного outbox. Отдельный короткий тикер (а не 2-минутный
	// AutoComplete) — иначе подтверждения/рефанды ждали бы публикации до 2 минут.
	// publish раньше mark = at-least-once. См. TECH_DEBT [BOOKING-PUBLISH-LOSS].
	go func() {
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-autoCompleteCtx.Done():
				return
			case <-tick.C:
				if _, err := uc.ProcessOutbox(autoCompleteCtx, 100); err != nil {
					log.Error().Err(err).Msg("booking: outbox poller tick failed")
				}
			}
		}
	}()

	sub := events.NewSubscriber(js, uc, log, m)
	if err := sub.SubscribePaymentEvents(); err != nil {
		log.Fatal().Err(err).Msg("failed to subscribe to payment events")
	}
	log.Info().Msg("subscribed to payment events")

	srvOpts, err := grpcutil.ServerOptionsFromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC server transport config error")
	}
	// Interceptor chain (innermost last = executes first on the way in):
	//   PgErrorUnaryInterceptor — maps PostgreSQL 22P02/22023/22007/22008 → InvalidArgument
	//                             so "invalid uuid syntax" never surfaces as Internal.
	//   CallerIDServerInterceptor — reads "x-caller-id" from gRPC metadata (set by
	//                             api-gateway after JWT verification) into context;
	//                             handlers call grpcutil.CallerIDFromCtx(ctx) for authz.
	srvOpts = append(srvOpts, grpc.ChainUnaryInterceptor(
		m.UnaryServerInterceptor(), // outermost: observes codes after PgError mapping
		grpcutil.PgErrorUnaryInterceptor(),
		grpcutil.CallerIDServerInterceptor(),
	))
	grpcServer := grpc.NewServer(srvOpts...)
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
