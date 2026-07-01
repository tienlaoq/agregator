package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	"github.com/tienlao/agregator/services/booking-service/internal/domain"
	"github.com/tienlao/agregator/services/booking-service/internal/usecase"
)

// mockRepo embeds domain.BookingRepository so only the read methods the tested
// handlers touch need an implementation; any other call panics loudly.
type mockRepo struct {
	domain.BookingRepository
	GetByIDFunc  func(ctx context.Context, id string) (*domain.Booking, error)
	GetByIDsFunc func(ctx context.Context, ids []string) ([]*domain.Booking, error)
}

func (m *mockRepo) GetByID(ctx context.Context, id string) (*domain.Booking, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, nil
}
func (m *mockRepo) GetByIDs(ctx context.Context, ids []string) ([]*domain.Booking, error) {
	if m.GetByIDsFunc != nil {
		return m.GetByIDsFunc(ctx, ids)
	}
	return nil, nil
}

// newServer wires a Server with a mock repo and nil downstream clients. The read
// handlers under test only touch the repo; clients/publisher stay unused.
func newServer(repo domain.BookingRepository) *Server {
	uc := usecase.NewBookingUseCase(repo, nil, nil, nil, nil, zerolog.Nop(), "Europe/Moscow", 2)
	return NewServer(uc)
}

func wantCode(t *testing.T, err error, c codes.Code) {
	t.Helper()
	if status.Code(err) != c {
		t.Fatalf("status = %v, want %v (err: %v)", status.Code(err), c, err)
	}
}

func tod(t *testing.T, s string) domain.TimeOfDay {
	t.Helper()
	v, err := domain.ParseTimeOfDay(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// ── toProto normalisation ──────────────────────────────────────────────────

func TestToProto_SingleService(t *testing.T) {
	b := &domain.Booking{
		ID: "b1", UserID: "u1", VenueID: "v1", VenueName: "Баня",
		ServiceID: "s1", HallIDs: []string{"h1", "h2"},
		Date:     time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		TimeFrom: tod(t, "10:00"), TimeTo: tod(t, "12:00"),
		Guests: 4, Status: domain.StatusConfirmed, TotalPrice: 5000,
	}
	p := toProto(b)
	if p.GetServiceId() != "s1" || len(p.GetPackageServiceIds()) != 0 {
		t.Errorf("single service: ServiceId=%q packages=%v", p.GetServiceId(), p.GetPackageServiceIds())
	}
	if p.GetDate() != "2026-07-01" || p.GetTimeFrom() != "10:00" || p.GetTimeTo() != "12:00" {
		t.Errorf("date/time mismatch: %s %s-%s", p.GetDate(), p.GetTimeFrom(), p.GetTimeTo())
	}
	if len(p.GetHallIds()) != 2 {
		t.Errorf("hall ids = %v, want 2", p.GetHallIds())
	}
	if p.GetStatus() != "confirmed" {
		t.Errorf("status = %q, want confirmed", p.GetStatus())
	}
}

func TestToProto_MultiplePackagesClearServiceId(t *testing.T) {
	b := &domain.Booking{ID: "b1", PackageServiceIDs: []string{"p1", "p2"}, ServiceID: "ignored",
		Date: time.Now(), TimeFrom: tod(t, "10:00"), TimeTo: tod(t, "11:00")}
	p := toProto(b)
	if p.GetServiceId() != "" {
		t.Errorf("multiple packages must clear ServiceId, got %q", p.GetServiceId())
	}
	if len(p.GetPackageServiceIds()) != 2 {
		t.Errorf("packages = %v, want 2", p.GetPackageServiceIds())
	}
}

func TestToProto_SinglePackagePromotedToServiceId(t *testing.T) {
	b := &domain.Booking{ID: "b1", PackageServiceIDs: []string{"only"},
		Date: time.Now(), TimeFrom: tod(t, "10:00"), TimeTo: tod(t, "11:00")}
	p := toProto(b)
	if p.GetServiceId() != "only" || len(p.GetPackageServiceIds()) != 0 {
		t.Errorf("single package should promote to ServiceId: id=%q packages=%v", p.GetServiceId(), p.GetPackageServiceIds())
	}
}

// ── auth guards (uc not reached) ───────────────────────────────────────────

func TestCreateBooking_MissingCaller(t *testing.T) {
	_, err := newServer(&mockRepo{}).CreateBooking(context.Background(), &bookingv1.CreateBookingRequest{VenueId: "v1"})
	wantCode(t, err, codes.Unauthenticated)
}

func TestListUserBookings_MissingCaller(t *testing.T) {
	_, err := newServer(&mockRepo{}).ListUserBookings(context.Background(), &bookingv1.ListUserBookingsRequest{})
	wantCode(t, err, codes.Unauthenticated)
}

func TestListVenueBookings_MissingCaller(t *testing.T) {
	_, err := newServer(&mockRepo{}).ListVenueBookings(context.Background(), &bookingv1.ListVenueBookingsRequest{VenueId: "v1"})
	wantCode(t, err, codes.Unauthenticated)
}

// ── GetBooking / GetBookingsBatch ──────────────────────────────────────────

func TestGetBooking_Success(t *testing.T) {
	repo := &mockRepo{GetByIDFunc: func(_ context.Context, id string) (*domain.Booking, error) {
		return &domain.Booking{ID: id, VenueName: "Баня", Status: domain.StatusPending,
			Date: time.Now(), TimeFrom: tod(t, "10:00"), TimeTo: tod(t, "11:00")}, nil
	}}
	resp, err := newServer(repo).GetBooking(context.Background(), &bookingv1.GetBookingRequest{Id: "b1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetId() != "b1" || resp.GetVenueName() != "Баня" {
		t.Errorf("response mismatch: %+v", resp)
	}
}

func TestGetBooking_ErrorPropagates(t *testing.T) {
	repo := &mockRepo{GetByIDFunc: func(context.Context, string) (*domain.Booking, error) {
		return nil, status.Error(codes.NotFound, "no booking")
	}}
	_, err := newServer(repo).GetBooking(context.Background(), &bookingv1.GetBookingRequest{Id: "b1"})
	wantCode(t, err, codes.NotFound)
}

func TestGetBookingsBatch(t *testing.T) {
	t.Run("empty ids → empty map, no repo call", func(t *testing.T) {
		repo := &mockRepo{GetByIDsFunc: func(context.Context, []string) ([]*domain.Booking, error) {
			t.Fatal("repo must not be called for an empty id list")
			return nil, nil
		}}
		resp, err := newServer(repo).GetBookingsBatch(context.Background(), &bookingv1.GetBookingsBatchRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.GetBookings()) != 0 {
			t.Errorf("expected empty map, got %d", len(resp.GetBookings()))
		}
	})

	t.Run("over max rejected", func(t *testing.T) {
		ids := make([]string, maxBatchIDs+1)
		_, err := newServer(&mockRepo{}).GetBookingsBatch(context.Background(), &bookingv1.GetBookingsBatchRequest{Ids: ids})
		wantCode(t, err, codes.InvalidArgument)
	})

	t.Run("success maps by id", func(t *testing.T) {
		repo := &mockRepo{GetByIDsFunc: func(_ context.Context, ids []string) ([]*domain.Booking, error) {
			return []*domain.Booking{
				{ID: "b1", Date: time.Now(), TimeFrom: tod(t, "10:00"), TimeTo: tod(t, "11:00")},
				{ID: "b2", Date: time.Now(), TimeFrom: tod(t, "12:00"), TimeTo: tod(t, "13:00")},
			}, nil
		}}
		resp, err := newServer(repo).GetBookingsBatch(context.Background(), &bookingv1.GetBookingsBatchRequest{Ids: []string{"b1", "b2"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.GetBookings()) != 2 || resp.GetBookings()["b1"] == nil || resp.GetBookings()["b2"] == nil {
			t.Errorf("batch map mismatch: %+v", resp.GetBookings())
		}
	})

	t.Run("repo error propagates", func(t *testing.T) {
		repo := &mockRepo{GetByIDsFunc: func(context.Context, []string) ([]*domain.Booking, error) {
			return nil, errors.New("db down")
		}}
		_, err := newServer(repo).GetBookingsBatch(context.Background(), &bookingv1.GetBookingsBatchRequest{Ids: []string{"b1"}})
		if err == nil {
			t.Fatal("expected repo error to propagate")
		}
	})
}
