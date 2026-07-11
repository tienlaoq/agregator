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
	masters    map[uuid.UUID]*domain.Master // keyed by master.ID
	bookings   map[uuid.UUID]*domain.MasterBooking
	slotBlocks map[uuid.UUID]*domain.MasterSlotBlock

	// injectable errors / overrides
	confirmErr error
	cancelErr  error
	insertErr  error // returned by InsertBooking

	// additional injectable errors exercised by the read/write wrapper tests.
	getByIDErr          error
	getByUserErr        error
	updateProfileErr    error
	listByStatusErr     error
	listPublicErr       error
	replaceServicesErr  error
	replaceCredsErr     error
	listBookingsErr     error
	addPhotoErr         error
	deletePhotoErr      error
	setCoverErr         error
	suspendByUserErr    error
	hasCompletedVal     bool
	hasCompletedErr     error
	slotConflictVal     bool  // returned by HasSlotBlockConflict
	slotConflictErr     error // returned by HasSlotBlockConflict
	listModHistoryErr   error
	getMasterUserIDsErr error

	// moderation history recorded by UpdateStatusWithHistory / returned by ListModerationHistory.
	history []domain.ModerationHistoryEntry

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

func (r *stubRepo) addMaster(m *domain.Master)         { r.masters[m.ID] = m }
func (r *stubRepo) addBooking(b *domain.MasterBooking) { r.bookings[b.ID] = b }

func (r *stubRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Master, error) {
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	if m, ok := r.masters[id]; ok {
		cp := *m
		return &cp, nil
	}
	return nil, nil
}

func (r *stubRepo) GetByUserID(_ context.Context, userID uuid.UUID) (*domain.Master, error) {
	if r.getByUserErr != nil {
		return nil, r.getByUserErr
	}
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

func (r *stubRepo) SuspendByUser(_ context.Context, userID uuid.UUID) (bool, error) {
	if r.suspendByUserErr != nil {
		return false, r.suspendByUserErr
	}
	for _, m := range r.masters {
		if m.UserID == userID && m.Status != domain.StatusSuspended {
			m.Status = domain.StatusSuspended
			return true, nil
		}
	}
	return false, nil
}

func (r *stubRepo) UpdateStatusWithHistory(_ context.Context, masterID uuid.UUID, status, comment string, moderatedBy *uuid.UUID, h *domain.ModerationHistoryEntry) error {
	if h != nil {
		r.history = append(r.history, *h)
	}
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
	if r.getMasterUserIDsErr != nil {
		return nil, r.getMasterUserIDsErr
	}
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

func (r *stubRepo) UpdateProfile(_ context.Context, m *domain.Master) error {
	if r.updateProfileErr != nil {
		return r.updateProfileErr
	}
	cp := *m
	r.masters[m.ID] = &cp
	return nil
}

// UpdateProfileWithAssociations mirrors the real single-tx repo method: injected
// errors (updateProfileErr / replaceServicesErr / replaceCredsErr) are checked
// up front so that when any step "fails" no scalar write is applied — the same
// all-or-nothing guarantee the transaction gives in production.
func (r *stubRepo) UpdateProfileWithAssociations(ctx context.Context, m *domain.Master, applyServices bool, services []domain.MasterServiceUpsert, applyCredentials bool, credentials []domain.MasterCredentialUpsert) error {
	if r.updateProfileErr != nil {
		return r.updateProfileErr
	}
	if applyServices && r.replaceServicesErr != nil {
		return r.replaceServicesErr
	}
	if applyCredentials && r.replaceCredsErr != nil {
		return r.replaceCredsErr
	}
	cp := *m
	r.masters[m.ID] = &cp
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
func (r *stubRepo) ListByStatus(_ context.Context, status string, limit, offset int32) ([]domain.Master, int32, error) {
	if r.listByStatusErr != nil {
		return nil, 0, r.listByStatusErr
	}
	var all []domain.Master
	for _, m := range r.masters {
		if status == "" || m.Status == status {
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
func (r *stubRepo) ListPublic(_ context.Context, _ domain.ListPublicMastersParams) ([]domain.Master, int32, error) {
	if r.listPublicErr != nil {
		return nil, 0, r.listPublicErr
	}
	var out []domain.Master
	for _, m := range r.masters {
		if m.Status == domain.StatusActive {
			out = append(out, *m)
		}
	}
	return out, int32(len(out)), nil
}
func (r *stubRepo) ReplaceServices(_ context.Context, masterID uuid.UUID, items []domain.MasterServiceUpsert) ([]domain.MasterService, error) {
	if r.replaceServicesErr != nil {
		return nil, r.replaceServicesErr
	}
	out := make([]domain.MasterService, 0, len(items))
	for i, it := range items {
		out = append(out, domain.MasterService{
			ID:          uuid.New(),
			MasterID:    masterID,
			Name:        it.Name,
			Description: it.Description,
			DurationMin: it.DurationMin,
			Price:       it.Price,
			SortOrder:   int32(i),
		})
	}
	if m, ok := r.masters[masterID]; ok {
		m.Services = out
	}
	return out, nil
}
func (r *stubRepo) ReplaceCredentials(_ context.Context, masterID uuid.UUID, items []domain.MasterCredentialUpsert) ([]domain.MasterCredential, error) {
	if r.replaceCredsErr != nil {
		return nil, r.replaceCredsErr
	}
	out := make([]domain.MasterCredential, 0, len(items))
	for i, it := range items {
		out = append(out, domain.MasterCredential{
			ID:        uuid.New(),
			MasterID:  masterID,
			Kind:      it.Kind,
			Title:     it.Title,
			Issuer:    it.Issuer,
			Year:      it.Year,
			SortOrder: int32(i),
		})
	}
	if m, ok := r.masters[masterID]; ok {
		m.Credentials = out
	}
	return out, nil
}
func (r *stubRepo) ListModerationHistory(_ context.Context, masterID uuid.UUID, limit int32) ([]domain.ModerationHistoryEntry, error) {
	if r.listModHistoryErr != nil {
		return nil, r.listModHistoryErr
	}
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
func (r *stubRepo) ListBookingsByMaster(_ context.Context, masterID uuid.UUID, statusFilter string) ([]domain.MasterBooking, error) {
	if r.listBookingsErr != nil {
		return nil, r.listBookingsErr
	}
	var out []domain.MasterBooking
	for _, b := range r.bookings {
		if b.MasterID == masterID && (statusFilter == "" || b.Status == statusFilter) {
			out = append(out, *b)
		}
	}
	return out, nil
}
func (r *stubRepo) ListBookingsByClient(_ context.Context, clientUserID uuid.UUID, statusFilter string) ([]domain.MasterBooking, error) {
	if r.listBookingsErr != nil {
		return nil, r.listBookingsErr
	}
	var out []domain.MasterBooking
	for _, b := range r.bookings {
		if b.ClientUserID == clientUserID && (statusFilter == "" || b.Status == statusFilter) {
			out = append(out, *b)
		}
	}
	return out, nil
}

func (r *stubRepo) InsertSlotBlock(_ context.Context, b *domain.MasterSlotBlock) error {
	if r.slotBlocks == nil {
		r.slotBlocks = make(map[uuid.UUID]*domain.MasterSlotBlock)
	}
	cp := *b
	cp.CreatedAt = time.Now()
	b.CreatedAt = cp.CreatedAt
	r.slotBlocks[b.ID] = &cp
	return nil
}

func (r *stubRepo) ListSlotBlocksByMaster(_ context.Context, masterID uuid.UUID) ([]domain.MasterSlotBlock, error) {
	var out []domain.MasterSlotBlock
	for _, b := range r.slotBlocks {
		if b.MasterID == masterID {
			out = append(out, *b)
		}
	}
	return out, nil
}

func (r *stubRepo) DeleteSlotBlock(_ context.Context, masterID, blockID uuid.UUID) error {
	b, ok := r.slotBlocks[blockID]
	if !ok || b.MasterID != masterID {
		return domain.ErrNotFound
	}
	delete(r.slotBlocks, blockID)
	return nil
}

func (r *stubRepo) HasSlotBlockConflict(_ context.Context, _ uuid.UUID, _, _, _ string) (bool, error) {
	return r.slotConflictVal, r.slotConflictErr
}

// ListClientsByMaster aggregates in-memory bookings the same way the Postgres
// repo does, so ListMyClients can be unit-tested without a database.
func (r *stubRepo) ListClientsByMaster(_ context.Context, masterID uuid.UUID) ([]domain.MasterClient, error) {
	today := time.Now().Format("2006-01-02")
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
		if bt, err := time.Parse(time.RFC3339, b.CreatedAt.Format(time.RFC3339)); err == nil {
			if c.LastBookingAt == nil || bt.After(*c.LastBookingAt) {
				t := bt
				c.LastBookingAt = &t
			}
		}
		visited := b.Status == domain.BookingStatusCompleted ||
			(b.Status == domain.BookingStatusConfirmed && b.Date < today)
		if !visited {
			continue
		}
		c.VisitsCount++
		c.TotalSpent += b.TotalPrice
		if d, err := time.Parse("2006-01-02", b.Date); err == nil {
			if c.FirstVisitAt == nil || d.Before(*c.FirstVisitAt) {
				t := d
				c.FirstVisitAt = &t
			}
			if c.LastVisitAt == nil || d.After(*c.LastVisitAt) {
				t := d
				c.LastVisitAt = &t
			}
		}
	}
	out := make([]domain.MasterClient, 0, len(agg))
	for _, c := range agg {
		out = append(out, *c)
	}
	return out, nil
}
func (r *stubRepo) UpdateBookingStatus(_ context.Context, _ uuid.UUID, _ string) error {
	panic("stubRepo.UpdateBookingStatus not implemented")
}
func (r *stubRepo) HasCompletedBookingByClientMaster(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return r.hasCompletedVal, r.hasCompletedErr
}
func (r *stubRepo) CountPhotosByMaster(_ context.Context, masterID uuid.UUID) (int32, error) {
	if m, ok := r.masters[masterID]; ok {
		return int32(len(m.Photos)), nil
	}
	return 0, nil
}
func (r *stubRepo) AddMasterPhoto(_ context.Context, masterID uuid.UUID, url string) (*domain.MasterPhoto, error) {
	if r.addPhotoErr != nil {
		return nil, r.addPhotoErr
	}
	m, ok := r.masters[masterID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	p := domain.MasterPhoto{
		ID:        uuid.New(),
		MasterID:  masterID,
		URL:       url,
		SortOrder: int32(len(m.Photos)),
		IsCover:   len(m.Photos) == 0,
	}
	m.Photos = append(m.Photos, p)
	return &p, nil
}
func (r *stubRepo) DeleteMasterPhoto(_ context.Context, masterID, photoID uuid.UUID) (string, error) {
	if r.deletePhotoErr != nil {
		return "", r.deletePhotoErr
	}
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
func (r *stubRepo) SetMasterCoverPhoto(_ context.Context, masterID, photoID uuid.UUID) error {
	if r.setCoverErr != nil {
		return r.setCoverErr
	}
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
		ID:         uuid.New(),
		UserID:     uuid.New(),
		Slug:       "test-slug",
		Status:     domain.StatusActive,
		HourlyRate: 10_000,
		Services: []domain.MasterService{
			{ID: uuid.New(), Price: 5000},
		},
		// Minimal payout profile that passes ValidatePayoutProfile (including
		// the mod-11 INN checksum added in refactor 3.18).
		// validINN12 = "500100732259" — verified checksum, see master_test.go.
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

// ── 1. Moderation state-machine ───────────────────────────────────────────────

func TestModerate_stateMachine(t *testing.T) {
	moderatorID := uuid.New()

	transitions := []struct {
		name       string
		fromStatus string
		action     string
		comment    string
		wantStatus string
		wantErr    bool
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
	otherClientID := uuid.New() // client of the second booking (NOT strangerID)
	masterOwnerID := uuid.New()
	strangerID := uuid.New() // truly unrelated: not a client of any booking, not a master owner

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
		ClientUserID: otherClientID,
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

// A booking over time the master blocked is rejected before any booking row is
// inserted or any payment is created.
func TestCreateBooking_slotBlocked(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)
	repo.slotConflictVal = true // HasSlotBlockConflict → blocked

	// A payment client that fails the test if it is ever reached: the block must
	// short-circuit before CreatePayment.
	pay := &stubPayment{err: errors.New("CreatePayment must not be called for a blocked slot")}
	uc := newUC(repo, pay)

	_, err := uc.CreateBooking(context.Background(), uuid.New(), m.Slug, nil, futureDate(), "09:00", "10:00", "")
	if err == nil {
		t.Fatal("expected error for blocked slot, got nil")
	}
	if len(repo.bookings) != 0 {
		t.Fatalf("no booking row must be inserted for a blocked slot, got %d", len(repo.bookings))
	}
}

// When the DB exclusion constraint rejects the insert (concurrent double-book),
// the repo returns domain.ErrSlotTaken and CreateBooking surfaces it as an error
// without proceeding to the payment saga.
func TestCreateBooking_slotTaken(t *testing.T) {
	repo := newStubRepo()
	m := activeMaster()
	repo.addMaster(m)
	repo.insertErr = domain.ErrSlotTaken // InsertBooking hits master_bookings_no_overlap

	pay := &stubPayment{err: errors.New("CreatePayment must not be called when the slot is taken")}
	uc := newUC(repo, pay)

	svcID := m.Services[0].ID
	_, err := uc.CreateBooking(context.Background(), uuid.New(), m.Slug, &svcID, futureDate(), "10:00", "11:00", "")
	if err == nil {
		t.Fatal("expected error for taken slot, got nil")
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
		{"9991234567", "79991234567"}, // 10 digits — prepend 7
		{"+7(999)1234567", "79991234567"},
		{"", ""},
		{"123", ""},           // too short
		{"1234567890123", ""}, // too long
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

// ── 6b. validateMasterCredentials ─────────────────────────────────────────────

func Test_validateMasterCredentials(t *testing.T) {
	t.Run("empty list is valid", func(t *testing.T) {
		out, err := validateMasterCredentials(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 0 {
			t.Fatalf("expected empty output, got %d", len(out))
		}
	})

	t.Run("defaults kind to certificate and trims fields", func(t *testing.T) {
		out, err := validateMasterCredentials([]domain.MasterCredentialUpsert{
			{Title: "  Мастер парения  ", Issuer: "  Школа бани ", Year: 2020},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out[0].Kind != domain.CredentialKindCertificate {
			t.Fatalf("expected default kind certificate, got %q", out[0].Kind)
		}
		if out[0].Title != "Мастер парения" || out[0].Issuer != "Школа бани" {
			t.Fatalf("fields not trimmed: %+v", out[0])
		}
		if out[0].SortOrder != 0 {
			t.Fatalf("expected sort_order 0, got %d", out[0].SortOrder)
		}
	})

	t.Run("award kind accepted and sort_order follows index", func(t *testing.T) {
		out, err := validateMasterCredentials([]domain.MasterCredentialUpsert{
			{Kind: domain.CredentialKindCertificate, Title: "A"},
			{Kind: domain.CredentialKindAward, Title: "B"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out[1].Kind != domain.CredentialKindAward || out[1].SortOrder != 1 {
			t.Fatalf("unexpected second item: %+v", out[1])
		}
	})

	t.Run("invalid kind rejected", func(t *testing.T) {
		if _, err := validateMasterCredentials([]domain.MasterCredentialUpsert{
			{Kind: "diploma", Title: "X"},
		}); err == nil {
			t.Fatal("expected error for invalid kind")
		}
	})

	t.Run("empty title rejected", func(t *testing.T) {
		if _, err := validateMasterCredentials([]domain.MasterCredentialUpsert{
			{Title: "   "},
		}); err == nil {
			t.Fatal("expected error for empty title")
		}
	})

	t.Run("title too long rejected", func(t *testing.T) {
		long := make([]rune, domain.MaxCredentialTitle+1)
		for i := range long {
			long[i] = 'я'
		}
		if _, err := validateMasterCredentials([]domain.MasterCredentialUpsert{
			{Title: string(long)},
		}); err == nil {
			t.Fatal("expected error for title exceeding MaxCredentialTitle")
		}
	})

	t.Run("zero year allowed, out-of-range rejected", func(t *testing.T) {
		if _, err := validateMasterCredentials([]domain.MasterCredentialUpsert{
			{Title: "X", Year: 0},
		}); err != nil {
			t.Fatalf("zero year should be allowed: %v", err)
		}
		if _, err := validateMasterCredentials([]domain.MasterCredentialUpsert{
			{Title: "X", Year: 1800},
		}); err == nil {
			t.Fatal("expected error for year before MinCredentialYear")
		}
	})

	t.Run("too many credentials rejected", func(t *testing.T) {
		items := make([]domain.MasterCredentialUpsert, domain.MaxCredentialsPerMaster+1)
		for i := range items {
			items[i] = domain.MasterCredentialUpsert{Title: "X"}
		}
		if _, err := validateMasterCredentials(items); err == nil {
			t.Fatal("expected error for too many credentials")
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
