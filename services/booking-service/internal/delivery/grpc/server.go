package grpc

import (
	"context"
	"strings"

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
		UserID:     req.UserId,
		VenueID:    req.VenueId,
		VenueName:  req.VenueName,
		ServiceID:  req.ServiceId,
		ServiceIDs: req.GetServiceIds(),
		HallIDs:    req.GetHallIds(),
		Date:       req.Date,
		TimeFrom:   req.TimeFrom,
		TimeTo:     req.TimeTo,
		Guests:     req.Guests,
		Comment:    req.Comment,
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
	bookings, total, err := s.uc.ListVenueBookings(ctx, req.VenueId, req.GetOwnerId(), req.Status, req.Date, req.Page, req.PageSize)
	if err != nil {
		return nil, err
	}
	resp := &bookingv1.ListBookingsResponse{Total: int32(total)}
	for _, b := range bookings {
		resp.Bookings = append(resp.Bookings, toProto(b))
	}
	return resp, nil
}

func (s *Server) ListBookingStaffNotes(ctx context.Context, req *bookingv1.ListBookingStaffNotesRequest) (*bookingv1.ListBookingStaffNotesResponse, error) {
	notes, err := s.uc.ListBookingStaffNotes(ctx, req.GetBookingId(), req.GetRequesterUserId())
	if err != nil {
		return nil, err
	}
	out := &bookingv1.ListBookingStaffNotesResponse{}
	for i := range notes {
		n := &notes[i]
		out.Notes = append(out.Notes, &bookingv1.BookingStaffNote{
			Id:            n.ID,
			BookingId:     n.BookingID,
			VenueId:       n.VenueID,
			AuthorUserId:  n.AuthorUserID,
			Body:          n.Body,
			CreatedAt:     timestamppb.New(n.CreatedAt),
		})
	}
	return out, nil
}

func (s *Server) AddBookingStaffNote(ctx context.Context, req *bookingv1.AddBookingStaffNoteRequest) (*bookingv1.AddBookingStaffNoteResponse, error) {
	n, err := s.uc.AddBookingStaffNote(ctx, req.GetBookingId(), req.GetRequesterUserId(), req.GetBody())
	if err != nil {
		return nil, err
	}
	return &bookingv1.AddBookingStaffNoteResponse{
		Note: &bookingv1.BookingStaffNote{
			Id:            n.ID,
			BookingId:     n.BookingID,
			VenueId:       n.VenueID,
			AuthorUserId:  n.AuthorUserID,
			Body:          n.Body,
			CreatedAt:     timestamppb.New(n.CreatedAt),
		},
	}, nil
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
	svcOut := b.ServiceID
	pkgOut := append([]string(nil), b.PackageServiceIDs...)
	if len(pkgOut) > 1 {
		svcOut = ""
	} else if len(pkgOut) == 1 {
		svcOut = pkgOut[0]
		pkgOut = nil
	} else if strings.TrimSpace(svcOut) != "" {
		pkgOut = nil
	}
	return &bookingv1.BookingResponse{
		Id:                b.ID,
		UserId:            b.UserID,
		VenueId:           b.VenueID,
		VenueName:         b.VenueName,
		ServiceId:         svcOut,
		Date:              b.Date.Format("2006-01-02"),
		TimeFrom:          b.TimeFrom,
		TimeTo:            b.TimeTo,
		Guests:            b.Guests,
		Comment:           b.Comment,
		Status:            b.Status,
		TotalPrice:        b.TotalPrice,
		PaymentUrl:        b.PaymentURL,
		CreatedAt:         timestamppb.New(b.CreatedAt),
		PackageServiceIds: pkgOut,
		HallIds:           append([]string(nil), b.HallIDs...),
	}
}
