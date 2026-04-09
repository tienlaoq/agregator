package grpc

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	pkgerrors "github.com/tienlao/agregator/pkg/errors"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
	"github.com/tienlao/agregator/services/venue-service/internal/events"
	"github.com/tienlao/agregator/services/venue-service/internal/telegram"
	"github.com/tienlao/agregator/services/venue-service/internal/usecase"
)

type Server struct {
	venuev1.UnimplementedVenueServiceServer
	uc        *usecase.VenueUseCase
	publisher *events.Publisher
	tg        *telegram.Notifier
}

func NewServer(uc *usecase.VenueUseCase, publisher *events.Publisher, tg *telegram.Notifier) *Server {
	return &Server{uc: uc, publisher: publisher, tg: tg}
}

func (s *Server) CreateVenue(ctx context.Context, req *venuev1.CreateVenueRequest) (*venuev1.VenueResponse, error) {
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid owner_id")
	}

	venue := &domain.Venue{
		OwnerID:      ownerID,
		Name:         req.GetName(),
		Type:         req.GetType(),
		Description:  req.GetDescription(),
		Address:      req.GetAddress(),
		City:         req.GetCity(),
		Latitude:     req.GetLatitude(),
		Longitude:    req.GetLongitude(),
		PriceFrom:    req.GetPriceFrom(),
		Capacity:     req.GetCapacity(),
		Amenities:    req.GetAmenities(),
		WorkingHours: req.GetWorkingHours(),
		Phone:        req.GetPhone(),
	}

	for _, svc := range req.GetServices() {
		venue.Services = append(venue.Services, domain.VenueService{
			Name:        svc.GetName(),
			DurationMin: svc.GetDurationMin(),
			Price:       svc.GetPrice(),
			Description: svc.GetDescription(),
		})
	}

	if err := s.uc.Create(ctx, venue); err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}

	_ = s.publisher.VenueCreated(venue)
	_ = s.tg.NotifyNewVenue(venue)

	return venueToProto(venue), nil
}

func (s *Server) UpdateVenue(ctx context.Context, req *venuev1.UpdateVenueRequest) (*venuev1.VenueResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue id")
	}

	existing, err := s.uc.GetByID(ctx, id)
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	if existing == nil {
		return nil, pkgerrors.NotFound("venue not found")
	}

	if req.GetOwnerId() != existing.OwnerID.String() {
		return nil, pkgerrors.PermissionDenied("not the venue owner")
	}

	if req.Name != nil {
		existing.Name = req.GetName()
	}
	if req.Description != nil {
		existing.Description = req.GetDescription()
	}
	if req.Address != nil {
		existing.Address = req.GetAddress()
	}
	if req.Latitude != nil {
		existing.Latitude = req.GetLatitude()
	}
	if req.Longitude != nil {
		existing.Longitude = req.GetLongitude()
	}
	if req.PriceFrom != nil {
		existing.PriceFrom = req.GetPriceFrom()
	}
	if req.Capacity != nil {
		existing.Capacity = req.GetCapacity()
	}
	if len(req.GetAmenities()) > 0 {
		existing.Amenities = req.GetAmenities()
	}
	if req.WorkingHours != nil {
		existing.WorkingHours = req.GetWorkingHours()
	}
	if req.Phone != nil {
		existing.Phone = req.GetPhone()
	}
	if req.City != nil {
		existing.City = req.GetCity()
	}

	if err := s.uc.Update(ctx, existing); err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}

	_ = s.publisher.VenueUpdated(existing)

	return venueToProto(existing), nil
}

func (s *Server) GetVenue(ctx context.Context, req *venuev1.GetVenueRequest) (*venuev1.VenueResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue id")
	}

	v, err := s.uc.GetByID(ctx, id)
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	if v == nil {
		return nil, pkgerrors.NotFound("venue not found")
	}
	return venueToProto(v), nil
}

func (s *Server) GetVenueBySlug(ctx context.Context, req *venuev1.GetVenueBySlugRequest) (*venuev1.VenueResponse, error) {
	v, err := s.uc.GetBySlug(ctx, req.GetSlug())
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	if v == nil {
		return nil, pkgerrors.NotFound("venue not found")
	}
	return venueToProto(v), nil
}

func (s *Server) ListVenues(ctx context.Context, req *venuev1.ListVenuesRequest) (*venuev1.ListVenuesResponse, error) {
	result, err := s.uc.List(ctx, req.GetPage(), req.GetPageSize(), req.GetType(), req.GetSortBy())
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	return listResultToProto(result), nil
}

func (s *Server) SearchVenues(ctx context.Context, req *venuev1.SearchVenuesRequest) (*venuev1.ListVenuesResponse, error) {
	params := domain.SearchParams{
		Query:     req.GetQuery(),
		Lat:       req.GetLatitude(),
		Lng:       req.GetLongitude(),
		RadiusKM:  req.GetRadiusKm(),
		VenueType: req.GetType(),
		PriceMin:  req.GetPriceMin(),
		PriceMax:  req.GetPriceMax(),
		RatingMin: req.GetRatingMin(),
		Amenities: req.GetAmenities(),
		Page:      req.GetPage(),
		PageSize:  req.GetPageSize(),
	}

	result, err := s.uc.Search(ctx, params)
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	return listResultToProto(result), nil
}

func (s *Server) ListOwnerVenues(ctx context.Context, req *venuev1.ListOwnerVenuesRequest) (*venuev1.ListVenuesResponse, error) {
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid owner_id")
	}

	venues, err := s.uc.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}

	resp := &venuev1.ListVenuesResponse{
		Total:    int32(len(venues)),
		Page:     1,
		PageSize: int32(len(venues)),
	}
	for i := range venues {
		resp.Venues = append(resp.Venues, venueToProto(&venues[i]))
	}
	return resp, nil
}

func (s *Server) CheckSlotAvailability(ctx context.Context, req *venuev1.CheckSlotRequest) (*venuev1.CheckSlotResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}

	available, err := s.uc.CheckSlot(ctx, venueID, req.GetDate(), req.GetTimeFrom(), req.GetTimeTo())
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	return &venuev1.CheckSlotResponse{Available: available}, nil
}

func (s *Server) ReserveSlot(ctx context.Context, req *venuev1.ReserveSlotRequest) (*venuev1.ReserveSlotResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	bookingID, err := uuid.Parse(req.GetBookingId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid booking_id")
	}

	if err := s.uc.ReserveSlot(ctx, venueID, bookingID, req.GetDate(), req.GetTimeFrom(), req.GetTimeTo()); err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	return &venuev1.ReserveSlotResponse{Success: true}, nil
}

func (s *Server) ReleaseSlot(ctx context.Context, req *venuev1.ReleaseSlotRequest) (*venuev1.ReleaseSlotResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	bookingID, err := uuid.Parse(req.GetBookingId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid booking_id")
	}

	if err := s.uc.ReleaseSlot(ctx, venueID, bookingID); err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	return &venuev1.ReleaseSlotResponse{Success: true}, nil
}

func (s *Server) UpdateRating(ctx context.Context, req *venuev1.UpdateRatingRequest) (*venuev1.UpdateRatingResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}

	if err := s.uc.UpdateRating(ctx, venueID, req.GetAvgRating(), req.GetReviewCount()); err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	return &venuev1.UpdateRatingResponse{}, nil
}

func (s *Server) ModerateVenue(ctx context.Context, req *venuev1.ModerateVenueRequest) (*venuev1.VenueResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}

	moderatedBy, err := uuid.Parse(req.GetModeratedBy())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid moderated_by: admin user ID required")
	}

	v, err := s.uc.Moderate(ctx, venueID, req.GetAction(), req.GetComment(), moderatedBy)
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}

	_ = s.publisher.VenueUpdated(v)
	_ = s.tg.NotifyModerated(v)

	return venueToProto(v), nil
}

func (s *Server) ListPendingVenues(ctx context.Context, req *venuev1.ListPendingVenuesRequest) (*venuev1.ListVenuesResponse, error) {
	status := req.GetStatus()
	if status == "" {
		status = "pending_review"
	}

	result, err := s.uc.ListByStatus(ctx, status, req.GetPage(), req.GetPageSize())
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	return listResultToProto(result), nil
}

func venueToProto(v *domain.Venue) *venuev1.VenueResponse {
	resp := &venuev1.VenueResponse{
		Id:                v.ID.String(),
		OwnerId:           v.OwnerID.String(),
		Slug:              v.Slug,
		Name:              v.Name,
		Type:              v.Type,
		Description:       v.Description,
		Address:           v.Address,
		City:              v.City,
		Latitude:          v.Latitude,
		Longitude:         v.Longitude,
		PriceFrom:         v.PriceFrom,
		Capacity:          v.Capacity,
		Amenities:         v.Amenities,
		WorkingHours:      v.WorkingHours,
		Phone:             v.Phone,
		AvgRating:         v.AvgRating,
		ReviewCount:       v.ReviewCount,
		IsActive:          v.IsActive,
		Status:            v.Status,
		ModerationComment: v.ModerationComment,
		CreatedAt:         timestamppb.New(v.CreatedAt),
	}

	if v.ModeratedAt != nil {
		resp.ModeratedAt = timestamppb.New(*v.ModeratedAt)
	}
	if v.ModeratedBy != nil {
		resp.ModeratedBy = v.ModeratedBy.String()
	}

	for _, svc := range v.Services {
		resp.Services = append(resp.Services, &venuev1.VenueServiceItem{
			Id:          svc.ID.String(),
			Name:        svc.Name,
			DurationMin: svc.DurationMin,
			Price:       svc.Price,
			Description: svc.Description,
		})
	}

	for _, p := range v.Photos {
		resp.Photos = append(resp.Photos, &venuev1.VenuePhoto{
			Id:        p.ID.String(),
			Url:       p.URL,
			SortOrder: p.SortOrder,
			IsCover:   p.IsCover,
		})
	}

	return resp
}

func listResultToProto(r *domain.ListResult) *venuev1.ListVenuesResponse {
	resp := &venuev1.ListVenuesResponse{
		Total:    r.Total,
		Page:     r.Page,
		PageSize: r.PageSize,
	}
	for i := range r.Venues {
		resp.Venues = append(resp.Venues, venueToProto(&r.Venues[i]))
	}
	return resp
}
