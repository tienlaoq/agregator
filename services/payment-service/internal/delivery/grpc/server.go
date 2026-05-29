package grpc

import (
	"context"
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
	// provider_seller_account_id takes precedence; fall back to the deprecated
	// yookassa_seller_account_id for callers that haven't migrated yet.
	sellerAccountID := req.GetProviderSellerAccountId()
	if sellerAccountID == "" {
		//nolint:staticcheck // intentional: bridging deprecated field during migration window
		sellerAccountID = req.GetYookassaSellerAccountId()
	}

	p, err := s.uc.CreatePayment(ctx, usecase.CreatePaymentInput{
		BookingID:               req.GetBookingId(),
		Amount:                  req.GetAmount(),
		Description:             req.GetDescription(),
		IdempotencyKey:          req.GetIdempotencyKey(),
		CounterpartyType:        req.GetCounterpartyType(),
		CounterpartyID:          req.GetCounterpartyId(),
		ProviderSellerAccountID: sellerAccountID,
	})
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

// HandleWebhook receives the raw provider notification forwarded by the
// gateway and delegates to the usecase, which in turn delegates parsing to
// the payment provider.
//
// LOGGING POLICY — only the following structural metadata may be logged here.
// See CLAUDE.md "Webhook logging policy" for the full permitted/forbidden list.
//
// Permitted:
//   - "event"         — notification type from WebhookEvent.RawEvent
//   - "object_id"     — provider payment UUID
//   - "object_status" — payload status hint from WebhookEvent.RawProviderStatus
//                       (informational; the provider pull-confirms internally)
//   - "err"           — error values on failure paths
//
// Never log:
//   - req.RawBody or any fragment of it
//   - amount, value, currency, metadata, payer, recipient, description
//   - Any field not in the Permitted list above
//
// Log fields are sourced from the WebhookEvent returned by HandleWebhook —
// the body is parsed exactly once inside the provider, eliminating the
// previous double-parse overhead on the hot webhook path.
func (s *Server) HandleWebhook(ctx context.Context, req *paymentv1.WebhookRequest) (*paymentv1.WebhookResponse, error) {
	event, err := s.uc.HandleWebhook(ctx, req.RawBody)
	if err != nil {
		// Log with whatever fields the provider managed to populate before the
		// error (event may be nil if ParseWebhook itself failed).
		if event != nil {
			slog.Error("handle webhook",
				"event", event.RawEvent,
				"object_id", event.ProviderPaymentID,
				"object_status", event.RawProviderStatus,
				"err", err,
			)
		} else {
			slog.Error("handle webhook", "err", err)
		}
		return &paymentv1.WebhookResponse{Ok: false}, nil
	}

	// Permitted fields only — see policy above.
	slog.Info("processed webhook",
		"event", event.RawEvent,
		"object_id", event.ProviderPaymentID,
		"object_status", event.RawProviderStatus,
	)

	return &paymentv1.WebhookResponse{Ok: true}, nil
}

func toProto(p *domain.Payment) *paymentv1.PaymentResponse {
	return &paymentv1.PaymentResponse{
		Id:                     p.ID,
		BookingId:              p.BookingID,
		Amount:                 p.Amount,
		Status:                 p.Status.String(),
		PaymentUrl:             p.PaymentURL,
		ProviderId:             p.ProviderID,
		CreatedAt:              timestamppb.New(p.CreatedAt),
		PlatformFeeKopecks:     p.PlatformFeeKopecks,
		CounterpartyNetKopecks: p.CounterpartyNetKopecks,
		ProviderName:           p.ProviderName,
	}
}
