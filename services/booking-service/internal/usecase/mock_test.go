package usecase

import (
	"context"

	"google.golang.org/grpc"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/booking-service/internal/domain"
)

type mockBookingRepo struct {
	CreateFunc        func(ctx context.Context, b *domain.Booking) error
	GetByIDFunc       func(ctx context.Context, id string) (*domain.Booking, error)
	ListByUserFunc    func(ctx context.Context, userID, status string, offset, limit int) ([]*domain.Booking, int, error)
	ListByVenueFunc   func(ctx context.Context, venueID, status, date string, offset, limit int) ([]*domain.Booking, int, error)
	UpdateStatusFunc  func(ctx context.Context, id, status string) error
	SetPaymentIDFunc  func(ctx context.Context, bookingID, paymentID string) error
	HasCompletedFunc  func(ctx context.Context, userID, venueID string) (bool, error)
}

func (m *mockBookingRepo) Create(ctx context.Context, b *domain.Booking) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, b)
	}
	return nil
}

func (m *mockBookingRepo) GetByID(ctx context.Context, id string) (*domain.Booking, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockBookingRepo) ListByUser(ctx context.Context, userID, status string, offset, limit int) ([]*domain.Booking, int, error) {
	if m.ListByUserFunc != nil {
		return m.ListByUserFunc(ctx, userID, status, offset, limit)
	}
	return nil, 0, nil
}

func (m *mockBookingRepo) ListByVenue(ctx context.Context, venueID, status, date string, offset, limit int) ([]*domain.Booking, int, error) {
	if m.ListByVenueFunc != nil {
		return m.ListByVenueFunc(ctx, venueID, status, date, offset, limit)
	}
	return nil, 0, nil
}

func (m *mockBookingRepo) UpdateStatus(ctx context.Context, id, status string) error {
	if m.UpdateStatusFunc != nil {
		return m.UpdateStatusFunc(ctx, id, status)
	}
	return nil
}

func (m *mockBookingRepo) SetPaymentID(ctx context.Context, bookingID, paymentID string) error {
	if m.SetPaymentIDFunc != nil {
		return m.SetPaymentIDFunc(ctx, bookingID, paymentID)
	}
	return nil
}

func (m *mockBookingRepo) HasCompleted(ctx context.Context, userID, venueID string) (bool, error) {
	if m.HasCompletedFunc != nil {
		return m.HasCompletedFunc(ctx, userID, venueID)
	}
	return false, nil
}

type mockVenueClient struct {
	CheckSlotAvailabilityFunc func(ctx context.Context, in *venuev1.CheckSlotRequest, opts ...grpc.CallOption) (*venuev1.CheckSlotResponse, error)
	ReserveSlotFunc           func(ctx context.Context, in *venuev1.ReserveSlotRequest, opts ...grpc.CallOption) (*venuev1.ReserveSlotResponse, error)
	ReleaseSlotFunc           func(ctx context.Context, in *venuev1.ReleaseSlotRequest, opts ...grpc.CallOption) (*venuev1.ReleaseSlotResponse, error)
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
	return nil, nil
}

func (m *mockVenueClient) ModerateVenue(ctx context.Context, in *venuev1.ModerateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}

func (m *mockVenueClient) ListPendingVenues(ctx context.Context, in *venuev1.ListPendingVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	return nil, nil
}

type mockPaymentClient struct {
	CreatePaymentFunc func(ctx context.Context, in *paymentv1.CreatePaymentRequest, opts ...grpc.CallOption) (*paymentv1.PaymentResponse, error)
}

func (m *mockPaymentClient) CreatePayment(ctx context.Context, in *paymentv1.CreatePaymentRequest, opts ...grpc.CallOption) (*paymentv1.PaymentResponse, error) {
	if m.CreatePaymentFunc != nil {
		return m.CreatePaymentFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockPaymentClient) GetPayment(ctx context.Context, in *paymentv1.GetPaymentRequest, opts ...grpc.CallOption) (*paymentv1.PaymentResponse, error) {
	return nil, nil
}

func (m *mockPaymentClient) GetPaymentByBooking(ctx context.Context, in *paymentv1.GetPaymentByBookingRequest, opts ...grpc.CallOption) (*paymentv1.PaymentResponse, error) {
	return nil, nil
}

func (m *mockPaymentClient) HandleWebhook(ctx context.Context, in *paymentv1.WebhookRequest, opts ...grpc.CallOption) (*paymentv1.WebhookResponse, error) {
	return nil, nil
}

type mockEventPublisher struct {
	PublishBookingCreatedFunc   func(ctx context.Context, b *domain.Booking) error
	PublishBookingConfirmedFunc func(ctx context.Context, b *domain.Booking) error
	PublishBookingCancelledFunc func(ctx context.Context, b *domain.Booking) error
}

func (m *mockEventPublisher) PublishBookingCreated(ctx context.Context, b *domain.Booking) error {
	if m.PublishBookingCreatedFunc != nil {
		return m.PublishBookingCreatedFunc(ctx, b)
	}
	return nil
}

func (m *mockEventPublisher) PublishBookingConfirmed(ctx context.Context, b *domain.Booking) error {
	if m.PublishBookingConfirmedFunc != nil {
		return m.PublishBookingConfirmedFunc(ctx, b)
	}
	return nil
}

func (m *mockEventPublisher) PublishBookingCancelled(ctx context.Context, b *domain.Booking) error {
	if m.PublishBookingCancelledFunc != nil {
		return m.PublishBookingCancelledFunc(ctx, b)
	}
	return nil
}
