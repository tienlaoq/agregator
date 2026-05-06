package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"github.com/tienlao/agregator/pkg/natsutil"
	"google.golang.org/grpc"

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
	if err := natsutil.EnsureStream(js, "PAYMENTS", []string{"payment.>"}); err != nil {
		log.Fatal().Err(err).Msg("failed to ensure PAYMENTS stream")
	}

	repo := repository.NewMasterRepo(pgPool)
	paymentConn, err := grpc.NewClient(cfg.PaymentServiceAddr, grpcutil.InsecureDialOptions()...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to dial payment-service")
	}
	defer paymentConn.Close()
	paymentClient := paymentv1.NewPaymentServiceClient(paymentConn)

	uc := usecase.NewMasterUseCase(repo, paymentClient)
	sub := events.NewSubscriber(js, uc, log)
	if err := sub.SubscribePaymentEvents(); err != nil {
		log.Fatal().Err(err).Msg("failed to subscribe to payment events")
	}

	grpcServer := grpc.NewServer(grpcutil.ServerOptions()...)
	masterv1.RegisterMasterServiceServer(grpcServer, delivery.NewServer(uc))

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
	<-quit
	log.Info().Msg("shutting down")
	grpcServer.GracefulStop()
	cancel()
	log.Info().Msg("master-service stopped")
}
