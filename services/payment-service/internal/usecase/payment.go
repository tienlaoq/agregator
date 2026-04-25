package usecase

import (
	"context"
	"fmt"
	"strings"
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
	repo       domain.PaymentRepository
	yookassa   *YooKassaClient
	rdb        *redis.Client
	publisher  EventPublisher
	returnURL  string
	feeBPS     int64
}

func NewPaymentUseCase(
	repo domain.PaymentRepository,
	yookassa *YooKassaClient,
	rdb *redis.Client,
	publisher EventPublisher,
	returnURL string,
	platformFeeBPS int,
) *PaymentUseCase {
	if platformFeeBPS <= 0 {
		platformFeeBPS = 1500
	}
	return &PaymentUseCase{
		repo:      repo,
		yookassa:  yookassa,
		rdb:       rdb,
		publisher: publisher,
		returnURL: returnURL,
		feeBPS:    int64(platformFeeBPS),
	}
}

type CreatePaymentInput struct {
	BookingID               string
	Amount                  int64
	Description             string
	IdempotencyKey          string
	CounterpartyType        string
	CounterpartyID          string
	YooKassaSellerAccountID string
}

func (uc *PaymentUseCase) CreatePayment(ctx context.Context, in CreatePaymentInput) (*domain.Payment, error) {
	if in.Amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}
	sellerID := strings.TrimSpace(in.YooKassaSellerAccountID)
	if !uc.yookassa.mockMode && sellerID == "" {
		return nil, status.Error(codes.InvalidArgument, "yookassa_seller_account_id is required for live ЮKassa split")
	}

	platformFee := PlatformFeeKopecks(in.Amount, uc.feeBPS)
	net := CounterpartyNetKopecks(in.Amount, platformFee)

	idempKey := fmt.Sprintf("payment:idempotency:%s", in.IdempotencyKey)
	set, err := uc.rdb.SetNX(ctx, idempKey, "1", 24*time.Hour).Result()
	if err == nil && !set {
		existing, err := uc.repo.GetByIdempotencyKey(ctx, in.IdempotencyKey)
		if err == nil {
			return existing, nil
		}
	}

	var result *YooKassaPaymentResult
	if uc.yookassa.mockMode && sellerID == "" {
		result, err = uc.yookassa.CreatePaymentSimple(in.Amount, in.Description, uc.returnURL, in.IdempotencyKey)
	} else {
		meta := map[string]string{"booking_id": in.BookingID}
		if strings.TrimSpace(in.CounterpartyID) != "" {
			meta["counterparty_id"] = strings.TrimSpace(in.CounterpartyID)
		}
		result, err = uc.yookassa.CreatePaymentSplit(uc.returnURL, in.IdempotencyKey, in.Description, SplitTransferParams{
			GrossKopecks:          in.Amount,
			PlatformFeeKopecks:    platformFee,
			SellerAccountID:       sellerID,
			TransferDescription:   in.Description,
			Metadata:              meta,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("yookassa create payment: %w", err)
	}

	p := &domain.Payment{
		BookingID:               in.BookingID,
		Amount:                  in.Amount,
		Status:                  "pending",
		ProviderID:              result.PaymentID,
		PaymentURL:              result.ConfirmationURL,
		IdempotencyKey:          in.IdempotencyKey,
		PlatformFeeKopecks:      platformFee,
		CounterpartyNetKopecks:  net,
		CounterpartyType:        strings.TrimSpace(in.CounterpartyType),
		CounterpartyID:          strings.TrimSpace(in.CounterpartyID),
		YooKassaSellerAccountID: sellerID,
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

	statusStr := payload.Object.Status

	if statusStr == "waiting_for_capture" {
		p, err := uc.findByProviderID(ctx, providerID)
		if err != nil {
			return err
		}
		if p.UsesYooKassaSplitCapture() {
			captureKey := p.ID + "-capture"
			if err := uc.yookassa.CapturePayment(providerID, captureKey); err != nil {
				return fmt.Errorf("yookassa capture: %w", err)
			}
		}
		return nil
	}

	var p *domain.Payment
	var err error

	switch statusStr {
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
