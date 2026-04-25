package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

	var reserveCalled bool
	repo := &mockBookingRepo{
		CreateFunc: func(_ context.Context, b *domain.Booking) error {
			b.ID = bookingID
			b.CreatedAt = time.Now()
			b.UpdatedAt = time.Now()
			return nil
		},
		SetPaymentIDFunc: func(_ context.Context, bid, pid string) error {
			require.Equal(t, bookingID, bid)
			require.Equal(t, paymentID, pid)
			return nil
		},
		UpdateStatusFunc: func(_ context.Context, id, st string) error {
			require.Equal(t, bookingID, id)
			require.Equal(t, "payment_pending", st)
			return nil
		},
	}

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
			require.Equal(t, bookingID, in.IdempotencyKey)
			return &paymentv1.PaymentResponse{Id: paymentID, PaymentUrl: "https://pay.example/p"}, nil
		},
	}

	pub := &mockEventPublisher{
		PublishBookingCreatedFunc: func(_ context.Context, b *domain.Booking) error {
			require.Equal(t, bookingID, b.ID)
			require.Equal(t, "payment_pending", b.Status)
			return nil
		},
	}

	uc := NewBookingUseCase(repo, venue, payment, pub, "Europe/Moscow")

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
	assert.Equal(t, "payment_pending", out.Status)
	assert.Equal(t, paymentID, out.PaymentID)
	assert.Equal(t, "https://pay.example/p", out.PaymentURL)
}

func TestCreateBooking_SlotNotAvailable(t *testing.T) {
	ctx := context.Background()
	testDate := visitDateNotInPast(t)
	venue := &mockVenueClient{
		CheckSlotAvailabilityFunc: func(_ context.Context, _ *venuev1.CheckSlotRequest, _ ...grpc.CallOption) (*venuev1.CheckSlotResponse, error) {
			return &venuev1.CheckSlotResponse{Available: false}, nil
		},
	}
	uc := NewBookingUseCase(&mockBookingRepo{}, venue, &mockPaymentClient{}, &mockEventPublisher{}, "Europe/Moscow")

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
	var deleted bool

	repo := &mockBookingRepo{
		CreateFunc: func(_ context.Context, b *domain.Booking) error {
			b.ID = bookingID
			b.CreatedAt = time.Now()
			b.UpdatedAt = time.Now()
			return nil
		},
		DeleteFunc: func(_ context.Context, id string) error {
			deleted = true
			assert.Equal(t, bookingID, id)
			return nil
		},
	}

	venue := &mockVenueClient{
		CheckSlotAvailabilityFunc: func(_ context.Context, _ *venuev1.CheckSlotRequest, _ ...grpc.CallOption) (*venuev1.CheckSlotResponse, error) {
			return &venuev1.CheckSlotResponse{Available: true}, nil
		},
		ReserveSlotFunc: func(_ context.Context, _ *venuev1.ReserveSlotRequest, _ ...grpc.CallOption) (*venuev1.ReserveSlotResponse, error) {
			return nil, status.Error(codes.InvalidArgument, "time slot not available")
		},
	}

	uc := NewBookingUseCase(repo, venue, &mockPaymentClient{}, &mockEventPublisher{}, "Europe/Moscow")

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
	assert.True(t, deleted)
}

func TestCancelBooking_Success(t *testing.T) {
	ctx := context.Background()
	const bookingID = "b1"
	const userID = "user-1"

	b := &domain.Booking{
		ID:       bookingID,
		UserID:   userID,
		VenueID:  "venue-1",
		Status:   "pending",
		TimeFrom: "10:00",
		TimeTo:   "12:00",
	}

	var released bool
	var published bool
	repo := &mockBookingRepo{
		GetByIDFunc: func(_ context.Context, id string) (*domain.Booking, error) {
			require.Equal(t, bookingID, id)
			cp := *b
			return &cp, nil
		},
		UpdateStatusFunc: func(_ context.Context, id, st string) error {
			require.Equal(t, bookingID, id)
			require.Equal(t, "cancelled", st)
			return nil
		},
	}

	venue := &mockVenueClient{
		ReleaseSlotFunc: func(_ context.Context, in *venuev1.ReleaseSlotRequest, _ ...grpc.CallOption) (*venuev1.ReleaseSlotResponse, error) {
			released = true
			require.Equal(t, "venue-1", in.VenueId)
			require.Equal(t, bookingID, in.BookingId)
			return &venuev1.ReleaseSlotResponse{}, nil
		},
	}

	pub := &mockEventPublisher{
		PublishBookingCancelledFunc: func(_ context.Context, got *domain.Booking) error {
			published = true
			assert.Equal(t, "cancelled", got.Status)
			return nil
		},
	}

	uc := NewBookingUseCase(repo, venue, &mockPaymentClient{}, pub, "Europe/Moscow")
	out, err := uc.CancelBooking(ctx, bookingID, userID)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "cancelled", out.Status)
	assert.True(t, released)
	assert.True(t, published)
}

func TestCancelBooking_NotOwner(t *testing.T) {
	ctx := context.Background()
	repo := &mockBookingRepo{
		GetByIDFunc: func(_ context.Context, id string) (*domain.Booking, error) {
			return &domain.Booking{ID: id, UserID: "owner"}, nil
		},
	}
	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockPaymentClient{}, &mockEventPublisher{}, "Europe/Moscow")

	_, err := uc.CancelBooking(ctx, "b1", "other")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestCancelBooking_AlreadyCancelled(t *testing.T) {
	ctx := context.Background()
	repo := &mockBookingRepo{
		GetByIDFunc: func(_ context.Context, _ string) (*domain.Booking, error) {
			return &domain.Booking{ID: "b1", UserID: "u1", Status: "cancelled"}, nil
		},
	}
	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockPaymentClient{}, &mockEventPublisher{}, "Europe/Moscow")

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

	var setPaymentCalls int
	var statusCalls int
	var published bool
	var getCalls int

	repo := &mockBookingRepo{
		SetPaymentIDFunc: func(_ context.Context, bid, pid string) error {
			setPaymentCalls++
			require.Equal(t, bookingID, bid)
			require.Equal(t, paymentID, pid)
			return nil
		},
		UpdateStatusFunc: func(_ context.Context, id, st string) error {
			statusCalls++
			require.Equal(t, bookingID, id)
			require.Equal(t, "confirmed", st)
			return nil
		},
		GetByIDFunc: func(_ context.Context, id string) (*domain.Booking, error) {
			require.Equal(t, bookingID, id)
			getCalls++
			if getCalls == 1 {
				return &domain.Booking{ID: bookingID, Status: "payment_pending", PaymentID: paymentID}, nil
			}
			return &domain.Booking{ID: bookingID, Status: "confirmed", PaymentID: paymentID}, nil
		},
	}

	pub := &mockEventPublisher{
		PublishBookingConfirmedFunc: func(_ context.Context, b *domain.Booking) error {
			published = true
			assert.Equal(t, "confirmed", b.Status)
			return nil
		},
	}

	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockPaymentClient{}, pub, "Europe/Moscow")
	err := uc.ConfirmBooking(ctx, bookingID, paymentID)
	require.NoError(t, err)
	assert.Equal(t, 0, setPaymentCalls)
	assert.Equal(t, 1, statusCalls)
	assert.Equal(t, 2, getCalls)
	assert.True(t, published)
}

func TestConfirmBooking_IdempotentWhenConfirmed(t *testing.T) {
	ctx := context.Background()
	var statusCalls int
	repo := &mockBookingRepo{
		GetByIDFunc: func(_ context.Context, _ string) (*domain.Booking, error) {
			return &domain.Booking{ID: "b1", Status: "confirmed", PaymentID: "p1"}, nil
		},
		UpdateStatusFunc: func(_ context.Context, _, _ string) error {
			statusCalls++
			return nil
		},
	}
	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockPaymentClient{}, &mockEventPublisher{}, "Europe/Moscow")
	require.NoError(t, uc.ConfirmBooking(ctx, "b1", "p1"))
	assert.Equal(t, 0, statusCalls)
}

func TestConfirmBooking_SetsPaymentIDWhenMissing(t *testing.T) {
	ctx := context.Background()
	const bookingID = "b1"
	const paymentID = "pay-new"
	var setPaymentCalls int
	var getCalls int
	repo := &mockBookingRepo{
		SetPaymentIDFunc: func(_ context.Context, bid, pid string) error {
			setPaymentCalls++
			assert.Equal(t, bookingID, bid)
			assert.Equal(t, paymentID, pid)
			return nil
		},
		UpdateStatusFunc: func(_ context.Context, _, st string) error {
			assert.Equal(t, "confirmed", st)
			return nil
		},
		GetByIDFunc: func(_ context.Context, _ string) (*domain.Booking, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.Booking{ID: bookingID, Status: "payment_pending", PaymentID: ""}, nil
			}
			return &domain.Booking{ID: bookingID, Status: "confirmed", PaymentID: paymentID}, nil
		},
	}
	pub := &mockEventPublisher{
		PublishBookingConfirmedFunc: func(_ context.Context, b *domain.Booking) error {
			assert.Equal(t, "confirmed", b.Status)
			return nil
		},
	}
	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockPaymentClient{}, pub, "Europe/Moscow")
	require.NoError(t, uc.ConfirmBooking(ctx, bookingID, paymentID))
	assert.Equal(t, 1, setPaymentCalls)
}

func TestCancelBookingByPayment_OnlyWhenPaymentPending(t *testing.T) {
	ctx := context.Background()
	var statusCalls int
	repo := &mockBookingRepo{
		GetByIDFunc: func(_ context.Context, _ string) (*domain.Booking, error) {
			return &domain.Booking{ID: "b1", UserID: "u1", VenueID: "v1", Status: "confirmed"}, nil
		},
		UpdateStatusFunc: func(_ context.Context, _, _ string) error {
			statusCalls++
			return nil
		},
	}
	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockPaymentClient{}, &mockEventPublisher{}, "Europe/Moscow")
	require.NoError(t, uc.CancelBookingByPayment(ctx, "b1"))
	assert.Equal(t, 0, statusCalls)
}

func TestAutoCompletePastVisits_PublishesCompleted(t *testing.T) {
	ctx := context.Background()
	refs := []domain.BookingCompletedRef{{ID: "b1", UserID: "u1", VenueID: "v1"}}
	var published bool
	repo := &mockBookingRepo{
		AutoCompleteVisitEndedFunc: func(_ context.Context, tz string) ([]domain.BookingCompletedRef, error) {
			assert.Equal(t, "Europe/Moscow", tz)
			return refs, nil
		},
	}
	pub := &mockEventPublisher{
		PublishBookingCompletedFunc: func(_ context.Context, b *domain.Booking) error {
			published = true
			assert.Equal(t, "completed", b.Status)
			assert.Equal(t, "b1", b.ID)
			return nil
		},
	}
	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockPaymentClient{}, pub, "Europe/Moscow")
	n, err := uc.AutoCompletePastVisits(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.True(t, published)
}

func TestHasCompletedBooking(t *testing.T) {
	ctx := context.Background()
	repo := &mockBookingRepo{
		HasCompletedFunc: func(_ context.Context, userID, venueID string) (bool, error) {
			require.Equal(t, "u1", userID)
			require.Equal(t, "v1", venueID)
			return true, nil
		},
	}
	uc := NewBookingUseCase(repo, &mockVenueClient{}, &mockPaymentClient{}, &mockEventPublisher{}, "Europe/Moscow")

	ok, err := uc.HasCompletedBooking(ctx, "u1", "v1")
	require.NoError(t, err)
	assert.True(t, ok)
}
