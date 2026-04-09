package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

type EventPublisher interface {
	PublishPaymentCompleted(ctx context.Context, p *domain.Payment) error
	PublishPaymentFailed(ctx context.Context, p *domain.Payment) error
}

type PaymentUseCase struct {
	repo      domain.PaymentRepository
	yookassa  *YooKassaClient
	rdb       *redis.Client
	publisher EventPublisher
	returnURL string
}

func NewPaymentUseCase(
	repo domain.PaymentRepository,
	yookassa *YooKassaClient,
	rdb *redis.Client,
	publisher EventPublisher,
	returnURL string,
) *PaymentUseCase {
	return &PaymentUseCase{
		repo:      repo,
		yookassa:  yookassa,
		rdb:       rdb,
		publisher: publisher,
		returnURL: returnURL,
	}
}

func (uc *PaymentUseCase) CreatePayment(ctx context.Context, bookingID string, amount int64, description, idempotencyKey string) (*domain.Payment, error) {
	idempKey := fmt.Sprintf("payment:idempotency:%s", idempotencyKey)
	set, err := uc.rdb.SetNX(ctx, idempKey, "1", 24*time.Hour).Result()
	if err == nil && !set {
		existing, err := uc.repo.GetByIdempotencyKey(ctx, idempotencyKey)
		if err == nil {
			return existing, nil
		}
	}

	result, err := uc.yookassa.CreatePayment(amount, description, uc.returnURL, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("yookassa create payment: %w", err)
	}

	p := &domain.Payment{
		BookingID:      bookingID,
		Amount:         amount,
		Status:         "pending",
		ProviderID:     result.PaymentID,
		PaymentURL:     result.ConfirmationURL,
		IdempotencyKey: idempotencyKey,
	}
	if err := uc.repo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create payment: %w", err)
	}
	return p, nil
}

func (uc *PaymentUseCase) GetPayment(ctx context.Context, id string) (*domain.Payment, error) {
	return uc.repo.GetByID(ctx, id)
}

func (uc *PaymentUseCase) GetByBooking(ctx context.Context, bookingID string) (*domain.Payment, error) {
	return uc.repo.GetByBookingID(ctx, bookingID)
}

type WebhookPayload struct {
	Event  string        `json:"event"`
	Object WebhookObject `json:"object"`
}

type WebhookObject struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (uc *PaymentUseCase) HandleWebhook(ctx context.Context, payload WebhookPayload) error {
	providerID := payload.Object.ID
	if providerID == "" {
		return status.Error(codes.InvalidArgument, "missing payment id in webhook")
	}

	var p *domain.Payment
	var err error

	// Find payment by provider ID - scan by booking since we don't index provider_id directly
	// In production, add an index on provider_id for efficiency
	switch payload.Object.Status {
	case "succeeded":
		p, err = uc.findByProviderID(ctx, providerID)
		if err != nil {
			return err
		}
		if err := uc.repo.UpdateStatus(ctx, p.ID, "succeeded", providerID); err != nil {
			return err
		}
		p.Status = "succeeded"
		return uc.publisher.PublishPaymentCompleted(ctx, p)

	case "canceled":
		p, err = uc.findByProviderID(ctx, providerID)
		if err != nil {
			return err
		}
		if err := uc.repo.UpdateStatus(ctx, p.ID, "cancelled", providerID); err != nil {
			return err
		}
		p.Status = "cancelled"
		return uc.publisher.PublishPaymentFailed(ctx, p)

	default:
		return nil
	}
}

func (uc *PaymentUseCase) findByProviderID(ctx context.Context, providerID string) (*domain.Payment, error) {
	p, err := uc.repo.GetByProviderID(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("payment not found for provider_id %s: %w", providerID, err)
	}
	return p, nil
}
