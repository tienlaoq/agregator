package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/booking-service/internal/domain"
)

type EventPublisher interface {
	PublishBookingCreated(ctx context.Context, b *domain.Booking) error
	PublishBookingConfirmed(ctx context.Context, b *domain.Booking) error
	PublishBookingCancelled(ctx context.Context, b *domain.Booking) error
	PublishBookingCompleted(ctx context.Context, b *domain.Booking) error
}

type BookingUseCase struct {
	repo           domain.BookingRepository
	venueClient    venuev1.VenueServiceClient
	paymentClient  paymentv1.PaymentServiceClient
	publisher      EventPublisher
	visitTimeZone  string
}

func NewBookingUseCase(
	repo domain.BookingRepository,
	venueClient venuev1.VenueServiceClient,
	paymentClient paymentv1.PaymentServiceClient,
	publisher EventPublisher,
	visitTimeZone string,
) *BookingUseCase {
	if visitTimeZone == "" {
		visitTimeZone = "Europe/Moscow"
	}
	return &BookingUseCase{
		repo:          repo,
		venueClient:   venueClient,
		paymentClient: paymentClient,
		publisher:     publisher,
		visitTimeZone: visitTimeZone,
	}
}

type CreateBookingInput struct {
	UserID    string
	VenueID   string
	VenueName string
	ServiceID string
	Date      string
	TimeFrom  string
	TimeTo    string
	Guests    int32
	Comment   string
}

func (uc *BookingUseCase) CreateBooking(ctx context.Context, in CreateBookingInput) (*domain.Booking, error) {
	if strings.TrimSpace(in.TimeTo) == "" {
		return nil, pkgerr.InvalidArgument("укажите время окончания визита")
	}

	vResp, err := uc.venueClient.GetVenue(ctx, &venuev1.GetVenueRequest{Id: in.VenueID})
	if err != nil {
		return nil, fmt.Errorf("get venue: %w", err)
	}
	if vResp == nil {
		return nil, pkgerr.NotFound("заведение не найдено")
	}

	minutes, err := slotDurationMinutes(in.TimeFrom, in.TimeTo)
	if err != nil {
		return nil, err
	}
	if minutes < 30 {
		return nil, pkgerr.InvalidArgument("минимальная длительность брони 30 минут")
	}
	if minutes > 720 {
		return nil, pkgerr.InvalidArgument("максимальная длительность брони 12 часов")
	}

	totalPrice, err := computeBookingTotalPrice(vResp, in.ServiceID, minutes)
	if err != nil {
		return nil, err
	}

	slotResp, err := uc.venueClient.CheckSlotAvailability(ctx, &venuev1.CheckSlotRequest{
		VenueId:  in.VenueID,
		Date:     in.Date,
		TimeFrom: in.TimeFrom,
		TimeTo:   in.TimeTo,
	})
	if err != nil {
		return nil, fmt.Errorf("check slot: %w", err)
	}
	if !slotResp.Available {
		return nil, pkgerr.InvalidArgument("выбранное время уже занято, выберите другое")
	}

	dateParsed, err := time.Parse("2006-01-02", in.Date)
	if err != nil {
		return nil, pkgerr.InvalidArgument("invalid date format, expected YYYY-MM-DD")
	}

	loc, lerr := time.LoadLocation(uc.visitTimeZone)
	if lerr != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	visitStart := time.Date(
		dateParsed.Year(), dateParsed.Month(), dateParsed.Day(),
		0, 0, 0, 0, loc,
	)
	if visitStart.Before(todayStart) {
		return nil, pkgerr.InvalidArgument("дата визита не может быть в прошлом")
	}

	b := &domain.Booking{
		UserID:     in.UserID,
		VenueID:    in.VenueID,
		VenueName:  in.VenueName,
		ServiceID:  in.ServiceID,
		Date:       dateParsed,
		TimeFrom:   in.TimeFrom,
		TimeTo:     in.TimeTo,
		Guests:     in.Guests,
		Comment:    in.Comment,
		Status:     "pending",
		TotalPrice: totalPrice,
	}
	if err := uc.repo.Create(ctx, b); err != nil {
		return nil, fmt.Errorf("create booking: %w", err)
	}

	_, err = uc.venueClient.ReserveSlot(ctx, &venuev1.ReserveSlotRequest{
		VenueId:   in.VenueID,
		BookingId: b.ID,
		Date:      in.Date,
		TimeFrom:  in.TimeFrom,
		TimeTo:    in.TimeTo,
	})
	if err != nil {
		_ = uc.repo.Delete(ctx, b.ID)
		if st, ok := status.FromError(err); ok && st.Code() == codes.InvalidArgument {
			return nil, pkgerr.InvalidArgument("выбранное время уже занято, выберите другое")
		}
		return nil, fmt.Errorf("reserve slot: %w", err)
	}

	payResp, err := uc.paymentClient.CreatePayment(ctx, &paymentv1.CreatePaymentRequest{
		BookingId:      b.ID,
		Amount:         b.TotalPrice,
		Description:    fmt.Sprintf("Booking %s", b.ID),
		IdempotencyKey: b.ID,
	})
	if err != nil {
		_, _ = uc.venueClient.ReleaseSlot(ctx, &venuev1.ReleaseSlotRequest{
			VenueId:   in.VenueID,
			BookingId: b.ID,
		})
		_ = uc.repo.Delete(ctx, b.ID)
		return nil, fmt.Errorf("create payment: %w", err)
	}

	if err := uc.repo.SetPaymentID(ctx, b.ID, payResp.Id); err != nil {
		return nil, fmt.Errorf("set payment id: %w", err)
	}
	if err := uc.repo.UpdateStatus(ctx, b.ID, "payment_pending"); err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	b.Status = "payment_pending"
	b.PaymentID = payResp.Id
	b.PaymentURL = payResp.PaymentUrl

	_ = uc.publisher.PublishBookingCreated(ctx, b)

	return b, nil
}

func (uc *BookingUseCase) GetBooking(ctx context.Context, id string) (*domain.Booking, error) {
	return uc.repo.GetByID(ctx, id)
}

func (uc *BookingUseCase) ListUserBookings(ctx context.Context, userID, statusFilter string, page, pageSize int32) ([]*domain.Booking, int, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int((page - 1) * pageSize)
	return uc.repo.ListByUser(ctx, userID, statusFilter, offset, int(pageSize))
}

func (uc *BookingUseCase) ListVenueBookings(ctx context.Context, venueID, statusFilter, date string, page, pageSize int32) ([]*domain.Booking, int, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int((page - 1) * pageSize)
	return uc.repo.ListByVenue(ctx, venueID, statusFilter, date, offset, int(pageSize))
}

func (uc *BookingUseCase) CancelBooking(ctx context.Context, id, userID string) (*domain.Booking, error) {
	b, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if b.UserID != userID {
		return nil, pkgerr.PermissionDenied("not your booking")
	}
	if b.Status == "completed" || b.Status == "cancelled" {
		return nil, pkgerr.InvalidArgument("booking cannot be cancelled in current status")
	}

	if err := uc.repo.UpdateStatus(ctx, id, "cancelled"); err != nil {
		return nil, err
	}
	b.Status = "cancelled"

	_, _ = uc.venueClient.ReleaseSlot(ctx, &venuev1.ReleaseSlotRequest{
		VenueId:   b.VenueID,
		BookingId: b.ID,
	})

	_ = uc.publisher.PublishBookingCancelled(ctx, b)

	return b, nil
}

func (uc *BookingUseCase) ConfirmBooking(ctx context.Context, id, paymentID string) error {
	b, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	switch b.Status {
	case "confirmed", "completed", "cancelled":
		return nil
	case "payment_pending":
		if b.PaymentID != "" && b.PaymentID != paymentID {
			return pkgerr.InvalidArgument("payment does not match booking")
		}
		if b.PaymentID == "" {
			if err := uc.repo.SetPaymentID(ctx, id, paymentID); err != nil {
				return fmt.Errorf("set payment id: %w", err)
			}
		}
		if err := uc.repo.UpdateStatus(ctx, id, "confirmed"); err != nil {
			return err
		}
		b, err = uc.repo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		_ = uc.publisher.PublishBookingConfirmed(ctx, b)
		return nil
	default:
		return pkgerr.InvalidArgument("booking cannot be confirmed from current status")
	}
}

func (uc *BookingUseCase) CompleteBooking(ctx context.Context, id string) (*domain.Booking, error) {
	b, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if b.Status != "confirmed" {
		return nil, pkgerr.InvalidArgument("only confirmed bookings can be completed")
	}
	if b.PaymentID == "" {
		return nil, pkgerr.InvalidArgument("booking has no payment")
	}
	if err := uc.repo.UpdateStatus(ctx, id, "completed"); err != nil {
		return nil, err
	}
	b.Status = "completed"
	return b, nil
}

func (uc *BookingUseCase) CancelBookingByPayment(ctx context.Context, bookingID string) error {
	b, err := uc.repo.GetByID(ctx, bookingID)
	if err != nil {
		s, ok := status.FromError(err)
		if ok && s.Code() == codes.NotFound {
			return nil
		}
		return err
	}
	if b.Status != "payment_pending" {
		return nil
	}

	if err := uc.repo.UpdateStatus(ctx, bookingID, "cancelled"); err != nil {
		return err
	}
	b.Status = "cancelled"

	_, _ = uc.venueClient.ReleaseSlot(ctx, &venuev1.ReleaseSlotRequest{
		VenueId:   b.VenueID,
		BookingId: b.ID,
	})

	_ = uc.publisher.PublishBookingCancelled(ctx, b)
	return nil
}

func (uc *BookingUseCase) HasCompletedBooking(ctx context.Context, userID, venueID string) (bool, error) {
	return uc.repo.HasCompleted(ctx, userID, venueID)
}

func slotDurationMinutes(timeFrom, timeTo string) (int, error) {
	tf, err := time.Parse("15:04", timeFrom)
	if err != nil {
		return 0, pkgerr.InvalidArgument("неверное время начала, ожидается ЧЧ:ММ")
	}
	tt, err := time.Parse("15:04", timeTo)
	if err != nil {
		return 0, pkgerr.InvalidArgument("неверное время окончания, ожидается ЧЧ:ММ")
	}
	if !tt.After(tf) {
		return 0, pkgerr.InvalidArgument("время окончания должно быть позже времени начала")
	}
	return int(tt.Sub(tf).Minutes()), nil
}

func hourlyTotal(priceFrom int64, minutes int) int64 {
	if minutes <= 0 || priceFrom <= 0 {
		return 0
	}
	h := (minutes + 59) / 60
	if h < 1 {
		h = 1
	}
	return priceFrom * int64(h)
}

func computeBookingTotalPrice(v *venuev1.VenueResponse, serviceID string, minutes int) (int64, error) {
	sid := strings.TrimSpace(serviceID)
	if sid != "" {
		for _, s := range v.GetServices() {
			if s.GetId() != sid {
				continue
			}
			if p := s.GetPrice(); p > 0 {
				return p, nil
			}
			return hourlyTotal(v.GetPriceFrom(), minutes), nil
		}
		return 0, pkgerr.InvalidArgument("выбранная услуга не найдена у этого заведения")
	}
	return hourlyTotal(v.GetPriceFrom(), minutes), nil
}

// AutoCompletePastVisits переводит confirmed → completed после окончания слота (date+time_to) в visitTimeZone.
func (uc *BookingUseCase) AutoCompletePastVisits(ctx context.Context) (int, error) {
	refs, err := uc.repo.AutoCompleteVisitEnded(ctx, uc.visitTimeZone)
	if err != nil {
		return 0, err
	}
	for _, ref := range refs {
		b := &domain.Booking{ID: ref.ID, UserID: ref.UserID, VenueID: ref.VenueID, Status: "completed"}
		_ = uc.publisher.PublishBookingCompleted(ctx, b)
	}
	return len(refs), nil
}
