package grpc

import (
	"context"
	"strings"

	"github.com/google/uuid"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
	"github.com/tienlao/agregator/services/master-service/internal/usecase"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	masterv1.UnimplementedMasterServiceServer
	uc *usecase.MasterUseCase
}

func NewServer(uc *usecase.MasterUseCase) *Server {
	return &Server{uc: uc}
}

func parseUUID(s string, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, pkgerrors.InvalidArgument("invalid " + field)
	}
	return id, nil
}

func (s *Server) CreateMyProfile(ctx context.Context, req *masterv1.CreateMyProfileRequest) (*masterv1.MasterResponse, error) {
	uid, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	m, err := s.uc.CreateMyProfile(ctx, uid, req.GetDisplayName())
	if err != nil {
		return nil, err
	}
	return &masterv1.MasterResponse{Master: masterToProto(m)}, nil
}

func (s *Server) GetMyProfile(ctx context.Context, req *masterv1.GetMyProfileRequest) (*masterv1.MasterResponse, error) {
	uid, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	m, err := s.uc.GetMyProfile(ctx, uid)
	if err != nil {
		return nil, err
	}
	return &masterv1.MasterResponse{Master: masterToProto(m)}, nil
}

func (s *Server) UpdateMyProfile(ctx context.Context, req *masterv1.UpdateMyProfileRequest) (*masterv1.MasterResponse, error) {
	uid, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	in := usecase.UpdateMasterInput{}
	if req.DisplayName != nil {
		v := req.GetDisplayName()
		in.DisplayName = &v
	}
	if req.Bio != nil {
		v := req.GetBio()
		in.Bio = &v
	}
	if req.Phone != nil {
		v := req.GetPhone()
		in.Phone = &v
	}
	if req.City != nil {
		v := req.GetCity()
		in.City = &v
	}
	if req.WorkFormat != nil {
		v := req.GetWorkFormat()
		in.WorkFormat = &v
	}
	if req.TravelRadiusKm != nil {
		v := req.GetTravelRadiusKm()
		in.TravelRadiusKm = &v
	}
	if req.TravelBaseLatitude != nil {
		v := req.GetTravelBaseLatitude()
		in.TravelBaseLatitude = &v
	}
	if req.TravelBaseLongitude != nil {
		v := req.GetTravelBaseLongitude()
		in.TravelBaseLongitude = &v
	}
	if req.ExperienceYears != nil {
		v := req.GetExperienceYears()
		in.ExperienceYears = &v
	}
	if req.GetApplySpecializations() {
		in.ApplySpecializations = true
		in.Specializations = req.GetSpecializations()
	}
	if req.HourlyRate != nil {
		v := req.GetHourlyRate()
		in.HourlyRate = &v
	}
	if req.AvailabilityJson != nil {
		v := req.GetAvailabilityJson()
		in.AvailabilityJSON = &v
	}
	if req.PayoutLegalForm != nil {
		v := *req.PayoutLegalForm
		in.PayoutLegalForm = &v
	}
	if req.GetApplyTravelExcludeZones() {
		in.ApplyTravelExcludeZones = true
		for _, z := range req.GetTravelExcludeZones() {
			in.TravelExcludeZones = append(in.TravelExcludeZones, domain.MasterTravelExcludeZone{
				ID:        z.GetId(),
				Latitude:  z.GetLatitude(),
				Longitude: z.GetLongitude(),
				RadiusKm:  z.GetRadiusKm(),
				Label:     z.GetLabel(),
			})
		}
	}
	if req.GetApplyServicesReplace() {
		in.ApplyServicesReplace = true
		for _, it := range req.GetServicesReplace() {
			u := domain.MasterServiceUpsert{
				Name:        it.GetName(),
				Description: it.GetDescription(),
				DurationMin: it.GetDurationMin(),
				Price:       it.GetPrice(),
				SortOrder:   it.GetSortOrder(),
			}
			if it.GetId() != "" {
				id, err := uuid.Parse(it.GetId())
				if err != nil {
					return nil, pkgerrors.InvalidArgument("invalid service id")
				}
				u.ID = &id
			}
			in.ServicesReplace = append(in.ServicesReplace, u)
		}
	}
	m, err := s.uc.UpdateMyProfile(ctx, uid, in)
	if err != nil {
		return nil, err
	}
	return &masterv1.MasterResponse{Master: masterToProto(m)}, nil
}

func (s *Server) SubmitForReview(ctx context.Context, req *masterv1.SubmitMasterForReviewRequest) (*masterv1.MasterResponse, error) {
	uid, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	m, err := s.uc.SubmitForReview(ctx, uid)
	if err != nil {
		return nil, err
	}
	return &masterv1.MasterResponse{Master: masterToProto(m)}, nil
}

func (s *Server) ListPublicMasters(ctx context.Context, req *masterv1.ListPublicMastersRequest) (*masterv1.ListMastersResponse, error) {
	cities := append([]string(nil), req.GetCities()...)
	if len(cities) == 0 {
		if c := strings.TrimSpace(req.GetCity()); c != "" {
			cities = []string{c}
		}
	}
	params := domain.ListPublicMastersParams{
		Query:           strings.TrimSpace(req.GetQ()),
		Cities:          cities,
		WorkFormat:      strings.TrimSpace(req.GetWorkFormat()),
		PriceMinKopecks: req.GetPriceMinKopecks(),
		PriceMaxKopecks: req.GetPriceMaxKopecks(),
		Limit:           req.GetLimit(),
		Offset:          req.GetOffset(),
	}
	list, total, err := s.uc.ListPublic(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]*masterv1.Master, 0, len(list))
	for i := range list {
		out = append(out, masterToProtoPublic(&list[i]))
	}
	return &masterv1.ListMastersResponse{Masters: out, Total: total}, nil
}

func (s *Server) GetPublicMaster(ctx context.Context, req *masterv1.GetPublicMasterRequest) (*masterv1.MasterResponse, error) {
	m, err := s.uc.GetPublicBySlug(ctx, req.GetSlug())
	if err != nil {
		return nil, err
	}
	return &masterv1.MasterResponse{Master: masterToProtoPublic(m)}, nil
}

func (s *Server) ListForModeration(ctx context.Context, req *masterv1.ListForModerationRequest) (*masterv1.ListMastersResponse, error) {
	list, total, err := s.uc.ListForModeration(ctx, req.GetStatusFilter(), req.GetLimit(), req.GetOffset())
	if err != nil {
		return nil, err
	}
	out := make([]*masterv1.Master, 0, len(list))
	for i := range list {
		out = append(out, masterToProto(&list[i]))
	}
	return &masterv1.ListMastersResponse{Masters: out, Total: total}, nil
}

func (s *Server) ModerateMaster(ctx context.Context, req *masterv1.ModerateMasterRequest) (*masterv1.MasterResponse, error) {
	mid, err := parseUUID(req.GetMasterId(), "master_id")
	if err != nil {
		return nil, err
	}
	modID, err := parseUUID(req.GetModeratorId(), "moderator_id")
	if err != nil {
		return nil, err
	}
	m, err := s.uc.Moderate(ctx, mid, modID, req.GetAction(), req.GetComment())
	if err != nil {
		return nil, err
	}
	return &masterv1.MasterResponse{Master: masterToProto(m)}, nil
}

func (s *Server) ListModerationHistory(ctx context.Context, req *masterv1.ListModerationHistoryRequest) (*masterv1.ListModerationHistoryResponse, error) {
	mid, err := parseUUID(req.GetMasterId(), "master_id")
	if err != nil {
		return nil, err
	}
	entries, err := s.uc.ListModerationHistory(ctx, mid, req.GetLimit())
	if err != nil {
		return nil, err
	}
	out := make([]*masterv1.ModerationHistoryEntry, 0, len(entries))
	for i := range entries {
		e := &entries[i]
		out = append(out, &masterv1.ModerationHistoryEntry{
			Id:        e.ID.String(),
			MasterId:  e.MasterID.String(),
			OldStatus: e.OldStatus,
			NewStatus: e.NewStatus,
			Comment:   e.Comment,
			ChangedBy: e.ChangedBy.String(),
			CreatedAt: timestamppb.New(e.CreatedAt),
		})
	}
	return &masterv1.ListModerationHistoryResponse{Entries: out}, nil
}

func (s *Server) CreateMasterBooking(ctx context.Context, req *masterv1.CreateMasterBookingRequest) (*masterv1.MasterBookingResponse, error) {
	cid, err := parseUUID(req.GetClientUserId(), "client_user_id")
	if err != nil {
		return nil, err
	}
	var svcID *uuid.UUID
	if req.GetMasterServiceId() != "" {
		id, err := uuid.Parse(req.GetMasterServiceId())
		if err != nil {
			return nil, pkgerrors.InvalidArgument("invalid master_service_id")
		}
		svcID = &id
	}
	b, err := s.uc.CreateBooking(ctx, cid, req.GetMasterSlug(), svcID, req.GetDate(), req.GetTimeFrom(), req.GetTimeTo(), req.GetComment())
	if err != nil {
		return nil, err
	}
	return &masterv1.MasterBookingResponse{Booking: bookingToProto(b)}, nil
}

func (s *Server) ListMyMasterBookings(ctx context.Context, req *masterv1.ListMyMasterBookingsRequest) (*masterv1.ListMasterBookingsResponse, error) {
	uid, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	list, err := s.uc.ListMyBookings(ctx, uid, req.GetStatusFilter())
	if err != nil {
		return nil, err
	}
	out := make([]*masterv1.MasterBooking, 0, len(list))
	for i := range list {
		out = append(out, bookingToProto(&list[i]))
	}
	return &masterv1.ListMasterBookingsResponse{Bookings: out}, nil
}

func (s *Server) AddMasterPhoto(ctx context.Context, req *masterv1.AddMasterPhotoRequest) (*masterv1.MasterResponse, error) {
	uid, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	m, err := s.uc.AddMasterPhoto(ctx, uid, req.GetUrl())
	if err != nil {
		return nil, err
	}
	return &masterv1.MasterResponse{Master: masterToProto(m)}, nil
}

func (s *Server) DeleteMasterPhoto(ctx context.Context, req *masterv1.DeleteMasterPhotoRequest) (*masterv1.DeleteMasterPhotoResponse, error) {
	uid, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	pid, err := parseUUID(req.GetPhotoId(), "photo_id")
	if err != nil {
		return nil, err
	}
	u, err := s.uc.DeleteMasterPhoto(ctx, uid, pid)
	if err != nil {
		return nil, err
	}
	return &masterv1.DeleteMasterPhotoResponse{DeletedUrl: u}, nil
}

func (s *Server) SetMasterCoverPhoto(ctx context.Context, req *masterv1.SetMasterCoverPhotoRequest) (*masterv1.MasterResponse, error) {
	uid, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	pid, err := parseUUID(req.GetPhotoId(), "photo_id")
	if err != nil {
		return nil, err
	}
	m, err := s.uc.SetMasterCoverPhoto(ctx, uid, pid)
	if err != nil {
		return nil, err
	}
	return &masterv1.MasterResponse{Master: masterToProto(m)}, nil
}

func masterToProto(m *domain.Master) *masterv1.Master {
	if m == nil {
		return nil
	}
	svcs := make([]*masterv1.MasterServiceItem, 0, len(m.Services))
	for i := range m.Services {
		s := &m.Services[i]
		svcs = append(svcs, &masterv1.MasterServiceItem{
			Id:          s.ID.String(),
			Name:        s.Name,
			Description: s.Description,
			DurationMin: s.DurationMin,
			Price:       s.Price,
			SortOrder:   s.SortOrder,
		})
	}
	ph := make([]*masterv1.MasterPhoto, 0, len(m.Photos))
	for i := range m.Photos {
		p := &m.Photos[i]
		ph = append(ph, &masterv1.MasterPhoto{
			Id:        p.ID.String(),
			Url:       p.URL,
			SortOrder: p.SortOrder,
			IsCover:   p.IsCover,
		})
	}
	pb := &masterv1.Master{
		Id:              m.ID.String(),
		UserId:          m.UserID.String(),
		Slug:            m.Slug,
		DisplayName:     m.DisplayName,
		Bio:             m.Bio,
		Phone:           m.Phone,
		City:            m.City,
		WorkFormat:      m.WorkFormat,
		TravelRadiusKm:  m.TravelRadiusKm,
		ExperienceYears: m.ExperienceYears,
		Specializations:    m.Specializations,
		HourlyRate:         m.HourlyRate,
		AvailabilityJson:   m.AvailabilityJSON,
		PayoutLegalForm:    m.PayoutLegalForm,
		Status:             m.Status,
		ModerationComment:  m.ModerationComment,
		Services:           svcs,
		Photos:             ph,
		CreatedAt:          timestamppb.New(m.CreatedAt),
		UpdatedAt:          timestamppb.New(m.UpdatedAt),
	}
	if m.ModeratedBy != nil {
		s := m.ModeratedBy.String()
		pb.ModeratedBy = &s
	}
	if m.ModeratedAt != nil {
		pb.ModeratedAt = timestamppb.New(*m.ModeratedAt)
	}
	if m.TravelBaseLatitude != nil {
		lat := *m.TravelBaseLatitude
		pb.TravelBaseLatitude = &lat
	}
	if m.TravelBaseLongitude != nil {
		lon := *m.TravelBaseLongitude
		pb.TravelBaseLongitude = &lon
	}
	for i := range m.TravelExcludeZones {
		z := &m.TravelExcludeZones[i]
		pb.TravelExcludeZones = append(pb.TravelExcludeZones, &masterv1.MasterTravelExcludeZone{
			Id:        z.ID,
			Latitude:  z.Latitude,
			Longitude: z.Longitude,
			RadiusKm:  z.RadiusKm,
			Label:     z.Label,
		})
	}
	return pb
}

func masterToProtoPublic(m *domain.Master) *masterv1.Master {
	p := masterToProto(m)
	if p != nil {
		p.PayoutLegalForm = ""
	}
	return p
}

func bookingToProto(b *domain.MasterBooking) *masterv1.MasterBooking {
	if b == nil {
		return nil
	}
	pb := &masterv1.MasterBooking{
		Id:           b.ID.String(),
		MasterId:     b.MasterID.String(),
		ClientUserId: b.ClientUserID.String(),
		Date:         b.Date,
		TimeFrom:     b.TimeFrom,
		TimeTo:       b.TimeTo,
		Comment:      b.Comment,
		Status:       b.Status,
		CreatedAt:    timestamppb.New(b.CreatedAt),
	}
	if b.MasterServiceID != nil {
		s := b.MasterServiceID.String()
		pb.MasterServiceId = s
	}
	return pb
}
