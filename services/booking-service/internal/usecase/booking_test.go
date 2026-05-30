package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/booking-service/internal/domain"
)

// visitDateNotInPast returns YYYY-MM-DD in Europe/Moscow strictly after "today"
// so CreateBooking date validation does not depend on the calendar when tests run.
func visitDateNotInPast(t *testing.T) string {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)
	return time.Now().In(loc).AddDate(0, 0, 7).Format("2006-01-02")
}

func TestCreateBooking_Success(t *testing.T) {
	ctx := context.Background()
	testDate := visitDateNotInPast(t)
	const bookingID = "b1"
	const paymentID = "pay-1"
	const paymentURL = "https://pay.example/p"

	repo := NewMockBookingRepository(t)
	repo.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, b *domain.Booking) {
			b.ID = bookingID
			b.CreatedAt = time.Now()
			b.UpdatedAt = time.Now()
		}).Return(nil)
	repo.EXPECT().SetPaymentAndStatus(mock.Anything, bookingID, paymentID, paymentURL, string(domain.StatusPaymentPending)).
		Return(nil)
	repo.EXPECT().DeleteOrphanPending(mock.Anything, mock.Anything).
		Return(int64(0), nil).Maybe()

	var reserveCalled bool
	venue := &mockVenueClient{
		GetVenueFunc: func(_ context.Context, in *venuev1.GetVenueRequest, _ ...grpc.CallOption) (*venuev1.VenueResponse, error) {
			require.Equal(t, "venue-1", in.Id)
			return &venuev1.VenueResponse{
				Id:        "venue-1",
				Name:      "Sauna",
				PriceFrom: 3000,
				Services: []*venuev1.VenueServiceItem{
					{Id: "svc-1", Name: "Аренда", DurationMin: 120, Price: 10000},
				},
			}, nil
		},
		CheckSlotAvailabilityFunc: func(_ context.Context, in *venuev1.CheckSlotRequest, _ ...grpc.CallOption) (*venuev1.CheckSlotResponse, error) {
			require.Equal(t, "venue-1", in.VenueId)
			require.Equal(t, testDate, in.Date)
			require.Equal(t, "10:00", in.TimeFrom)
			require.Equal(t, "12:00", in.TimeTo)
			return &venuev1.CheckSlotResponse{Available: true}, nil
		},
		ReserveSlotFunc: func(_ context.Context, in *venuev1.ReserveSlotRequest, _ ...grpc.CallOption) (*venuev1.ReserveSlotResponse, error) {
			reserveCalled = true
			require.Equal(t, "venue-1", in.VenueId)
			require.Equal(t, bookingID, in.BookingId)
			require.Equal(t, testDate, in.Date)
			return &venuev1.ReserveSlotResponse{}, nil
		},
	}

	payment := &mockPaymentClient{
		CreatePaymentFunc: func(_ context.Context, in *paymentv1.CreatePaymentRequest, _ ...grpc.CallOption) (*paymentv1.PaymentResponse, error) {
			require.Equal(t, bookingID, in.BookingId)
			require.Equal(t, int64(10000), in.Amount)
			require.Contains(t, in.Description, bookingID)
			require.Equal(t, bookingIdempotencyKey(bookingID, "user-1"), in.IdempotencyKey)
			return &paymentv1.PaymentResponse{Id: paymentID, PaymentUrl: paymentURL}, nil
		},
	}

	pub := &mockEventPublisher{
		PublishBookingCreatedFunc: func(_ context.Context, b *domain.Booking) error {
			require.Equal(t, bookingID, b.ID)
			require.Equal(t, domain.StatusPaymentPending, b.Status)
			return nil
		},
	}

	uc := NewBookingUseCase(repo, venue, &mockCRMClient{}, payment, pub, zerolog.Nop(), "Europe/Moscow", 0)

	out, err := uc.CreateBooking(ctx, CreateBookingInput{
		UserID:    "user-1",
		VenueID:   "venue-1",
		VenueName: "Sauna",
		ServiceID: "svc-1",
		Date:      testDate,
		TimeFrom:  "10:00",
		TimeTo:    "12:00",
		Guests:    2,
		Comment:   "hi",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.True(t, reserveCalled)
	assert.Equal(t, bookingID, out.ID)
	assert.Equal(t, domain.StatusPaymentPending, out.Status)
	assert.Equal(t, paymentID, out.PaymentID)
	assert.Equal(t, paymentURL, out.PaymentURL)
}

func TestCreateBooking_SlotNotAvailable(t *testing.T) {
	ctx := context.Background()
	testDate := visitDateNotInPast(t)

	repo := NewMockBookingRepository(t)

	venue := &mockVenueClient{
		CheckSlotAvailabilityFunc: func(_ context.Context, _ *venuev1.CheckSlotRequest, _ ...grpc.CallOption) (*venuev1.CheckSlotResponse, error) {
			return &venuev1.CheckSlotResponse{Available: false}, nil
		},
	}
	uc := NewBookingUseCase(repo, venue, &mockCRMClient{}, &mockPaymentClient{}, &mockEventPublisher{}, zerolog.Nop(), "Europe/Moscow", 0)

	_, err := uc.CreateBooking(ctx, CreateBookingInput{
		UserID:   "user-1",
		VenueID:  "venue-1",
		Date:     testDate,
		TimeFrom: "10:00",
		TimeTo:   "12:00",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "занято")
}

func TestCreateBooking_ReserveConflictDeletesBooking(t *testing.T) {
	ctx := context.Background()
	testDate := visitDateNotInPast(t)
	const bookingID = "b-conflict"

	repo := NewMockBookingRepository(t)
	repo.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, b *domain.Booking) {
			b.ID = bookingID
			b.CreatedAt = time.Now()
			b.UpdatedAt = time.Now()
		}).Return(nil)
	repo.EXPECT().Delete(mock.Anything, bookingID).Return(nil)

	venue := &mockVenueClient{
		CheckSlotAvailabilityFunc: func(_ context.Context, _ *venuev1.CheckSlotRequest, _ ...grpc.CallOption) (*venuev1.CheckSlotResponse, error) {
			return &venuev1.CheckSlotResponse{Available: true}, nil
		},
		ReserveSlotFunc: func(_ context.Context, _ *venuev1.ReserveSlotRequest, _ ...grpc.CallOption) (*venuev1.ReserveSlotResponse, error) {
			return nil, status.Error(codes.InvalidArgument, "time slot not available")
		},
	}

	uc := NewBookingUseCase(repo, venue, &mockCRMClient{}, &mockPaymentClient{}, &mockEventPublisher{}, zerolog.Nop(), "Europe/Moscow", 0)

	_, err := uc.CreateBooking(ctx, CreateBookingInput{
		UserID:   "user-1",
		VenueID:  "venue-1",
		Date:     testDate,
		TimeFrom: "10:00",
		TimeTo:   "12:00",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "занято")
}

func TestCancelBooking_Success(t *testing.T) {
	ctx := context.Background()
	const bookingID = "b1"
	const userID = "user-1"

	b := &domain.Booking{
		ID:      bookingID,
		UserID:  userID,
		VenueID: "venue-1",
		Status:  domain.StatusPending,
	}
	cancelled := *b
	cancelled.Status = domain.StatusCancelled

	repo := NewMockBookingRepository(t)
	repo.EXPECT().GetByID(mock.Anything, bookingID).Return(b, nil)
	repo.EXPECT().UpdateStatus(mock.Anything, bookingID, string(domain.StatusCancelled)).Return(nil)

	var released bool
	venue := &mockVenueClient{
		ReleaseSlotFunc: func(_ context.Context, in *venuev1.ReleaseSlotRequest, _ ...grpc.CallOption) (*venuev1.ReleaseSlotResponse, error) {
			released = true
			require.Equal(t, "venue-1", in.VenueId)
			require.Equal(t, bookingID, in.BookingId)
			return &venuev1.ReleaseSlotResponse{}, nil
		},
	}

	var published bool
	pub := &mockEventPublisher{
		PublishBookingCancelledFunc: func(_ context.Context, got *domain.Booking) error {
			published = true
			assert.Equal(t, domain.StatusCancelled, got.Status)
			return nil
		},
	}

	uc := NewBookingUseCase(repo, venue, &mockCRMClient{}, &mockPaymentClient{}, pub, zerolog.Nop(), "Europe/Moscow", 0)
	out, err := uc.CancelBooking(ctx, bookingID, userID)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, domain.StatusCancelled, out.Status)
	assert.True(t, released)
	assert.True(t, published)
}

func TestCancelBooking_NotOwner(t *testing.T) {
	ctx := context.Background()

	repo := NewMockBookingRepository(t)
	repo.EXPECT().GetByID(mock.Anything, "b1").Return(&domain.Booking{ID: "b1", UserID: "owner"}, nil)

	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockCRMClient{}, &mockPaymentClient{}, &mockEventPublisher{}, zerolog.Nop(), "Europe/Moscow", 0)

	_, err := uc.CancelBooking(ctx, "b1", "other")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestCancelBooking_AlreadyCancelled(t *testing.T) {
	ctx := context.Background()

	repo := NewMockBookingRepository(t)
	repo.EXPECT().GetByID(mock.Anything, "b1").Return(&domain.Booking{ID: "b1", UserID: "u1", Status: domain.StatusCancelled}, nil)

	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockCRMClient{}, &mockPaymentClient{}, &mockEventPublisher{}, zerolog.Nop(), "Europe/Moscow", 0)

	_, err := uc.CancelBooking(ctx, "b1", "u1")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestConfirmBooking_Success(t *testing.T) {
	ctx := context.Background()
	const bookingID = "b1"
	const paymentID = "pay-xyz"

	confirmed := &domain.Booking{ID: bookingID, Status: domain.StatusConfirmed, PaymentID: paymentID}

	repo := NewMockBookingRepository(t)
	repo.EXPECT().ConfirmPayment(mock.Anything, bookingID, paymentID).Return(confirmed, true, nil)

	var published bool
	pub := &mockEventPublisher{
		PublishBookingConfirmedFunc: func(_ context.Context, b *domain.Booking) error {
			published = true
			assert.Equal(t, domain.StatusConfirmed, b.Status)
			return nil
		},
	}
	repo.EXPECT().MarkCompletedEventSent(mock.Anything, mock.Anything).Return(nil).Maybe()

	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockCRMClient{}, &mockPaymentClient{}, pub, zerolog.Nop(), "Europe/Moscow", 0)
	err := uc.ConfirmBooking(ctx, bookingID, paymentID)
	require.NoError(t, err)
	assert.True(t, published)
}

func TestConfirmBooking_IdempotentWhenConfirmed(t *testing.T) {
	ctx := context.Background()

	already := &domain.Booking{ID: "b1", Status: domain.StatusConfirmed, PaymentID: "p1"}

	repo := NewMockBookingRepository(t)
	repo.EXPECT().ConfirmPayment(mock.Anything, "b1", "p1").Return(already, false, nil)

	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockCRMClient{}, &mockPaymentClient{}, &mockEventPublisher{}, zerolog.Nop(), "Europe/Moscow", 0)
	require.NoError(t, uc.ConfirmBooking(ctx, "b1", "p1"))
}

func TestCancelBookingByPayment_OnlyWhenPaymentPending(t *testing.T) {
	ctx := context.Background()

	repo := NewMockBookingRepository(t)
	repo.EXPECT().GetByID(mock.Anything, "b1").Return(&domain.Booking{ID: "b1", UserID: "u1", VenueID: "v1", Status: domain.StatusConfirmed}, nil)

	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockCRMClient{}, &mockPaymentClient{}, &mockEventPublisher{}, zerolog.Nop(), "Europe/Moscow", 0)
	require.NoError(t, uc.CancelBookingByPayment(ctx, "b1"))
}

func TestAutoCompletePastVisits_PublishesCompleted(t *testing.T) {
	ctx := context.Background()
	refs := []domain.BookingCompletedRef{{ID: "b1", UserID: "u1", VenueID: "v1"}}

	repo := NewMockBookingRepository(t)
	repo.EXPECT().AutoCompleteVisitEnded(mock.Anything, "Europe/Moscow").Return(refs, nil)
	repo.EXPECT().MarkCompletedEventSent(mock.Anything, "b1").Return(nil)
	repo.EXPECT().FindUnpublishedCompleted(mock.Anything, mock.Anything).Return(nil, nil)
	repo.EXPECT().DeleteOrphanPending(mock.Anything, mock.Anything).Return(int64(0), nil)

	var published bool
	pub := &mockEventPublisher{
		PublishBookingCompletedFunc: func(_ context.Context, b *domain.Booking) error {
			published = true
			assert.Equal(t, domain.StatusCompleted, b.Status)
			assert.Equal(t, "b1", b.ID)
			return nil
		},
	}

	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockCRMClient{}, &mockPaymentClient{}, pub, zerolog.Nop(), "Europe/Moscow", 0)
	n, err := uc.AutoCompletePastVisits(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.True(t, published)
}

func TestHasCompletedBooking(t *testing.T) {
	ctx := context.Background()

	repo := NewMockBookingRepository(t)
	repo.EXPECT().HasCompleted(mock.Anything, "u1", "v1").Return(true, nil)

	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockCRMClient{}, &mockPaymentClient{}, &mockEventPublisher{}, zerolog.Nop(), "Europe/Moscow", 0)

	ok, err := uc.HasCompletedBooking(ctx, "u1", "v1")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestBookingIdempotencyKey(t *testing.T) {
	t.Parallel()

	t.Run("deterministic — same inputs produce same key", func(t *testing.T) {
		k1 := bookingIdempotencyKey("booking-uuid-123", "user-uuid-456")
		k2 := bookingIdempotencyKey("booking-uuid-123", "user-uuid-456")
		assert.Equal(t, k1, k2)
	})

	t.Run("different booking IDs produce different keys", func(t *testing.T) {
		k1 := bookingIdempotencyKey("booking-aaa", "user-111")
		k2 := bookingIdempotencyKey("booking-bbb", "user-111")
		assert.NotEqual(t, k1, k2)
	})

	t.Run("different user IDs produce different keys", func(t *testing.T) {
		k1 := bookingIdempotencyKey("booking-aaa", "user-111")
		k2 := bookingIdempotencyKey("booking-aaa", "user-222")
		assert.NotEqual(t, k1, k2)
	})

	t.Run("key is 64 hex chars (sha256)", func(t *testing.T) {
		k := bookingIdempotencyKey("any-booking", "any-user")
		assert.Len(t, k, 64)
	})

	t.Run("no key collision between swapped IDs", func(t *testing.T) {
		// sha256("a:b") != sha256("b:a") — separator ensures ordering matters
		k1 := bookingIdempotencyKey("aaaaa", "bbbbb")
		k2 := bookingIdempotencyKey("bbbbb", "aaaaa")
		assert.NotEqual(t, k1, k2)
	})
}
