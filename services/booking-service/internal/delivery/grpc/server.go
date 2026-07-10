package grpc

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
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
	// callerID берётся из gRPC metadata ("x-caller-id"), выставленной gateway после
	// верификации JWT — не из proto-поля req.UserId, которое клиент мог бы подменить.
	callerID := grpcutil.CallerIDFromCtx(ctx)
	if callerID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing caller identity")
	}
	b, err := s.uc.CreateBooking(ctx, usecase.CreateBookingInput{
		UserID:     callerID,
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

const maxBatchIDs = 200

func (s *Server) GetBookingsBatch(ctx context.Context, req *bookingv1.GetBookingsBatchRequest) (*bookingv1.GetBookingsBatchResponse, error) {
	ids := req.GetIds()
	if len(ids) == 0 {
		return &bookingv1.GetBookingsBatchResponse{Bookings: map[string]*bookingv1.BookingResponse{}}, nil
	}
	if len(ids) > maxBatchIDs {
		return nil, status.Errorf(codes.InvalidArgument, "too many ids: got %d, max %d", len(ids), maxBatchIDs)
	}
	bookings, err := s.uc.GetBookingsBatch(ctx, ids)
	if err != nil {
		return nil, err
	}
	resp := &bookingv1.GetBookingsBatchResponse{
		Bookings: make(map[string]*bookingv1.BookingResponse, len(bookings)),
	}
	for _, b := range bookings {
		resp.Bookings[b.ID] = toProto(b)
	}
	return resp, nil
}

func (s *Server) ListUserBookings(ctx context.Context, req *bookingv1.ListUserBookingsRequest) (*bookingv1.ListBookingsResponse, error) {
	callerID := grpcutil.CallerIDFromCtx(ctx)
	if callerID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing caller identity")
	}
	bookings, total, err := s.uc.ListUserBookings(ctx, callerID, req.Status, req.Page, req.PageSize)
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
	// callerID берётся из gRPC metadata ("x-caller-id"), выставленной gateway
	// после верификации JWT — не из proto-поля req.OwnerId, которое клиент
	// мог бы подменить при прямом доступе к сервису.
	callerID := grpcutil.CallerIDFromCtx(ctx)
	if callerID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing caller identity")
	}
	bookings, total, nextCursor, err := s.uc.ListVenueBookings(ctx, req.VenueId, callerID, req.Status, req.Date, req.GetDateFrom(), req.GetDateTo(), req.GetCursor(), req.PageSize)
	if err != nil {
		return nil, err
	}
	resp := &bookingv1.ListBookingsResponse{
		Total:      int32(total),
		NextCursor: nextCursor,
	}
	for _, b := range bookings {
		resp.Bookings = append(resp.Bookings, toProto(b))
	}
	return resp, nil
}

func (s *Server) ListBookingStaffNotes(ctx context.Context, req *bookingv1.ListBookingStaffNotesRequest) (*bookingv1.ListBookingStaffNotesResponse, error) {
	callerID := grpcutil.CallerIDFromCtx(ctx)
	if callerID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing caller identity")
	}
	notes, err := s.uc.ListBookingStaffNotes(ctx, req.GetBookingId(), callerID)
	if err != nil {
		return nil, err
	}
	out := &bookingv1.ListBookingStaffNotesResponse{}
	for i := range notes {
		n := &notes[i]
		out.Notes = append(out.Notes, &bookingv1.BookingStaffNote{
			Id:           n.ID,
			BookingId:    n.BookingID,
			VenueId:      n.VenueID,
			AuthorUserId: n.AuthorUserID,
			Body:         n.Body,
			CreatedAt:    timestamppb.New(n.CreatedAt),
		})
	}
	return out, nil
}

func (s *Server) AddBookingStaffNote(ctx context.Context, req *bookingv1.AddBookingStaffNoteRequest) (*bookingv1.AddBookingStaffNoteResponse, error) {
	callerID := grpcutil.CallerIDFromCtx(ctx)
	if callerID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing caller identity")
	}
	n, err := s.uc.AddBookingStaffNote(ctx, req.GetBookingId(), callerID, req.GetBody())
	if err != nil {
		return nil, err
	}
	return &bookingv1.AddBookingStaffNoteResponse{
		Note: &bookingv1.BookingStaffNote{
			Id:           n.ID,
			BookingId:    n.BookingID,
			VenueId:      n.VenueID,
			AuthorUserId: n.AuthorUserID,
			Body:         n.Body,
			CreatedAt:    timestamppb.New(n.CreatedAt),
		},
	}, nil
}

func (s *Server) CancelBooking(ctx context.Context, req *bookingv1.CancelBookingRequest) (*bookingv1.BookingResponse, error) {
	callerID := grpcutil.CallerIDFromCtx(ctx)
	if callerID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing caller identity")
	}
	b, err := s.uc.CancelBooking(ctx, req.Id, callerID)
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
	// Вызывается из review-service (service-to-service): req.UserId — проверяемый пользователь,
	// а не вызывающий сервис. callerID содержит service identity "review-service", выставленный
	// CallerIDClientInterceptor в review-service/cmd/main.go — присутствие заголовка означает,
	// что запрос прошёл через авторизованный internal-канал, а не напрямую.
	if grpcutil.CallerIDFromCtx(ctx) == "" {
		return nil, status.Error(codes.Unauthenticated, "missing caller identity")
	}
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
		TimeFrom:          b.TimeFrom.String(),
		TimeTo:            b.TimeTo.String(),
		Guests:            b.Guests,
		Comment:           b.Comment,
		Status:            string(b.Status),
		TotalPrice:        b.TotalPrice,
		PaymentUrl:        b.PaymentURL,
		CreatedAt:         timestamppb.New(b.CreatedAt),
		PackageServiceIds: pkgOut,
		HallIds:           append([]string(nil), b.HallIDs...),
		StaffNotesCount:   b.StaffNotesCount,
	}
}
