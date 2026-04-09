package usecase

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

func testRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:16379"})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func testPaymentUseCase(t *testing.T, repo *mockPaymentRepo, pub *mockEventPublisher) *PaymentUseCase {
	t.Helper()
	return NewPaymentUseCase(repo, NewYooKassaClient("", ""), testRedisClient(t), pub, "https://example.com/return")
}

func TestHandleWebhook_Succeeded(t *testing.T) {
	t.Parallel()
	providerID := "yk_pay_123"
	p := &domain.Payment{ID: "pay-1", BookingID: "book-1", Status: "pending", ProviderID: providerID}

	var updateCalled bool
	var published *domain.Payment
	repo := &mockPaymentRepo{
		GetByProviderIDFunc: func(ctx context.Context, pid string) (*domain.Payment, error) {
			assert.Equal(t, providerID, pid)
			return p, nil
		},
		UpdateStatusFunc: func(ctx context.Context, id, st, provID string) error {
			updateCalled = true
			assert.Equal(t, p.ID, id)
			assert.Equal(t, "succeeded", st)
			assert.Equal(t, providerID, provID)
			return nil
		},
	}
	pub := &mockEventPublisher{
		PublishPaymentCompletedFunc: func(ctx context.Context, pay *domain.Payment) error {
			published = pay
			assert.Equal(t, "succeeded", pay.Status)
			return nil
		},
	}

	uc := testPaymentUseCase(t, repo, pub)
	err := uc.HandleWebhook(context.Background(), WebhookPayload{
		Object: WebhookObject{ID: providerID, Status: "succeeded"},
	})
	require.NoError(t, err)
	require.True(t, updateCalled)
	require.NotNil(t, published)
	assert.Equal(t, "succeeded", published.Status)
	assert.Equal(t, p.ID, published.ID)
}

func TestHandleWebhook_Cancelled(t *testing.T) {
	t.Parallel()
	providerID := "yk_pay_cancel"
	p := &domain.Payment{ID: "pay-2", BookingID: "book-2", Status: "pending", ProviderID: providerID}

	var updateCalled bool
	var published *domain.Payment
	repo := &mockPaymentRepo{
		GetByProviderIDFunc: func(ctx context.Context, pid string) (*domain.Payment, error) {
			assert.Equal(t, providerID, pid)
			return p, nil
		},
		UpdateStatusFunc: func(ctx context.Context, id, st, provID string) error {
			updateCalled = true
			assert.Equal(t, p.ID, id)
			assert.Equal(t, "cancelled", st)
			assert.Equal(t, providerID, provID)
			return nil
		},
	}
	pub := &mockEventPublisher{
		PublishPaymentFailedFunc: func(ctx context.Context, pay *domain.Payment) error {
			published = pay
			assert.Equal(t, "cancelled", pay.Status)
			return nil
		},
	}

	uc := testPaymentUseCase(t, repo, pub)
	err := uc.HandleWebhook(context.Background(), WebhookPayload{
		Object: WebhookObject{ID: providerID, Status: "canceled"},
	})
	require.NoError(t, err)
	require.True(t, updateCalled)
	require.NotNil(t, published)
	assert.Equal(t, "cancelled", published.Status)
}

func TestHandleWebhook_UnknownStatus(t *testing.T) {
	t.Parallel()
	repo := &mockPaymentRepo{
		GetByProviderIDFunc: func(ctx context.Context, providerID string) (*domain.Payment, error) {
			t.Fatal("GetByProviderID should not be called for unknown status")
			return nil, nil
		},
		UpdateStatusFunc: func(ctx context.Context, id, st, providerID string) error {
			t.Fatal("UpdateStatus should not be called for unknown status")
			return nil
		},
	}
	pub := &mockEventPublisher{
		PublishPaymentCompletedFunc: func(ctx context.Context, p *domain.Payment) error {
			t.Fatal("PublishPaymentCompleted should not be called")
			return nil
		},
		PublishPaymentFailedFunc: func(ctx context.Context, p *domain.Payment) error {
			t.Fatal("PublishPaymentFailed should not be called")
			return nil
		},
	}

	uc := testPaymentUseCase(t, repo, pub)
	err := uc.HandleWebhook(context.Background(), WebhookPayload{
		Object: WebhookObject{ID: "yk_any", Status: "pending"},
	})
	require.NoError(t, err)
}

func TestHandleWebhook_MissingID(t *testing.T) {
	t.Parallel()
	repo := &mockPaymentRepo{}
	pub := &mockEventPublisher{}
	uc := testPaymentUseCase(t, repo, pub)

	err := uc.HandleWebhook(context.Background(), WebhookPayload{
		Object: WebhookObject{ID: "", Status: "succeeded"},
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "missing payment id")
}

func TestGetPayment_Success(t *testing.T) {
	t.Parallel()
	want := &domain.Payment{ID: "pay-x", BookingID: "b1", Status: "pending"}
	repo := &mockPaymentRepo{
		GetByIDFunc: func(ctx context.Context, id string) (*domain.Payment, error) {
			assert.Equal(t, "pay-x", id)
			return want, nil
		},
	}
	uc := testPaymentUseCase(t, repo, &mockEventPublisher{})

	got, err := uc.GetPayment(context.Background(), "pay-x")
	require.NoError(t, err)
	assert.Same(t, want, got)
}

func TestGetByBooking_Success(t *testing.T) {
	t.Parallel()
	want := &domain.Payment{ID: "pay-y", BookingID: "book-y", Status: "succeeded"}
	repo := &mockPaymentRepo{
		GetByBookingIDFunc: func(ctx context.Context, bookingID string) (*domain.Payment, error) {
			assert.Equal(t, "book-y", bookingID)
			return want, nil
		},
	}
	uc := testPaymentUseCase(t, repo, &mockEventPublisher{})

	got, err := uc.GetByBooking(context.Background(), "book-y")
	require.NoError(t, err)
	assert.Same(t, want, got)
}
