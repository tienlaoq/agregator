package usecase

import (
	"context"

	"google.golang.org/grpc"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/review-service/internal/domain"
)

type mockReviewRepo struct {
	CreateFunc            func(ctx context.Context, review *domain.Review) error
	GetByIDFunc           func(ctx context.Context, id string) (*domain.Review, error)
	ListByVenueFunc       func(ctx context.Context, venueID string, page, pageSize int32) ([]*domain.Review, int32, error)
	GetVenueRatingFunc    func(ctx context.Context, venueID string) (*domain.VenueRating, error)
	UpdateVenueRatingFunc func(ctx context.Context, venueID string) error
}

func (m *mockReviewRepo) Create(ctx context.Context, review *domain.Review) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, review)
	}
	return nil
}

func (m *mockReviewRepo) GetByID(ctx context.Context, id string) (*domain.Review, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockReviewRepo) ListByVenue(ctx context.Context, venueID string, page, pageSize int32) ([]*domain.Review, int32, error) {
	if m.ListByVenueFunc != nil {
		return m.ListByVenueFunc(ctx, venueID, page, pageSize)
	}
	return nil, 0, nil
}

func (m *mockReviewRepo) GetVenueRating(ctx context.Context, venueID string) (*domain.VenueRating, error) {
	if m.GetVenueRatingFunc != nil {
		return m.GetVenueRatingFunc(ctx, venueID)
	}
	return nil, nil
}

func (m *mockReviewRepo) UpdateVenueRating(ctx context.Context, venueID string) error {
	if m.UpdateVenueRatingFunc != nil {
		return m.UpdateVenueRatingFunc(ctx, venueID)
	}
	return nil
}

type mockBookingClient struct {
	CreateBookingFunc        func(ctx context.Context, in *bookingv1.CreateBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error)
	GetBookingFunc           func(ctx context.Context, in *bookingv1.GetBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error)
	ListUserBookingsFunc     func(ctx context.Context, in *bookingv1.ListUserBookingsRequest, opts ...grpc.CallOption) (*bookingv1.ListBookingsResponse, error)
	ListVenueBookingsFunc    func(ctx context.Context, in *bookingv1.ListVenueBookingsRequest, opts ...grpc.CallOption) (*bookingv1.ListBookingsResponse, error)
	CancelBookingFunc        func(ctx context.Context, in *bookingv1.CancelBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error)
	ConfirmBookingFunc       func(ctx context.Context, in *bookingv1.ConfirmBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error)
	CompleteBookingFunc      func(ctx context.Context, in *bookingv1.CompleteBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error)
	HasCompletedBookingFunc  func(ctx context.Context, in *bookingv1.HasCompletedBookingRequest, opts ...grpc.CallOption) (*bookingv1.HasCompletedBookingResponse, error)
}

func (m *mockBookingClient) CreateBooking(ctx context.Context, in *bookingv1.CreateBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if m.CreateBookingFunc != nil {
		return m.CreateBookingFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockBookingClient) GetBooking(ctx context.Context, in *bookingv1.GetBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if m.GetBookingFunc != nil {
		return m.GetBookingFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockBookingClient) ListUserBookings(ctx context.Context, in *bookingv1.ListUserBookingsRequest, opts ...grpc.CallOption) (*bookingv1.ListBookingsResponse, error) {
	if m.ListUserBookingsFunc != nil {
		return m.ListUserBookingsFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockBookingClient) ListVenueBookings(ctx context.Context, in *bookingv1.ListVenueBookingsRequest, opts ...grpc.CallOption) (*bookingv1.ListBookingsResponse, error) {
	if m.ListVenueBookingsFunc != nil {
		return m.ListVenueBookingsFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockBookingClient) CancelBooking(ctx context.Context, in *bookingv1.CancelBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if m.CancelBookingFunc != nil {
		return m.CancelBookingFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockBookingClient) ConfirmBooking(ctx context.Context, in *bookingv1.ConfirmBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if m.ConfirmBookingFunc != nil {
		return m.ConfirmBookingFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockBookingClient) CompleteBooking(ctx context.Context, in *bookingv1.CompleteBookingRequest, opts ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if m.CompleteBookingFunc != nil {
		return m.CompleteBookingFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockBookingClient) HasCompletedBooking(ctx context.Context, in *bookingv1.HasCompletedBookingRequest, opts ...grpc.CallOption) (*bookingv1.HasCompletedBookingResponse, error) {
	if m.HasCompletedBookingFunc != nil {
		return m.HasCompletedBookingFunc(ctx, in, opts...)
	}
	return &bookingv1.HasCompletedBookingResponse{}, nil
}

type mockVenueClient struct {
	CheckSlotAvailabilityFunc func(ctx context.Context, in *venuev1.CheckSlotRequest, opts ...grpc.CallOption) (*venuev1.CheckSlotResponse, error)
	ReserveSlotFunc           func(ctx context.Context, in *venuev1.ReserveSlotRequest, opts ...grpc.CallOption) (*venuev1.ReserveSlotResponse, error)
	ReleaseSlotFunc           func(ctx context.Context, in *venuev1.ReleaseSlotRequest, opts ...grpc.CallOption) (*venuev1.ReleaseSlotResponse, error)
	UpdateRatingFunc          func(ctx context.Context, in *venuev1.UpdateRatingRequest, opts ...grpc.CallOption) (*venuev1.UpdateRatingResponse, error)
}

func (m *mockVenueClient) CreateVenue(ctx context.Context, in *venuev1.CreateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}

func (m *mockVenueClient) UpdateVenue(ctx context.Context, in *venuev1.UpdateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}

func (m *mockVenueClient) GetVenue(ctx context.Context, in *venuev1.GetVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}

func (m *mockVenueClient) GetVenueBySlug(ctx context.Context, in *venuev1.GetVenueBySlugRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}

func (m *mockVenueClient) ListVenues(ctx context.Context, in *venuev1.ListVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	return nil, nil
}

func (m *mockVenueClient) SearchVenues(ctx context.Context, in *venuev1.SearchVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	return nil, nil
}

func (m *mockVenueClient) ListOwnerVenues(ctx context.Context, in *venuev1.ListOwnerVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	return nil, nil
}

func (m *mockVenueClient) CheckSlotAvailability(ctx context.Context, in *venuev1.CheckSlotRequest, opts ...grpc.CallOption) (*venuev1.CheckSlotResponse, error) {
	if m.CheckSlotAvailabilityFunc != nil {
		return m.CheckSlotAvailabilityFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) ReserveSlot(ctx context.Context, in *venuev1.ReserveSlotRequest, opts ...grpc.CallOption) (*venuev1.ReserveSlotResponse, error) {
	if m.ReserveSlotFunc != nil {
		return m.ReserveSlotFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) ReleaseSlot(ctx context.Context, in *venuev1.ReleaseSlotRequest, opts ...grpc.CallOption) (*venuev1.ReleaseSlotResponse, error) {
	if m.ReleaseSlotFunc != nil {
		return m.ReleaseSlotFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) UpdateRating(ctx context.Context, in *venuev1.UpdateRatingRequest, opts ...grpc.CallOption) (*venuev1.UpdateRatingResponse, error) {
	if m.UpdateRatingFunc != nil {
		return m.UpdateRatingFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockVenueClient) ModerateVenue(ctx context.Context, in *venuev1.ModerateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}

func (m *mockVenueClient) ListPendingVenues(ctx context.Context, in *venuev1.ListPendingVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	return nil, nil
}
