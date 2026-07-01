package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
	"github.com/tienlao/agregator/services/master-service/internal/usecase"
)

// mockRepo embeds domain.MasterRepository so only the methods a given handler
// exercises need an implementation; any unstubbed method panics with a nil
// dereference, which surfaces an unexpected call loudly in tests.
type mockRepo struct {
	domain.MasterRepository

	ListPublicFunc            func(ctx context.Context, p domain.ListPublicMastersParams) ([]domain.Master, int32, error)
	GetBySlugFunc             func(ctx context.Context, slug string) (*domain.Master, error)
	GetByIDFunc               func(ctx context.Context, id uuid.UUID) (*domain.Master, error)
	SuspendByUserFunc         func(ctx context.Context, userID uuid.UUID) (bool, error)
	ListByStatusFunc          func(ctx context.Context, status string, limit, offset int32) ([]domain.Master, int32, error)
	ModerateAtomicFunc        func(ctx context.Context, masterID uuid.UUID, oldS, newS, comment string, by *uuid.UUID) error
	ListModerationHistoryFunc func(ctx context.Context, masterID uuid.UUID, limit int32) ([]domain.ModerationHistoryEntry, error)
}

func (m *mockRepo) ListPublic(ctx context.Context, p domain.ListPublicMastersParams) ([]domain.Master, int32, error) {
	return m.ListPublicFunc(ctx, p)
}
func (m *mockRepo) GetBySlug(ctx context.Context, slug string) (*domain.Master, error) {
	return m.GetBySlugFunc(ctx, slug)
}
func (m *mockRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Master, error) {
	return m.GetByIDFunc(ctx, id)
}
func (m *mockRepo) SuspendByUser(ctx context.Context, userID uuid.UUID) (bool, error) {
	return m.SuspendByUserFunc(ctx, userID)
}
func (m *mockRepo) ListByStatus(ctx context.Context, status string, limit, offset int32) ([]domain.Master, int32, error) {
	return m.ListByStatusFunc(ctx, status, limit, offset)
}
func (m *mockRepo) ModerateAtomic(ctx context.Context, masterID uuid.UUID, oldS, newS, comment string, by *uuid.UUID) error {
	return m.ModerateAtomicFunc(ctx, masterID, oldS, newS, comment, by)
}
func (m *mockRepo) ListModerationHistory(ctx context.Context, masterID uuid.UUID, limit int32) ([]domain.ModerationHistoryEntry, error) {
	return m.ListModerationHistoryFunc(ctx, masterID, limit)
}

// stubPayment is a non-nil payment client (NewMasterUseCase panics on nil) that
// is never invoked by the read/moderation handlers under test.
type stubPayment struct{}

func (stubPayment) CreatePayment(context.Context, *paymentv1.CreatePaymentRequest, ...grpc.CallOption) (*paymentv1.PaymentResponse, error) {
	return nil, nil
}

func newServer(repo domain.MasterRepository) *Server {
	uc := usecase.NewMasterUseCase(repo, stubPayment{}, zerolog.Nop())
	return NewServer(uc)
}

// ctxWithCaller builds a context carrying the x-caller-id the same way the
// production server interceptor does, so callerUUID can resolve it.
func ctxWithCaller(uid string) context.Context {
	interceptor := grpcutil.CallerIDServerInterceptor()
	in := metadata.NewIncomingContext(context.Background(),
		metadata.New(map[string]string{grpcutil.CallerIDHeader: uid}))
	var out context.Context
	_, _ = interceptor(in, nil, &grpc.UnaryServerInfo{}, func(c context.Context, _ any) (any, error) {
		out = c
		return nil, nil
	})
	return out
}

func TestParseUUID(t *testing.T) {
	valid := "11111111-1111-1111-1111-111111111111"
	got, err := parseUUID(valid, "id")
	if err != nil {
		t.Fatalf("parseUUID(valid) returned error: %v", err)
	}
	if got.String() != valid {
		t.Errorf("parseUUID = %q, want %q", got.String(), valid)
	}

	if _, err := parseUUID("not-a-uuid", "id"); err == nil {
		t.Error("parseUUID(invalid) expected error, got nil")
	}
}

// fullMaster builds a domain.Master populated across every field group so the
// converters are exercised end-to-end (services, photos, credentials, optional
// pointers, travel zones).
func fullMaster() *domain.Master {
	moderatedBy := uuid.MustParse("99999999-9999-9999-9999-999999999999")
	moderatedAt := time.Unix(1700000000, 0).UTC()
	lat, lon := 55.75, 37.61
	return &domain.Master{
		ID:                         uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		UserID:                     uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Slug:                       "ivan-master",
		DisplayName:                "Иван",
		Bio:                        "Опытный банщик",
		Phone:                      "+79990000000",
		City:                       "Москва",
		WorkFormat:                 domain.WorkFormatBoth,
		TravelRadiusKm:             30,
		TravelBaseLatitude:         &lat,
		TravelBaseLongitude:        &lon,
		ExperienceYears:            10,
		Specializations:            []string{"парение"},
		HourlyRate:                 500000,
		AvailabilityJSON:           `{"mon":true}`,
		PayoutLegalForm:            domain.PayoutLegalFormIP,
		PayoutLegalName:            "ИП Иванов",
		PayoutBankName:             "Тинькофф",
		PayoutBIK:                  "044525974",
		PayoutSettlementAccount:    "40802810500000000001",
		PayoutCorrespondentAccount: "30101810145250000974",
		PayoutINN:                  "500100732259",
		PayoutOGRNIP:               "304500116000157",
		PayoutVerificationStatus:   domain.PayoutVerificationVerified,
		Status:                     domain.StatusActive,
		ModerationComment:          "ok",
		ModeratedBy:                &moderatedBy,
		ModeratedAt:                &moderatedAt,
		Services: []domain.MasterService{
			{ID: uuid.New(), Name: "Парение", Description: "veniki", DurationMin: 60, Price: 300000, SortOrder: 0},
		},
		Photos: []domain.MasterPhoto{
			{ID: uuid.New(), URL: "/p1.jpg", SortOrder: 0, IsCover: true},
		},
		Credentials: []domain.MasterCredential{
			{ID: uuid.New(), Kind: domain.CredentialKindCertificate, Title: "Сертификат", Issuer: "Школа", Year: 2020},
		},
		TravelExcludeZones: []domain.MasterTravelExcludeZone{
			{ID: "z1", Latitude: 55.0, Longitude: 37.0, RadiusKm: 5, Label: "центр"},
		},
		CreatedAt: time.Unix(1600000000, 0).UTC(),
		UpdatedAt: time.Unix(1600000001, 0).UTC(),
	}
}

func TestMasterToProto_NilReturnsNil(t *testing.T) {
	if masterToProto(nil) != nil {
		t.Error("masterToProto(nil) must return nil")
	}
	if masterToProtoPublic(nil) != nil {
		t.Error("masterToProtoPublic(nil) must return nil")
	}
	if masterToProtoModerator(nil) != nil {
		t.Error("masterToProtoModerator(nil) must return nil")
	}
}

func TestMasterToProto_FullMapping(t *testing.T) {
	m := fullMaster()
	p := masterToProto(m)

	if p.GetId() != m.ID.String() || p.GetUserId() != m.UserID.String() {
		t.Error("id/user_id not mapped")
	}
	if p.GetSlug() != "ivan-master" || p.GetDisplayName() != "Иван" {
		t.Error("slug/display_name not mapped")
	}
	if len(p.GetServices()) != 1 || p.GetServices()[0].GetName() != "Парение" {
		t.Error("services not mapped")
	}
	if len(p.GetPhotos()) != 1 || !p.GetPhotos()[0].GetIsCover() {
		t.Error("photos not mapped")
	}
	if len(p.GetCredentials()) != 1 || p.GetCredentials()[0].GetTitle() != "Сертификат" {
		t.Error("credentials not mapped")
	}
	if len(p.GetTravelExcludeZones()) != 1 || p.GetTravelExcludeZones()[0].GetId() != "z1" {
		t.Error("travel exclude zones not mapped")
	}
	if p.TravelBaseLatitude == nil || *p.TravelBaseLatitude != 55.75 {
		t.Error("travel base latitude not mapped")
	}
	if p.GetModeratedBy() != m.ModeratedBy.String() {
		t.Error("moderated_by not mapped")
	}
	if p.ModeratedAt == nil {
		t.Error("moderated_at not mapped")
	}
	// A fully valid ИП payout profile must surface payout_ready = true.
	if !p.GetPayoutReady() {
		t.Error("payout_ready should be true for a complete ИП profile")
	}
	if p.GetPayoutInn() != "500100732259" {
		t.Error("private profile must expose payout INN to the owner")
	}
}

func TestMasterToProtoPublic_StripsPayoutData(t *testing.T) {
	p := masterToProtoPublic(fullMaster())

	if p.GetPayoutInn() != "" || p.GetPayoutBik() != "" || p.GetPayoutLegalName() != "" ||
		p.GetPayoutBankName() != "" || p.GetPayoutSettlementAccount() != "" ||
		p.GetPayoutCorrespondentAccount() != "" || p.GetPayoutLegalForm() != "" ||
		p.GetPayoutVerificationStatus() != "" {
		t.Error("public projection must strip ALL payout fields")
	}
	if p.GetPayoutReady() {
		t.Error("public projection must not expose payout_ready")
	}
	// Non-payout public fields are preserved.
	if p.GetDisplayName() != "Иван" || p.GetCity() != "Москва" {
		t.Error("public projection dropped non-sensitive fields")
	}
}

func TestMasterToProtoModerator_KeepsFormStripsCredentials(t *testing.T) {
	p := masterToProtoModerator(fullMaster())

	// Kept: legal form + verification status + payout_ready (moderation context).
	if p.GetPayoutLegalForm() != domain.PayoutLegalFormIP {
		t.Error("moderator view must keep payout_legal_form")
	}
	if p.GetPayoutVerificationStatus() != domain.PayoutVerificationVerified {
		t.Error("moderator view must keep payout_verification_status")
	}
	if !p.GetPayoutReady() {
		t.Error("moderator view must keep payout_ready")
	}
	// Stripped: raw credentials + phone (PII under 152-ФЗ).
	if p.GetPayoutInn() != "" || p.GetPayoutBik() != "" || p.GetPayoutLegalName() != "" ||
		p.GetPayoutSettlementAccount() != "" || p.GetPayoutCorrespondentAccount() != "" {
		t.Error("moderator view must strip raw payout credentials")
	}
	if p.GetPhone() != "" {
		t.Error("moderator view must strip phone")
	}
}

func TestListPublicMasters_StripsPayoutAndReturnsTotal(t *testing.T) {
	repo := &mockRepo{
		ListPublicFunc: func(_ context.Context, p domain.ListPublicMastersParams) ([]domain.Master, int32, error) {
			// City falls back from the singular `city` field when `cities` is empty.
			if len(p.Cities) != 1 || p.Cities[0] != "Москва" {
				t.Errorf("city fallback not applied: %+v", p.Cities)
			}
			return []domain.Master{*fullMaster()}, 1, nil
		},
	}
	s := newServer(repo)

	resp, err := s.ListPublicMasters(context.Background(), &masterv1.ListPublicMastersRequest{City: "Москва"})
	if err != nil {
		t.Fatalf("ListPublicMasters error: %v", err)
	}
	if resp.GetTotal() != 1 || len(resp.GetMasters()) != 1 {
		t.Fatalf("expected 1 master / total 1, got %d / %d", len(resp.GetMasters()), resp.GetTotal())
	}
	if resp.GetMasters()[0].GetPayoutInn() != "" {
		t.Error("public list must strip payout INN")
	}
}

func TestGetPublicMaster_NotFoundWhenInactive(t *testing.T) {
	repo := &mockRepo{
		GetBySlugFunc: func(context.Context, string) (*domain.Master, error) {
			m := fullMaster()
			m.Status = domain.StatusPendingReview // not active → hidden from public
			return m, nil
		},
	}
	s := newServer(repo)

	if _, err := s.GetPublicMaster(context.Background(), &masterv1.GetPublicMasterRequest{Slug: "x"}); err == nil {
		t.Fatal("expected NotFound for a non-active master, got nil")
	}
}

func TestGetMaster_InvalidUUID(t *testing.T) {
	s := newServer(&mockRepo{})
	if _, err := s.GetMaster(context.Background(), &masterv1.GetMasterRequest{Id: "nope"}); err == nil {
		t.Fatal("expected error for malformed id, got nil")
	}
}

func TestSuspendMasterByUser(t *testing.T) {
	repo := &mockRepo{
		SuspendByUserFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
	}
	s := newServer(repo)

	resp, err := s.SuspendMasterByUser(context.Background(), &masterv1.SuspendMasterByUserRequest{
		UserId: uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("SuspendMasterByUser error: %v", err)
	}
	if !resp.GetSuspended() {
		t.Error("expected Suspended:true")
	}
}

func TestListForModeration_UsesModeratorProjection(t *testing.T) {
	repo := &mockRepo{
		ListByStatusFunc: func(_ context.Context, status string, _, _ int32) ([]domain.Master, int32, error) {
			if status != domain.StatusPendingReview {
				t.Errorf("status filter = %q, want pending_review", status)
			}
			return []domain.Master{*fullMaster()}, 1, nil
		},
	}
	s := newServer(repo)

	resp, err := s.ListForModeration(context.Background(), &masterv1.ListForModerationRequest{
		StatusFilter: domain.StatusPendingReview,
	})
	if err != nil {
		t.Fatalf("ListForModeration error: %v", err)
	}
	m := resp.GetMasters()[0]
	if m.GetPayoutLegalForm() != domain.PayoutLegalFormIP {
		t.Error("moderator list must keep legal form")
	}
	if m.GetPayoutInn() != "" || m.GetPhone() != "" {
		t.Error("moderator list must strip raw credentials and phone")
	}
}

func TestModerateMaster_MissingCaller(t *testing.T) {
	s := newServer(&mockRepo{})
	_, err := s.ModerateMaster(context.Background(), &masterv1.ModerateMasterRequest{
		MasterId: uuid.NewString(),
		Action:   "approve",
	})
	if err == nil {
		t.Fatal("expected Unauthenticated when caller identity is absent, got nil")
	}
}

func TestModerateMaster_ApproveHappyPath(t *testing.T) {
	masterID := uuid.New()
	var moderated bool
	repo := &mockRepo{
		GetByIDFunc: func(_ context.Context, id uuid.UUID) (*domain.Master, error) {
			m := fullMaster() // valid ИП payout → approvable
			m.ID = id
			if moderated {
				m.Status = domain.StatusActive
			} else {
				m.Status = domain.StatusPendingReview
			}
			return m, nil
		},
		ModerateAtomicFunc: func(_ context.Context, _ uuid.UUID, oldS, newS, _ string, _ *uuid.UUID) error {
			if oldS != domain.StatusPendingReview || newS != domain.StatusActive {
				t.Errorf("transition %s→%s, want pending_review→active", oldS, newS)
			}
			moderated = true
			return nil
		},
	}
	s := newServer(repo)

	resp, err := s.ModerateMaster(ctxWithCaller(uuid.NewString()), &masterv1.ModerateMasterRequest{
		MasterId: masterID.String(),
		Action:   "approve",
	})
	if err != nil {
		t.Fatalf("ModerateMaster error: %v", err)
	}
	if resp.GetMaster().GetStatus() != domain.StatusActive {
		t.Errorf("status = %q, want active", resp.GetMaster().GetStatus())
	}
	if !moderated {
		t.Error("ModerateAtomic was not called")
	}
}

func TestBookingToProto(t *testing.T) {
	if bookingToProto(nil) != nil {
		t.Error("bookingToProto(nil) must return nil")
	}

	svcID := uuid.New()
	b := &domain.MasterBooking{
		ID:              uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		MasterID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		ClientUserID:    uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		MasterServiceID: &svcID,
		Date:            "2026-07-01",
		TimeFrom:        "10:00",
		TimeTo:          "11:00",
		Comment:         "перед баней",
		Status:          domain.BookingStatusConfirmed,
		PaymentID:       "pay-1",
		PaymentURL:      "https://pay",
		TotalPrice:      300000,
		CreatedAt:       time.Unix(1700000000, 0).UTC(),
	}
	p := bookingToProto(b)

	if p.GetId() != b.ID.String() || p.GetMasterId() != b.MasterID.String() {
		t.Error("booking ids not mapped")
	}
	if p.GetMasterServiceId() != svcID.String() {
		t.Error("master_service_id pointer not mapped")
	}
	if p.GetStatus() != domain.BookingStatusConfirmed || p.GetTotalPrice() != 300000 {
		t.Error("status/total_price not mapped")
	}

	// Nil service id → optional field left unset.
	b.MasterServiceID = nil
	if bookingToProto(b).MasterServiceId != nil {
		t.Error("nil MasterServiceID must leave proto field unset")
	}
}
