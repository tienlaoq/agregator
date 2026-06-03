package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors returned by repository methods.
// Usecase maps these to appropriate gRPC status codes or retry logic.
var (
	// ErrPaymentMismatch is returned when the booking already has a different
	// payment_id than the one in the incoming event. This signals a logic error
	// in the payment flow and should be Ack'd (not retried).
	ErrPaymentMismatch = errors.New("payment does not belong to this booking")

	// ErrBookingNotPending is returned when the booking's current status does
	// not allow the requested transition (e.g. already cancelled when
	// payment.completed arrives, or completed when payment.failed arrives).
	// Treat as a no-op / Ack — the booking reached a terminal state via another path.
	ErrBookingNotPending = errors.New("booking is not in payment_pending status")

	// ErrSlugConflict is returned by Insert when a concurrent INSERT races and
	// wins the unique constraint on the slug column. The usecase retries slug
	// generation up to a small limit rather than surfacing a 5xx to the caller.
	ErrSlugConflict = errors.New("slug already taken")

	// ErrUserProfileExists is returned by Insert when the unique constraint on
	// masters.user_id fires — i.e. two concurrent CreateMyProfile calls for the
	// same userID raced through the GetByUserID check. The usecase maps this to
	// AlreadyExists rather than letting a raw pgconn.PgError surface as a 5xx.
	ErrUserProfileExists = errors.New("master profile already exists for this user")

	// ErrSubmitNotAllowed is returned by SubmitForReviewAtomic when the UPDATE
	// touched 0 rows — either the master does not exist, or its current status
	// is not one that allows submission (draft / needs_revision / rejected).
	// The most common cause is a duplicate submit: the second call races through
	// the in-memory status check but the DB already transitioned to pending_review.
	ErrSubmitNotAllowed = errors.New("profile cannot be submitted from its current status")

	// ErrNotFound is returned by repository methods when the requested entity
	// does not exist (e.g. UPDATE/DELETE touched 0 rows). Using a domain-level
	// sentinel keeps the usecase free of infrastructure imports (pgx, sql, etc.).
	ErrNotFound = errors.New("not found")

	// ErrInvalidArgument is returned when the caller passes a value that cannot
	// be processed regardless of retries (e.g. a malformed UUID). NATS subscribers
	// should Ack on this error — retrying will not fix a malformed message.
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrPhotoLimitReached is returned by AddMasterPhoto when the master already
	// has MaxMasterPhotos photos. The check is performed inside the repo
	// transaction so concurrent uploads cannot race past the limit.
	ErrPhotoLimitReached = errors.New("photo limit reached")

	// ErrModerationConflict is returned by ModerateAtomic when the conditional
	// UPDATE touches 0 rows because another moderator already changed the
	// master's status between the caller's GetByID read and the UPDATE.
	// The usecase maps this to an Aborted gRPC status so the client can refresh
	// and retry with the current state rather than silently overwriting it.
	ErrModerationConflict = errors.New("moderation conflict: status changed by another moderator")
)

// MaxMasterPhotos is the maximum number of photos allowed per master profile.
// Defined here so the repo and usecase layers share the same constant without
// either importing the other.
const MaxMasterPhotos int32 = 12

// Service field length limits. Enforced in usecase.ValidateMasterServiceUpserts
// and mirrored by CHECK constraints in migration 016.
const (
	MaxServiceName        = 200
	MaxServiceDescription = 5000
	MaxServicesPerMaster  = 50
)

// Specialization field limits. Enforced in usecase.UpdateMyProfile.
// PostgreSQL text[] can hold arbitrarily many elements; the limits here protect
// against abusive payloads before they reach the DB.
const (
	MaxSpecializations      = 20  // maximum number of items in the list
	MaxSpecializationLength = 100 // maximum characters per item
)

// Booking comment limit. Enforced in usecase.CreateBooking.
// The DB column (TEXT) has no length constraint; the limit is applied in the
// usecase layer before the row is inserted.
const MaxBookingCommentLength = 1000

const (
	StatusDraft         = "draft"
	StatusPendingReview = "pending_review"
	StatusNeedsRevision = "needs_revision"
	StatusActive        = "active"
	StatusRejected      = "rejected"
	StatusSuspended     = "suspended"
)

// Booking status constants. Mirror the CHECK constraint in master_bookings.
const (
	BookingStatusPending        = "pending"
	BookingStatusPaymentPending = "payment_pending"
	BookingStatusConfirmed      = "confirmed"
	BookingStatusCancelled      = "cancelled"
	// BookingStatusCompleted is set when a confirmed booking's end time has
	// passed. Until a cron-job that drives confirmed→completed is implemented,
	// HasCompletedBookingByClientMaster recognises both 'completed' rows and
	// 'confirmed' rows whose (date + time_to) is in the past.
	BookingStatusCompleted = "completed"
)

const (
	WorkFormatVenue  = "venue"
	WorkFormatMobile = "mobile"
	WorkFormatBoth   = "both"
)

// Форма, по которой мастер принимает выплаты (ИП / ООО / физлицо / самозанятость).
const (
	PayoutLegalFormIP           = "ip"
	PayoutLegalFormOOO          = "ooo"
	PayoutLegalFormIndividual   = "individual"
	PayoutLegalFormSelfEmployed = "self_employed"
)

const (
	PayoutVerificationUnverified = "unverified"
	PayoutVerificationPending    = "pending"
	PayoutVerificationVerified   = "verified"
	PayoutVerificationRejected   = "rejected"
)

type Master struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	Slug           string
	DisplayName    string
	Bio            string
	Phone          string
	City           string
	WorkFormat     string
	TravelRadiusKm int32
	// Точка отсчёта радиуса выезда (если WorkFormat mobile/both).
	TravelBaseLatitude  *float64
	TravelBaseLongitude *float64
	TravelExcludeZones  []MasterTravelExcludeZone
	ExperienceYears     int32
	Specializations     []string
	HourlyRate          int64
	AvailabilityJSON    string
	// NOTE: yookassa_seller_account_id was dropped when the platform moved from
	// marketplace-split to escrow + per-partner payouts. The DB column lives on
	// (NOT NULL DEFAULT '') until a follow-up migration removes it; master-service
	// simply stops reading/writing it. Partner payout rails now live in
	// payment-service (partner_payout_methods + partner_ledger + payouts).
	PayoutLegalForm            string
	PayoutLegalName            string
	PayoutINN                  string
	PayoutKPP                  string
	PayoutOGRN                 string
	PayoutOGRNIP               string
	PayoutBankName             string
	PayoutBIK                  string
	PayoutSettlementAccount    string
	PayoutCorrespondentAccount string
	PayoutVerificationStatus   string
	Status                     string
	ModerationComment          string
	ModeratedBy                *uuid.UUID
	ModeratedAt                *time.Time
	Services                   []MasterService
	Photos                     []MasterPhoto
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

type MasterPhoto struct {
	ID        uuid.UUID
	MasterID  uuid.UUID
	URL       string
	SortOrder int32
	IsCover   bool
}

type MasterService struct {
	ID          uuid.UUID
	MasterID    uuid.UUID
	Name        string
	Description string
	DurationMin int32
	Price       int64
	SortOrder   int32
}

// MasterTravelExcludeZone — круг на карте «сюда не выезжаю» (внутри общей зоны от метки).
// Намеренно без json-тегов: сериализация для хранения в БД выполняется через
// отдельный тип dbZone в пакете repository, чтобы DB wire-format не влиял
// на доменную структуру.
type MasterTravelExcludeZone struct {
	ID        string
	Latitude  float64
	Longitude float64
	RadiusKm  float64
	Label     string
}

type MasterServiceUpsert struct {
	ID          *uuid.UUID
	Name        string
	Description string
	DurationMin int32
	Price       int64
	SortOrder   int32
	// SortOrderSet is true when the caller explicitly provided a sort_order
	// value (including 0). When false the repository assigns position by list
	// index so that omitting sort_order and sending sort_order=0 have different
	// meanings: "let me be wherever I fall" vs "put me first".
	SortOrderSet bool
}

type ModerationHistoryEntry struct {
	ID        uuid.UUID
	MasterID  uuid.UUID
	OldStatus string
	NewStatus string
	Comment   string
	ChangedBy uuid.UUID
	CreatedAt time.Time
}

// PayoutValidationError is returned by ValidatePayoutProfile when a payout
// field is missing or has the wrong length. It carries both a stable machine-
// readable code (for tests and programmatic checks) and a human-readable
// Russian message (for direct display in the UI via err.Error()).
//
// Usage in tests:
//
//	var pve *domain.PayoutValidationError
//	if errors.As(err, &pve) { t.Logf("field=%s", pve.Field) }
type PayoutValidationError struct {
	// Field is a stable snake_case identifier for the offending field,
	// e.g. "bik", "inn", "legal_form". Safe to use in switch statements.
	Field string
	// Message is a human-readable Russian description suitable for UI display.
	Message string
}

func (e *PayoutValidationError) Error() string { return e.Message }

// payoutErr is a shorthand constructor used inside ValidatePayoutProfile.
func payoutErr(field, message string) *PayoutValidationError {
	return &PayoutValidationError{Field: field, Message: message}
}

// countDigits returns the number of ASCII digit characters in s, ignoring
// spaces and dashes (common formatting in Russian bank requisites).
func countDigits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}

// extractDigits strips all non-digit characters from s and returns the
// resulting string. Used to normalise INN values before checksum validation.
func extractDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// innChecksum computes a single INN checksum digit using the supplied weight
// vector. The weight vector must have length == len(digits)-1 (i.e. it covers
// all digits except the control digit being validated).
//
// Russian standard (ФНС): control digit = (weighted_sum mod 11) mod 10.
func innChecksum(digits string, weights []int) int {
	sum := 0
	for i, w := range weights {
		sum += w * int(digits[i]-'0')
	}
	return (sum % 11) % 10
}

// ValidateINN verifies the Russian INN checksum according to the Federal Tax
// Service (ФНС) algorithm (Приказ МНС РФ от 03.03.2004 № БГ-3-09/178):
//
//   - 10-digit INN (ООО / ОАО / organisation): one control digit at position 10.
//   - 12-digit INN (ИП / физлицо / самозанятый): two control digits at positions
//     11 and 12, computed with different weight vectors.
//
// Returns false when the checksum is wrong — the INN has a typo or was invented.
// The length check is the caller's responsibility; ValidateINN panics on lengths
// other than 10 or 12 (programmer error, not user error).
//
// Exported so that the domain test package can test the algorithm directly
// without going through ValidatePayoutProfile.
func ValidateINN(raw string) bool {
	d := extractDigits(raw)
	// All-zeros INN has a checksum of zero and would otherwise pass the
	// weighted-sum check below. It is not a real assignable INN — reject it
	// explicitly so callers do not have to.
	if strings.Trim(d, "0") == "" {
		return false
	}
	switch len(d) {
	case 10:
		w := []int{2, 4, 10, 3, 5, 9, 4, 6, 8}
		return innChecksum(d, w) == int(d[9]-'0')
	case 12:
		w11 := []int{7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
		w12 := []int{3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
		c11 := innChecksum(d, w11)
		c12 := innChecksum(d, w12)
		return c11 == int(d[10]-'0') && c12 == int(d[11]-'0')
	default:
		// Length must be validated before calling validateINN.
		panic(fmt.Sprintf("validateINN: unexpected digit count %d", len(d)))
	}
}

// NormalizePayoutLegalForm trims, lower-cases, and maps legacy aliases to the
// canonical constant. Currently maps "gph" → PayoutLegalFormIndividual
// (migration 005 renamed the value in the DB; this guard handles rows written
// before that migration ran and direct gRPC calls that send the old value).
//
// Use this function everywhere a payout_legal_form value is read or written so
// that the alias lives in exactly one place.
func NormalizePayoutLegalForm(s string) string {
	v := strings.TrimSpace(strings.ToLower(s))
	if v == "gph" {
		return PayoutLegalFormIndividual
	}
	return v
}

// ValidatePayoutProfile checks that all payout fields required for the given
// legal form are present and have the correct length. It is the single source
// of truth for payout readiness — used by both the usecase (before creating a
// booking) and the delivery layer (to compute the payout_ready flag in the
// profile response).
//
// Returns nil when the profile is valid and ready to receive payouts.
func (m *Master) ValidatePayoutProfile() error {
	plf := NormalizePayoutLegalForm(m.PayoutLegalForm)
	switch plf {
	case PayoutLegalFormIP, PayoutLegalFormOOO, PayoutLegalFormIndividual, PayoutLegalFormSelfEmployed:
	default:
		return payoutErr("legal_form", "укажите форму получения выплат: ИП, ООО, физическое лицо или самозанятость")
	}
	if strings.TrimSpace(m.PayoutLegalName) == "" {
		return payoutErr("legal_name", "укажите ФИО или наименование получателя выплат")
	}
	if strings.TrimSpace(m.PayoutBankName) == "" {
		return payoutErr("bank_name", "укажите банк получателя")
	}
	if countDigits(m.PayoutBIK) != 9 {
		return payoutErr("bik", "БИК должен содержать 9 цифр")
	}
	if countDigits(m.PayoutSettlementAccount) != 20 {
		return payoutErr("settlement_account", "расчетный счет должен содержать 20 цифр")
	}
	if countDigits(m.PayoutCorrespondentAccount) != 20 {
		return payoutErr("correspondent_account", "корреспондентский счет должен содержать 20 цифр")
	}
	innDigits := countDigits(m.PayoutINN)
	switch plf {
	case PayoutLegalFormOOO:
		if innDigits != 10 {
			return payoutErr("inn", "для ООО ИНН должен содержать 10 цифр")
		}
		if !ValidateINN(m.PayoutINN) {
			return payoutErr("inn", "ИНН содержит ошибку: проверьте контрольную цифру")
		}
		if countDigits(m.PayoutKPP) != 9 {
			return payoutErr("kpp", "для ООО КПП должен содержать 9 цифр")
		}
		if countDigits(m.PayoutOGRN) != 13 {
			return payoutErr("ogrn", "для ООО ОГРН должен содержать 13 цифр")
		}
	case PayoutLegalFormIP:
		if innDigits != 12 {
			return payoutErr("inn", "для ИП ИНН должен содержать 12 цифр")
		}
		if !ValidateINN(m.PayoutINN) {
			return payoutErr("inn", "ИНН содержит ошибку: проверьте контрольную цифру")
		}
		if countDigits(m.PayoutOGRNIP) != 15 {
			return payoutErr("ogrnip", "для ИП ОГРНИП должен содержать 15 цифр")
		}
	case PayoutLegalFormSelfEmployed, PayoutLegalFormIndividual:
		if innDigits != 12 {
			return payoutErr("inn", fmt.Sprintf("для %s ИНН должен содержать 12 цифр", plf))
		}
		if !ValidateINN(m.PayoutINN) {
			return payoutErr("inn", "ИНН содержит ошибку: проверьте контрольную цифру")
		}
	}
	return nil
}

// PayoutProfileReady reports whether the master's payout profile passes full
// validation. It is the canonical source for the payout_ready flag returned in
// the profile proto response. Delivery and usecase both call this method so
// the flag and the booking guard are always in sync.
func (m *Master) PayoutProfileReady() bool {
	if m == nil {
		return false
	}
	return m.ValidatePayoutProfile() == nil
}

type MasterBooking struct {
	ID              uuid.UUID
	MasterID        uuid.UUID
	ClientUserID    uuid.UUID
	MasterServiceID *uuid.UUID
	Date            string
	TimeFrom        string
	TimeTo          string
	Comment         string
	Status          string
	PaymentID       string
	PaymentURL      string
	TotalPrice      int64
	CreatedAt       time.Time
}
