package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"google.golang.org/grpc"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/postgres"

	"github.com/tienlao/agregator/services/payment-service/config"
	delivery "github.com/tienlao/agregator/services/payment-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/payment-service/internal/events"
	"github.com/tienlao/agregator/services/payment-service/internal/outbox"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
	"github.com/tienlao/agregator/services/payment-service/internal/provider/yookassa"
	"github.com/tienlao/agregator/services/payment-service/internal/repository"
	"github.com/tienlao/agregator/services/payment-service/internal/usecase"
)

func main() {
	log := logger.New("payment-service")

	cfg := config.Load()
	if err := cfg.Postgres.Validate(); err != nil {
		log.Fatal().Err(err).Msg("invalid postgres config")
	}
	if err := cfg.Validate(os.Getenv("ENV")); err != nil {
		log.Fatal().Err(err).Msg("invalid config")
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

	if err := natsutil.EnsureStream(js, "PAYMENTS", []string{"payment.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure PAYMENTS stream")
	}
	if err := natsutil.EnsureStream(js, "BOOKINGS", []string{"booking.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure BOOKINGS stream")
	}
	// PAYMENTS_DLQ captures booking.cancelled messages that exhausted all delivery
	// attempts.  The subscriber publishes them here after calling msg.Term() so
	// they are never silently dropped and can be inspected / replayed by ops.
	if err := natsutil.EnsureStream(js, "PAYMENTS_DLQ", []string{"dlq.payment.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure PAYMENTS_DLQ stream")
	}

	// Wire the payment provider from PAYMENT_PROVIDER env.
	// Switching providers is a deploy-time decision: change the env var and redeploy.
	// No usecase or domain code changes required.
	paymentProvider, err := buildProvider(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to build payment provider")
	}
	if paymentProvider.IsMockMode() {
		log.Warn().
			Str("provider", string(cfg.ActiveProvider)).
			Msg("payment provider running in mock mode — no real credentials set")
	} else {
		log.Info().Str("provider", string(cfg.ActiveProvider)).Msg("payment provider ready")
	}

	paymentRepo := repository.NewPaymentRepo(pgPool)
	outboxRepo := repository.NewOutboxRepo(pgPool)

	uc := usecase.NewPaymentUseCase(paymentRepo, outboxRepo, paymentProvider, string(cfg.ActiveProvider), cfg.PaymentReturnURL, cfg.PlatformFeeBPS)

	// Outbox worker relays pending events from payment_outbox to NATS.
	// It runs asynchronously and is stopped via context cancellation on shutdown.
	outboxWorker := outbox.NewWorker(outboxRepo, js, log, 0 /* use default poll interval */)
	go outboxWorker.Run(ctx)
	log.Info().Msg("outbox worker started")

	bookingSub := events.NewSubscriber(js, uc, log)
	if err := bookingSub.SubscribeBookingEvents(); err != nil {
		log.Fatal().Err(err).Msg("failed to subscribe to booking events")
	}
	log.Info().Msg("subscribed to booking.cancelled for refund processing")

	// Non-production warning: Validate() already fatals in production when the
	// token is empty; here we only need to warn for dev/CI runs.
	if strings.TrimSpace(cfg.InternalServiceToken) == "" {
		log.Warn().Msg("payment-service: INTERNAL_SERVICE_TOKEN not set — service-to-service auth disabled (dev/CI only)")
	}

	srvOpts, err := grpcutil.ServerOptionsFromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("gRPC server transport config error")
	}
	srvOpts = append(srvOpts, grpc.ChainUnaryInterceptor(
		grpcutil.PgErrorUnaryInterceptor(),
		grpcutil.ServiceTokenServerInterceptor(cfg.InternalServiceToken),
	))
	grpcServer := grpc.NewServer(srvOpts...)
	paymentv1.RegisterPaymentServiceServer(grpcServer, delivery.NewServer(uc))

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
	cancel() // stops outbox worker and any in-flight context-aware operations
	log.Info().Msg("payment-service stopped")
}

// buildProvider constructs the PaymentProvider selected by cfg.ActiveProvider.
// Adding a new gateway requires:
//  1. Implement provider.PaymentProvider in internal/provider/<name>/client.go.
//  2. Add its credentials to config.ProviderConfig + config.Load().
//  3. Add a case here.
func buildProvider(cfg config.Config) (provider.PaymentProvider, error) {
	p := cfg.Provider
	switch cfg.ActiveProvider {
	case config.ProviderYooKassa:
		// Empty ShopID → mock mode (dev/CI without real credentials).
		return yookassa.NewClient(p.YooKassaShopID, p.YooKassaSecretKey), nil

	case config.ProviderTBank:
		// Placeholder: implement internal/provider/tbank/client.go,
		// then replace this error with: return tbank.NewClient(p.TBankTerminalKey, p.TBankSecretKey), nil
		return nil, fmt.Errorf("tbank provider not yet implemented: set PAYMENT_PROVIDER=yookassa or implement internal/provider/tbank")

	case config.ProviderSber:
		// Placeholder: implement internal/provider/sber/client.go,
		// then replace this error with: return sber.NewClient(p.SberMerchantLogin, p.SberSecretKey), nil
		return nil, fmt.Errorf("sber provider not yet implemented: set PAYMENT_PROVIDER=yookassa or implement internal/provider/sber")

	default:
		// Should never reach here — cfg.Validate() catches unknown values at startup.
		return nil, fmt.Errorf("unknown payment provider %q", cfg.ActiveProvider)
	}
}
