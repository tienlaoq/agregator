package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/pkg/metrics"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/postgres"

	"github.com/tienlao/agregator/services/payment-service/config"
	delivery "github.com/tienlao/agregator/services/payment-service/internal/delivery/grpc"
	"github.com/tienlao/agregator/services/payment-service/internal/events"
	"github.com/tienlao/agregator/services/payment-service/internal/kpi"
	"github.com/tienlao/agregator/services/payment-service/internal/outbox"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
	"github.com/tienlao/agregator/services/payment-service/internal/provider/alfa"
	"github.com/tienlao/agregator/services/payment-service/internal/provider/mock"
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

	m := metrics.New("payment-service")
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

	if err := natsutil.EnsureStream(js, "PAYMENTS", []string{"payment.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure PAYMENTS stream")
	}
	if err := natsutil.EnsureStream(js, "BOOKINGS", []string{"booking.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure BOOKINGS stream")
	}
	// PAYMENTS_DLQ captures booking.cancelled messages that exhausted all delivery
	// attempts.  The subscriber publishes them here after calling msg.Term() so
	// they are never silently dropped and can be inspected / replayed by ops.
	if err := natsutil.EnsureStream(js, "PAYMENTS_DLQ", []string{events.DLQSubjectWildcard}); err != nil {
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
	ledgerRepo := repository.NewLedgerRepo(pgPool)
	payoutRepo := repository.NewPayoutRepo(pgPool)
	payoutMethodRepo := repository.NewPayoutMethodRepo(pgPool)

	holdDuration := time.Duration(cfg.PayoutHoldHours) * time.Hour
	uc := usecase.NewPaymentUseCase(
		paymentRepo, outboxRepo, ledgerRepo,
		paymentProvider, string(cfg.ActiveProvider),
		cfg.PaymentReturnURL, cfg.PlatformFeeBPS, holdDuration,
		log,
	)
	// PAYOUT_WEEKDAY in 0..6 enables weekly batched payouts on that weekday;
	// any other value (e.g. -1) reverts to paying every ripe balance per tick.
	weeklyPayout := cfg.PayoutWeekday >= 0 && cfg.PayoutWeekday <= 6
	if !weeklyPayout && cfg.PayoutWeekday != -1 {
		log.Warn().
			Int("payout_weekday", cfg.PayoutWeekday).
			Msg("PAYOUT_WEEKDAY out of range (expected 0..6, or -1 to disable) — weekly payouts disabled, paying per tick")
	}
	// PayoutWeekday is evaluated in this location so the payout day tracks the
	// business calendar, not the container's UTC day. Fall back to UTC on an
	// unknown zone rather than failing startup.
	payoutLoc, err := time.LoadLocation(cfg.PayoutTimezone)
	if err != nil {
		log.Warn().
			Str("payout_timezone", cfg.PayoutTimezone).
			Err(err).
			Msg("PAYOUT_TIMEZONE not loadable — falling back to UTC for payout weekday")
		payoutLoc = time.UTC
	}
	payoutUC := usecase.NewPayoutUseCase(
		payoutRepo, payoutMethodRepo, ledgerRepo,
		paymentProvider, string(cfg.ActiveProvider),
		usecase.PayoutSchedulerConfig{
			MinPayoutKopecks: cfg.PayoutMinKopecks,
			WeeklyPayout:     weeklyPayout,
			PayoutWeekday:    time.Weekday(cfg.PayoutWeekday),
			PayoutLocation:   payoutLoc,
		},
		log,
	)
	stopScheduler := payoutUC.StartSchedulerInBackground(ctx)
	defer stopScheduler()
	log.Info().Msg("payout scheduler started")

	// Outbox worker relays pending events from payment_outbox to NATS.
	// It runs asynchronously and is stopped via context cancellation on shutdown.
	outboxWorker := outbox.NewWorker(outboxRepo, js, log, 0 /* use default poll interval */)
	go outboxWorker.Run(ctx)
	log.Info().Msg("outbox worker started")

	bookingSub := events.NewSubscriber(js, uc, log, m)
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
		m.UnaryServerInterceptor(), // outermost: observes codes after PgError mapping
		grpcutil.PgErrorUnaryInterceptor(),
		grpcutil.ServiceTokenServerInterceptor(cfg.InternalServiceToken),
	))
	grpcServer := grpc.NewServer(srvOpts...)
	paymentv1.RegisterPaymentServiceServer(grpcServer, delivery.NewServer(uc, payoutUC, log))

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
	switch cfg.ActiveProvider {
	case config.ProviderMock:
		// No-network stand-in for dev/CI. Rejected in production by Validate.
		return mock.NewClient(), nil

	case config.ProviderTBank:
		// Placeholder: implement internal/provider/tbank/client.go,
		// then replace this error with: return tbank.NewClient(p.TBankTerminalKey, p.TBankSecretKey), nil
		return nil, fmt.Errorf("tbank provider not yet implemented: set PAYMENT_PROVIDER=mock or implement internal/provider/tbank")

	case config.ProviderSber:
		// Placeholder: implement internal/provider/sber/client.go,
		// then replace this error with: return sber.NewClient(p.SberMerchantLogin, p.SberSecretKey), nil
		return nil, fmt.Errorf("sber provider not yet implemented: set PAYMENT_PROVIDER=mock or implement internal/provider/sber")

	case config.ProviderAlfa:
		// Alfa-Bank internet-acquiring on the RBS REST platform.
		return alfa.NewClient(cfg.Provider.AlfaUsername, cfg.Provider.AlfaPassword, cfg.Provider.AlfaGatewayURL), nil

	default:
		// Should never reach here — cfg.Validate() catches unknown values at startup.
		return nil, fmt.Errorf("unknown payment provider %q", cfg.ActiveProvider)
	}
}
