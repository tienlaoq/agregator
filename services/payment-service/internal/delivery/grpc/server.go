package grpc

import (
	"context"
	"encoding/json"
	"log/slog"

	"google.golang.org/protobuf/types/known/timestamppb"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/usecase"
)

type Server struct {
	paymentv1.UnimplementedPaymentServiceServer
	uc *usecase.PaymentUseCase
}

func NewServer(uc *usecase.PaymentUseCase) *Server {
	return &Server{uc: uc}
}

func (s *Server) CreatePayment(ctx context.Context, req *paymentv1.CreatePaymentRequest) (*paymentv1.PaymentResponse, error) {
	p, err := s.uc.CreatePayment(ctx, req.BookingId, req.Amount, req.Description, req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	return toProto(p), nil
}

func (s *Server) GetPayment(ctx context.Context, req *paymentv1.GetPaymentRequest) (*paymentv1.PaymentResponse, error) {
	p, err := s.uc.GetPayment(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return toProto(p), nil
}

func (s *Server) GetPaymentByBooking(ctx context.Context, req *paymentv1.GetPaymentByBookingRequest) (*paymentv1.PaymentResponse, error) {
	p, err := s.uc.GetByBooking(ctx, req.BookingId)
	if err != nil {
		return nil, err
	}
	return toProto(p), nil
}

func (s *Server) HandleWebhook(ctx context.Context, req *paymentv1.WebhookRequest) (*paymentv1.WebhookResponse, error) {
	var payload usecase.WebhookPayload
	if err := json.Unmarshal(req.RawBody, &payload); err != nil {
		slog.Error("unmarshal webhook payload", "err", err)
		return &paymentv1.WebhookResponse{Ok: false}, nil
	}

	slog.Info("processing webhook",
		"event", payload.Event,
		"object_id", payload.Object.ID,
		"object_status", payload.Object.Status,
	)

	if err := s.uc.HandleWebhook(ctx, payload); err != nil {
		slog.Error("handle webhook", "err", err)
		return &paymentv1.WebhookResponse{Ok: false}, nil
	}

	return &paymentv1.WebhookResponse{Ok: true}, nil
}

func toProto(p *domain.Payment) *paymentv1.PaymentResponse {
	return &paymentv1.PaymentResponse{
		Id:         p.ID,
		BookingId:  p.BookingID,
		Amount:     p.Amount,
		Status:     p.Status,
		PaymentUrl: p.PaymentURL,
		ProviderId: p.ProviderID,
		CreatedAt:  timestamppb.New(p.CreatedAt),
	}
}
