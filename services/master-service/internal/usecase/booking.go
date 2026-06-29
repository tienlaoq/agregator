package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

func (uc *MasterUseCase) CreateBooking(ctx context.Context, clientUserID uuid.UUID, masterSlug string, serviceID *uuid.UUID, date, timeFrom, timeTo, comment string) (*domain.MasterBooking, error) {
	comment = strings.TrimSpace(comment)
	if len([]rune(comment)) > domain.MaxBookingCommentLength {
		return nil, pkgerrors.InvalidArgument(fmt.Sprintf(
			"comment must not exceed %d characters", domain.MaxBookingCommentLength))
	}
	m, err := uc.repo.GetBySlug(ctx, masterSlug)
	if err != nil {
		return nil, err
	}
	if m == nil || m.Status != domain.StatusActive {
		return nil, pkgerrors.NotFound("master is not available for booking")
	}
	if serviceID != nil {
		var found bool
		for _, s := range m.Services {
			if s.ID == *serviceID {
				found = true
				break
			}
		}
		if !found {
			return nil, pkgerrors.InvalidArgument("unknown service")
		}
	}
	if err := m.ValidatePayoutProfile(); err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	if err := validateBookingSlot(date, timeFrom, timeTo); err != nil {
		return nil, err
	}
	totalPrice, err := estimateMasterBookingPriceKopecks(m, serviceID, timeFrom, timeTo)
	if err != nil {
		return nil, err
	}
	b := &domain.MasterBooking{
		ID:              uuid.New(),
		MasterID:        m.ID,
		ClientUserID:    clientUserID,
		MasterServiceID: serviceID,
		Date:            date,
		TimeFrom:        timeFrom,
		TimeTo:          timeTo,
		Comment:         comment,
		Status:          "pending",
		TotalPrice:      totalPrice,
	}
	if err := uc.repo.InsertBooking(ctx, b); err != nil {
		return nil, err
	}

	// ── Payment saga ──────────────────────────────────────────────────────────
	// Step 1: create the payment in payment-service.
	// On failure: hard-delete the pending booking so it does not linger. We use
	// context.Background so a cancelled request context does not prevent cleanup.
	//
	// Idempotency key: sha256(booking_id + ":" + client_user_id).
	// Binds the key to both the booking and the authenticated user, preventing
	// key squatting by another user who might learn the booking UUID.
	idemKey := masterBookingIdempotencyKey(b.ID, b.ClientUserID)
	// TODO(escrow): pass ServiceAt once MasterBooking exposes a parsed time.Time
	// for the slot start (currently Date+TimeFrom are strings).  Payment-service
	// falls back to now()+hold when ServiceAt is unset — acceptable while master
	// bookings remain short-term, but the partner receives funds earlier than
	// service date for far-future slots.
	payResp, err := uc.paymentClient.CreatePayment(ctx, &paymentv1.CreatePaymentRequest{
		BookingId:        b.ID.String(),
		Amount:           totalPrice,
		Description:      fmt.Sprintf("Master booking %s", b.ID.String()),
		IdempotencyKey:   idemKey,
		CounterpartyType: "master",
		CounterpartyId:   m.ID.String(),
	})
	if err != nil {
		if delErr := uc.repo.DeleteBooking(context.Background(), b.ID); delErr != nil {
			uc.log.Error().Err(delErr).Stringer("booking_id", b.ID).
				Msg("CreateBooking saga: CreatePayment failed AND DeleteBooking failed — booking is stale pending")
		}
		return nil, fmt.Errorf("create payment: %w", err)
	}

	// Step 2: persist the payment reference on the booking row.
	// On failure: the payment exists at the provider but the booking does not know
	// its payment_id. Delete the booking to avoid a permanently stale row.
	// The provider payment will expire on its own TTL (~1 h for pending).
	// This residual risk is documented in docs/TECH_DEBT.md [BOOKING-ORPHAN-PAYMENT].
	b.PaymentID = strings.TrimSpace(payResp.GetId())
	b.PaymentURL = strings.TrimSpace(payResp.GetPaymentUrl())
	b.Status = "payment_pending"
	if err := uc.repo.SetBookingPayment(ctx, b.ID, b.PaymentID, b.PaymentURL, b.TotalPrice, b.Status); err != nil {
		uc.log.Error().Err(err).
			Stringer("booking_id", b.ID).
			Str("payment_id", b.PaymentID).
			Msg("CreateBooking saga: SetBookingPayment failed after CreatePayment — attempting booking deletion; payment may be orphaned at the provider")
		if delErr := uc.repo.DeleteBooking(context.Background(), b.ID); delErr != nil {
			uc.log.Error().Err(delErr).Stringer("booking_id", b.ID).
				Msg("CreateBooking saga: DeleteBooking also failed — booking AND payment are both orphaned; manual intervention required")
		}
		return nil, fmt.Errorf("set booking payment: %w", err)
	}
	fresh, err := uc.repo.GetBookingByID(ctx, b.ID)
	if err != nil {
		// Non-fatal: SetBookingPayment succeeded, so the booking exists in the DB
		// with the correct payment fields. Return the pre-update snapshot rather
		// than surfacing a spurious read error to the caller.
		return b, nil
	}
	return fresh, nil
}

// bookingMaxAdvanceDays is the maximum number of calendar days in the future
// a booking date may fall. Six months (≈183 days) is a reasonable booking
// horizon for personal-care masters; anything beyond that is almost certainly
// a client error or an abuse vector.
const bookingMaxAdvanceDays = 183

// validateBookingSlot checks that date, time_from and time_to are well-formed,
// internally consistent, and fall within the allowed booking window:
//
//   - date must be today or later (past dates are rejected — booking in the past
//     is nonsensical and could be exploited to manufacture fake "completed"
//     bookings for review manipulation via HasCompletedMasterBooking).
//   - date must not exceed today + bookingMaxAdvanceDays (far-future dates are
//     almost always a client mistake and inflate the master's calendar noise).
//
// "Today" is evaluated in Moscow time (UTC+3, fixed since 2014-10-26). All
// master profiles are Russian, so Moscow time is the correct floor for the
// booking window on the MVP. If multi-timezone support is added later, pass
// the master's *time.Location as a parameter instead of the hard-coded offset.
//
// Format expectations match the DB column types (DATE / TIME):
//   - date:     "2006-01-02"  (layout from time.DateOnly)
//   - timeFrom: "15:04"
//   - timeTo:   "15:04", must be strictly after timeFrom
func validateBookingSlot(date, timeFrom, timeTo string) error {
	parsedDate, err := time.Parse(time.DateOnly, strings.TrimSpace(date))
	if err != nil {
		return pkgerrors.InvalidArgument("invalid date: expected YYYY-MM-DD")
	}

	// Evaluate "today" in Moscow time (UTC+3, no DST).
	const moscowOffset = 3 * time.Hour
	nowMoscow := time.Now().UTC().Add(moscowOffset)
	today := time.Date(nowMoscow.Year(), nowMoscow.Month(), nowMoscow.Day(), 0, 0, 0, 0, time.UTC)

	if parsedDate.Before(today) {
		return pkgerrors.InvalidArgument("date must not be in the past")
	}
	if parsedDate.After(today.AddDate(0, 0, bookingMaxAdvanceDays)) {
		return pkgerrors.InvalidArgument(fmt.Sprintf("date must be within %d days from today", bookingMaxAdvanceDays))
	}

	start, err := time.Parse("15:04", strings.TrimSpace(timeFrom))
	if err != nil {
		return pkgerrors.InvalidArgument("invalid time_from: expected HH:MM")
	}
	end, err := time.Parse("15:04", strings.TrimSpace(timeTo))
	if err != nil {
		return pkgerrors.InvalidArgument("invalid time_to: expected HH:MM")
	}
	if !end.After(start) {
		return pkgerrors.InvalidArgument("time_to must be later than time_from")
	}
	return nil
}

func estimateMasterBookingPriceKopecks(m *domain.Master, serviceID *uuid.UUID, timeFrom, timeTo string) (int64, error) {
	if serviceID != nil {
		for _, s := range m.Services {
			if s.ID == *serviceID {
				if s.Price <= 0 {
					return 0, pkgerrors.InvalidArgument("стоимость услуги не настроена")
				}
				return s.Price, nil
			}
		}
		return 0, pkgerrors.InvalidArgument("unknown service")
	}
	if m.HourlyRate <= 0 {
		return 0, pkgerrors.InvalidArgument("master hourly rate is not configured")
	}
	// Formats are pre-validated by validateBookingSlot; parse errors cannot occur here.
	startAt, _ := time.Parse("15:04", strings.TrimSpace(timeFrom))
	endAt, _ := time.Parse("15:04", strings.TrimSpace(timeTo))
	minutes := int64(endAt.Sub(startAt).Minutes())
	if minutes <= 0 {
		return 0, pkgerrors.InvalidArgument("invalid booking duration")
	}
	// Round up partial hours so master is not underpaid on uneven slots.
	amount := (m.HourlyRate*minutes + 59) / 60
	if amount <= 0 {
		return 0, pkgerrors.InvalidArgument("invalid booking amount")
	}
	return amount, nil
}

func (uc *MasterUseCase) ListMyBookings(ctx context.Context, userID uuid.UUID, statusFilter string) ([]domain.MasterBooking, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master profile not found")
	}
	return uc.repo.ListBookingsByMaster(ctx, m.ID, statusFilter)
}

func (uc *MasterUseCase) ListClientBookings(ctx context.Context, userID uuid.UUID, statusFilter string) ([]domain.MasterBooking, error) {
	return uc.repo.ListBookingsByClient(ctx, userID, statusFilter)
}

// GetBookingsForActorBatch fetches multiple master bookings visible to actorUserID.
// Bookings the actor cannot access are silently omitted (not an error).
// Two DB round-trips total: one for bookings, one for the distinct master profiles needed
// to resolve ownership.
func (uc *MasterUseCase) GetBookingsForActorBatch(ctx context.Context, bookingIDs []uuid.UUID, actorUserID uuid.UUID) ([]domain.MasterBooking, error) {
	if len(bookingIDs) == 0 {
		return nil, nil
	}
	bookings, err := uc.repo.GetBookingsByIDs(ctx, bookingIDs)
	if err != nil {
		return nil, err
	}

	// Collect master ids that aren't accessible as client (need ownership check).
	masterIDsToCheck := make(map[uuid.UUID]struct{})
	for i := range bookings {
		if bookings[i].ClientUserID != actorUserID {
			masterIDsToCheck[bookings[i].MasterID] = struct{}{}
		}
	}

	// Resolve master owner user ids in one query.
	masterIDs := make([]uuid.UUID, 0, len(masterIDsToCheck))
	for mid := range masterIDsToCheck {
		masterIDs = append(masterIDs, mid)
	}
	ownerByMaster, err := uc.repo.GetMasterUserIDsByIDs(ctx, masterIDs)
	if err != nil {
		return nil, err
	}

	out := make([]domain.MasterBooking, 0, len(bookings))
	for i := range bookings {
		b := &bookings[i]
		if b.ClientUserID == actorUserID {
			out = append(out, *b)
			continue
		}
		if ownerByMaster[b.MasterID] == actorUserID {
			out = append(out, *b)
		}
		// otherwise: actor has no access — silently skip
	}
	return out, nil
}

func (uc *MasterUseCase) GetBookingForActor(ctx context.Context, bookingID, actorUserID uuid.UUID) (*domain.MasterBooking, error) {
	b, err := uc.repo.GetBookingByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, pkgerrors.NotFound("booking not found")
	}
	if b.ClientUserID == actorUserID {
		return b, nil
	}
	m, err := uc.repo.GetByID(ctx, b.MasterID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master not found")
	}
	if m.UserID != actorUserID {
		return nil, pkgerrors.PermissionDenied("access denied")
	}
	return b, nil
}

func (uc *MasterUseCase) HasCompletedBookingByClientMaster(ctx context.Context, clientUserID, masterID uuid.UUID) (bool, error) {
	return uc.repo.HasCompletedBookingByClientMaster(ctx, clientUserID, masterID)
}

// ConfirmBookingByPayment transitions a master booking to confirmed in response
// to a payment.completed NATS event.
//
// The actual state transition is done atomically in the repository via a single
// conditional UPDATE (payment_pending → confirmed, matching payment_id). This
// eliminates the GET→check→UPDATE race that would allow two concurrent NATS
// redeliveries to both succeed, or a payment.completed/payment.failed pair to
// corrupt the booking status.
//
// Sentinel errors returned (caller must use errors.Is):
//
//   - domain.ErrNotFound          — booking does not exist; caller should Nak and retry
//   - domain.ErrBookingNotPending — booking is already in a terminal state; caller should Ack
//   - domain.ErrPaymentMismatch   — payment_id does not match stored value; caller should Ack
func (uc *MasterUseCase) ConfirmBookingByPayment(ctx context.Context, bookingID, paymentID string) error {
	bid, err := uuid.Parse(strings.TrimSpace(bookingID))
	if err != nil {
		return fmt.Errorf("%w: invalid booking_id: %v", domain.ErrInvalidArgument, err)
	}
	return uc.repo.ConfirmBookingByPayment(ctx, bid, paymentID)
}

// CancelBookingByPayment transitions a master booking to cancelled in response
// to a payment.failed NATS event. Same atomicity guarantees as ConfirmBookingByPayment.
//
// Sentinel errors returned (caller must use errors.Is):
//
//   - domain.ErrNotFound          — booking does not exist; caller should Nak and retry
//   - domain.ErrBookingNotPending — booking is already in a terminal state; caller should Ack
//   - domain.ErrPaymentMismatch   — payment_id does not match stored value; caller should Ack
func (uc *MasterUseCase) CancelBookingByPayment(ctx context.Context, bookingID, paymentID string) error {
	bid, err := uuid.Parse(strings.TrimSpace(bookingID))
	if err != nil {
		return fmt.Errorf("%w: invalid booking_id: %v", domain.ErrInvalidArgument, err)
	}
	return uc.repo.CancelBookingByPayment(ctx, bid, paymentID)
}

// masterBookingIdempotencyKey derives a stable, user-bound idempotency key for
// the CreatePayment call from master-service.
//
// key = hex(sha256(bookingID + ":" + clientUserID))
//
// Binding both IDs prevents idempotency-key squatting: even if an attacker
// learns the MasterBooking UUID, they cannot register the same key on behalf of
// a different user because the key is cryptographically tied to clientUserID.
func masterBookingIdempotencyKey(bookingID, clientUserID uuid.UUID) string {
	h := sha256.Sum256([]byte(bookingID.String() + ":" + clientUserID.String()))
	return hex.EncodeToString(h[:])
}
