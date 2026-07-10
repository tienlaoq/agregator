package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	"github.com/tienlao/agregator/services/booking-service/internal/domain"
)

// ucWith builds a BookingUseCase with the given repo and CRM client; the venue,
// payment, and publisher mocks default to no-op success. cancelDeadlineHours=0
// disables the cancel-deadline guard so success paths are deterministic.
func ucWith(repo domain.BookingRepository, crm crmv1.CRMServiceClient) *BookingUseCase {
	if crm == nil {
		crm = &mockCRMClient{}
	}
	return NewBookingUseCase(repo, &mockVenueClient{}, crm, &mockPaymentClient{}, &mockEventPublisher{}, zerolog.Nop(), "Europe/Moscow", 0)
}

func confirmedBooking(id, userID string) *domain.Booking {
	from, _ := domain.ParseTimeOfDay("10:00")
	to, _ := domain.ParseTimeOfDay("12:00")
	return &domain.Booking{ID: id, UserID: userID, VenueID: "v1", Status: domain.StatusConfirmed, TimeFrom: from, TimeTo: to}
}

func TestGetBooking_And_Batch(t *testing.T) {
	ctx := context.Background()
	repo := NewMockBookingRepository(t)
	repo.EXPECT().GetByID(ctx, "b1").Return(&domain.Booking{ID: "b1"}, nil)
	repo.EXPECT().GetByIDs(ctx, []string{"b1", "b2"}).Return([]*domain.Booking{{ID: "b1"}, {ID: "b2"}}, nil)
	uc := ucWith(repo, nil)

	b, err := uc.GetBooking(ctx, "b1")
	require.NoError(t, err)
	require.Equal(t, "b1", b.ID)

	list, err := uc.GetBookingsBatch(ctx, []string{"b1", "b2"})
	require.NoError(t, err)
	require.Len(t, list, 2)
}

func TestListUserBookings_PaginationDefaults(t *testing.T) {
	ctx := context.Background()
	repo := NewMockBookingRepository(t)
	// page<=0 → 1, pageSize<=0 → 20, so offset must be 0 and limit 20.
	repo.EXPECT().ListByUser(ctx, "u1", "", 0, 20).Return([]*domain.Booking{{ID: "b1"}}, 1, nil)
	uc := ucWith(repo, nil)

	list, total, err := uc.ListUserBookings(ctx, "u1", "", 0, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, list, 1)
}

func TestListVenueBookings_AccessControl(t *testing.T) {
	ctx := context.Background()

	t.Run("empty requester rejected", func(t *testing.T) {
		uc := ucWith(NewMockBookingRepository(t), nil)
		_, _, _, err := uc.ListVenueBookings(ctx, "v1", "", "", "", "", "", "", 20)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("no management access → PermissionDenied", func(t *testing.T) {
		crm := &mockCRMClient{GetManagementAccessFunc: func(context.Context, *crmv1.GetManagementAccessRequest, ...grpc.CallOption) (*crmv1.GetManagementAccessResponse, error) {
			return &crmv1.GetManagementAccessResponse{Access: ""}, nil
		}}
		uc := ucWith(NewMockBookingRepository(t), crm)
		_, _, _, err := uc.ListVenueBookings(ctx, "v1", "u1", "", "", "", "", "", 20)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("access granted → repo result", func(t *testing.T) {
		repo := NewMockBookingRepository(t)
		repo.EXPECT().ListByVenue(ctx, "v1", "", "", "", "", "", 20).Return([]*domain.Booking{{ID: "b1"}}, 1, "next", nil)
		// default mockCRMClient grants "owner" access.
		uc := ucWith(repo, nil)
		list, total, cursor, err := uc.ListVenueBookings(ctx, "v1", "u1", "", "", "", "", "", 20)
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, 1, total)
		require.Equal(t, "next", cursor)
	})
}

func TestAddBookingStaffNote(t *testing.T) {
	ctx := context.Background()

	t.Run("empty body rejected", func(t *testing.T) {
		uc := ucWith(NewMockBookingRepository(t), nil)
		_, err := uc.AddBookingStaffNote(ctx, "b1", "u1", "   ")
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("too long rejected", func(t *testing.T) {
		uc := ucWith(NewMockBookingRepository(t), nil)
		_, err := uc.AddBookingStaffNote(ctx, "b1", "u1", strings.Repeat("я", maxBookingStaffNoteRunes+1))
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("success", func(t *testing.T) {
		repo := NewMockBookingRepository(t)
		repo.EXPECT().GetByID(ctx, "b1").Return(&domain.Booking{ID: "b1", VenueID: "v1"}, nil)
		repo.EXPECT().AddBookingStaffNote(ctx, mock.Anything).
			Run(func(args mock.Arguments) { args.Get(1).(*domain.BookingStaffNote).ID = "note-1" }).Return(nil)
		uc := ucWith(repo, nil) // default CRM grants access
		note, err := uc.AddBookingStaffNote(ctx, "b1", "u1", "  гость опаздывает  ")
		require.NoError(t, err)
		require.Equal(t, "note-1", note.ID)
		require.Equal(t, "гость опаздывает", note.Body, "body is trimmed")
		require.Equal(t, "u1", note.AuthorUserID)
	})
}

func TestListBookingStaffNotes_Success(t *testing.T) {
	ctx := context.Background()
	repo := NewMockBookingRepository(t)
	repo.EXPECT().GetByID(ctx, "b1").Return(&domain.Booking{ID: "b1", VenueID: "v1"}, nil)
	repo.EXPECT().ListBookingStaffNotes(ctx, "b1").Return([]domain.BookingStaffNote{{ID: "n1", Body: "x"}}, nil)
	uc := ucWith(repo, nil)

	notes, err := uc.ListBookingStaffNotes(ctx, "b1", "u1")
	require.NoError(t, err)
	require.Len(t, notes, 1)
}

func TestCancelBooking(t *testing.T) {
	ctx := context.Background()

	t.Run("not your booking", func(t *testing.T) {
		repo := NewMockBookingRepository(t)
		repo.EXPECT().GetByID(ctx, "b1").Return(confirmedBooking("b1", "owner"), nil)
		uc := ucWith(repo, nil)
		_, err := uc.CancelBooking(ctx, "b1", "intruder")
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("terminal status rejected", func(t *testing.T) {
		b := confirmedBooking("b1", "u1")
		b.Status = domain.StatusCancelled
		repo := NewMockBookingRepository(t)
		repo.EXPECT().GetByID(ctx, "b1").Return(b, nil)
		uc := ucWith(repo, nil)
		_, err := uc.CancelBooking(ctx, "b1", "u1")
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("success writes cancelled event", func(t *testing.T) {
		repo := NewMockBookingRepository(t)
		repo.EXPECT().GetByID(ctx, "b1").Return(confirmedBooking("b1", "u1"), nil)
		repo.EXPECT().CancelWithEvent(ctx, "b1", mock.Anything).Return(nil)
		uc := ucWith(repo, nil) // cancelDeadlineHours=0 skips the deadline guard
		got, err := uc.CancelBooking(ctx, "b1", "u1")
		require.NoError(t, err)
		require.Equal(t, domain.StatusCancelled, got.Status)
	})
}

func TestCompleteBooking(t *testing.T) {
	ctx := context.Background()

	t.Run("not completable rejected", func(t *testing.T) {
		repo := NewMockBookingRepository(t)
		repo.EXPECT().GetByID(ctx, "b1").Return(confirmedBooking("b1", "u1"), nil) // no payment id
		uc := ucWith(repo, nil)
		_, err := uc.CompleteBooking(ctx, "b1")
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("success", func(t *testing.T) {
		b := confirmedBooking("b1", "u1")
		b.PaymentID = "pay-1"
		repo := NewMockBookingRepository(t)
		repo.EXPECT().GetByID(ctx, "b1").Return(b, nil)
		repo.EXPECT().UpdateStatus(ctx, "b1", string(domain.StatusCompleted)).Return(nil)
		uc := ucWith(repo, nil)
		got, err := uc.CompleteBooking(ctx, "b1")
		require.NoError(t, err)
		require.Equal(t, domain.StatusCompleted, got.Status)
	})
}

func TestCancelBookingByPayment(t *testing.T) {
	ctx := context.Background()

	t.Run("not found is a no-op", func(t *testing.T) {
		repo := NewMockBookingRepository(t)
		repo.EXPECT().GetByID(ctx, "b1").Return(nil, status.Error(codes.NotFound, "gone"))
		uc := ucWith(repo, nil)
		require.NoError(t, uc.CancelBookingByPayment(ctx, "b1"))
	})

	t.Run("non-payment-pending is a no-op", func(t *testing.T) {
		repo := NewMockBookingRepository(t)
		repo.EXPECT().GetByID(ctx, "b1").Return(confirmedBooking("b1", "u1"), nil) // confirmed, not payment_pending
		uc := ucWith(repo, nil)
		require.NoError(t, uc.CancelBookingByPayment(ctx, "b1"))
	})

	t.Run("payment_pending → cancelled + event", func(t *testing.T) {
		b := confirmedBooking("b1", "u1")
		b.Status = domain.StatusPaymentPending
		repo := NewMockBookingRepository(t)
		repo.EXPECT().GetByID(ctx, "b1").Return(b, nil)
		repo.EXPECT().CancelWithEvent(ctx, "b1", mock.Anything).Return(nil)
		uc := ucWith(repo, nil)
		require.NoError(t, uc.CancelBookingByPayment(ctx, "b1"))
	})
}

func TestConfirmBooking(t *testing.T) {
	ctx := context.Background()

	t.Run("transition confirmed", func(t *testing.T) {
		repo := NewMockBookingRepository(t)
		repo.EXPECT().ConfirmPayment(ctx, "b1", "pay-1", "booking.confirmed").Return(&domain.Booking{ID: "b1"}, true, nil)
		uc := ucWith(repo, nil)
		require.NoError(t, uc.ConfirmBooking(ctx, "b1", "pay-1"))
	})

	t.Run("already terminal is idempotent no-op", func(t *testing.T) {
		repo := NewMockBookingRepository(t)
		repo.EXPECT().ConfirmPayment(ctx, "b1", "pay-1", "booking.confirmed").Return(nil, false, nil)
		uc := ucWith(repo, nil)
		require.NoError(t, uc.ConfirmBooking(ctx, "b1", "pay-1"))
	})

	t.Run("repo error propagates", func(t *testing.T) {
		repo := NewMockBookingRepository(t)
		repo.EXPECT().ConfirmPayment(ctx, "b1", "pay-1", "booking.confirmed").Return(nil, false, errors.New("db down"))
		uc := ucWith(repo, nil)
		require.Error(t, uc.ConfirmBooking(ctx, "b1", "pay-1"))
	})
}
