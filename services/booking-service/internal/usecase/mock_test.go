// Вспомогательные моки для usecase-тестов.
//
// MockBookingRepository — в mock_repo_gen_test.go (генерируется mockery, не правится руками).
// Чтобы перегенерировать после изменения domain.BookingRepository:
//
//	go generate ./services/booking-service/internal/usecase/...
//
// mockVenueClient, mockCRMClient, mockPaymentClient, mockEventPublisher оставлены ручными:
// они реализуют proto-сгенерированные интерфейсы с большим количеством методов,
// большинство из которых в тестах не используются. Для них func-поля удобнее —
// ненужные методы возвращают nil молча.

package usecase

import (
	"context"

	"google.golang.org/grpc"

	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/booking-service/internal/domain"
)

type mockVenueClient struct {
	GetVenueFunc              func(ctx context.Context, in *venuev1.GetVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error)
	CheckSlotAvailabilityFunc func(ctx context.Context, in *venuev1.CheckSlotRequest, opts ...grpc.CallOption) (*venuev1.CheckSlotResponse, error)
	ReserveSlotFunc           func(ctx context.Context, in *venuev1.ReserveSlotRequest, opts ...grpc.CallOption) (*venuev1.ReserveSlotResponse, error)
	ReleaseSlotFunc           func(ctx context.Context, in *venuev1.ReleaseSlotRequest, opts ...grpc.CallOption) (*venuev1.ReleaseSlotResponse, error)
}

func (m *mockVenueClient) CreateVenue(ctx context.Context, in *venuev1.CreateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) SubmitVenueForReview(ctx context.Context, in *venuev1.SubmitVenueForReviewRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) UpdateVenue(ctx context.Context, in *venuev1.UpdateVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) GetVenue(ctx context.Context, in *venuev1.GetVenueRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	if m.GetVenueFunc != nil {
		return m.GetVenueFunc(ctx, in, opts...)
	}
	return &venuev1.VenueResponse{Id: in.GetId(), Name: "Mock venue", PriceFrom: 0}, nil
}
func (m *mockVenueClient) GetVenueBySlug(ctx context.Context, in *venuev1.GetVenueBySlugRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) GetVenuesBatch(ctx context.Context, in *venuev1.GetVenuesBatchRequest, opts ...grpc.CallOption) (*venuev1.GetVenuesBatchResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) ListVenues(ctx context.Context, in *venuev1.ListVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) SearchVenues(ctx context.Context, in *venuev1.SearchVenuesRequest, opts ...grpc.CallOption) (*venuev1.ListVenuesResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) CheckSlotAvailability(ctx context.Context, in *venuev1.CheckSlotRequest, opts ...grpc.CallOption) (*venuev1.CheckSlotResponse, error) {
	if m.CheckSlotAvailabilityFunc != nil {
		return m.CheckSlotAvailabilityFunc(ctx, in, opts...)
	}
	return nil, nil
}
func (m *mockVenueClient) BatchCheckSlotAvailability(ctx context.Context, in *venuev1.BatchCheckSlotRequest, opts ...grpc.CallOption) (*venuev1.BatchCheckSlotResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) GetPopularCities(ctx context.Context, in *venuev1.GetPopularCitiesRequest, opts ...grpc.CallOption) (*venuev1.GetPopularCitiesResponse, error) {
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
func (m *mockVenueClient) CreateManualSlotBlock(ctx context.Context, in *venuev1.CreateManualSlotBlockRequest, opts ...grpc.CallOption) (*venuev1.CreateManualSlotBlockResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) DeleteManualSlotBlock(ctx context.Context, in *venuev1.DeleteManualSlotBlockRequest, opts ...grpc.CallOption) (*venuev1.DeleteManualSlotBlockResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) ListManualSlotBlocks(ctx context.Context, in *venuev1.ListManualSlotBlocksRequest, opts ...grpc.CallOption) (*venuev1.ListManualSlotBlocksResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) GetVenueSchedule(ctx context.Context, in *venuev1.GetVenueScheduleRequest, opts ...grpc.CallOption) (*venuev1.GetVenueScheduleResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) GetVenueBookingMode(ctx context.Context, in *venuev1.GetVenueBookingModeRequest, opts ...grpc.CallOption) (*venuev1.GetVenueBookingModeResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) SetVenueBookingMode(ctx context.Context, in *venuev1.SetVenueBookingModeRequest, opts ...grpc.CallOption) (*venuev1.SetVenueBookingModeResponse, error) {
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
func (m *mockVenueClient) AddVenuePhoto(ctx context.Context, in *venuev1.AddVenuePhotoRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) DeleteVenuePhoto(ctx context.Context, in *venuev1.DeleteVenuePhotoRequest, opts ...grpc.CallOption) (*venuev1.DeleteVenuePhotoResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) SetVenueCoverPhoto(ctx context.Context, in *venuev1.SetVenueCoverPhotoRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) AddVenueHallPhoto(ctx context.Context, in *venuev1.AddVenueHallPhotoRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) DeleteVenueHallPhoto(ctx context.Context, in *venuev1.DeleteVenueHallPhotoRequest, opts ...grpc.CallOption) (*venuev1.DeleteVenueHallPhotoResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) SetVenueHallCoverPhoto(ctx context.Context, in *venuev1.SetVenueHallCoverPhotoRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) AddVenueVideo(ctx context.Context, in *venuev1.AddVenueVideoRequest, opts ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) DeleteVenueVideo(ctx context.Context, in *venuev1.DeleteVenueVideoRequest, opts ...grpc.CallOption) (*venuev1.DeleteVenueVideoResponse, error) {
	return nil, nil
}
func (m *mockVenueClient) SuspendVenuesByOwner(ctx context.Context, in *venuev1.SuspendVenuesByOwnerRequest, opts ...grpc.CallOption) (*venuev1.SuspendVenuesByOwnerResponse, error) {
	return nil, nil
}

// mockCRMClient implements crmv1.CRMServiceClient.
// GetManagementAccessFunc is overridable; default returns "owner" so existing
// tests that pre-date the extraction still pass without explicit setup.
type mockCRMClient struct {
	GetManagementAccessFunc func(ctx context.Context, in *crmv1.GetManagementAccessRequest, opts ...grpc.CallOption) (*crmv1.GetManagementAccessResponse, error)
}

func (m *mockCRMClient) GetManagementAccess(ctx context.Context, in *crmv1.GetManagementAccessRequest, opts ...grpc.CallOption) (*crmv1.GetManagementAccessResponse, error) {
	if m.GetManagementAccessFunc != nil {
		return m.GetManagementAccessFunc(ctx, in, opts...)
	}
	return &crmv1.GetManagementAccessResponse{Access: "owner"}, nil
}
func (m *mockCRMClient) BatchGetManagementAccess(ctx context.Context, in *crmv1.BatchGetManagementAccessRequest, opts ...grpc.CallOption) (*crmv1.BatchGetManagementAccessResponse, error) {
	return nil, nil
}
func (m *mockCRMClient) ListManagedVenues(ctx context.Context, in *crmv1.ListManagedVenuesRequest, opts ...grpc.CallOption) (*crmv1.ListManagedVenuesResponse, error) {
	return nil, nil
}
func (m *mockCRMClient) ListStaff(ctx context.Context, in *crmv1.ListStaffRequest, opts ...grpc.CallOption) (*crmv1.ListStaffResponse, error) {
	return nil, nil
}
func (m *mockCRMClient) AddStaff(ctx context.Context, in *crmv1.AddStaffRequest, opts ...grpc.CallOption) (*crmv1.AddStaffResponse, error) {
	return nil, nil
}
func (m *mockCRMClient) RemoveStaff(ctx context.Context, in *crmv1.RemoveStaffRequest, opts ...grpc.CallOption) (*crmv1.RemoveStaffResponse, error) {
	return nil, nil
}
func (m *mockCRMClient) ListTasks(ctx context.Context, in *crmv1.ListTasksRequest, opts ...grpc.CallOption) (*crmv1.ListTasksResponse, error) {
	return nil, nil
}
func (m *mockCRMClient) CreateTask(ctx context.Context, in *crmv1.CreateTaskRequest, opts ...grpc.CallOption) (*crmv1.CreateTaskResponse, error) {
	return nil, nil
}
func (m *mockCRMClient) CompleteTask(ctx context.Context, in *crmv1.CompleteTaskRequest, opts ...grpc.CallOption) (*crmv1.CompleteTaskResponse, error) {
	return nil, nil
}
func (m *mockCRMClient) UpdateTask(ctx context.Context, in *crmv1.UpdateTaskRequest, opts ...grpc.CallOption) (*crmv1.UpdateTaskResponse, error) {
	return nil, nil
}
func (m *mockCRMClient) ReopenTask(ctx context.Context, in *crmv1.ReopenTaskRequest, opts ...grpc.CallOption) (*crmv1.ReopenTaskResponse, error) {
	return nil, nil
}
func (m *mockCRMClient) CancelTask(ctx context.Context, in *crmv1.CancelTaskRequest, opts ...grpc.CallOption) (*crmv1.CancelTaskResponse, error) {
	return nil, nil
}
func (m *mockCRMClient) ListGuests(ctx context.Context, in *crmv1.ListGuestsRequest, opts ...grpc.CallOption) (*crmv1.ListGuestsResponse, error) {
	return nil, nil
}
func (m *mockCRMClient) GetGuest(ctx context.Context, in *crmv1.GetGuestRequest, opts ...grpc.CallOption) (*crmv1.GetGuestResponse, error) {
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
func (m *mockPaymentClient) SetPayoutMethod(ctx context.Context, in *paymentv1.SetPayoutMethodRequest, opts ...grpc.CallOption) (*paymentv1.PayoutMethodResponse, error) {
	return nil, nil
}
func (m *mockPaymentClient) GetPayoutMethod(ctx context.Context, in *paymentv1.GetPayoutMethodRequest, opts ...grpc.CallOption) (*paymentv1.PayoutMethodResponse, error) {
	return nil, nil
}
func (m *mockPaymentClient) GetPartnerBalance(ctx context.Context, in *paymentv1.GetPartnerBalanceRequest, opts ...grpc.CallOption) (*paymentv1.PartnerBalanceResponse, error) {
	return nil, nil
}
func (m *mockPaymentClient) ListPartnerLedger(ctx context.Context, in *paymentv1.ListPartnerLedgerRequest, opts ...grpc.CallOption) (*paymentv1.ListPartnerLedgerResponse, error) {
	return nil, nil
}
func (m *mockPaymentClient) ListPartnerPayouts(ctx context.Context, in *paymentv1.ListPartnerPayoutsRequest, opts ...grpc.CallOption) (*paymentv1.ListPartnerPayoutsResponse, error) {
	return nil, nil
}

type mockEventPublisher struct {
	PublishBookingCompletedFunc func(ctx context.Context, b *domain.Booking) error
	PublishRawFunc              func(ctx context.Context, subject string, payload []byte) error
}

func (m *mockEventPublisher) PublishBookingCompleted(ctx context.Context, b *domain.Booking) error {
	if m.PublishBookingCompletedFunc != nil {
		return m.PublishBookingCompletedFunc(ctx, b)
	}
	return nil
}
func (m *mockEventPublisher) PublishRaw(ctx context.Context, subject string, payload []byte) error {
	if m.PublishRawFunc != nil {
		return m.PublishRawFunc(ctx, subject, payload)
	}
	return nil
}
