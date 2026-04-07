package grpc

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	"github.com/tienlao/agregator/services/booking-service/internal/domain"
	"github.com/tienlao/agregator/services/booking-service/internal/usecase"
)

type Server struct {
	bookingv1.UnimplementedBookingServiceServer
	uc *usecase.BookingUseCase
}

func NewServer(uc *usecase.BookingUseCase) *Server {
	return &Server{uc: uc}
}

func (s *Server) CreateBooking(ctx context.Context, req *bookingv1.CreateBookingRequest) (*bookingv1.BookingResponse, error) {
	b, err := s.uc.CreateBooking(ctx, usecase.CreateBookingInput{
		UserID:    req.UserId,
		VenueID:   req.VenueId,
		ServiceID: req.ServiceId,
		Date:      req.Date,
		TimeFrom:  req.TimeFrom,
		TimeTo:    req.TimeTo,
		Guests:    req.Guests,
		Comment:   req.Comment,
	})
	if err != nil {
		return nil, err
	}
	return toProto(b), nil
}

func (s *Server) GetBooking(ctx context.Context, req *bookingv1.GetBookingRequest) (*bookingv1.BookingResponse, error) {
	b, err := s.uc.GetBooking(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return toProto(b), nil
}

func (s *Server) ListUserBookings(ctx context.Context, req *bookingv1.ListUserBookingsRequest) (*bookingv1.ListBookingsResponse, error) {
	bookings, total, err := s.uc.ListUserBookings(ctx, req.UserId, req.Status, req.Page, req.PageSize)
	if err != nil {
		return nil, err
	}
	resp := &bookingv1.ListBookingsResponse{Total: int32(total)}
	for _, b := range bookings {
		resp.Bookings = append(resp.Bookings, toProto(b))
	}
	return resp, nil
}

func (s *Server) ListVenueBookings(ctx context.Context, req *bookingv1.ListVenueBookingsRequest) (*bookingv1.ListBookingsResponse, error) {
	bookings, total, err := s.uc.ListVenueBookings(ctx, req.VenueId, req.Status, req.Date, req.Page, req.PageSize)
	if err != nil {
		return nil, err
	}
	resp := &bookingv1.ListBookingsResponse{Total: int32(total)}
	for _, b := range bookings {
		resp.Bookings = append(resp.Bookings, toProto(b))
	}
	return resp, nil
}

func (s *Server) CancelBooking(ctx context.Context, req *bookingv1.CancelBookingRequest) (*bookingv1.BookingResponse, error) {
	b, err := s.uc.CancelBooking(ctx, req.Id, req.UserId)
	if err != nil {
		return nil, err
	}
	return toProto(b), nil
}

func (s *Server) ConfirmBooking(ctx context.Context, req *bookingv1.ConfirmBookingRequest) (*bookingv1.BookingResponse, error) {
	if err := s.uc.ConfirmBooking(ctx, req.Id, req.PaymentId); err != nil {
		return nil, err
	}
	b, err := s.uc.GetBooking(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return toProto(b), nil
}

func (s *Server) CompleteBooking(ctx context.Context, req *bookingv1.CompleteBookingRequest) (*bookingv1.BookingResponse, error) {
	b, err := s.uc.CompleteBooking(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return toProto(b), nil
}

func (s *Server) HasCompletedBooking(ctx context.Context, req *bookingv1.HasCompletedBookingRequest) (*bookingv1.HasCompletedBookingResponse, error) {
	has, err := s.uc.HasCompletedBooking(ctx, req.UserId, req.VenueId)
	if err != nil {
		return nil, err
	}
	return &bookingv1.HasCompletedBookingResponse{HasCompleted: has}, nil
}

func toProto(b *domain.Booking) *bookingv1.BookingResponse {
	return &bookingv1.BookingResponse{
		Id:         b.ID,
		UserId:     b.UserID,
		VenueId:    b.VenueID,
		ServiceId:  b.ServiceID,
		Date:       b.Date.Format("2006-01-02"),
		TimeFrom:   b.TimeFrom,
		TimeTo:     b.TimeTo,
		Guests:     b.Guests,
		Comment:    b.Comment,
		Status:     b.Status,
		TotalPrice: b.TotalPrice,
		PaymentUrl: b.PaymentURL,
		CreatedAt:  timestamppb.New(b.CreatedAt),
	}
}
