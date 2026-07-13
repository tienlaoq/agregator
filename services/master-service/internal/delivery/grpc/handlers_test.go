package grpc

// End-to-end handler tests that drive the gRPC Server through a real
// MasterUseCase backed by an in-memory fake repository. These exercise the
// caller-identity plumbing, argument parsing, usecase orchestration and the
// proto-marshalling helpers (masterToProto*, bookingToProto, clientToProto)
// in a single pass — no DB or network required.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

// fakeRepo is a full in-memory implementation of domain.MasterRepository.
// Unlike the per-func mockRepo it backs every method with maps, so handlers can
// be driven through the real usecase without stubbing each call site.
type fakeRepo struct {
	masters    map[uuid.UUID]*domain.Master
	bookings   map[uuid.UUID]*domain.MasterBooking
	slotBlocks map[uuid.UUID]*domain.MasterSlotBlock
	history    []domain.ModerationHistoryEntry

	hasCompleted bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		masters:  make(map[uuid.UUID]*domain.Master),
		bookings: make(map[uuid.UUID]*domain.MasterBooking),
	}
}

func (r *fakeRepo) add(m *domain.Master)               { r.masters[m.ID] = m }
func (r *fakeRepo) addBooking(b *domain.MasterBooking) { r.bookings[b.ID] = b }

func (r *fakeRepo) Insert(_ context.Context, m *domain.Master) error {
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	cp := *m
	r.masters[m.ID] = &cp
	return nil
}
func (r *fakeRepo) GetByUserID(_ context.Context, userID uuid.UUID) (*domain.Master, error) {
	for _, m := range r.masters {
		if m.UserID == userID {
			cp := *m
			return &cp, nil
		}
	}
	return nil, nil
}
func (r *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Master, error) {
	if m, ok := r.masters[id]; ok {
		cp := *m
		return &cp, nil
	}
	return nil, nil
}
func (r *fakeRepo) GetBySlug(_ context.Context, slug string) (*domain.Master, error) {
	for _, m := range r.masters {
		if m.Slug == slug {
			cp := *m
			return &cp, nil
		}
	}
	return nil, nil
}
func (r *fakeRepo) UpdateProfile(_ context.Context, m *domain.Master) error {
	cp := *m
	r.masters[m.ID] = &cp
	return nil
}
func (r *fakeRepo) UpdateProfileWithAssociations(ctx context.Context, m *domain.Master, applyServices bool, services []domain.MasterServiceUpsert, applyCredentials bool, credentials []domain.MasterCredentialUpsert) error {
	if err := r.UpdateProfile(ctx, m); err != nil {
		return err
	}
	if applyServices {
		if _, err := r.ReplaceServices(ctx, m.ID, services); err != nil {
			return err
		}
	}
	if applyCredentials {
		if _, err := r.ReplaceCredentials(ctx, m.ID, credentials); err != nil {
			return err
		}
	}
	return nil
}
func (r *fakeRepo) UpdateStatus(_ context.Context, masterID uuid.UUID, st, comment string, by *uuid.UUID) error {
	m, ok := r.masters[masterID]
	if !ok {
		return domain.ErrNotFound
	}
	m.Status = st
	m.ModerationComment = comment
	m.ModeratedBy = by
	return nil
}
func (r *fakeRepo) SuspendByUser(_ context.Context, userID uuid.UUID) (bool, error) {
	for _, m := range r.masters {
		if m.UserID == userID && m.Status != domain.StatusSuspended {
			m.Status = domain.StatusSuspended
			return true, nil
		}
	}
	return false, nil
}
func (r *fakeRepo) SubmitForReviewAtomic(_ context.Context, masterID, _ uuid.UUID) error {
	m, ok := r.masters[masterID]
	if !ok {
		return domain.ErrSubmitNotAllowed
	}
	switch m.Status {
	case domain.StatusDraft, domain.StatusNeedsRevision, domain.StatusRejected:
		m.Status = domain.StatusPendingReview
		return nil
	}
	return domain.ErrSubmitNotAllowed
}
func (r *fakeRepo) ListByStatus(_ context.Context, statusFilter string, limit, offset int32) ([]domain.Master, int32, error) {
	var all []domain.Master
	for _, m := range r.masters {
		if statusFilter == "" || m.Status == statusFilter {
			all = append(all, *m)
		}
	}
	total := int32(len(all))
	if offset >= total {
		return nil, total, nil
	}
	all = all[offset:]
	if limit > 0 && int32(len(all)) > limit {
		all = all[:limit]
	}
	return all, total, nil
}
func (r *fakeRepo) ListPublic(_ context.Context, _ domain.ListPublicMastersParams) ([]domain.Master, int32, error) {
	var out []domain.Master
	for _, m := range r.masters {
		if m.Status == domain.StatusActive {
			out = append(out, *m)
		}
	}
	return out, int32(len(out)), nil
}
func (r *fakeRepo) ReplaceServices(_ context.Context, masterID uuid.UUID, items []domain.MasterServiceUpsert) ([]domain.MasterService, error) {
	out := make([]domain.MasterService, 0, len(items))
	for i, it := range items {
		out = append(out, domain.MasterService{ID: uuid.New(), MasterID: masterID, Name: it.Name, Price: it.Price, DurationMin: it.DurationMin, SortOrder: int32(i)})
	}
	if m, ok := r.masters[masterID]; ok {
		m.Services = out
	}
	return out, nil
}
func (r *fakeRepo) ReplaceCredentials(_ context.Context, masterID uuid.UUID, items []domain.MasterCredentialUpsert) ([]domain.MasterCredential, error) {
	out := make([]domain.MasterCredential, 0, len(items))
	for i, it := range items {
		out = append(out, domain.MasterCredential{ID: uuid.New(), MasterID: masterID, Kind: it.Kind, Title: it.Title, SortOrder: int32(i)})
	}
	if m, ok := r.masters[masterID]; ok {
		m.Credentials = out
	}
	return out, nil
}
func (r *fakeRepo) InsertModerationHistory(_ context.Context, e *domain.ModerationHistoryEntry) error {
	r.history = append(r.history, *e)
	return nil
}
func (r *fakeRepo) UpdateStatusWithHistory(_ context.Context, masterID uuid.UUID, st, comment string, by *uuid.UUID, e *domain.ModerationHistoryEntry) error {
	if e != nil {
		r.history = append(r.history, *e)
	}
	return r.UpdateStatus(context.Background(), masterID, st, comment, by)
}
func (r *fakeRepo) ModerateAtomic(_ context.Context, masterID uuid.UUID, oldS, newS, comment string, by *uuid.UUID) error {
	m, ok := r.masters[masterID]
	if !ok {
		return domain.ErrNotFound
	}
	if m.Status != oldS {
		return domain.ErrModerationConflict
	}
	m.Status = newS
	m.ModerationComment = comment
	m.ModeratedBy = by
	return nil
}
func (r *fakeRepo) ListModerationHistory(_ context.Context, masterID uuid.UUID, limit int32) ([]domain.ModerationHistoryEntry, error) {
	var out []domain.ModerationHistoryEntry
	for _, h := range r.history {
		if h.MasterID == masterID {
			out = append(out, h)
		}
	}
	if limit > 0 && int32(len(out)) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (r *fakeRepo) InsertBooking(_ context.Context, b *domain.MasterBooking) error {
	cp := *b
	r.bookings[b.ID] = &cp
	return nil
}
func (r *fakeRepo) GetBookingByID(_ context.Context, id uuid.UUID) (*domain.MasterBooking, error) {
	if b, ok := r.bookings[id]; ok {
		cp := *b
		return &cp, nil
	}
	return nil, nil
}
func (r *fakeRepo) GetBookingsByIDs(_ context.Context, ids []uuid.UUID) ([]domain.MasterBooking, error) {
	var out []domain.MasterBooking
	for _, id := range ids {
		if b, ok := r.bookings[id]; ok {
			out = append(out, *b)
		}
	}
	return out, nil
}
func (r *fakeRepo) GetMasterUserIDsByIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	out := make(map[uuid.UUID]uuid.UUID, len(ids))
	for _, id := range ids {
		if m, ok := r.masters[id]; ok {
			out[id] = m.UserID
		}
	}
	return out, nil
}
func (r *fakeRepo) SetBookingPayment(_ context.Context, bookingID uuid.UUID, paymentID, paymentURL string, total int64, st string) error {
	if b, ok := r.bookings[bookingID]; ok {
		b.PaymentID = paymentID
		b.PaymentURL = paymentURL
		b.TotalPrice = total
		b.Status = st
	}
	return nil
}
func (r *fakeRepo) DeleteBooking(_ context.Context, id uuid.UUID) error {
	delete(r.bookings, id)
	return nil
}
func (r *fakeRepo) ConfirmBookingByPayment(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (r *fakeRepo) CancelBookingByPayment(_ context.Context, _ uuid.UUID, _ string) error { return nil }
func (r *fakeRepo) ListBookingsByMaster(_ context.Context, masterID uuid.UUID, statusFilter string) ([]domain.MasterBooking, error) {
	var out []domain.MasterBooking
	for _, b := range r.bookings {
		if b.MasterID == masterID && (statusFilter == "" || b.Status == statusFilter) {
			out = append(out, *b)
		}
	}
	return out, nil
}
func (r *fakeRepo) ListClientsByMaster(_ context.Context, masterID uuid.UUID) ([]domain.MasterClient, error) {
	agg := make(map[uuid.UUID]*domain.MasterClient)
	for _, b := range r.bookings {
		if b.MasterID != masterID {
			continue
		}
		c := agg[b.ClientUserID]
		if c == nil {
			c = &domain.MasterClient{UserID: b.ClientUserID}
			agg[b.ClientUserID] = c
		}
		c.BookingsCount++
	}
	out := make([]domain.MasterClient, 0, len(agg))
	for _, c := range agg {
		out = append(out, *c)
	}
	return out, nil
}
func (r *fakeRepo) ListBookingsByClient(_ context.Context, clientUserID uuid.UUID, statusFilter string) ([]domain.MasterBooking, error) {
	var out []domain.MasterBooking
	for _, b := range r.bookings {
		if b.ClientUserID == clientUserID && (statusFilter == "" || b.Status == statusFilter) {
			out = append(out, *b)
		}
	}
	return out, nil
}
func (r *fakeRepo) InsertSlotBlock(_ context.Context, b *domain.MasterSlotBlock) error {
	if r.slotBlocks == nil {
		r.slotBlocks = make(map[uuid.UUID]*domain.MasterSlotBlock)
	}
	cp := *b
	r.slotBlocks[b.ID] = &cp
	return nil
}
func (r *fakeRepo) ListSlotBlocksByMaster(_ context.Context, masterID uuid.UUID) ([]domain.MasterSlotBlock, error) {
	var out []domain.MasterSlotBlock
	for _, b := range r.slotBlocks {
		if b.MasterID == masterID {
			out = append(out, *b)
		}
	}
	return out, nil
}
func (r *fakeRepo) DeleteSlotBlock(_ context.Context, masterID, blockID uuid.UUID) error {
	if b, ok := r.slotBlocks[blockID]; !ok || b.MasterID != masterID {
		return domain.ErrNotFound
	}
	delete(r.slotBlocks, blockID)
	return nil
}
func (r *fakeRepo) HasSlotBlockConflict(_ context.Context, _ uuid.UUID, _, _, _ string) (bool, error) {
	return false, nil
}
func (r *fakeRepo) UpdateBookingStatus(_ context.Context, id uuid.UUID, st string) error {
	if b, ok := r.bookings[id]; ok {
		b.Status = st
	}
	return nil
}
func (r *fakeRepo) HasCompletedBookingByClientMaster(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return r.hasCompleted, nil
}
func (r *fakeRepo) NewSlug(_ context.Context, _ string, _ uuid.UUID) (string, error) {
	return "slug-" + uuid.NewString()[:8], nil
}
func (r *fakeRepo) CountPhotosByMaster(_ context.Context, masterID uuid.UUID) (int32, error) {
	if m, ok := r.masters[masterID]; ok {
		return int32(len(m.Photos)), nil
	}
	return 0, nil
}
func (r *fakeRepo) AddMasterPhoto(_ context.Context, masterID uuid.UUID, url string) (*domain.MasterPhoto, error) {
	m, ok := r.masters[masterID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	p := domain.MasterPhoto{ID: uuid.New(), MasterID: masterID, URL: url, SortOrder: int32(len(m.Photos)), IsCover: len(m.Photos) == 0}
	m.Photos = append(m.Photos, p)
	return &p, nil
}
func (r *fakeRepo) DeleteMasterPhoto(_ context.Context, masterID, photoID uuid.UUID) (string, error) {
	m, ok := r.masters[masterID]
	if !ok {
		return "", domain.ErrNotFound
	}
	for i, p := range m.Photos {
		if p.ID == photoID {
			m.Photos = append(m.Photos[:i], m.Photos[i+1:]...)
			return p.URL, nil
		}
	}
	return "", domain.ErrNotFound
}
func (r *fakeRepo) AddMasterVideo(_ context.Context, masterID uuid.UUID, url string) (*domain.MasterVideo, error) {
	m, ok := r.masters[masterID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	v := domain.MasterVideo{ID: uuid.New(), MasterID: masterID, URL: url, SortOrder: int32(len(m.Videos))}
	m.Videos = append(m.Videos, v)
	return &v, nil
}
func (r *fakeRepo) DeleteMasterVideo(_ context.Context, masterID, videoID uuid.UUID) (string, error) {
	m, ok := r.masters[masterID]
	if !ok {
		return "", domain.ErrNotFound
	}
	for i, v := range m.Videos {
		if v.ID == videoID {
			m.Videos = append(m.Videos[:i], m.Videos[i+1:]...)
			return v.URL, nil
		}
	}
	return "", domain.ErrNotFound
}
func (r *fakeRepo) SetMasterCoverPhoto(_ context.Context, masterID, photoID uuid.UUID) error {
	m, ok := r.masters[masterID]
	if !ok {
		return domain.ErrNotFound
	}
	found := false
	for i := range m.Photos {
		if m.Photos[i].ID == photoID {
			found = true
		}
	}
	if !found {
		return domain.ErrNotFound
	}
	for i := range m.Photos {
		m.Photos[i].IsCover = m.Photos[i].ID == photoID
	}
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

const validINN12 = "500100732259"

func activeMaster() *domain.Master {
	return &domain.Master{
		ID:                         uuid.New(),
		UserID:                     uuid.New(),
		Slug:                       "master-" + uuid.NewString()[:8],
		Status:                     domain.StatusActive,
		DisplayName:                "Иван Банщик",
		City:                       "москва",
		Phone:                      "79991234567",
		Bio:                        "Опытный банщик с большим стажем",
		HourlyRate:                 10_000,
		Services:                   []domain.MasterService{{ID: uuid.New(), Price: 5000}},
		PayoutLegalForm:            domain.PayoutLegalFormSelfEmployed,
		PayoutLegalName:            "Тест Тестов",
		PayoutBankName:             "Т-Банк",
		PayoutBIK:                  "044525974",
		PayoutINN:                  validINN12,
		PayoutSettlementAccount:    "40702810123456789012",
		PayoutCorrespondentAccount: "30101810400000000974",
		PayoutVerificationStatus:   domain.PayoutVerificationVerified,
	}
}

// ── profile handlers ─────────────────────────────────────────────────────────

func TestCreateMyProfileHandler(t *testing.T) {
	repo := newFakeRepo()
	srv := newServer(repo)
	uid := uuid.New()

	t.Run("creates profile", func(t *testing.T) {
		resp, err := srv.CreateMyProfile(ctxWithCaller(uid.String()), &masterv1.CreateMyProfileRequest{DisplayName: "Пётр"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetMaster().GetDisplayName() != "Пётр" {
			t.Fatalf("display name = %q", resp.GetMaster().GetDisplayName())
		}
	})

	t.Run("missing caller is unauthenticated", func(t *testing.T) {
		_, err := srv.CreateMyProfile(context.Background(), &masterv1.CreateMyProfileRequest{DisplayName: "X"})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("got code %v, want Unauthenticated", status.Code(err))
		}
	})

	t.Run("empty display name rejected", func(t *testing.T) {
		_, err := srv.CreateMyProfile(ctxWithCaller(uuid.NewString()), &masterv1.CreateMyProfileRequest{DisplayName: "  "})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})
}

func TestGetMyProfileHandler(t *testing.T) {
	repo := newFakeRepo()
	m := activeMaster()
	repo.add(m)
	srv := newServer(repo)

	t.Run("found", func(t *testing.T) {
		resp, err := srv.GetMyProfile(ctxWithCaller(m.UserID.String()), &masterv1.GetMyProfileRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetMaster().GetId() != m.ID.String() {
			t.Fatalf("id = %q", resp.GetMaster().GetId())
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := srv.GetMyProfile(ctxWithCaller(uuid.NewString()), &masterv1.GetMyProfileRequest{})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})
}

func TestUpdateMyProfileHandler(t *testing.T) {
	repo := newFakeRepo()
	m := activeMaster()
	m.Status = domain.StatusDraft
	repo.add(m)
	srv := newServer(repo)

	bio := "Совершенно новое описание профиля мастера"
	resp, err := srv.UpdateMyProfile(ctxWithCaller(m.UserID.String()), &masterv1.UpdateMyProfileRequest{Bio: &bio})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetMaster().GetBio() != bio {
		t.Fatalf("bio = %q", resp.GetMaster().GetBio())
	}

	t.Run("missing caller", func(t *testing.T) {
		_, err := srv.UpdateMyProfile(context.Background(), &masterv1.UpdateMyProfileRequest{Bio: &bio})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("got code %v, want Unauthenticated", status.Code(err))
		}
	})

	// Exercise the full proto→domain field-mapping in one request so every
	// `if req.X != nil` branch of the handler is covered.
	t.Run("maps all optional fields", func(t *testing.T) {
		repo := newFakeRepo()
		m := activeMaster()
		m.Status = domain.StatusDraft
		repo.add(m)
		srv := newServer(repo)

		s := func(v string) *string { return &v }
		i32 := func(v int32) *int32 { return &v }
		i64 := func(v int64) *int64 { return &v }

		req := &masterv1.UpdateMyProfileRequest{
			DisplayName:                s("Пётр Мастер"),
			Bio:                        s("Подробное описание профиля мастера банщика"),
			Phone:                      s("+7 (999) 111-22-33"),
			City:                       s("Казань"),
			WorkFormat:                 s(domain.WorkFormatMobile),
			TravelRadiusKm:             i32(30),
			TravelBaseLatitude:         float64ptrH(55.79),
			TravelBaseLongitude:        float64ptrH(49.12),
			ExperienceYears:            i32(5),
			HourlyRate:                 i64(12000),
			AvailabilityJson:           s(`{"mon":["10:00-18:00"]}`),
			ApplySpecializations:       true,
			Specializations:            []string{"парение", "массаж"},
			PayoutLegalForm:            s(domain.PayoutLegalFormSelfEmployed),
			PayoutLegalName:            s("Пётр Мастеров"),
			PayoutInn:                  s(validINN12),
			PayoutBankName:             s("Т-Банк"),
			PayoutBik:                  s("044525974"),
			PayoutSettlementAccount:    s("40702810123456789012"),
			PayoutCorrespondentAccount: s("30101810400000000974"),
			PayoutVerificationStatus:   s(domain.PayoutVerificationVerified),
			ApplyServicesReplace:       true,
			ServicesReplace: []*masterv1.MasterServiceItemInput{
				{Name: "Парение", DurationMin: 60, Price: 5000},
			},
			ApplyCredentials: true,
			CredentialsReplace: []*masterv1.MasterCredentialItemInput{
				{Kind: domain.CredentialKindCertificate, Title: "Сертификат банщика"},
			},
			ApplyTravelExcludeZones: true,
			TravelExcludeZones: []*masterv1.MasterTravelExcludeZone{
				{Id: "z1", Latitude: 55.79, Longitude: 49.12, RadiusKm: 1},
			},
		}
		resp, err := srv.UpdateMyProfile(ctxWithCaller(m.UserID.String()), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := resp.GetMaster()
		if got.GetCity() != "казань" {
			t.Fatalf("city = %q, want lowercased", got.GetCity())
		}
		if got.GetWorkFormat() != domain.WorkFormatMobile {
			t.Fatalf("work_format = %q", got.GetWorkFormat())
		}
		if len(got.GetServices()) != 1 {
			t.Fatalf("got %d services, want 1", len(got.GetServices()))
		}
	})

	t.Run("invalid service id rejected", func(t *testing.T) {
		repo := newFakeRepo()
		m := activeMaster()
		repo.add(m)
		srv := newServer(repo)
		badID := "not-a-uuid"
		_, err := srv.UpdateMyProfile(ctxWithCaller(m.UserID.String()), &masterv1.UpdateMyProfileRequest{
			ApplyServicesReplace: true,
			ServicesReplace:      []*masterv1.MasterServiceItemInput{{Id: &badID, Name: "X"}},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})
}

func float64ptrH(v float64) *float64 { return &v }

func TestSubmitForReviewHandler(t *testing.T) {
	repo := newFakeRepo()
	m := activeMaster()
	m.Status = domain.StatusDraft
	repo.add(m)
	srv := newServer(repo)

	resp, err := srv.SubmitForReview(ctxWithCaller(m.UserID.String()), &masterv1.SubmitMasterForReviewRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetMaster().GetStatus() != domain.StatusPendingReview {
		t.Fatalf("status = %q, want pending_review", resp.GetMaster().GetStatus())
	}
}

// ── master lookup / moderation ───────────────────────────────────────────────

func TestGetMasterHandler(t *testing.T) {
	repo := newFakeRepo()
	m := activeMaster()
	repo.add(m)
	srv := newServer(repo)

	t.Run("found", func(t *testing.T) {
		resp, err := srv.GetMaster(context.Background(), &masterv1.GetMasterRequest{Id: m.ID.String()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetMaster().GetUserId() != m.UserID.String() {
			t.Fatalf("user id = %q", resp.GetMaster().GetUserId())
		}
	})

	t.Run("malformed id", func(t *testing.T) {
		_, err := srv.GetMaster(context.Background(), &masterv1.GetMasterRequest{Id: "not-a-uuid"})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := srv.GetMaster(context.Background(), &masterv1.GetMasterRequest{Id: uuid.NewString()})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})
}

func TestListModerationHistoryHandler(t *testing.T) {
	repo := newFakeRepo()
	m := activeMaster()
	repo.add(m)
	repo.history = []domain.ModerationHistoryEntry{
		{ID: uuid.New(), MasterID: m.ID, OldStatus: domain.StatusPendingReview, NewStatus: domain.StatusActive, ChangedBy: uuid.New(), CreatedAt: time.Now()},
	}
	srv := newServer(repo)

	resp, err := srv.ListModerationHistory(context.Background(), &masterv1.ListModerationHistoryRequest{MasterId: m.ID.String(), Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetEntries()) != 1 {
		t.Fatalf("got %d entries, want 1", len(resp.GetEntries()))
	}

	t.Run("malformed master id", func(t *testing.T) {
		_, err := srv.ListModerationHistory(context.Background(), &masterv1.ListModerationHistoryRequest{MasterId: "x"})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})
}

// ── booking handlers ─────────────────────────────────────────────────────────

func TestListMyMasterBookingsHandler(t *testing.T) {
	repo := newFakeRepo()
	m := activeMaster()
	repo.add(m)
	repo.addBooking(&domain.MasterBooking{ID: uuid.New(), MasterID: m.ID, ClientUserID: uuid.New(), Status: domain.BookingStatusConfirmed, PaymentURL: "https://pay/secret"})
	srv := newServer(repo)

	resp, err := srv.ListMyMasterBookings(ctxWithCaller(m.UserID.String()), &masterv1.ListMyMasterBookingsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetBookings()) != 1 {
		t.Fatalf("got %d bookings, want 1", len(resp.GetBookings()))
	}
	// Master owner must not receive the client payment URL.
	if resp.GetBookings()[0].GetPaymentUrl() != "" {
		t.Fatal("payment url must be stripped for master owner")
	}
}

func TestListClientMasterBookingsHandler(t *testing.T) {
	repo := newFakeRepo()
	client := uuid.New()
	repo.addBooking(&domain.MasterBooking{ID: uuid.New(), MasterID: uuid.New(), ClientUserID: client, Status: domain.BookingStatusConfirmed})
	srv := newServer(repo)

	resp, err := srv.ListClientMasterBookings(ctxWithCaller(client.String()), &masterv1.ListClientMasterBookingsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetBookings()) != 1 {
		t.Fatalf("got %d bookings, want 1", len(resp.GetBookings()))
	}
}

func TestListMyMasterClientsHandler(t *testing.T) {
	repo := newFakeRepo()
	m := activeMaster()
	repo.add(m)
	repo.addBooking(&domain.MasterBooking{ID: uuid.New(), MasterID: m.ID, ClientUserID: uuid.New(), Status: domain.BookingStatusConfirmed})
	srv := newServer(repo)

	resp, err := srv.ListMyMasterClients(ctxWithCaller(m.UserID.String()), &masterv1.ListMyMasterClientsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetClients()) != 1 {
		t.Fatalf("got %d clients, want 1", len(resp.GetClients()))
	}
}

func TestGetMasterBookingHandler(t *testing.T) {
	repo := newFakeRepo()
	m := activeMaster()
	repo.add(m)
	client := uuid.New()
	b := &domain.MasterBooking{ID: uuid.New(), MasterID: m.ID, ClientUserID: client, Status: domain.BookingStatusConfirmed, PaymentURL: "https://pay/secret"}
	repo.addBooking(b)
	srv := newServer(repo)

	t.Run("client sees payment url and master_user_id", func(t *testing.T) {
		resp, err := srv.GetMasterBooking(ctxWithCaller(client.String()), &masterv1.GetMasterBookingRequest{BookingId: b.ID.String()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetBooking().GetPaymentUrl() != "https://pay/secret" {
			t.Fatal("client should keep the payment url")
		}
		if resp.GetBooking().GetMasterUserId() != m.UserID.String() {
			t.Fatalf("master_user_id = %q", resp.GetBooking().GetMasterUserId())
		}
	})

	t.Run("master owner has payment url stripped", func(t *testing.T) {
		resp, err := srv.GetMasterBooking(ctxWithCaller(m.UserID.String()), &masterv1.GetMasterBookingRequest{BookingId: b.ID.String()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetBooking().GetPaymentUrl() != "" {
			t.Fatal("master owner payment url must be stripped")
		}
	})

	t.Run("stranger denied", func(t *testing.T) {
		_, err := srv.GetMasterBooking(ctxWithCaller(uuid.NewString()), &masterv1.GetMasterBookingRequest{BookingId: b.ID.String()})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("got code %v, want PermissionDenied", status.Code(err))
		}
	})

	t.Run("malformed booking id", func(t *testing.T) {
		_, err := srv.GetMasterBooking(ctxWithCaller(client.String()), &masterv1.GetMasterBookingRequest{BookingId: "x"})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})
}

func TestGetMasterBookingsBatchHandler(t *testing.T) {
	repo := newFakeRepo()
	m := activeMaster()
	repo.add(m)
	client := uuid.New()
	b1 := &domain.MasterBooking{ID: uuid.New(), MasterID: m.ID, ClientUserID: client, Status: domain.BookingStatusConfirmed}
	repo.addBooking(b1)
	srv := newServer(repo)

	t.Run("empty ids returns empty map", func(t *testing.T) {
		resp, err := srv.GetMasterBookingsBatch(ctxWithCaller(client.String()), &masterv1.GetMasterBookingsBatchRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.GetBookings()) != 0 {
			t.Fatalf("expected empty map, got %d", len(resp.GetBookings()))
		}
	})

	t.Run("returns accessible bookings, skips malformed ids", func(t *testing.T) {
		resp, err := srv.GetMasterBookingsBatch(ctxWithCaller(client.String()), &masterv1.GetMasterBookingsBatchRequest{
			BookingIds: []string{b1.ID.String(), "not-a-uuid"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.GetBookings()) != 1 {
			t.Fatalf("got %d bookings, want 1", len(resp.GetBookings()))
		}
		if resp.GetBookings()[b1.ID.String()].GetMasterUserId() != m.UserID.String() {
			t.Fatal("master_user_id should be populated")
		}
	})
}

func TestHasCompletedMasterBookingHandler(t *testing.T) {
	repo := newFakeRepo()
	repo.hasCompleted = true
	srv := newServer(repo)
	client := uuid.New()
	master := uuid.New()

	t.Run("self query allowed", func(t *testing.T) {
		resp, err := srv.HasCompletedMasterBooking(ctxWithCaller(client.String()), &masterv1.HasCompletedMasterBookingRequest{
			ClientUserId: client.String(), MasterId: master.String(),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !resp.GetHasCompleted() {
			t.Fatal("expected has_completed=true")
		}
	})

	t.Run("cross-user query denied", func(t *testing.T) {
		_, err := srv.HasCompletedMasterBooking(ctxWithCaller(uuid.NewString()), &masterv1.HasCompletedMasterBookingRequest{
			ClientUserId: client.String(), MasterId: master.String(),
		})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("got code %v, want PermissionDenied", status.Code(err))
		}
	})

	t.Run("malformed client id", func(t *testing.T) {
		_, err := srv.HasCompletedMasterBooking(ctxWithCaller(client.String()), &masterv1.HasCompletedMasterBookingRequest{
			ClientUserId: "x", MasterId: master.String(),
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})
}

func TestCreateMasterBookingHandler_InvalidServiceID(t *testing.T) {
	repo := newFakeRepo()
	srv := newServer(repo)
	badSvc := "not-a-uuid"
	_, err := srv.CreateMasterBooking(ctxWithCaller(uuid.NewString()), &masterv1.CreateMasterBookingRequest{
		MasterServiceId: &badSvc,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
	}
}

// futureBookingDate returns a date 7 days from now in Moscow time, matching the
// clock validateBookingSlot uses so the slot is always valid.
func futureBookingDate() string {
	const moscowOffset = 3 * time.Hour
	return time.Now().UTC().Add(moscowOffset).AddDate(0, 0, 7).Format(time.DateOnly)
}

func TestCreateMasterBookingHandler_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	m := activeMaster()
	m.Slug = "ivan-booking"
	repo.add(m)
	srv := newServer(repo)

	resp, err := srv.CreateMasterBooking(ctxWithCaller(uuid.NewString()), &masterv1.CreateMasterBookingRequest{
		MasterSlug: "ivan-booking",
		Date:       futureBookingDate(),
		TimeFrom:   "10:00",
		TimeTo:     "12:00",
		Comment:    "приду вовремя",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetBooking().GetStatus() != "payment_pending" {
		t.Fatalf("status = %q, want payment_pending", resp.GetBooking().GetStatus())
	}
	// Two hours at 10_000 kopecks/hour = 20_000.
	if resp.GetBooking().GetTotalPrice() != 20_000 {
		t.Fatalf("total_price = %d, want 20000", resp.GetBooking().GetTotalPrice())
	}
}

func TestCreateMasterBookingHandler_Unavailable(t *testing.T) {
	repo := newFakeRepo()
	srv := newServer(repo)
	_, err := srv.CreateMasterBooking(ctxWithCaller(uuid.NewString()), &masterv1.CreateMasterBookingRequest{
		MasterSlug: "does-not-exist",
		Date:       futureBookingDate(),
		TimeFrom:   "10:00",
		TimeTo:     "12:00",
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("got code %v, want NotFound", status.Code(err))
	}
}

// ── photo handlers ───────────────────────────────────────────────────────────

func TestPhotoHandlers(t *testing.T) {
	repo := newFakeRepo()
	m := activeMaster()
	repo.add(m)
	srv := newServer(repo)
	ctx := ctxWithCaller(m.UserID.String())
	url := "https://cdn.example.com/masters/" + m.ID.String() + "/a.jpg"

	addResp, err := srv.AddMasterPhoto(ctx, &masterv1.AddMasterPhotoRequest{Url: url})
	if err != nil {
		t.Fatalf("AddMasterPhoto error: %v", err)
	}
	if len(addResp.GetMaster().GetPhotos()) != 1 {
		t.Fatalf("got %d photos, want 1", len(addResp.GetMaster().GetPhotos()))
	}
	photoID := addResp.GetMaster().GetPhotos()[0].GetId()

	t.Run("set cover", func(t *testing.T) {
		_, err := srv.SetMasterCoverPhoto(ctx, &masterv1.SetMasterCoverPhotoRequest{PhotoId: photoID})
		if err != nil {
			t.Fatalf("SetMasterCoverPhoto error: %v", err)
		}
	})

	t.Run("set cover malformed id", func(t *testing.T) {
		_, err := srv.SetMasterCoverPhoto(ctx, &masterv1.SetMasterCoverPhotoRequest{PhotoId: "x"})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("delete photo", func(t *testing.T) {
		resp, err := srv.DeleteMasterPhoto(ctx, &masterv1.DeleteMasterPhotoRequest{PhotoId: photoID})
		if err != nil {
			t.Fatalf("DeleteMasterPhoto error: %v", err)
		}
		if resp.GetDeletedUrl() != url {
			t.Fatalf("deleted url = %q, want %q", resp.GetDeletedUrl(), url)
		}
	})

	t.Run("delete unknown photo maps to NotFound", func(t *testing.T) {
		_, err := srv.DeleteMasterPhoto(ctx, &masterv1.DeleteMasterPhotoRequest{PhotoId: uuid.NewString()})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got code %v, want NotFound", status.Code(err))
		}
	})

	t.Run("add photo missing caller", func(t *testing.T) {
		_, err := srv.AddMasterPhoto(context.Background(), &masterv1.AddMasterPhotoRequest{Url: url})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("got code %v, want Unauthenticated", status.Code(err))
		}
	})
}

// sanity: ensure fakeRepo satisfies the full interface at compile time.
var _ domain.MasterRepository = (*fakeRepo)(nil)

// guard against unused import of errors in case all uses are removed later.
var _ = errors.New
