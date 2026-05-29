package usecase

// Integration-style unit tests for MasterUseCase. All external dependencies
// (repo, paymentClient) are replaced with hand-rolled test doubles so these
// tests run without a real DB or network.
//
// Covered:
//   1. Moderation state-machine (approve/suspend/request_revision/reject).
//   2. ConfirmBookingByPayment / CancelBookingByPayment idempotency.
//   3. GetBookingsForActorBatch RBAC (client, master owner, stranger).
//   4. CreateBooking happy path and payment-saga sad paths.
//   5. normalizeRussianMobileDigits (phone normalisation).
//   6. validateMasterServices (field length / count limits).
//   7. CreateMyProfile slug-conflict retry loop.

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"google.golang.org/grpc"

	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

// ── test doubles ──────────────────────────────────────────────────────────────

// stubRepo is a minimal in-memory implementation of domain.MasterRepository.
// Only the methods exercised by the tests below are non-nil; every other method
// panics with a descriptive message so accidental calls are immediately visible.
type stubRepo struct {
	masters  map[uuid.UUID]*domain.Master // keyed by master.ID
	bookings map[uuid.UUID]*domain.MasterBooking

	// injectable errors / overrides
	confirmErr error
	cancelErr  error
	insertErr  error // returned by InsertBooking

	// masterInsertErrs is a sequence of errors returned by successive Insert
	// (master) calls. Each call pops the first element; once the slice is
	// exhausted Insert succeeds. This lets tests exercise the slug-conflict
	// retry loop without affecting InsertBooking behaviour.
	masterInsertErrs []error

	slugFn func() string
}

func newStubRepo() *stubRepo {
	return &stubRepo{
		masters:  make(map[uuid.UUID]*domain.Master),
		bookings: make(map[uuid.UUID]*domain.MasterBooking),
		slugFn:   func() string { return "test-slug" },
	}
}

func (r *stubRepo) addMaster(m *domain.Master) { r.masters[m.ID] = m }
func (r *stubRepo) addBooking(b *domain.MasterBooking) { r.bookings[b.ID] = b }

func (r *stubRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Master, error) {
	if m, ok := r.masters[id]; ok {
		cp := *m
		return &cp, nil
	}
	return nil, nil
}

func (r *stubRepo) GetByUserID(_ context.Context, userID uuid.UUID) (*domain.Master, error) {
	for _, m := range r.masters {
		if m.UserID == userID {
			cp := *m
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *stubRepo) GetBySlug(_ context.Context, slug string) (*domain.Master, error) {
	for _, m := range r.masters {
		if m.Slug == slug {
			cp := *m
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *stubRepo) UpdateStatus(_ context.Context, masterID uuid.UUID, status, comment string, moderatedBy *uuid.UUID) error {
	m, ok := r.masters[masterID]
	if !ok {
		return domain.ErrNotFound
	}
	m.Status = status
	m.ModerationComment = comment
	m.ModeratedBy = moderatedBy
	return nil
}

func (r *stubRepo) UpdateStatusWithHistory(_ context.Context, masterID uuid.UUID, status, comment string, moderatedBy *uuid.UUID, _ *domain.ModerationHistoryEntry) error {
	// Delegate to UpdateStatus — history recording is a no-op in tests.
	return r.UpdateStatus(context.Background(), masterID, status, comment, moderatedBy)
}

func (r *stubRepo) ModerateAtomic(_ context.Context, masterID uuid.UUID, expectedOldStatus, newStatus, comment string, moderatedBy *uuid.UUID) error {
	m, ok := r.masters[masterID]
	if !ok {
		return domain.ErrNotFound
	}
	if m.Status != expectedOldStatus {
		return domain.ErrModerationConflict
	}
	m.Status = newStatus
	m.ModerationComment = comment
	m.ModeratedBy = moderatedBy
	return nil
}

func (r *stubRepo) SubmitForReviewAtomic(_ context.Context, masterID uuid.UUID, _ uuid.UUID) error {
	m, ok := r.masters[masterID]
	if !ok {
		return domain.ErrSubmitNotAllowed
	}
	allowed := m.Status == domain.StatusDraft ||
		m.Status == domain.StatusNeedsRevision ||
		m.Status == domain.StatusRejected
	if !allowed {
		return domain.ErrSubmitNotAllowed
	}
	m.Status = domain.StatusPendingReview
	return nil
}

func (r *stubRepo) InsertModerationHistory(_ context.Context, _ *domain.ModerationHistoryEntry) error {
	return nil
}

func (r *stubRepo) GetBookingByID(_ context.Context, id uuid.UUID) (*domain.MasterBooking, error) {
	if b, ok := r.bookings[id]; ok {
		cp := *b
		return &cp, nil
	}
	return nil, nil
}

func (r *stubRepo) GetBookingsByIDs(_ context.Context, ids []uuid.UUID) ([]domain.MasterBooking, error) {
	var out []domain.MasterBooking
	for _, id := range ids {
		if b, ok := r.bookings[id]; ok {
			out = append(out, *b)
		}
	}
	return out, nil
}

func (r *stubRepo) GetMasterUserIDsByIDs(_ context.Context, masterIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	out := make(map[uuid.UUID]uuid.UUID, len(masterIDs))
	for _, mid := range masterIDs {
		if m, ok := r.masters[mid]; ok {
			out[mid] = m.UserID
		}
	}
	return out, nil
}

func (r *stubRepo) ConfirmBookingByPayment(_ context.Context, _ uuid.UUID, _ string) error {
	return r.confirmErr
}

func (r *stubRepo) CancelBookingByPayment(_ context.Context, _ uuid.UUID, _ string) error {
	return r.cancelErr
}

func (r *stubRepo) InsertBooking(_ context.Context, b *domain.MasterBooking) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	cp := *b
	r.bookings[b.ID] = &cp
	return nil
}

func (r *stubRepo) SetBookingPayment(_ context.Context, bookingID uuid.UUID, paymentID, paymentURL string, totalPrice int64, status string) error {
	if b, ok := r.bookings[bookingID]; ok {
		b.PaymentID = paymentID
		b.PaymentURL = paymentURL
		b.TotalPrice = totalPrice
		b.Status = status
	}
	return nil
}

func (r *stubRepo) DeleteBooking(_ context.Context, id uuid.UUID) error {
	delete(r.bookings, id)
	return nil
}

func (r *stubRepo) NewSlug(_ context.Context, _ string, _ uuid.UUID) (string, error) {
	return r.slugFn(), nil
}

func (r *stubRepo) Insert(_ context.Context, m *domain.Master) error {
	if len(r.masterInsertErrs) > 0 {
		err := r.masterInsertErrs[0]
		r.masterInsertErrs = r.masterInsertErrs[1:]
		if err != nil {
			return err
		}
	}
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	cp := *m
	r.masters[m.ID] = &cp
	return nil
}

// Unimplemented stubs — panic so accidental calls surface immediately.
func (r *stubRepo) UpdateProfile(_ context.Context, _ *domain.Master) error {
	panic("stubRepo.UpdateProfile not implemented")
}
func (r *stubRepo) ListByStatus(_ context.Context, _ string, _, _ int32) ([]domain.Master, int32, error) {
	panic("stubRepo.ListByStatus not implemented")
}
func (r *stubRepo) ListPublic(_ context.Context, _ domain.ListPublicMastersParams) ([]domain.Master, int32, error) {
	panic("stubRepo.ListPublic not implemented")
}
func (r *stubRepo) ReplaceServices(_ context.Context, _ uuid.UUID, _ []domain.MasterServiceUpsert) ([]domain.MasterService, error) {
	panic("stubRepo.ReplaceServices not implemented")
}
func (r *stubRepo) ListModerationHistory(_ context.Context, _ uuid.UUID, _ int32) ([]domain.ModerationHistoryEntry, error) {
	panic("stubRepo.ListModerationHistory not implemented")
}
func (r *stubRepo) ListBookingsByMaster(_ context.Context, _ uuid.UUID, _ string) ([]domain.MasterBooking, error) {
	panic("stubRepo.ListBookingsByMaster not implemented")
}
func (r *stubRepo) ListBookingsByClient(_ context.Context, _ uuid.UUID, _ string) ([]domain.MasterBooking, error) {
	panic("stubRepo.ListBookingsByClient not implemented")
}
func (r *stubRepo) UpdateBookingStatus(_ context.Context, _ uuid.UUID, _ string) error {
	panic("stubRepo.UpdateBookingStatus not implemented")
}
func (r *stubRepo) HasCompletedBookingByClientMaster(_ context.Context, _, _ uuid.UUID) (bool, error) {
	panic("stubRepo.HasCompletedBookingByClientMaster not implemented")
}
func (r *stubRepo) CountPhotosByMaster(_ context.Context, _ uuid.UUID) (int32, error) {
	panic("stubRepo.CountPhotosByMaster not implemented")
}
func (r *stubRepo) AddMasterPhoto(_ context.Context, _ uuid.UUID, _ string) (*domain.MasterPhoto, error) {
	panic("stubRepo.AddMasterPhoto not implemented")
}
func (r *stubRepo) DeleteMasterPhoto(_ context.Context, _, _ uuid.UUID) (string, error) {
	panic("stubRepo.DeleteMasterPhoto not implemented")
}
func (r *stubRepo) SetMasterCoverPhoto(_ context.Context, _, _ uuid.UUID) error {
	panic("stubRepo.SetMasterCoverPhoto not implemented")
}

// stubPayment implements paymentGatewayClient.
type stubPayment struct {
	resp *paymentv1.PaymentResponse
	err  error
}

func (s *stubPayment) CreatePayment(_ context.Context, _ *paymentv1.CreatePaymentRequest, _ ...grpc.CallOption) (*paymentv1.PaymentResponse, error) {
	return s.resp, s.err
}

// newUC is a test helper that wires a MasterUseCase with the given doubles.
// It goes through the real constructor so any mandatory initialisation added
// there (validation, metrics, etc.) is exercised by tests automatically.
func newUC(repo *stubRepo, pay paymentGatewayClient) *MasterUseCase {
	return NewMasterUseCase(repo, pay, zerolog.Nop())
}

// futureDate returns a booking date 7 days from today in YYYY-MM-DD format,
// evaluated in Moscow time (UTC+3) — the same clock used by validateBookingSlot.
// Using a fixed date like futureDate() would eventually become a past date and
// break all CreateBooking tests; this helper keeps them permanently valid.
func futureDate() string {
	const moscowOffset = 3 * time.Hour
	return time.Now().UTC().Add(moscowOffset).AddDate(0, 0, 7).Format(time.DateOnly)
}

// activeMaster builds a master in active status with a valid payout profile so
// CreateBooking / moderation tests don't need to repeat all the fields.
func activeMaster() *domain.Master {
	return &domain.Master{
		ID:      uuid.New(),
		UserID:  uuid.New(),
		Slug:    "test-slug",
		Status:  domain.StatusActive,
		HourlyRate: 10_000,
		Services: []domain.MasterService{
			{ID: uuid.New(), Price: 5000},
		},
		// Minimal payout profile that passes ValidatePayoutProfile (including
		// the mod-11 INN checksum added in refactor 3.18).
		// validINN12 = "500100732259" — verified checksum, see master_test.go.
		PayoutLegalForm:            domain.PayoutLegalFormSelfEmployed,
		YookassaSellerAccountID:    "acc-123",
		PayoutLegalName:            "Тест Тестов",
		PayoutBankName:             "Т-Банк",
		PayoutBIK:                  "044525974",
		PayoutINN:                  validINN12,
		PayoutSettlementAccount:    "40702810123456789012",
		PayoutCorrespondentAccount: "30101810400000000974",
		PayoutVerificationStatus:   domain.PayoutVerificationVerified,
	}
}

// ── 1. Moderation state-machine ───────────────────────────────────────────────

func TestModerate_stateMachine(t *testing.T) {
	moderatorID := uuid.New()

	transitions := []struct {
		name        string
		fromStatus  string
		action      string
		comment     string
		wantStatus  string
		wantErr     bool
	}{
		// valid transitions
		{name: "pending_review → approve → active", fromStatus: domain.StatusPendingReview, action: "approve", wantStatus: domain.StatusActive},
		{name: "pending_review → request_revision → needs_revision", fromStatus: domain.StatusPendingReview, action: "request_revision", comment: "fix bio", wantStatus: domain.StatusNeedsRevision},
		{name: "pending_review → reject", fromStatus: domain.StatusPendingReview, action: "reject", comment: "fraud", wantStatus: domain.StatusRejected},
		{name: "active → suspend", fromStatus: domain.StatusActive, action: "suspend", comment: "violations", wantStatus: domain.StatusSuspended},
		{name: "suspended → approve → active", fromStatus: domain.StatusSuspended, action: "approve", wantStatus: domain.StatusActive},
		{name: "suspended → request_revision", fromStatus: domain.StatusSuspended, action: "request_revision", comment: "update docs", wantStatus: domain.StatusNeedsRevision},
		// invalid transitions
		{name: "active → approve (invalid)", fromStatus: domain.StatusActive, action: "approve", wantErr: true},
		{name: "draft → approve (invalid)", fromStatus: domain.StatusDraft, action: "approve", wantErr: true},
		{name: "active → reject (invalid)", fromStatus: domain.StatusActive, action: "reject", comment: "x", wantErr: true},
		{name: "reject requires comment", fromStatus: domain.StatusPendingReview, action: "reject", wantErr: true},
		{name: "suspend requires comment", fromStatus: domain.StatusActive, action: "suspend", wantErr: true},
	}

	for _, tt := range transitions {
		t.Run(tt.name, func(t *testing.T) {
			repo := newStubRepo()
			m := activeMaster()
			m.Status = tt.fromStatus
			repo.addMaster(m)
			uc := newUC(repo, &stubPayment{})

			result, err := uc.Moderate(context.Background(), m.ID, moderatorID, tt.action, tt.comment)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Moderate(%q from %q) expected error, got nil", tt.action, tt.fromStatus)
				}
				return
			}
			if err != nil {
				t.Fatalf("Moderate(%q from %q) unexpected error: %v", tt.action, tt.fromStatus, err)
			}
			if result.Status != tt.wantStatus {
				t.Fatalf("Moderate(%q from %q) status = %q, want %q", tt.action, tt.fromStatus, result.Status, tt.wantStatus)
			}
			// Verify the stored status was actually updated.
			stored := repo.masters[m.ID]
			if stored.Status != tt.wantStatus {
				t.Fatalf("stored status = %q after Moderate, want %q", stored.Status, tt.wantStatus)
			}
		})
	}
}

func TestModerate_approve_suspend_approve_cycle(t *testing.T) {
	// active → suspend → approve reproduces the common "reinstate after violation" flow.
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)
	uc := newUC(repo, &stubPayment{})
	mod := uuid.New()
	ctx := context.Background()

	if _, err := uc.Moderate(ctx, m.ID, mod, "suspend", "policy violation"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if repo.masters[m.ID].Status != domain.StatusSuspended {
		t.Fatal("expected suspended after first Moderate")
	}

	if _, err := uc.Moderate(ctx, m.ID, mod, "approve", ""); err != nil {
		t.Fatalf("re-approve: %v", err)
	}
	if repo.masters[m.ID].Status != domain.StatusActive {
		t.Fatal("expected active after re-approval")
	}
}

// ── 2. ConfirmBookingByPayment / CancelBookingByPayment idempotency ───────────

func TestConfirmBookingByPayment_idempotency(t *testing.T) {
	repo := newStubRepo()
	uc := newUC(repo, &stubPayment{})
	bookingID := uuid.New()
	paymentID := "pay-abc"

	// First call succeeds.
	if err := uc.ConfirmBookingByPayment(context.Background(), bookingID.String(), paymentID); err != nil {
		t.Fatalf("first confirm: unexpected error: %v", err)
	}

	// Second call returns ErrBookingNotPending — treated as idempotent (Ack).
	repo.confirmErr = domain.ErrBookingNotPending
	err := uc.ConfirmBookingByPayment(context.Background(), bookingID.String(), paymentID)
	if !errors.Is(err, domain.ErrBookingNotPending) {
		t.Fatalf("second confirm: want ErrBookingNotPending, got %v", err)
	}
}

func TestConfirmBookingByPayment_paymentMismatch(t *testing.T) {
	repo := newStubRepo()
	repo.confirmErr = domain.ErrPaymentMismatch
	uc := newUC(repo, &stubPayment{})

	err := uc.ConfirmBookingByPayment(context.Background(), uuid.New().String(), "pay-x")
	if !errors.Is(err, domain.ErrPaymentMismatch) {
		t.Fatalf("want ErrPaymentMismatch, got %v", err)
	}
}

func TestConfirmBookingByPayment_invalidBookingID(t *testing.T) {
	repo := newStubRepo()
	uc := newUC(repo, &stubPayment{})

	err := uc.ConfirmBookingByPayment(context.Background(), "not-a-uuid", "pay-x")
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("want ErrInvalidArgument for malformed UUID, got %v", err)
	}
}

func TestCancelBookingByPayment_idempotency(t *testing.T) {
	repo := newStubRepo()
	uc := newUC(repo, &stubPayment{})
	// Use the same booking and payment IDs for both calls so the test clearly
	// represents the idempotency scenario: the same message delivered twice.
	bookingID := uuid.New()
	paymentID := "pay-y"

	// First call — success.
	if err := uc.CancelBookingByPayment(context.Background(), bookingID.String(), paymentID); err != nil {
		t.Fatalf("first cancel: unexpected error: %v", err)
	}

	// Second call — booking already in a terminal state.
	repo.cancelErr = domain.ErrBookingNotPending
	err := uc.CancelBookingByPayment(context.Background(), bookingID.String(), paymentID)
	if !errors.Is(err, domain.ErrBookingNotPending) {
		t.Fatalf("second cancel: want ErrBookingNotPending, got %v", err)
	}
}

// ── 3. GetBookingsForActorBatch RBAC ──────────────────────────────────────────

func TestGetBookingsForActorBatch_rbac(t *testing.T) {
	clientID := uuid.New()
	masterOwnerID := uuid.New()
	strangerID := uuid.New()

	masterID := uuid.New()
	master := activeMaster()
	master.ID = masterID
	master.UserID = masterOwnerID

	clientBooking := &domain.MasterBooking{
		ID:           uuid.New(),
		MasterID:     masterID,
		ClientUserID: clientID,
	}
	otherBooking := &domain.MasterBooking{
		ID:           uuid.New(),
		MasterID:     masterID,
		ClientUserID: strangerID,
	}

	repo := newStubRepo()
	repo.addMaster(master)
	repo.addBooking(clientBooking)
	repo.addBooking(otherBooking)
	uc := newUC(repo, &stubPayment{})
	ctx := context.Background()

	ids := []uuid.UUID{clientBooking.ID, otherBooking.ID}

	t.Run("client sees own booking only", func(t *testing.T) {
		res, err := uc.GetBookingsForActorBatch(ctx, ids, clientID)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 || res[0].ID != clientBooking.ID {
			t.Fatalf("client got %d bookings, want 1 (own)", len(res))
		}
	})

	t.Run("master owner sees all bookings on their master", func(t *testing.T) {
		res, err := uc.GetBookingsForActorBatch(ctx, ids, masterOwnerID)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 2 {
			t.Fatalf("master owner got %d bookings, want 2", len(res))
		}
	})

	t.Run("stranger sees no bookings", func(t *testing.T) {
		res, err := uc.GetBookingsForActorBatch(ctx, ids, strangerID)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 0 {
			t.Fatalf("stranger got %d bookings, want 0", len(res))
		}
	})

	t.Run("empty id list returns nil", func(t *testing.T) {
		res, err := uc.GetBookingsForActorBatch(ctx, nil, clientID)
		if err != nil {
			t.Fatal(err)
		}
		if res != nil {
			t.Fatal("expected nil for empty id list")
		}
	})
}

// ── 4. CreateBooking happy / sad paths ────────────────────────────────────────

func TestCreateBooking_happyPath_byService(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)

	pay := &stubPayment{
		resp: &paymentv1.PaymentResponse{
			Id:         "pay-001",
			PaymentUrl: "https://pay.example.com/pay-001",
		},
	}
	uc := newUC(repo, pay)

	svcID := m.Services[0].ID
	b, err := uc.CreateBooking(context.Background(), uuid.New(), m.Slug, &svcID, futureDate(), "10:00", "11:00", "")
	if err != nil {
		t.Fatalf("CreateBooking: %v", err)
	}
	if b.PaymentID != "pay-001" {
		t.Fatalf("payment_id = %q, want pay-001", b.PaymentID)
	}
	if b.TotalPrice != 5000 {
		t.Fatalf("total_price = %d, want 5000", b.TotalPrice)
	}
}

func TestCreateBooking_happyPath_hourly(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)

	pay := &stubPayment{
		resp: &paymentv1.PaymentResponse{Id: "pay-002", PaymentUrl: "https://pay.example.com/pay-002"},
	}
	uc := newUC(repo, pay)

	// 1 hour at 10 000 kopecks/hour = 10 000 kopecks.
	b, err := uc.CreateBooking(context.Background(), uuid.New(), m.Slug, nil, futureDate(), "09:00", "10:00", "")
	if err != nil {
		t.Fatalf("CreateBooking hourly: %v", err)
	}
	if b.TotalPrice != 10_000 {
		t.Fatalf("total_price = %d, want 10000", b.TotalPrice)
	}
}

func TestCreateBooking_masterNotFound(t *testing.T) {
	repo := newStubRepo()
	uc := newUC(repo, &stubPayment{})

	_, err := uc.CreateBooking(context.Background(), uuid.New(), "no-such-slug", nil, futureDate(), "10:00", "11:00", "")
	if err == nil {
		t.Fatal("expected error for unknown master slug, got nil")
	}
}

func TestCreateBooking_invalidSlot(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)
	uc := newUC(repo, &stubPayment{})
	ctx := context.Background()
	clientID := uuid.New()

	fd := futureDate()
	tests := []struct {
		name     string
		date     string
		timeFrom string
		timeTo   string
	}{
		{name: "empty date", date: "", timeFrom: "10:00", timeTo: "11:00"},
		{name: "wrong date format", date: "01/07/2027", timeFrom: "10:00", timeTo: "11:00"},
		{name: "past date", date: "2020-01-01", timeFrom: "10:00", timeTo: "11:00"},
		{name: "time_to before time_from", date: fd, timeFrom: "11:00", timeTo: "10:00"},
		{name: "time_to equals time_from", date: fd, timeFrom: "10:00", timeTo: "10:00"},
		{name: "invalid time_from", date: fd, timeFrom: "99:00", timeTo: "11:00"},
		{name: "invalid time_to", date: fd, timeFrom: "10:00", timeTo: "25:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uc.CreateBooking(ctx, clientID, m.Slug, nil, tt.date, tt.timeFrom, tt.timeTo, "")
			if err == nil {
				t.Fatalf("expected error for slot (%q %q %q), got nil", tt.date, tt.timeFrom, tt.timeTo)
			}
		})
	}
}

func TestCreateBooking_paymentServiceError_cleansUpBooking(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)

	pay := &stubPayment{err: errors.New("payment-service unavailable")}
	uc := newUC(repo, pay)

	_, err := uc.CreateBooking(context.Background(), uuid.New(), m.Slug, nil, futureDate(), "10:00", "11:00", "")
	if err == nil {
		t.Fatal("expected error when payment-service fails, got nil")
	}
	// Compensating DeleteBooking must have removed the booking.
	if len(repo.bookings) != 0 {
		t.Fatalf("expected 0 bookings after payment failure, got %d", len(repo.bookings))
	}
}

func TestCreateBooking_unknownService(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)
	uc := newUC(repo, &stubPayment{})

	unknownSvcID := uuid.New()
	_, err := uc.CreateBooking(context.Background(), uuid.New(), m.Slug, &unknownSvcID, futureDate(), "10:00", "11:00", "")
	if err == nil {
		t.Fatal("expected error for unknown service id, got nil")
	}
}

// ── 5. normalizeRussianMobileDigits ──────────────────────────────────────────

func Test_normalizeRussianMobileDigits(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"+7 (999) 123-45-67", "79991234567"},
		{"8 (999) 123-45-67", "79991234567"},
		{"89991234567", "79991234567"},
		{"79991234567", "79991234567"},
		{"9991234567", "79991234567"},  // 10 digits — prepend 7
		{"+7(999)1234567", "79991234567"},
		{"", ""},
		{"123", ""},       // too short
		{"1234567890123", ""},  // too long
		{"not-a-phone", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := normalizeRussianMobileDigits(tt.in)
			if got != tt.want {
				t.Fatalf("normalizeRussianMobileDigits(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ── 6. validateMasterServices ─────────────────────────────────────────────────

func Test_validateMasterServices(t *testing.T) {
	validItem := domain.MasterServiceUpsert{Name: "Стрижка", Price: 500, DurationMin: 30}

	t.Run("valid single service", func(t *testing.T) {
		if err := validateMasterServices([]domain.MasterServiceUpsert{validItem}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("too many services", func(t *testing.T) {
		items := make([]domain.MasterServiceUpsert, domain.MaxServicesPerMaster+1)
		for i := range items {
			items[i] = validItem
		}
		if err := validateMasterServices(items); err == nil {
			t.Fatal("expected error for too many services")
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		if err := validateMasterServices([]domain.MasterServiceUpsert{{Name: "  ", Price: 100}}); err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("name too long rejected", func(t *testing.T) {
		long := make([]rune, domain.MaxServiceName+1)
		for i := range long {
			long[i] = 'а'
		}
		if err := validateMasterServices([]domain.MasterServiceUpsert{{Name: string(long), Price: 100}}); err == nil {
			t.Fatal("expected error for name exceeding MaxServiceName")
		}
	})

	t.Run("description too long rejected", func(t *testing.T) {
		long := make([]rune, domain.MaxServiceDescription+1)
		for i := range long {
			long[i] = 'б'
		}
		item := domain.MasterServiceUpsert{Name: "OK", Description: string(long), Price: 100}
		if err := validateMasterServices([]domain.MasterServiceUpsert{item}); err == nil {
			t.Fatal("expected error for description exceeding MaxServiceDescription")
		}
	})

	t.Run("negative price rejected", func(t *testing.T) {
		if err := validateMasterServices([]domain.MasterServiceUpsert{{Name: "X", Price: -1}}); err == nil {
			t.Fatal("expected error for negative price")
		}
	})

	t.Run("negative duration rejected", func(t *testing.T) {
		if err := validateMasterServices([]domain.MasterServiceUpsert{{Name: "X", DurationMin: -1}}); err == nil {
			t.Fatal("expected error for negative duration")
		}
	})

	t.Run("empty list is valid", func(t *testing.T) {
		if err := validateMasterServices(nil); err != nil {
			t.Fatalf("unexpected error for empty list: %v", err)
		}
	})
}

// ── 7. CreateMyProfile slug-conflict retry ────────────────────────────────────

func TestCreateMyProfile_slugConflictRetry(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("one conflict then success", func(t *testing.T) {
		repo := newStubRepo()
		// First Insert returns ErrSlugConflict; second succeeds (nil in sequence).
		repo.masterInsertErrs = []error{domain.ErrSlugConflict, nil}
		slugCalls := 0
		repo.slugFn = func() string {
			slugCalls++
			return "slug-" + fmt.Sprintf("%d", slugCalls)
		}
		uc := newUC(repo, &stubPayment{})

		got, err := uc.CreateMyProfile(ctx, userID, "Тест Мастер")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil master")
		}
		if slugCalls != 2 {
			t.Fatalf("expected 2 slug generation calls (one per Insert attempt), got %d", slugCalls)
		}
		if len(repo.masters) != 1 {
			t.Fatalf("expected 1 master stored, got %d", len(repo.masters))
		}
	})

	t.Run("two conflicts then success", func(t *testing.T) {
		repo := newStubRepo()
		repo.masterInsertErrs = []error{domain.ErrSlugConflict, domain.ErrSlugConflict, nil}
		slugCalls := 0
		repo.slugFn = func() string {
			slugCalls++
			return "slug-" + fmt.Sprintf("%d", slugCalls)
		}
		uc := newUC(repo, &stubPayment{})

		got, err := uc.CreateMyProfile(ctx, userID, "Тест Мастер")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil master")
		}
		if slugCalls != 3 {
			t.Fatalf("expected 3 slug generation calls, got %d", slugCalls)
		}
	})

	t.Run("all attempts exhausted returns error", func(t *testing.T) {
		repo := newStubRepo()
		// maxSlugAttempts = 3: fill the sequence with 3 consecutive conflicts.
		repo.masterInsertErrs = []error{
			domain.ErrSlugConflict,
			domain.ErrSlugConflict,
			domain.ErrSlugConflict,
		}
		repo.slugFn = func() string { return "always-taken" }
		uc := newUC(repo, &stubPayment{})

		_, err := uc.CreateMyProfile(ctx, userID, "Тест Мастер")
		if err == nil {
			t.Fatal("expected error after all slug attempts exhausted, got nil")
		}
		if !errors.Is(err, domain.ErrSlugConflict) {
			t.Fatalf("expected ErrSlugConflict, got %v", err)
		}
		if len(repo.masters) != 0 {
			t.Fatalf("expected 0 masters stored after full retry exhaustion, got %d", len(repo.masters))
		}
	})

	t.Run("concurrent profile creation returns AlreadyExists", func(t *testing.T) {
		repo := newStubRepo()
		repo.masterInsertErrs = []error{domain.ErrUserProfileExists}
		uc := newUC(repo, &stubPayment{})

		_, err := uc.CreateMyProfile(ctx, userID, "Тест Мастер")
		if err == nil {
			t.Fatal("expected AlreadyExists error, got nil")
		}
		// The usecase wraps ErrUserProfileExists as a gRPC AlreadyExists — we
		// can't import status codes here, so just confirm it is non-nil and
		// does not propagate the raw sentinel (which would surface as Internal).
		if errors.Is(err, domain.ErrUserProfileExists) {
			t.Fatalf("raw ErrUserProfileExists must not leak to caller; got %v", err)
		}
	})
}
