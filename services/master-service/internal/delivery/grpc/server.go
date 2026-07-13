package grpc

import (
	"context"
	"strings"

	"github.com/google/uuid"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/pkg/grpcutil"
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

// parseUUID parses a UUID string received from a proto field. On failure it
// returns an InvalidArgument status with a structured BadRequest detail naming
// the offending field. The human-readable message is deliberately generic
// ("malformed identifier") so that internal proto field names are not leaked
// into user-facing strings by API gateways that forward gRPC error messages
// verbatim (e.g. grpc-gateway's default error mapper).
func parseUUID(s string, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, pkgerrors.InvalidArgumentField(field, "must be a valid UUID")
	}
	return id, nil
}

// callerUUID extracts the authenticated caller's user_id from gRPC metadata
// (injected by api-gateway via CallerIDClientInterceptor) and parses it as UUID.
// Returns Unauthenticated if the header is absent (unauthenticated call) and
// InvalidArgument if the value is not a valid UUID (should never happen in practice).
func callerUUID(ctx context.Context) (uuid.UUID, error) {
	raw := grpcutil.CallerIDFromCtx(ctx)
	if raw == "" {
		return uuid.Nil, pkgerrors.Unauthenticated("missing caller identity")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, pkgerrors.InvalidArgument("invalid caller user_id")
	}
	return id, nil
}

func (s *Server) CreateMyProfile(ctx context.Context, req *masterv1.CreateMyProfileRequest) (*masterv1.MasterResponse, error) {
	uid, err := callerUUID(ctx)
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
	uid, err := callerUUID(ctx)
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
	uid, err := callerUUID(ctx)
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
	if req.PayoutLegalName != nil {
		v := *req.PayoutLegalName
		in.PayoutLegalName = &v
	}
	if req.PayoutInn != nil {
		v := *req.PayoutInn
		in.PayoutINN = &v
	}
	if req.PayoutKpp != nil {
		v := *req.PayoutKpp
		in.PayoutKPP = &v
	}
	if req.PayoutOgrn != nil {
		v := *req.PayoutOgrn
		in.PayoutOGRN = &v
	}
	if req.PayoutOgrnip != nil {
		v := *req.PayoutOgrnip
		in.PayoutOGRNIP = &v
	}
	if req.PayoutBankName != nil {
		v := *req.PayoutBankName
		in.PayoutBankName = &v
	}
	if req.PayoutBik != nil {
		v := *req.PayoutBik
		in.PayoutBIK = &v
	}
	if req.PayoutSettlementAccount != nil {
		v := *req.PayoutSettlementAccount
		in.PayoutSettlementAccount = &v
	}
	if req.PayoutCorrespondentAccount != nil {
		v := *req.PayoutCorrespondentAccount
		in.PayoutCorrespondentAccount = &v
	}
	if req.PayoutVerificationStatus != nil {
		v := *req.PayoutVerificationStatus
		in.PayoutVerificationStatus = &v
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
				Name:         it.GetName(),
				Description:  it.GetDescription(),
				DurationMin:  it.GetDurationMin(),
				Price:        it.GetPrice(),
				SortOrder:    it.GetSortOrder(),
				SortOrderSet: it.SortOrder != nil,
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
	if req.GetApplyCredentials() {
		in.ApplyCredentialsReplace = true
		for _, it := range req.GetCredentialsReplace() {
			in.CredentialsReplace = append(in.CredentialsReplace, domain.MasterCredentialUpsert{
				Kind:   it.GetKind(),
				Title:  it.GetTitle(),
				Issuer: it.GetIssuer(),
				Year:   it.GetYear(),
			})
		}
	}
	m, err := s.uc.UpdateMyProfile(ctx, uid, in)
	if err != nil {
		return nil, err
	}
	return &masterv1.MasterResponse{Master: masterToProto(m)}, nil
}

func (s *Server) SubmitForReview(ctx context.Context, req *masterv1.SubmitMasterForReviewRequest) (*masterv1.MasterResponse, error) {
	uid, err := callerUUID(ctx)
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

// GetMaster resolves a master profile by id for internal callers. Payout
// credentials are stripped (masterToProtoPublic); the response carries id,
// user_id and display_name — enough for the gateway to address an owner
// notification.
func (s *Server) GetMaster(ctx context.Context, req *masterv1.GetMasterRequest) (*masterv1.MasterResponse, error) {
	mid, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	m, err := s.uc.GetByID(ctx, mid)
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
		out = append(out, masterToProtoModerator(&list[i]))
	}
	return &masterv1.ListMastersResponse{Masters: out, Total: total}, nil
}

func (s *Server) ModerateMaster(ctx context.Context, req *masterv1.ModerateMasterRequest) (*masterv1.MasterResponse, error) {
	mid, err := parseUUID(req.GetMasterId(), "master_id")
	if err != nil {
		return nil, err
	}
	// moderator identity comes from the gateway-verified x-caller-id metadata,
	// not from the proto field (which the caller controls and could spoof).
	// req.ModeratorId is intentionally ignored to prevent privilege escalation.
	modID, err := callerUUID(ctx)
	if err != nil {
		return nil, err
	}
	m, err := s.uc.Moderate(ctx, mid, modID, req.GetAction(), req.GetComment())
	if err != nil {
		return nil, err
	}
	return &masterv1.MasterResponse{Master: masterToProtoModerator(m)}, nil
}

func (s *Server) SuspendMasterByUser(ctx context.Context, req *masterv1.SuspendMasterByUserRequest) (*masterv1.SuspendMasterByUserResponse, error) {
	uid, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	suspended, err := s.uc.SuspendByUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	return &masterv1.SuspendMasterByUserResponse{Suspended: suspended}, nil
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
	// client identity comes from the gateway-verified x-caller-id metadata,
	// not from the proto field (which the caller controls and could spoof).
	// req.ClientUserId is intentionally ignored to prevent booking on behalf of another user.
	cid, err := callerUUID(ctx)
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
	uid, err := callerUUID(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.uc.ListMyBookings(ctx, uid, req.GetStatusFilter())
	if err != nil {
		return nil, err
	}
	out := make([]*masterv1.MasterBooking, 0, len(list))
	for i := range list {
		pb := bookingToProto(&list[i])
		// The caller is the master owner, not the client who created the booking.
		// Strip the payment URL so master owners cannot intercept or
		// replay client payment links.
		pb.PaymentUrl = ""
		out = append(out, pb)
	}
	return &masterv1.ListMasterBookingsResponse{Bookings: out}, nil
}

func (s *Server) ListClientMasterBookings(ctx context.Context, req *masterv1.ListClientMasterBookingsRequest) (*masterv1.ListMasterBookingsResponse, error) {
	uid, err := callerUUID(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.uc.ListClientBookings(ctx, uid, req.GetStatusFilter())
	if err != nil {
		return nil, err
	}
	out := make([]*masterv1.MasterBooking, 0, len(list))
	for i := range list {
		out = append(out, bookingToProto(&list[i]))
	}
	return &masterv1.ListMasterBookingsResponse{Bookings: out}, nil
}

func (s *Server) ListMyMasterClients(ctx context.Context, req *masterv1.ListMyMasterClientsRequest) (*masterv1.ListMyMasterClientsResponse, error) {
	uid, err := callerUUID(ctx)
	if err != nil {
		return nil, err
	}
	clients, total, err := s.uc.ListMyClients(ctx, uid, domain.MasterClientListParams{
		Segment: req.GetSegment(),
		Sort:    req.GetSort(),
		Limit:   int(req.GetLimit()),
		Offset:  int(req.GetOffset()),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*masterv1.MasterClient, 0, len(clients))
	for i := range clients {
		out = append(out, clientToProto(&clients[i]))
	}
	return &masterv1.ListMyMasterClientsResponse{Clients: out, Total: int32(total)}, nil
}

func clientToProto(c *domain.MasterClient) *masterv1.MasterClient {
	if c == nil {
		return nil
	}
	pb := &masterv1.MasterClient{
		UserId:        c.UserID.String(),
		BookingsCount: c.BookingsCount,
		VisitsCount:   c.VisitsCount,
		TotalSpent:    c.TotalSpent,
		Segments:      c.Segments,
	}
	if c.FirstVisitAt != nil {
		pb.FirstVisitAt = timestamppb.New(*c.FirstVisitAt)
	}
	if c.LastVisitAt != nil {
		pb.LastVisitAt = timestamppb.New(*c.LastVisitAt)
	}
	if c.LastBookingAt != nil {
		pb.LastBookingAt = timestamppb.New(*c.LastBookingAt)
	}
	return pb
}

func (s *Server) CreateMasterSlotBlock(ctx context.Context, req *masterv1.CreateMasterSlotBlockRequest) (*masterv1.MasterSlotBlockResponse, error) {
	uid, err := callerUUID(ctx)
	if err != nil {
		return nil, err
	}
	b, err := s.uc.CreateSlotBlock(ctx, uid, req.GetDate(), req.GetTimeFrom(), req.GetTimeTo(), req.GetNote())
	if err != nil {
		return nil, err
	}
	return &masterv1.MasterSlotBlockResponse{Block: slotBlockToProto(b)}, nil
}

func (s *Server) ListMasterSlotBlocks(ctx context.Context, req *masterv1.ListMasterSlotBlocksRequest) (*masterv1.ListMasterSlotBlocksResponse, error) {
	uid, err := callerUUID(ctx)
	if err != nil {
		return nil, err
	}
	blocks, err := s.uc.ListSlotBlocks(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := make([]*masterv1.MasterSlotBlock, 0, len(blocks))
	for i := range blocks {
		out = append(out, slotBlockToProto(&blocks[i]))
	}
	return &masterv1.ListMasterSlotBlocksResponse{Blocks: out}, nil
}

func (s *Server) DeleteMasterSlotBlock(ctx context.Context, req *masterv1.DeleteMasterSlotBlockRequest) (*masterv1.DeleteMasterSlotBlockResponse, error) {
	uid, err := callerUUID(ctx)
	if err != nil {
		return nil, err
	}
	bid, err := parseUUID(req.GetBlockId(), "block_id")
	if err != nil {
		return nil, err
	}
	if err := s.uc.DeleteSlotBlock(ctx, uid, bid); err != nil {
		return nil, err
	}
	return &masterv1.DeleteMasterSlotBlockResponse{Deleted: true}, nil
}

func slotBlockToProto(b *domain.MasterSlotBlock) *masterv1.MasterSlotBlock {
	if b == nil {
		return nil
	}
	return &masterv1.MasterSlotBlock{
		Id:        b.ID.String(),
		MasterId:  b.MasterID.String(),
		Date:      b.Date,
		TimeFrom:  b.TimeFrom,
		TimeTo:    b.TimeTo,
		Note:      b.Note,
		CreatedAt: timestamppb.New(b.CreatedAt),
	}
}

func (s *Server) GetMasterBooking(ctx context.Context, req *masterv1.GetMasterBookingRequest) (*masterv1.MasterBookingResponse, error) {
	bid, err := parseUUID(req.GetBookingId(), "booking_id")
	if err != nil {
		return nil, err
	}
	// actor identity comes from the gateway-verified x-caller-id metadata,
	// not from the proto field (which the caller controls and could spoof).
	aid, err := callerUUID(ctx)
	if err != nil {
		return nil, err
	}
	b, err := s.uc.GetBookingForActor(ctx, bid, aid)
	if err != nil {
		return nil, err
	}
	pb := bookingToProto(b)
	if ownerID, err := s.uc.MasterOwnerUserID(ctx, b.MasterID); err == nil {
		s := ownerID.String()
		pb.MasterUserId = &s
	}
	// Strip the payment URL when the actor is the master owner rather
	// than the client who created the booking.  Master owners must not receive
	// client payment links (replay / interception risk).
	if aid != b.ClientUserID {
		pb.PaymentUrl = ""
	}
	return &masterv1.MasterBookingResponse{Booking: pb}, nil
}

func (s *Server) GetMasterBookingsBatch(ctx context.Context, req *masterv1.GetMasterBookingsBatchRequest) (*masterv1.GetMasterBookingsBatchResponse, error) {
	// actor identity comes from the gateway-verified x-caller-id metadata,
	// not from the proto field (which the caller controls and could spoof).
	actorID, err := callerUUID(ctx)
	if err != nil {
		return nil, err
	}
	rawIDs := req.GetBookingIds()
	if len(rawIDs) == 0 {
		return &masterv1.GetMasterBookingsBatchResponse{Bookings: map[string]*masterv1.MasterBooking{}}, nil
	}
	ids := make([]uuid.UUID, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, err := parseUUID(raw, "booking_id")
		if err != nil {
			continue // silently skip malformed ids
		}
		ids = append(ids, id)
	}

	bookings, err := s.uc.GetBookingsForActorBatch(ctx, ids, actorID)
	if err != nil {
		return nil, err
	}

	// Resolve master_user_id for all distinct master profiles in one query.
	masterIDs := make([]uuid.UUID, 0, len(bookings))
	seen := make(map[uuid.UUID]struct{}, len(bookings))
	for i := range bookings {
		mid := bookings[i].MasterID
		if _, ok := seen[mid]; !ok {
			seen[mid] = struct{}{}
			masterIDs = append(masterIDs, mid)
		}
	}
	ownerByMaster, err := s.uc.GetMasterUserIDsBatch(ctx, masterIDs)
	if err != nil {
		ownerByMaster = map[uuid.UUID]uuid.UUID{} // non-fatal — omit master_user_id
	}

	resp := &masterv1.GetMasterBookingsBatchResponse{
		Bookings: make(map[string]*masterv1.MasterBooking, len(bookings)),
	}
	for i := range bookings {
		b := &bookings[i]
		pb := bookingToProto(b)
		if ownerID, ok := ownerByMaster[b.MasterID]; ok {
			s := ownerID.String()
			pb.MasterUserId = &s
		}
		// Strip the payment URL when the actor is the master owner rather
		// than the client who created the booking.  Master owners must not receive
		// client payment links (replay / interception risk).
		if actorID != b.ClientUserID {
			pb.PaymentUrl = ""
		}
		resp.Bookings[b.ID.String()] = pb
	}
	return resp, nil
}

func (s *Server) HasCompletedMasterBooking(ctx context.Context, req *masterv1.HasCompletedMasterBookingRequest) (*masterv1.HasCompletedMasterBookingResponse, error) {
	clientID, err := parseUUID(req.GetClientUserId(), "client_user_id")
	if err != nil {
		return nil, err
	}
	masterID, err := parseUUID(req.GetMasterId(), "master_id")
	if err != nil {
		return nil, err
	}

	// Identity check: prevent end-users from querying the gate on behalf of
	// others (which would let them bypass the review eligibility check).
	//
	// Internal services (e.g. review-service) send a non-UUID caller ID such
	// as "review-service" via CallerIDClientInterceptor. They are trusted to
	// supply the correct client_user_id from their own verified context.
	//
	// End-users arrive through api-gateway with a UUID caller ID. They may
	// only query their own booking history, so caller must equal client_user_id.
	raw := grpcutil.CallerIDFromCtx(ctx)
	if !grpcutil.IsServiceCallerID(raw) {
		// Caller is an end-user — enforce self-query only.
		callerID, err := callerUUID(ctx)
		if err != nil {
			return nil, err
		}
		if callerID != clientID {
			return nil, pkgerrors.PermissionDenied("cannot query booking history for another user")
		}
	}
	// Service caller: trust req.client_user_id as set by the calling service.

	ok, err := s.uc.HasCompletedBookingByClientMaster(ctx, clientID, masterID)
	if err != nil {
		return nil, err
	}
	return &masterv1.HasCompletedMasterBookingResponse{HasCompleted: ok}, nil
}

func (s *Server) AddMasterPhoto(ctx context.Context, req *masterv1.AddMasterPhotoRequest) (*masterv1.MasterResponse, error) {
	uid, err := callerUUID(ctx)
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
	uid, err := callerUUID(ctx)
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

func (s *Server) AddMasterVideo(ctx context.Context, req *masterv1.AddMasterVideoRequest) (*masterv1.MasterResponse, error) {
	uid, err := callerUUID(ctx)
	if err != nil {
		return nil, err
	}
	m, err := s.uc.AddMasterVideo(ctx, uid, req.GetUrl())
	if err != nil {
		return nil, err
	}
	return &masterv1.MasterResponse{Master: masterToProto(m)}, nil
}

func (s *Server) DeleteMasterVideo(ctx context.Context, req *masterv1.DeleteMasterVideoRequest) (*masterv1.DeleteMasterVideoResponse, error) {
	uid, err := callerUUID(ctx)
	if err != nil {
		return nil, err
	}
	vid, err := parseUUID(req.GetVideoId(), "video_id")
	if err != nil {
		return nil, err
	}
	u, err := s.uc.DeleteMasterVideo(ctx, uid, vid)
	if err != nil {
		return nil, err
	}
	return &masterv1.DeleteMasterVideoResponse{DeletedUrl: u}, nil
}

func (s *Server) SetMasterCoverPhoto(ctx context.Context, req *masterv1.SetMasterCoverPhotoRequest) (*masterv1.MasterResponse, error) {
	uid, err := callerUUID(ctx)
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
	vids := make([]*masterv1.MasterVideo, 0, len(m.Videos))
	for i := range m.Videos {
		v := &m.Videos[i]
		vids = append(vids, &masterv1.MasterVideo{
			Id:        v.ID.String(),
			Url:       v.URL,
			SortOrder: v.SortOrder,
		})
	}
	creds := make([]*masterv1.MasterCredentialItem, 0, len(m.Credentials))
	for i := range m.Credentials {
		c := &m.Credentials[i]
		creds = append(creds, &masterv1.MasterCredentialItem{
			Id:        c.ID.String(),
			Kind:      c.Kind,
			Title:     c.Title,
			Issuer:    c.Issuer,
			Year:      c.Year,
			SortOrder: c.SortOrder,
		})
	}
	pb := &masterv1.Master{
		Id:                         m.ID.String(),
		UserId:                     m.UserID.String(),
		Slug:                       m.Slug,
		DisplayName:                m.DisplayName,
		Bio:                        m.Bio,
		Phone:                      m.Phone,
		City:                       m.City,
		WorkFormat:                 m.WorkFormat,
		TravelRadiusKm:             m.TravelRadiusKm,
		ExperienceYears:            m.ExperienceYears,
		Specializations:            m.Specializations,
		HourlyRate:                 m.HourlyRate,
		AvailabilityJson:           m.AvailabilityJSON,
		PayoutLegalForm:            m.PayoutLegalForm,
		PayoutLegalName:            m.PayoutLegalName,
		PayoutInn:                  m.PayoutINN,
		PayoutKpp:                  m.PayoutKPP,
		PayoutOgrn:                 m.PayoutOGRN,
		PayoutOgrnip:               m.PayoutOGRNIP,
		PayoutBankName:             m.PayoutBankName,
		PayoutBik:                  m.PayoutBIK,
		PayoutSettlementAccount:    m.PayoutSettlementAccount,
		PayoutCorrespondentAccount: m.PayoutCorrespondentAccount,
		PayoutVerificationStatus:   m.PayoutVerificationStatus,
		PayoutReady:                m.PayoutProfileReady(),
		Status:                     m.Status,
		ModerationComment:          m.ModerationComment,
		Services:                   svcs,
		Photos:                     ph,
		Videos:                     vids,
		Credentials:                creds,
		CreatedAt:                  timestamppb.New(m.CreatedAt),
		UpdatedAt:                  timestamppb.New(m.UpdatedAt),
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
		p.PayoutLegalName = ""
		p.PayoutInn = ""
		p.PayoutKpp = ""
		p.PayoutOgrn = ""
		p.PayoutOgrnip = ""
		p.PayoutBankName = ""
		p.PayoutBik = ""
		p.PayoutSettlementAccount = ""
		p.PayoutCorrespondentAccount = ""
		p.PayoutVerificationStatus = ""
		p.PayoutReady = false
	}
	return p
}

// masterToProtoModerator returns a master proto safe for admin/moderation endpoints.
// It preserves payout_legal_form and payout_verification_status so moderators
// can see *what kind* of legal entity the master has and whether reqs are verified,
// but strips all raw payment credentials (INN, BIK, account numbers, OGRN, etc.)
// that are personal data under 152-FZ and not needed for the moderation decision.
// Phone is also stripped — it is visible to the master's own profile endpoint.
func masterToProtoModerator(m *domain.Master) *masterv1.Master {
	p := masterToProto(m)
	if p != nil {
		// Keep: payout_legal_form, payout_verification_status, payout_ready
		// (needed to understand payment setup completeness for moderation).
		// Strip: all raw credentials — personal/payment data, not needed for approve/reject.
		p.PayoutLegalName = ""
		p.PayoutInn = ""
		p.PayoutKpp = ""
		p.PayoutOgrn = ""
		p.PayoutOgrnip = ""
		p.PayoutBankName = ""
		p.PayoutBik = ""
		p.PayoutSettlementAccount = ""
		p.PayoutCorrespondentAccount = ""
		// Phone is personal data; moderator sees display_name, bio, city, status.
		p.Phone = ""
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
		PaymentId:    b.PaymentID,
		PaymentUrl:   b.PaymentURL,
		TotalPrice:   b.TotalPrice,
		CreatedAt:    timestamppb.New(b.CreatedAt),
	}
	if b.MasterServiceID != nil {
		s := b.MasterServiceID.String()
		pb.MasterServiceId = &s
	}
	return pb
}
