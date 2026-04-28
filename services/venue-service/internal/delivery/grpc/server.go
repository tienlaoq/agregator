package grpc

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	pkgcities "github.com/tienlao/agregator/pkg/cities"
	pkgerrors "github.com/tienlao/agregator/pkg/errors"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
	"github.com/tienlao/agregator/services/venue-service/internal/events"
	"github.com/tienlao/agregator/services/venue-service/internal/repository"
	"github.com/tienlao/agregator/services/venue-service/internal/telegram"
	"github.com/tienlao/agregator/services/venue-service/internal/usecase"
)

type Server struct {
	venuev1.UnimplementedVenueServiceServer
	uc        *usecase.VenueUseCase
	publisher *events.Publisher
	tg        *telegram.Notifier
	log       zerolog.Logger
}

func NewServer(uc *usecase.VenueUseCase, publisher *events.Publisher, tg *telegram.Notifier, log zerolog.Logger) *Server {
	return &Server{uc: uc, publisher: publisher, tg: tg, log: log}
}

func (s *Server) CreateVenue(ctx context.Context, req *venuev1.CreateVenueRequest) (*venuev1.VenueResponse, error) {
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid owner_id")
	}

	venue := &domain.Venue{
		OwnerID:                 ownerID,
		Name:                    req.GetName(),
		Type:                    req.GetType(),
		Description:             req.GetDescription(),
		Address:                 req.GetAddress(),
		City:                    req.GetCity(),
		Latitude:                req.GetLatitude(),
		Longitude:               req.GetLongitude(),
		PriceFrom:               req.GetPriceFrom(),
		Capacity:                req.GetCapacity(),
		Amenities:               req.GetAmenities(),
		WorkingHours:            req.GetWorkingHours(),
		Phone:                   req.GetPhone(),
		LegalEntityName:         req.GetLegalEntityName(),
		INN:                     req.GetInn(),
		OGRN:                    req.GetOgrn(),
		PublicListingURL:        req.GetPublicListingUrl(),
		VerificationNote:        req.GetVerificationNote(),
		SocialLinks:             req.GetSocialLinks(),
		PayoutLegalForm:         req.GetPayoutLegalForm(),
		YooKassaSellerAccountID: req.GetYookassaSellerAccountId(),
	}
	if req.GetStartAsDraft() {
		venue.Status = domain.StatusDraft
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
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}

	if len(req.GetHalls()) > 0 {
		ups, convErr := hallProtoInputsToUpserts(req.GetHalls())
		if convErr != nil {
			return nil, pkgerrors.InvalidArgument("invalid hall id")
		}
		if err := s.uc.ReplaceVenueHalls(ctx, venue.ID, ownerID, ups); err != nil {
			if _, ok := status.FromError(err); ok {
				return nil, err
			}
			return nil, pkgerrors.Internal(err.Error())
		}
		v2, err := s.uc.GetByID(ctx, venue.ID)
		if err != nil {
			return nil, pkgerrors.Internal(err.Error())
		}
		if v2 != nil {
			venue = v2
		}
	}

	_ = s.publisher.VenueCreated(venue)
	v := venue
	if v.Status != domain.StatusDraft {
		go func() {
			if err := s.tg.NotifyNewVenue(v); err != nil {
				s.log.Warn().Err(err).Str("venue_id", v.ID.String()).Msg("telegram notify new venue failed")
			}
		}()
	}

	return venueToProto(venue), nil
}

func (s *Server) SubmitVenueForReview(ctx context.Context, req *venuev1.SubmitVenueForReviewRequest) (*venuev1.VenueResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue id")
	}
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid owner_id")
	}
	v, err := s.uc.SubmitVenueForReview(ctx, venueID, ownerID)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	_ = s.publisher.VenueUpdated(v)
	return venueToProto(v), nil
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
	if req.GetAmenities() != nil {
		existing.Amenities = append([]string(nil), req.GetAmenities()...)
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
	if req.LegalEntityName != nil {
		existing.LegalEntityName = req.GetLegalEntityName()
	}
	if req.Inn != nil {
		existing.INN = req.GetInn()
	}
	if req.Ogrn != nil {
		existing.OGRN = req.GetOgrn()
	}
	if req.PublicListingUrl != nil {
		existing.PublicListingURL = req.GetPublicListingUrl()
	}
	if req.VerificationNote != nil {
		existing.VerificationNote = req.GetVerificationNote()
	}
	if req.SocialLinks != nil {
		existing.SocialLinks = *req.SocialLinks
	}
	if req.PayoutLegalForm != nil {
		existing.PayoutLegalForm = *req.PayoutLegalForm
	}
	if req.YookassaSellerAccountId != nil {
		existing.YooKassaSellerAccountID = *req.YookassaSellerAccountId
	}

	if err := s.uc.Update(ctx, existing); err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}

	if sr := req.GetServicesReplace(); sr != nil {
		var items []domain.VenueService
		for _, it := range sr.GetItems() {
			name := strings.TrimSpace(it.GetName())
			if name == "" {
				continue
			}
			items = append(items, domain.VenueService{
				Name:        name,
				DurationMin: it.GetDurationMin(),
				Price:       it.GetPrice(),
				Description: strings.TrimSpace(it.GetDescription()),
			})
		}
		if err := s.uc.ReplaceVenueServices(ctx, id, existing.OwnerID, items); err != nil {
			if _, ok := status.FromError(err); ok {
				return nil, err
			}
			return nil, pkgerrors.Internal(err.Error())
		}
	}

	if hr := req.GetHallsReplace(); hr != nil {
		ups, convErr := hallProtoInputsToUpserts(hr.GetItems())
		if convErr != nil {
			return nil, pkgerrors.InvalidArgument("invalid hall id")
		}
		if err := s.uc.ReplaceVenueHalls(ctx, id, existing.OwnerID, ups); err != nil {
			if _, ok := status.FromError(err); ok {
				return nil, err
			}
			return nil, pkgerrors.Internal(err.Error())
		}
	}

	out, err := s.uc.GetByID(ctx, id)
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	if out == nil {
		return nil, pkgerrors.NotFound("venue not found")
	}

	_ = s.publisher.VenueUpdated(out)

	return venueToProto(out), nil
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
	query := strings.TrimSpace(req.GetQuery())
	var cs []string
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for _, enc := range md.Get("x-city-b64") {
			b, err := base64.RawURLEncoding.DecodeString(enc)
			if err != nil {
				continue
			}
			if t := strings.TrimSpace(string(b)); t != "" {
				cs = append(cs, t)
			}
		}
	}
	if len(cs) == 0 {
		cs = pkgcities.Split(strings.TrimSpace(req.GetCity()))
	}
	// Один город и пустой query — дублируем в query (старые клиенты / полнотекст по подстроке).
	if len(cs) == 1 && query == "" {
		query = cs[0]
	}
	params := domain.SearchParams{
		Query:     query,
		City:      "",
		Cities:    cs,
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

	venues, err := s.uc.ListForManagingUser(ctx, ownerID)
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
		if errors.Is(err, repository.ErrSlotUnavailable) {
			return nil, pkgerrors.InvalidArgument("time slot not available")
		}
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

func (s *Server) CreateManualSlotBlock(ctx context.Context, req *venuev1.CreateManualSlotBlockRequest) (*venuev1.CreateManualSlotBlockResponse, error) {
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid owner_id")
	}
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}

	id, err := s.uc.CreateManualSlotBlock(ctx, ownerID, venueID, req.GetDate(), req.GetTimeFrom(), req.GetTimeTo(), req.GetNote())
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	return &venuev1.CreateManualSlotBlockResponse{
		Block: &venuev1.ManualSlotBlock{
			Id:       id.String(),
			VenueId:  venueID.String(),
			Date:     req.GetDate(),
			TimeFrom: req.GetTimeFrom(),
			TimeTo:   req.GetTimeTo(),
			Note:     req.GetNote(),
		},
	}, nil
}

func (s *Server) DeleteManualSlotBlock(ctx context.Context, req *venuev1.DeleteManualSlotBlockRequest) (*venuev1.DeleteManualSlotBlockResponse, error) {
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid owner_id")
	}
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	blockID, err := uuid.Parse(req.GetBlockId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid block_id")
	}

	if err := s.uc.DeleteManualSlotBlock(ctx, ownerID, venueID, blockID); err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	return &venuev1.DeleteManualSlotBlockResponse{Success: true}, nil
}

func (s *Server) ListManualSlotBlocks(ctx context.Context, req *venuev1.ListManualSlotBlocksRequest) (*venuev1.ListManualSlotBlocksResponse, error) {
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid owner_id")
	}
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}

	blocks, err := s.uc.ListManualSlotBlocks(ctx, ownerID, venueID, req.GetDateFrom(), req.GetDateTo())
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	out := make([]*venuev1.ManualSlotBlock, len(blocks))
	for i := range blocks {
		out[i] = manualBlockToProto(&blocks[i])
	}
	return &venuev1.ListManualSlotBlocksResponse{Blocks: out}, nil
}

func (s *Server) GetVenueManagementAccess(ctx context.Context, req *venuev1.GetVenueManagementAccessRequest) (*venuev1.GetVenueManagementAccessResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid user_id")
	}
	access, err := s.uc.GetVenueManagementAccess(ctx, venueID, userID)
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	return &venuev1.GetVenueManagementAccessResponse{Access: access}, nil
}

func (s *Server) ListVenueStaff(ctx context.Context, req *venuev1.ListVenueStaffRequest) (*venuev1.ListVenueStaffResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	actorID, err := uuid.Parse(req.GetActorId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid actor_id")
	}
	members, err := s.uc.ListVenueStaff(ctx, venueID, actorID)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	out := &venuev1.ListVenueStaffResponse{}
	for i := range members {
		m := &members[i]
		out.Members = append(out.Members, &venuev1.VenueStaffMember{
			UserId:    m.UserID.String(),
			Role:      m.Role,
			InvitedBy: m.InvitedBy.String(),
			CreatedAt: timestamppb.New(m.CreatedAt),
		})
	}
	return out, nil
}

func (s *Server) AddVenueStaff(ctx context.Context, req *venuev1.AddVenueStaffRequest) (*venuev1.AddVenueStaffResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	actorID, err := uuid.Parse(req.GetActorId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid actor_id")
	}
	targetID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid user_id")
	}
	if err := s.uc.AddVenueStaff(ctx, venueID, actorID, targetID, req.GetRole()); err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	return &venuev1.AddVenueStaffResponse{}, nil
}

func (s *Server) RemoveVenueStaff(ctx context.Context, req *venuev1.RemoveVenueStaffRequest) (*venuev1.RemoveVenueStaffResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	actorID, err := uuid.Parse(req.GetActorId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid actor_id")
	}
	targetID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid user_id")
	}
	if err := s.uc.RemoveVenueStaff(ctx, venueID, actorID, targetID); err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	return &venuev1.RemoveVenueStaffResponse{}, nil
}

func (s *Server) ListVenueCRMTasks(ctx context.Context, req *venuev1.ListVenueCRMTasksRequest) (*venuev1.ListVenueCRMTasksResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	actorID, err := uuid.Parse(req.GetActorId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid actor_id")
	}
	tasks, err := s.uc.ListVenueCRMTasks(ctx, venueID, actorID, req.GetStatus())
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	out := &venuev1.ListVenueCRMTasksResponse{}
	for i := range tasks {
		out.Tasks = append(out.Tasks, venueCRMTaskToProto(&tasks[i]))
	}
	return out, nil
}

func (s *Server) CreateVenueCRMTask(ctx context.Context, req *venuev1.CreateVenueCRMTaskRequest) (*venuev1.CreateVenueCRMTaskResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	actorID, err := uuid.Parse(req.GetActorId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid actor_id")
	}
	var bookingID *uuid.UUID
	if b := strings.TrimSpace(req.GetBookingId()); b != "" {
		u, perr := uuid.Parse(b)
		if perr != nil {
			return nil, pkgerrors.InvalidArgument("invalid booking_id")
		}
		bookingID = &u
	}
	var assignee *uuid.UUID
	if a := strings.TrimSpace(req.GetAssigneeUserId()); a != "" {
		u, perr := uuid.Parse(a)
		if perr != nil {
			return nil, pkgerrors.InvalidArgument("invalid assignee_user_id")
		}
		assignee = &u
	}
	t, err := s.uc.CreateVenueCRMTask(ctx, venueID, actorID, req.GetTitle(), req.GetBody(), bookingID, assignee)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	return &venuev1.CreateVenueCRMTaskResponse{Task: venueCRMTaskToProto(t)}, nil
}

func (s *Server) CompleteVenueCRMTask(ctx context.Context, req *venuev1.CompleteVenueCRMTaskRequest) (*venuev1.CompleteVenueCRMTaskResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	actorID, err := uuid.Parse(req.GetActorId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid actor_id")
	}
	taskID, err := uuid.Parse(req.GetTaskId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid task_id")
	}
	if err := s.uc.CompleteVenueCRMTask(ctx, venueID, actorID, taskID); err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	return &venuev1.CompleteVenueCRMTaskResponse{}, nil
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

func (s *Server) publishVenueManagementAlert(ctx context.Context, venueID uuid.UUID, v *domain.Venue, typ, comment string) {
	ids, err := s.uc.RecipientUserIDsForVenue(ctx, venueID)
	if err != nil {
		s.log.Warn().Err(err).Str("venue_id", venueID.String()).Msg("recipient ids for management alert failed")
		return
	}
	uidStrs := make([]string, len(ids))
	for i := range ids {
		uidStrs[i] = ids[i].String()
	}
	alert := events.VenueManagementAlert{
		Type:              typ,
		VenueID:           v.ID.String(),
		VenueName:         v.Name,
		OwnerID:           v.OwnerID.String(),
		RecipientUserIDs:  uidStrs,
		ModerationComment: comment,
	}
	if pubErr := s.publisher.VenueManagementAlert(alert); pubErr != nil {
		s.log.Warn().Err(pubErr).Str("venue_id", v.ID.String()).Str("alert_type", typ).Msg("venue.management.alert publish failed")
	}
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
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}

	_ = s.publisher.VenueUpdated(v)
	actionLower := strings.ToLower(strings.TrimSpace(req.GetAction()))
	if v.Status == domain.StatusSuspended && actionLower == "suspend" {
		s.publishVenueManagementAlert(ctx, venueID, v, "suspended", req.GetComment())
	}
	if v.Status == domain.StatusActive && actionLower == "resume" {
		s.publishVenueManagementAlert(ctx, venueID, v, "resumed", req.GetComment())
	}
	if err := s.tg.NotifyModerated(v); err != nil {
		s.log.Warn().Err(err).Str("venue_id", v.ID.String()).Msg("telegram notify moderated failed")
	}

	return venueToProto(v), nil
}

func (s *Server) ListPendingVenues(ctx context.Context, req *venuev1.ListPendingVenuesRequest) (*venuev1.ListVenuesResponse, error) {
	status := req.GetStatus()
	if status == "" {
		status = "pending_review"
	}

	result, err := s.uc.ListByStatus(ctx, status, req.GetPage(), req.GetPageSize(), req.GetNameQuery())
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	return listResultToProto(result), nil
}

func (s *Server) AddVenuePhoto(ctx context.Context, req *venuev1.AddVenuePhotoRequest) (*venuev1.VenueResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid owner_id")
	}
	url := strings.TrimSpace(req.GetUrl())
	if url == "" {
		return nil, pkgerrors.InvalidArgument("url is required")
	}

	v, err := s.uc.AddVenuePhoto(ctx, venueID, ownerID, url)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	_ = s.publisher.VenueUpdated(v)
	return venueToProto(v), nil
}

func (s *Server) DeleteVenuePhoto(ctx context.Context, req *venuev1.DeleteVenuePhotoRequest) (*venuev1.DeleteVenuePhotoResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid owner_id")
	}
	photoID, err := uuid.Parse(req.GetPhotoId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid photo_id")
	}

	deletedURL, err := s.uc.DeleteVenuePhoto(ctx, venueID, ownerID, photoID)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	v, err := s.uc.GetByID(ctx, venueID)
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	if v != nil {
		_ = s.publisher.VenueUpdated(v)
	}
	return &venuev1.DeleteVenuePhotoResponse{Url: deletedURL}, nil
}

func (s *Server) SetVenueCoverPhoto(ctx context.Context, req *venuev1.SetVenueCoverPhotoRequest) (*venuev1.VenueResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid owner_id")
	}
	photoID, err := uuid.Parse(req.GetPhotoId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid photo_id")
	}

	v, err := s.uc.SetVenueCoverPhoto(ctx, venueID, ownerID, photoID)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	_ = s.publisher.VenueUpdated(v)
	return venueToProto(v), nil
}

func (s *Server) AddVenueHallPhoto(ctx context.Context, req *venuev1.AddVenueHallPhotoRequest) (*venuev1.VenueResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	hallID, err := uuid.Parse(req.GetHallId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid hall_id")
	}
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid owner_id")
	}
	url := strings.TrimSpace(req.GetUrl())
	if url == "" {
		return nil, pkgerrors.InvalidArgument("url is required")
	}
	v, err := s.uc.AddVenueHallPhoto(ctx, venueID, hallID, ownerID, url)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	_ = s.publisher.VenueUpdated(v)
	return venueToProto(v), nil
}

func (s *Server) DeleteVenueHallPhoto(ctx context.Context, req *venuev1.DeleteVenueHallPhotoRequest) (*venuev1.DeleteVenueHallPhotoResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	hallID, err := uuid.Parse(req.GetHallId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid hall_id")
	}
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid owner_id")
	}
	photoID, err := uuid.Parse(req.GetPhotoId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid photo_id")
	}
	deletedURL, err := s.uc.DeleteVenueHallPhoto(ctx, venueID, hallID, ownerID, photoID)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	v, err := s.uc.GetByID(ctx, venueID)
	if err != nil {
		return nil, pkgerrors.Internal(err.Error())
	}
	if v != nil {
		_ = s.publisher.VenueUpdated(v)
	}
	return &venuev1.DeleteVenueHallPhotoResponse{Url: deletedURL}, nil
}

func (s *Server) SetVenueHallCoverPhoto(ctx context.Context, req *venuev1.SetVenueHallCoverPhotoRequest) (*venuev1.VenueResponse, error) {
	venueID, err := uuid.Parse(req.GetVenueId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid venue_id")
	}
	hallID, err := uuid.Parse(req.GetHallId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid hall_id")
	}
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid owner_id")
	}
	photoID, err := uuid.Parse(req.GetPhotoId())
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid photo_id")
	}
	v, err := s.uc.SetVenueHallCoverPhoto(ctx, venueID, hallID, ownerID, photoID)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, pkgerrors.Internal(err.Error())
	}
	_ = s.publisher.VenueUpdated(v)
	return venueToProto(v), nil
}

func venueCRMTaskToProto(t *domain.VenueCRMTask) *venuev1.VenueCRMTask {
	p := &venuev1.VenueCRMTask{
		Id:        t.ID.String(),
		VenueId:   t.VenueID.String(),
		Title:     t.Title,
		Body:      t.Body,
		Status:    t.Status,
		CreatedBy: t.CreatedBy.String(),
		CreatedAt: timestamppb.New(t.CreatedAt),
		UpdatedAt: timestamppb.New(t.UpdatedAt),
	}
	if t.BookingID != nil {
		s := t.BookingID.String()
		p.BookingId = &s
	}
	if t.AssigneeUserID != nil {
		s := t.AssigneeUserID.String()
		p.AssigneeUserId = &s
	}
	return p
}

func manualBlockToProto(b *domain.ManualSlotBlock) *venuev1.ManualSlotBlock {
	return &venuev1.ManualSlotBlock{
		Id:       b.ID.String(),
		VenueId:  b.VenueID.String(),
		Date:     b.Date,
		TimeFrom: b.TimeFrom,
		TimeTo:   b.TimeTo,
		Note:     b.Note,
	}
}

func venueToProto(v *domain.Venue) *venuev1.VenueResponse {
	resp := &venuev1.VenueResponse{
		Id:                      v.ID.String(),
		OwnerId:                 v.OwnerID.String(),
		Slug:                    v.Slug,
		Name:                    v.Name,
		Type:                    v.Type,
		Description:             v.Description,
		Address:                 v.Address,
		City:                    v.City,
		Latitude:                v.Latitude,
		Longitude:               v.Longitude,
		PriceFrom:               v.PriceFrom,
		Capacity:                v.Capacity,
		Amenities:               v.Amenities,
		WorkingHours:            v.WorkingHours,
		Phone:                   v.Phone,
		AvgRating:               v.AvgRating,
		ReviewCount:             v.ReviewCount,
		IsActive:                v.IsActive,
		Status:                  v.Status,
		ModerationComment:       v.ModerationComment,
		CreatedAt:               timestamppb.New(v.CreatedAt),
		LegalEntityName:         v.LegalEntityName,
		Inn:                     v.INN,
		Ogrn:                    v.OGRN,
		PublicListingUrl:        v.PublicListingURL,
		VerificationNote:        v.VerificationNote,
		SocialLinks:             v.SocialLinks,
		PayoutLegalForm:         v.PayoutLegalForm,
		YookassaSellerAccountId: v.YooKassaSellerAccountID,
		ManagementAccess:        v.ManagementAccess,
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

	for i := range v.Halls {
		resp.Halls = append(resp.Halls, hallToProto(&v.Halls[i]))
	}

	return resp
}

func hallProtoInputsToUpserts(items []*venuev1.VenueHallInput) ([]domain.VenueHallUpsert, error) {
	out := make([]domain.VenueHallUpsert, 0, len(items))
	for _, it := range items {
		var id *uuid.UUID
		if sid := strings.TrimSpace(it.GetId()); sid != "" {
			u, err := uuid.Parse(sid)
			if err != nil {
				return nil, err
			}
			id = &u
		}
		am := it.GetAmenities()
		if am == nil {
			am = []string{}
		}
		out = append(out, domain.VenueHallUpsert{
			ID:        id,
			Name:      it.GetName(),
			PriceFrom: it.GetPriceFrom(),
			Capacity:  it.GetCapacity(),
			Amenities: append([]string(nil), am...),
			SortOrder: it.GetSortOrder(),
		})
	}
	return out, nil
}

func hallToProto(h *domain.VenueHall) *venuev1.VenueHall {
	ph := &venuev1.VenueHall{
		Id:        h.ID.String(),
		Name:      h.Name,
		PriceFrom: h.PriceFrom,
		Capacity:  h.Capacity,
		Amenities: h.Amenities,
		SortOrder: h.SortOrder,
	}
	for _, p := range h.Photos {
		ph.Photos = append(ph.Photos, &venuev1.VenueHallPhoto{
			Id:        p.ID.String(),
			Url:       p.URL,
			SortOrder: p.SortOrder,
			IsCover:   p.IsCover,
		})
	}
	return ph
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
