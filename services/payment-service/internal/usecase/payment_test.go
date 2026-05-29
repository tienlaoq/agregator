package usecase

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// testPaymentUseCase builds a PaymentUseCase wired to in-memory mocks.
// The default provider stub succeeds on all calls.
func testPaymentUseCase(t *testing.T, repo *mockPaymentRepo, outbox *mockOutboxRepo) *PaymentUseCase {
	t.Helper()
	return testPaymentUseCaseWithProvider(t, repo, outbox, &mockPaymentProvider{MockMode: true})
}

func testPaymentUseCaseWithProvider(t *testing.T, repo *mockPaymentRepo, outbox *mockOutboxRepo, prov provider.PaymentProvider) *PaymentUseCase {
	t.Helper()
	// activeProvider="" disables the provider-mismatch guard — tests that need
	// it should call NewPaymentUseCase directly with a non-empty activeProvider.
	return NewPaymentUseCase(repo, outbox, prov, "", "https://example.com/return", 1500)
}

// ── CreatePayment ─────────────────────────────────────────────────────────────

func TestCreatePayment_HappyPath(t *testing.T) {
	t.Parallel()

	var savedID, savedProviderID, savedURL string
	repo := &mockPaymentRepo{
		CreateIdempotentFunc: func(_ context.Context, p *domain.Payment) (bool, error) {
			p.ID = "pay-new"
			return true, nil
		},
		UpdateProviderInfoFunc: func(_ context.Context, id, providerID, paymentURL string) error {
			savedID = id
			savedProviderID = providerID
			savedURL = paymentURL
			return nil
		},
	}

	uc := testPaymentUseCase(t, repo, &mockOutboxRepo{})
	got, err := uc.CreatePayment(context.Background(), CreatePaymentInput{
		BookingID:      "book-1",
		Amount:         5000,
		Description:    "Test booking",
		IdempotencyKey: "idem-1",
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "pay-new", got.ID)
	assert.NotEmpty(t, got.ProviderID, "ProviderID must be set after provider call")
	assert.NotEmpty(t, got.PaymentURL, "PaymentURL must be set after provider call")
	assert.Equal(t, "pay-new", savedID)
	assert.Equal(t, got.ProviderID, savedProviderID)
	assert.Equal(t, got.PaymentURL, savedURL)
}

func TestCreatePayment_InvalidAmount(t *testing.T) {
	t.Parallel()
	repo := &mockPaymentRepo{}
	uc := testPaymentUseCase(t, repo, &mockOutboxRepo{})

	_, err := uc.CreatePayment(context.Background(), CreatePaymentInput{
		BookingID: "book-1", Amount: 0, IdempotencyKey: "idem-zero",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

// TestCreatePayment_Idempotent_FullyCompleted covers the normal idempotent
// case: a second request with the same key finds an already-complete row
// (ProviderID != "") and returns it without touching the provider.
func TestCreatePayment_Idempotent_FullyCompleted(t *testing.T) {
	t.Parallel()

	existing := &domain.Payment{
		ID:         "pay-existing",
		BookingID:  "book-1",
		ProviderID: "provider_123",
		PaymentURL: "https://pay.example.com/provider_123",
		Status:     domain.StatusPending,
	}

	var providerCalled bool
	repo := &mockPaymentRepo{
		CreateIdempotentFunc: func(_ context.Context, _ *domain.Payment) (bool, error) {
			return false, nil
		},
		GetByIdempotencyKeyFunc: func(_ context.Context, key string) (*domain.Payment, error) {
			assert.Equal(t, "idem-dup", key)
			return existing, nil
		},
		UpdateProviderInfoFunc: func(_ context.Context, _ string, _ string, _ string) error {
			providerCalled = true
			return nil
		},
	}

	uc := testPaymentUseCase(t, repo, &mockOutboxRepo{})
	got, err := uc.CreatePayment(context.Background(), CreatePaymentInput{
		BookingID:      "book-1",
		Amount:         5000,
		IdempotencyKey: "idem-dup",
	})

	require.NoError(t, err)
	assert.Equal(t, existing.ID, got.ID)
	assert.Equal(t, existing.ProviderID, got.ProviderID)
	assert.False(t, providerCalled, "provider must not be called for a complete idempotent hit")
}

// TestCreatePayment_Idempotent_CrashRecovery covers the crash-recovery path:
// a previous request inserted the DB row (ProviderID="") but crashed before
// calling the provider.  The retry must re-drive the provider via
// DriveProviderWithLock and return the completed row.
func TestCreatePayment_Idempotent_CrashRecovery(t *testing.T) {
	t.Parallel()

	incomplete := &domain.Payment{
		ID:             "pay-incomplete",
		BookingID:      "book-2",
		ProviderID:     "",
		PaymentURL:     "",
		Status:         domain.StatusPending,
		IdempotencyKey: "idem-crash",
	}

	var lockAcquired bool
	repo := &mockPaymentRepo{
		CreateIdempotentFunc: func(_ context.Context, _ *domain.Payment) (bool, error) {
			return false, nil
		},
		GetByIdempotencyKeyFunc: func(_ context.Context, _ string) (*domain.Payment, error) {
			return incomplete, nil
		},
		// DriveProviderWithLock simulates the lock being acquired, fn called once,
		// and the result written atomically.
		DriveProviderWithLockFunc: func(_ context.Context, key string, fn func(p *domain.Payment) (string, string, error)) (*domain.Payment, error) {
			assert.Equal(t, "idem-crash", key)
			lockAcquired = true
			providerID, paymentURL, err := fn(incomplete)
			if err != nil {
				return nil, err
			}
			incomplete.ProviderID = providerID
			incomplete.PaymentURL = paymentURL
			return incomplete, nil
		},
	}

	uc := testPaymentUseCase(t, repo, &mockOutboxRepo{})
	got, err := uc.CreatePayment(context.Background(), CreatePaymentInput{
		BookingID:      "book-2",
		Amount:         7000,
		IdempotencyKey: "idem-crash",
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, lockAcquired, "DriveProviderWithLock must be called for crash-recovery path")
	assert.Equal(t, "pay-incomplete", got.ID)
	assert.NotEmpty(t, got.ProviderID)
	assert.NotEmpty(t, got.PaymentURL)
}

// TestCreatePayment_ConcurrentRetry_OnlyOneProviderCall verifies the race fix:
// when two requests reach the crash-recovery branch simultaneously, only one
// calls the provider.  The second sees ProviderID already filled after the lock
// is released and returns the existing row without touching the provider.
func TestCreatePayment_ConcurrentRetry_OnlyOneProviderCall(t *testing.T) {
	t.Parallel()

	const idemKey = "idem-race"
	// Shared state: starts empty, filled by the first lock winner.
	row := &domain.Payment{
		ID:             "pay-race",
		BookingID:      "book-race",
		ProviderID:     "",
		IdempotencyKey: idemKey,
		Status:         domain.StatusPending,
	}

	var providerCallCount int

	repo := &mockPaymentRepo{
		CreateIdempotentFunc: func(_ context.Context, _ *domain.Payment) (bool, error) {
			return false, nil // both requests see existing row
		},
		GetByIdempotencyKeyFunc: func(_ context.Context, _ string) (*domain.Payment, error) {
			// Both requests read ProviderID="" on the fast-path check.
			return &domain.Payment{
				ID:             row.ID,
				BookingID:      row.BookingID,
				ProviderID:     "",
				IdempotencyKey: idemKey,
				Status:         domain.StatusPending,
			}, nil
		},
		DriveProviderWithLockFunc: func(_ context.Context, key string, fn func(p *domain.Payment) (string, string, error)) (*domain.Payment, error) {
			// Simulate the advisory lock serialisation:
			// – First caller: row.ProviderID still empty → calls fn.
			// – Second caller: row.ProviderID now set    → returns row, skips fn.
			if row.ProviderID != "" {
				// Second requester sees the result of the first — no fn call.
				return row, nil
			}
			providerID, paymentURL, err := fn(row)
			if err != nil {
				return nil, err
			}
			row.ProviderID = providerID
			row.PaymentURL = paymentURL
			return row, nil
		},
	}

	prov := &mockPaymentProvider{
		MockMode: true,
		CreatePaymentFunc: func(_ context.Context, _ provider.CreateRequest) (*provider.Result, error) {
			providerCallCount++
			return &provider.Result{
				ProviderPaymentID: "provider-race-id",
				ConfirmationURL:   "https://pay.example.com/race",
			}, nil
		},
	}

	uc := testPaymentUseCaseWithProvider(t, repo, &mockOutboxRepo{}, prov)
	input := CreatePaymentInput{BookingID: "book-race", Amount: 5000, IdempotencyKey: idemKey}

	// Simulate two concurrent retries calling CreatePayment sequentially
	// (the mock serialises what the advisory lock does in production).
	got1, err1 := uc.CreatePayment(context.Background(), input)
	got2, err2 := uc.CreatePayment(context.Background(), input)

	require.NoError(t, err1)
	require.NoError(t, err2)

	assert.Equal(t, 1, providerCallCount, "provider must be called exactly once across concurrent retries")
	assert.Equal(t, got1.ProviderID, got2.ProviderID, "both retries must return the same provider ID")
}

// TestCreatePayment_DBError covers the case where CreateIdempotent returns an
// unexpected DB error — must propagate to caller.
func TestCreatePayment_DBError(t *testing.T) {
	t.Parallel()
	dbErr := errors.New("connection reset")
	repo := &mockPaymentRepo{
		CreateIdempotentFunc: func(_ context.Context, _ *domain.Payment) (bool, error) {
			return false, dbErr
		},
	}
	uc := testPaymentUseCase(t, repo, &mockOutboxRepo{})
	_, err := uc.CreatePayment(context.Background(), CreatePaymentInput{
		BookingID: "book-3", Amount: 1000, IdempotencyKey: "idem-dberr",
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "create payment")
}

// ── HandleWebhook ─────────────────────────────────────────────────────────────
//
// HandleWebhook accepts raw []byte and delegates ALL provider-specific parsing
// to provider.PaymentProvider.ParseWebhook.  Tests stub ParseWebhookFunc to
// return a *domain.WebhookEvent directly — no JSON marshalling required.
//
// With the outbox pattern there is no publisher mock.  Tests verify that
// UpdateStatusWithOutbox is called with the correct arguments and that an
// outbox event with the right subject is written.

var rawWebhookBody = []byte(`{"event":"stub","object":{"id":"stub","status":"stub"}}`)

func TestHandleWebhook_Succeeded(t *testing.T) {
	t.Parallel()
	providerID := "pay_123"
	p := &domain.Payment{ID: "pay-1", BookingID: "book-1", Status: domain.StatusPending, ProviderID: providerID}

	var updateCalled bool
	var outboxSubject domain.OutboxSubject

	repo := &mockPaymentRepo{
		GetByProviderIDFunc: func(_ context.Context, pid string) (*domain.Payment, error) {
			assert.Equal(t, providerID, pid)
			return p, nil
		},
		UpdateStatusWithOutboxFunc: func(_ context.Context, id string, st domain.PaymentStatus, provID string, event *domain.OutboxEvent) (bool, error) {
			updateCalled = true
			assert.Equal(t, p.ID, id)
			assert.Equal(t, domain.StatusSucceeded, st)
			assert.Equal(t, providerID, provID)
			outboxSubject = event.Subject
			return true, nil
		},
	}
	prov := &mockPaymentProvider{
		ParseWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*domain.WebhookEvent, error) {
			return &domain.WebhookEvent{
				ProviderPaymentID: providerID,
				Status:            domain.StatusSucceeded,
			}, nil
		},
	}

	uc := testPaymentUseCaseWithProvider(t, repo, &mockOutboxRepo{}, prov)
	_, err := uc.HandleWebhook(context.Background(), rawWebhookBody)
	require.NoError(t, err)
	require.True(t, updateCalled)
	assert.Equal(t, domain.SubjectPaymentCompleted, outboxSubject)
}

func TestHandleWebhook_Cancelled(t *testing.T) {
	t.Parallel()
	providerID := "pay_cancel"
	p := &domain.Payment{ID: "pay-2", BookingID: "book-2", Status: domain.StatusPending, ProviderID: providerID}

	var outboxSubject domain.OutboxSubject
	repo := &mockPaymentRepo{
		GetByProviderIDFunc: func(_ context.Context, _ string) (*domain.Payment, error) {
			return p, nil
		},
		UpdateStatusWithOutboxFunc: func(_ context.Context, _ string, st domain.PaymentStatus, _ string, event *domain.OutboxEvent) (bool, error) {
			assert.Equal(t, domain.StatusCancelled, st)
			outboxSubject = event.Subject
			return true, nil
		},
	}
	prov := &mockPaymentProvider{
		ParseWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*domain.WebhookEvent, error) {
			return &domain.WebhookEvent{
				ProviderPaymentID: providerID,
				Status:            domain.StatusCancelled,
			}, nil
		},
	}

	uc := testPaymentUseCaseWithProvider(t, repo, &mockOutboxRepo{}, prov)
	_, err := uc.HandleWebhook(context.Background(), rawWebhookBody)
	require.NoError(t, err)
	assert.Equal(t, domain.SubjectPaymentFailed, outboxSubject)
}

// TestHandleWebhook_WaitingForCapture verifies the two-phase capture path:
// ParseWebhook returns RequiresCapture=true, the usecase must call Capture and
// return without touching the DB or writing any outbox event.
func TestHandleWebhook_WaitingForCapture(t *testing.T) {
	t.Parallel()
	providerID := "pay_wait_cap"
	captureKey := providerID + "-capture"

	var captureCalled bool
	repo := &mockPaymentRepo{
		GetByProviderIDFunc: func(_ context.Context, _ string) (*domain.Payment, error) {
			t.Fatal("GetByProviderID must not be called when RequiresCapture=true")
			return nil, nil
		},
		UpdateStatusWithOutboxFunc: func(_ context.Context, _ string, _ domain.PaymentStatus, _ string, _ *domain.OutboxEvent) (bool, error) {
			t.Fatal("UpdateStatusWithOutbox must not be called when RequiresCapture=true")
			return false, nil
		},
	}
	prov := &mockPaymentProvider{
		ParseWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*domain.WebhookEvent, error) {
			return &domain.WebhookEvent{
				ProviderPaymentID: providerID,
				Status:            domain.StatusPending,
				RequiresCapture:   true,
				CaptureKey:        captureKey,
			}, nil
		},
		CaptureFunc: func(_ context.Context, pid, key string) error {
			captureCalled = true
			assert.Equal(t, providerID, pid)
			assert.Equal(t, captureKey, key)
			return nil
		},
	}
	uc := testPaymentUseCaseWithProvider(t, repo, &mockOutboxRepo{}, prov)
	_, err := uc.HandleWebhook(context.Background(), rawWebhookBody)
	require.NoError(t, err)
	require.True(t, captureCalled, "Capture must be called when RequiresCapture=true")
}

// TestHandleWebhook_Succeeded_Duplicate verifies idempotency on duplicate delivery.
// UpdateStatusWithOutbox returns updated=false (row already terminal) and no
// outbox event should be counted.
func TestHandleWebhook_Succeeded_Duplicate(t *testing.T) {
	t.Parallel()
	providerID := "pay_dup_123"
	p := &domain.Payment{ID: "pay-dup", BookingID: "book-dup", Status: domain.StatusSucceeded, ProviderID: providerID}

	var updateCalls int
	repo := &mockPaymentRepo{
		GetByProviderIDFunc: func(_ context.Context, _ string) (*domain.Payment, error) {
			return p, nil
		},
		UpdateStatusWithOutboxFunc: func(_ context.Context, _ string, _ domain.PaymentStatus, _ string, _ *domain.OutboxEvent) (bool, error) {
			updateCalls++
			return false, nil // already terminal
		},
	}
	prov := &mockPaymentProvider{
		ParseWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*domain.WebhookEvent, error) {
			return &domain.WebhookEvent{
				ProviderPaymentID: providerID,
				Status:            domain.StatusSucceeded,
			}, nil
		},
	}
	uc := testPaymentUseCaseWithProvider(t, repo, &mockOutboxRepo{}, prov)
	_, err := uc.HandleWebhook(context.Background(), rawWebhookBody)
	require.NoError(t, err)
	// UpdateStatusWithOutbox is still called once (the guard is inside the repo/tx),
	// but it reports updated=false so no downstream side-effects occur.
	assert.Equal(t, 1, updateCalls)
}

// TestHandleWebhook_NonTerminalStatus_NoSideEffects verifies that a non-terminal
// ParseWebhook result causes no DB update or outbox write.
func TestHandleWebhook_NonTerminalStatus_NoSideEffects(t *testing.T) {
	t.Parallel()
	repo := &mockPaymentRepo{
		GetByProviderIDFunc: func(_ context.Context, _ string) (*domain.Payment, error) {
			t.Fatal("GetByProviderID should not be called for non-terminal status")
			return nil, nil
		},
		UpdateStatusWithOutboxFunc: func(_ context.Context, _ string, _ domain.PaymentStatus, _ string, _ *domain.OutboxEvent) (bool, error) {
			t.Fatal("UpdateStatusWithOutbox should not be called for non-terminal status")
			return false, nil
		},
	}
	prov := &mockPaymentProvider{
		ParseWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*domain.WebhookEvent, error) {
			return &domain.WebhookEvent{
				ProviderPaymentID: "pay_any",
				Status:            domain.StatusPending,
			}, nil
		},
	}
	uc := testPaymentUseCaseWithProvider(t, repo, &mockOutboxRepo{}, prov)
	_, err := uc.HandleWebhook(context.Background(), rawWebhookBody)
	require.NoError(t, err)
}

// TestHandleWebhook_PullConfirmOverridesPayload documents the pull-confirm
// contract: ParseWebhook is the sole authority on status.  When ParseWebhook
// returns StatusPending (e.g. pull-confirm contradicts payload), usecase no-ops.
func TestHandleWebhook_PullConfirmOverridesPayload(t *testing.T) {
	t.Parallel()
	repo := &mockPaymentRepo{
		GetByProviderIDFunc: func(_ context.Context, _ string) (*domain.Payment, error) {
			t.Fatal("GetByProviderID must not be called when ParseWebhook returns non-terminal")
			return nil, nil
		},
		UpdateStatusWithOutboxFunc: func(_ context.Context, _ string, _ domain.PaymentStatus, _ string, _ *domain.OutboxEvent) (bool, error) {
			t.Fatal("UpdateStatusWithOutbox must not be called when ParseWebhook returns non-terminal")
			return false, nil
		},
	}
	prov := &mockPaymentProvider{
		ParseWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*domain.WebhookEvent, error) {
			return &domain.WebhookEvent{
				ProviderPaymentID: "pay_tampered",
				Status:            domain.StatusPending,
			}, nil
		},
	}

	uc := testPaymentUseCaseWithProvider(t, repo, &mockOutboxRepo{}, prov)
	_, err := uc.HandleWebhook(context.Background(), rawWebhookBody)
	require.NoError(t, err)
}

// TestHandleWebhook_UnrecognisedProviderStatus verifies that an unrecognised
// raw provider status (one not in terminalProviderStatuses) is treated as a
// no-op and does not trigger any DB write.  The returned event still carries
// the raw status for caller logging.
func TestHandleWebhook_UnrecognisedProviderStatus(t *testing.T) {
	t.Parallel()
	repo := &mockPaymentRepo{
		GetByProviderIDFunc: func(_ context.Context, _ string) (*domain.Payment, error) {
			t.Fatal("GetByProviderID must not be called for unrecognised status")
			return nil, nil
		},
		UpdateStatusWithOutboxFunc: func(_ context.Context, _ string, _ domain.PaymentStatus, _ string, _ *domain.OutboxEvent) (bool, error) {
			t.Fatal("UpdateStatusWithOutbox must not be called for unrecognised status")
			return false, nil
		},
	}
	prov := &mockPaymentProvider{
		ParseWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*domain.WebhookEvent, error) {
			// Simulate a provider returning a status we have never seen before.
			return &domain.WebhookEvent{
				ProviderPaymentID: "pay_unknown",
				Status:            domain.StatusPending, // coerced by the provider
				RawEvent:          "payment.unknown_future_status",
				RawProviderStatus: "unknown_future_status",
			}, nil
		},
	}
	uc := testPaymentUseCaseWithProvider(t, repo, &mockOutboxRepo{}, prov)
	event, err := uc.HandleWebhook(context.Background(), rawWebhookBody)
	require.NoError(t, err)
	require.NotNil(t, event)
	assert.Equal(t, "unknown_future_status", event.RawProviderStatus)
}

func TestHandleWebhook_ParseError(t *testing.T) {
	t.Parallel()
	parseErr := errors.New("invalid webhook body")
	prov := &mockPaymentProvider{
		ParseWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*domain.WebhookEvent, error) {
			return nil, parseErr
		},
	}
	uc := testPaymentUseCaseWithProvider(t, &mockPaymentRepo{}, &mockOutboxRepo{}, prov)
	_, err := uc.HandleWebhook(context.Background(), []byte(`{}`))
	require.Error(t, err)
	assert.ErrorContains(t, err, "parse webhook")
}

// ── Provider-mismatch guard ───────────────────────────────────────────────────

// TestHandleWebhook_ProviderMismatch verifies that a terminal webhook whose
// stored payment was created by a different provider is rejected with
// FailedPrecondition and does not update the DB or write an outbox event.
// Scenario: migrated from yookassa → tbank; a stale yookassa notification
// arrives and matches a now-tbank payment ID by coincidence or replay.
func TestHandleWebhook_ProviderMismatch(t *testing.T) {
	t.Parallel()
	providerID := "pay_stale_yookassa"
	// Payment row was created by tbank; the active provider is also tbank.
	p := &domain.Payment{
		ID:           "pay-tb-1",
		ProviderID:   providerID,
		ProviderName: "tbank",
		Status:       domain.StatusPending,
	}
	repo := &mockPaymentRepo{
		GetByProviderIDFunc: func(_ context.Context, _ string) (*domain.Payment, error) {
			return p, nil
		},
		UpdateStatusWithOutboxFunc: func(_ context.Context, _ string, _ domain.PaymentStatus, _ string, _ *domain.OutboxEvent) (bool, error) {
			t.Fatal("UpdateStatusWithOutbox must not be called on provider mismatch")
			return false, nil
		},
	}
	prov := &mockPaymentProvider{
		ParseWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*domain.WebhookEvent, error) {
			return &domain.WebhookEvent{
				ProviderPaymentID: providerID,
				Status:            domain.StatusSucceeded,
				RawEvent:          "payment.succeeded",
				RawProviderStatus: "succeeded",
			}, nil
		},
	}
	// Explicitly set activeProvider="tbank"; payment.ProviderName="tbank" but
	// the webhook was parsed as a yookassa event — the check fires on name mismatch
	// between what the webhook claims vs what the payment row records.
	// Here we simulate: activeProvider=yookassa, payment stored as tbank.
	uc := NewPaymentUseCase(repo, &mockOutboxRepo{}, prov, "yookassa", "https://example.com/return", 1500)
	_, err := uc.HandleWebhook(context.Background(), rawWebhookBody)
	require.Error(t, err)
	// Must be FailedPrecondition so the gateway returns 4xx (not retried).
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}

// TestHandleWebhook_ProviderMatch verifies that a terminal webhook whose
// stored payment matches the active provider is processed normally.
func TestHandleWebhook_ProviderMatch(t *testing.T) {
	t.Parallel()
	providerID := "pay_yookassa_ok"
	p := &domain.Payment{
		ID:           "pay-yk-1",
		BookingID:    "book-yk-1",
		ProviderID:   providerID,
		ProviderName: "yookassa",
		Status:       domain.StatusPending,
	}
	updateCalled := false
	repo := &mockPaymentRepo{
		GetByProviderIDFunc: func(_ context.Context, _ string) (*domain.Payment, error) {
			return p, nil
		},
		UpdateStatusWithOutboxFunc: func(_ context.Context, _ string, _ domain.PaymentStatus, _ string, _ *domain.OutboxEvent) (bool, error) {
			updateCalled = true
			return true, nil
		},
	}
	prov := &mockPaymentProvider{
		ParseWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*domain.WebhookEvent, error) {
			return &domain.WebhookEvent{
				ProviderPaymentID: providerID,
				Status:            domain.StatusSucceeded,
				RawEvent:          "payment.succeeded",
				RawProviderStatus: "succeeded",
			}, nil
		},
	}
	uc := NewPaymentUseCase(repo, &mockOutboxRepo{}, prov, "yookassa", "https://example.com/return", 1500)
	_, err := uc.HandleWebhook(context.Background(), rawWebhookBody)
	require.NoError(t, err)
	require.True(t, updateCalled, "UpdateStatusWithOutbox must be called when providers match")
}

// ── GetPayment / GetByBooking ─────────────────────────────────────────────────

func TestGetPayment_Success(t *testing.T) {
	t.Parallel()
	want := &domain.Payment{ID: "pay-x", BookingID: "b1", Status: domain.StatusPending}
	repo := &mockPaymentRepo{
		GetByIDFunc: func(_ context.Context, id string) (*domain.Payment, error) {
			assert.Equal(t, "pay-x", id)
			return want, nil
		},
	}
	uc := testPaymentUseCase(t, repo, &mockOutboxRepo{})
	got, err := uc.GetPayment(context.Background(), "pay-x")
	require.NoError(t, err)
	assert.Same(t, want, got)
}

func TestGetByBooking_Success(t *testing.T) {
	t.Parallel()
	want := &domain.Payment{ID: "pay-y", BookingID: "book-y", Status: domain.StatusSucceeded}
	repo := &mockPaymentRepo{
		GetByBookingIDFunc: func(_ context.Context, bookingID string) (*domain.Payment, error) {
			assert.Equal(t, "book-y", bookingID)
			return want, nil
		},
	}
	uc := testPaymentUseCase(t, repo, &mockOutboxRepo{})
	got, err := uc.GetByBooking(context.Background(), "book-y")
	require.NoError(t, err)
	assert.Same(t, want, got)
}
