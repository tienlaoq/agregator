package grpc

import (
	"context"

	"github.com/rs/zerolog"
	"google.golang.org/protobuf/types/known/timestamppb"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/usecase"
)

type Server struct {
	paymentv1.UnimplementedPaymentServiceServer
	uc     *usecase.PaymentUseCase
	payout *usecase.PayoutUseCase
	log    zerolog.Logger
}

func NewServer(uc *usecase.PaymentUseCase, payout *usecase.PayoutUseCase, lg zerolog.Logger) *Server {
	return &Server{
		uc:     uc,
		payout: payout,
		log:    lg.With().Str("component", "payment-grpc").Logger(),
	}
}

func (s *Server) CreatePayment(ctx context.Context, req *paymentv1.CreatePaymentRequest) (*paymentv1.PaymentResponse, error) {
	in := usecase.CreatePaymentInput{
		BookingID:        req.GetBookingId(),
		Amount:           req.GetAmount(),
		Description:      req.GetDescription(),
		IdempotencyKey:   req.GetIdempotencyKey(),
		CounterpartyType: req.GetCounterpartyType(),
		CounterpartyID:   req.GetCounterpartyId(),
	}
	if t := req.GetServiceAt(); t != nil {
		in.ServiceAt = t.AsTime()
	}

	p, err := s.uc.CreatePayment(ctx, in)
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
//     (informational; the provider pull-confirms internally)
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
	// Try payout dispatch first: ParsePayoutWebhook returns nil when the body
	// is not a payout event, in which case we fall back to the payment path.
	// This keeps a single gRPC endpoint while supporting both event families
	// without an extra round-trip from the gateway.
	if s.payout != nil {
		payoutEvent, perr := s.payout.HandlePayoutWebhook(ctx, req.RawBody)
		if perr != nil {
			var rawEvent, objectID, objectStatus string
			if payoutEvent != nil {
				rawEvent = payoutEvent.RawEvent
				objectID = payoutEvent.ProviderPayoutID
				objectStatus = string(payoutEvent.Status)
			}
			s.log.Error().
				Str("event", rawEvent).
				Str("object_id", objectID).
				Str("object_status", objectStatus).
				Err(perr).
				Msg("handle payout webhook")
			return &paymentv1.WebhookResponse{Ok: false}, nil
		}
		if payoutEvent != nil {
			s.log.Info().
				Str("event", payoutEvent.RawEvent).
				Str("object_id", payoutEvent.ProviderPayoutID).
				Str("object_status", string(payoutEvent.Status)).
				Msg("processed payout webhook")
			return &paymentv1.WebhookResponse{Ok: true}, nil
		}
	}

	event, err := s.uc.HandleWebhook(ctx, req.RawBody)
	if err != nil {
		// Log with whatever fields the provider managed to populate before the
		// error (event may be nil if ParseWebhook itself failed).
		if event != nil {
			s.log.Error().
				Str("event", event.RawEvent).
				Str("object_id", event.ProviderPaymentID).
				Str("object_status", event.RawProviderStatus).
				Err(err).
				Msg("handle webhook")
		} else {
			s.log.Error().Err(err).Msg("handle webhook")
		}
		return &paymentv1.WebhookResponse{Ok: false}, nil
	}

	// Permitted fields only — see policy above.
	s.log.Info().
		Str("event", event.RawEvent).
		Str("object_id", event.ProviderPaymentID).
		Str("object_status", event.RawProviderStatus).
		Msg("processed webhook")

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
