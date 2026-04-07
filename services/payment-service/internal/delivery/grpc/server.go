package grpc

import (
	"context"

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
