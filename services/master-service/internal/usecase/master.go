package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"google.golang.org/grpc"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/pkg/geo"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

type MasterUseCase struct {
	repo          domain.MasterRepository
	paymentClient paymentGatewayClient
	log           zerolog.Logger
}

type paymentGatewayClient interface {
	CreatePayment(ctx context.Context, in *paymentv1.CreatePaymentRequest, opts ...grpc.CallOption) (*paymentv1.PaymentResponse, error)
}

// NewMasterUseCase constructs a MasterUseCase. paymentClient must be non-nil;
// a nil payment client is a programmer error and panics at startup rather than
// surfacing as a runtime error deep inside CreateBooking.
func NewMasterUseCase(repo domain.MasterRepository, paymentClient paymentGatewayClient, log zerolog.Logger) *MasterUseCase {
	if paymentClient == nil {
		panic("NewMasterUseCase: paymentClient must not be nil")
	}
	return &MasterUseCase{repo: repo, paymentClient: paymentClient, log: log}
}

// maxMasterPhotos moved to domain.MaxMasterPhotos so the repo and usecase
// share the same value; the limit is now enforced inside the repo transaction.

// masterPhotoKeyRe matches the storage object key for a master photo after the
// key is extracted from the public URL and cleaned.
// Format: masters/<masterID>/<filename>
// Filename may contain letters, digits, dots, underscores and hyphens only —
// no path separators, no percent signs, no unicode tricks.
var masterPhotoKeyRe = regexp.MustCompile(`^masters/[0-9a-f-]{36}/[a-zA-Z0-9._-]+$`)

// extractMasterPhotoKey extracts the storage object key from a public URL
// produced by either DiskUploader or MinioUploader:
//
//	"/api/v1/uploads/masters/<id>/photo.jpg" → "masters/<id>/photo.jpg"
//	"https://cdn.example.com/photos/masters/<id>/photo.jpg" → "masters/<id>/photo.jpg"
//
// The URL path is parsed structurally by splitting on "/" and locating the
// first "masters" segment. This avoids LastIndex ambiguity where a crafted URL
// with multiple "/masters/" occurrences could cause key extraction to start
// from the wrong (attacker-controlled) segment.
//
// Only exactly two segments after "masters" are captured (UUID + filename),
// so any trailing path components are silently dropped rather than passed
// through to the regexp and prefix checks.
//
// Returns ("", false) if the URL is unparseable or does not contain a
// "masters" segment followed by at least two more segments.
func extractMasterPhotoKey(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	// Operate on the URL path only — ignore scheme, host, query, fragment.
	// path.Clean normalises duplicate slashes and dots before splitting.
	segments := strings.Split(strings.Trim(path.Clean(parsed.Path), "/"), "/")
	for i, seg := range segments {
		// Require exactly three trailing segments: "masters", UUID, filename.
		// Allowing extra segments (e.g. "masters/<uuid>/dir/photo.jpg") would
		// silently drop everything past the filename and accept a URL that
		// targets a different storage object than what was provided.
		if seg == "masters" && i+3 == len(segments) {
			return "masters/" + segments[i+1] + "/" + segments[i+2], true
		}
	}
	return "", false
}

// validateMasterServices checks the service list supplied to ReplaceServices
// before it reaches the repository. Limits are defined as domain constants so
// the DB CHECK constraints (migration 016) and the application layer stay in
// sync automatically.
func validateMasterServices(items []domain.MasterServiceUpsert) error {
	if int32(len(items)) > domain.MaxServicesPerMaster {
		return pkgerrors.InvalidArgument(fmt.Sprintf(
			"too many services: %d provided, max %d", len(items), domain.MaxServicesPerMaster,
		))
	}
	for i, it := range items {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			return pkgerrors.InvalidArgument(fmt.Sprintf("service[%d]: name is required", i))
		}
		if len([]rune(name)) > domain.MaxServiceName {
			return pkgerrors.InvalidArgument(fmt.Sprintf(
				"service[%d]: name exceeds %d characters", i, domain.MaxServiceName,
			))
		}
		if len([]rune(strings.TrimSpace(it.Description))) > domain.MaxServiceDescription {
			return pkgerrors.InvalidArgument(fmt.Sprintf(
				"service[%d]: description exceeds %d characters", i, domain.MaxServiceDescription,
			))
		}
		if it.DurationMin < 0 {
			return pkgerrors.InvalidArgument(fmt.Sprintf("service[%d]: duration_min must be non-negative", i))
		}
		if it.Price < 0 {
			return pkgerrors.InvalidArgument(fmt.Sprintf("service[%d]: price must be non-negative", i))
		}
	}
	return nil
}

// normalizeMasterPhotoURL validates rawURL and returns a canonical URL that
// should be persisted. The caller must use the returned value — not rawURL —
// so that percent-encoding, double-slashes, and query strings are stripped
// before the URL reaches the database.
//
// Canonicalisation steps:
//  1. Parse the URL to separate scheme+host from path.
//  2. Extract the storage key via structural path segment matching
//     (extractMasterPhotoKey), which takes the first "masters" segment and
//     exactly two following segments — preventing double-marker attacks.
//  3. URL-decode the key to catch %2e%2e / %2f / %5c variants.
//  4. path.Clean to collapse any remaining ".." or duplicate slashes.
//  5. Allowlist regexp: ^masters/<uuid>/<safe-filename>$.
//  6. Owner check: cleaned key must start with "masters/<masterID>/".
//  7. Reconstruct a clean URL: scheme://host/<path-prefix>/<cleaned-key>.
//     rawURL is never stored; only the reconstructed canonical form is.
func normalizeMasterPhotoURL(masterID uuid.UUID, rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("photo url is empty")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("photo url is not a valid URL")
	}

	key, ok := extractMasterPhotoKey(rawURL)
	if !ok {
		return "", fmt.Errorf("photo url does not contain a master upload path")
	}

	// URL-decode to normalise %2e%2e, %2f, %5c and similar before cleaning.
	decoded, err := url.PathUnescape(key)
	if err != nil {
		return "", fmt.Errorf("photo url contains invalid percent-encoding")
	}

	// path.Clean collapses "..", ".", duplicate slashes so that any residual
	// traversal sequences become a non-matching string before the regexp check.
	cleaned := path.Clean(decoded)

	// Strict allowlist: masters/<uuid>/<safe-filename-no-slashes>
	if !masterPhotoKeyRe.MatchString(cleaned) {
		return "", fmt.Errorf("photo url has invalid format")
	}

	// Verify the key belongs to this master specifically (not another master's directory).
	expectedPrefix := "masters/" + masterID.String() + "/"
	if !strings.HasPrefix(cleaned, expectedPrefix) {
		return "", fmt.Errorf("photo url does not belong to this master")
	}

	// Reconstruct a canonical URL from the verified, cleaned key.
	// This strips query strings, fragments, double-slashes, and any
	// percent-encoding from what gets stored in the database.
	//
	// For relative URLs (no host): path only, e.g. "/api/v1/uploads/masters/<id>/photo.jpg".
	// For absolute URLs (with host): preserve scheme+host, e.g. "https://cdn.example.com/masters/<id>/photo.jpg".
	//
	// Path prefix: everything in the original cleaned path up to (but not
	// including) the "masters/" segment, so the public URL shape is preserved.
	origCleanPath := path.Clean(parsed.Path)
	mastersIdx := strings.Index(origCleanPath, "/masters/")
	var canonicalURL string
	if mastersIdx >= 0 {
		prefix := origCleanPath[:mastersIdx]
		canonicalURL = prefix + "/" + cleaned
	} else {
		canonicalURL = "/" + cleaned
	}
	if parsed.Host != "" {
		canonicalURL = parsed.Scheme + "://" + parsed.Host + canonicalURL
	}
	return canonicalURL, nil
}

func (uc *MasterUseCase) CreateMyProfile(ctx context.Context, userID uuid.UUID, displayName string) (*domain.Master, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, pkgerrors.InvalidArgument("display_name is required")
	}
	existing, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, pkgerrors.AlreadyExists("master profile already exists")
	}
	// Build the master record and insert it. NewSlug produces a candidate slug
	// using crypto/rand; if a concurrent insert wins the UNIQUE constraint race
	// the repo returns ErrSlugConflict and we retry with a fresh candidate.
	// Three attempts are sufficient: the probability of three consecutive
	// collisions with a 32-bit random suffix is negligibly small in practice.
	const maxSlugAttempts = 3
	m := &domain.Master{
		ID:                       uuid.New(),
		UserID:                   userID,
		DisplayName:              displayName,
		Bio:                      "",
		Phone:                    "",
		City:                     "",
		WorkFormat:               domain.WorkFormatBoth,
		Specializations:          []string{},
		TravelExcludeZones:       []domain.MasterTravelExcludeZone{},
		Services:                 []domain.MasterService{},
		Photos:                   []domain.MasterPhoto{},
		Status:                   domain.StatusDraft,
		AvailabilityJSON:         "{}",
		PayoutVerificationStatus: domain.PayoutVerificationUnverified,
	}
	var insertErr error
	for i := 0; i < maxSlugAttempts; i++ {
		s, err := uc.repo.NewSlug(ctx, displayName, userID)
		if err != nil {
			return nil, err
		}
		m.Slug = s
		insertErr = uc.repo.Insert(ctx, m)
		if !errors.Is(insertErr, domain.ErrSlugConflict) {
			break
		}
	}
	if insertErr != nil {
		// A concurrent CreateMyProfile for the same userID won the race between
		// GetByUserID and Insert. Surface as AlreadyExists — same as the pre-check
		// path — so the client gets a clean 6/AlreadyExists rather than a 5xx.
		if errors.Is(insertErr, domain.ErrUserProfileExists) {
			return nil, pkgerrors.AlreadyExists("master profile already exists")
		}
		return nil, insertErr
	}
	// Insert writes back CreatedAt/UpdatedAt via RETURNING — no round-trip needed.
	// Services and Photos are empty slices: a freshly created profile has none.
	return m, nil
}

type UpdateMasterInput struct {
	DisplayName                *string
	Bio                        *string
	Phone                      *string
	City                       *string
	WorkFormat                 *string
	TravelRadiusKm             *int32
	TravelBaseLatitude         *float64
	TravelBaseLongitude        *float64
	ExperienceYears            *int32
	HourlyRate                 *int64
	AvailabilityJSON           *string
	ApplyServicesReplace       bool
	ServicesReplace            []domain.MasterServiceUpsert
	ApplySpecializations       bool
	Specializations            []string
	PayoutLegalForm            *string
	YookassaSellerAccountID    *string
	PayoutLegalName            *string
	PayoutINN                  *string
	PayoutKPP                  *string
	PayoutOGRN                 *string
	PayoutOGRNIP               *string
	PayoutBankName             *string
	PayoutBIK                  *string
	PayoutSettlementAccount    *string
	PayoutCorrespondentAccount *string
	PayoutVerificationStatus   *string
	ApplyTravelExcludeZones    bool
	TravelExcludeZones         []domain.MasterTravelExcludeZone
}

func (uc *MasterUseCase) UpdateMyProfile(ctx context.Context, userID uuid.UUID, in UpdateMasterInput) (*domain.Master, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master profile not found")
	}
	if in.DisplayName != nil {
		m.DisplayName = strings.TrimSpace(*in.DisplayName)
	}
	if in.Bio != nil {
		m.Bio = strings.TrimSpace(*in.Bio)
	}
	if in.Phone != nil {
		normalized := normalizeRussianMobileDigits(*in.Phone)
		if strings.TrimSpace(*in.Phone) == "" {
			// Explicit empty string: refuse to clear a phone that was already set.
			// Allowing silent erasure would leave the profile in a state where
			// SubmitForReview fails with an opaque "укажите телефон" error and the
			// user has no idea why — they just "cleared" a field in the UI.
			// If a phone genuinely needs to be replaced, the client must supply
			// the new number in the same request; clearing without a replacement
			// is never a valid operation on an existing profile.
			if m.Phone != "" {
				return nil, pkgerrors.InvalidArgument("нельзя удалить номер телефона — укажите новый номер")
			}
			// Phone was already empty (fresh profile, no phone set yet): allow
			// the no-op so partial profile saves work without error.
		} else if normalized == "" {
			// Non-empty input that produced no digits — malformed number.
			return nil, pkgerrors.InvalidArgument("укажите полный номер телефона в формате +7 (999) 123-45-67")
		} else {
			m.Phone = normalized
		}
	}
	if in.City != nil {
		// Normalise to lowercase so the city column can be compared with plain
		// equality (m.city = ANY($n)) and the existing idx_masters_city B-Tree
		// index is usable. The LOWER(TRIM(...)) applied at read-time in the old
		// filter is no longer needed once all writes go through this path.
		m.City = strings.ToLower(strings.TrimSpace(*in.City))
	}
	if in.WorkFormat != nil {
		wf := strings.TrimSpace(*in.WorkFormat)
		if wf != domain.WorkFormatVenue && wf != domain.WorkFormatMobile && wf != domain.WorkFormatBoth {
			return nil, pkgerrors.InvalidArgument("invalid work_format")
		}
		m.WorkFormat = wf
	}
	if in.TravelRadiusKm != nil {
		m.TravelRadiusKm = *in.TravelRadiusKm
	}
	if in.TravelBaseLatitude != nil || in.TravelBaseLongitude != nil {
		if in.TravelBaseLatitude == nil || in.TravelBaseLongitude == nil {
			return nil, pkgerrors.InvalidArgument("укажите широту и долготу метки на карте вместе")
		}
		m.TravelBaseLatitude = in.TravelBaseLatitude
		m.TravelBaseLongitude = in.TravelBaseLongitude
	}
	if in.ExperienceYears != nil {
		m.ExperienceYears = *in.ExperienceYears
	}
	if in.ApplySpecializations {
		if len(in.Specializations) > domain.MaxSpecializations {
			return nil, pkgerrors.InvalidArgument(fmt.Sprintf(
				"specializations must not exceed %d items", domain.MaxSpecializations))
		}
		for _, s := range in.Specializations {
			if len([]rune(s)) > domain.MaxSpecializationLength {
				return nil, pkgerrors.InvalidArgument(fmt.Sprintf(
					"each specialization must not exceed %d characters", domain.MaxSpecializationLength))
			}
		}
		m.Specializations = in.Specializations
	}
	if in.HourlyRate != nil {
		m.HourlyRate = *in.HourlyRate
	}
	if in.AvailabilityJSON != nil {
		s := strings.TrimSpace(*in.AvailabilityJSON)
		if s == "" {
			s = "{}"
		}
		// Unmarshal into a concrete map[string]any rather than any:
		// - arrays, strings, numbers, null all produce a type mismatch error
		// - invalid JSON produces a syntax error
		// One pass covers both the "valid JSON?" and "is it an object?" checks,
		// matching the DB column's JSONB CHECK (jsonb_typeof(...) = 'object').
		var top map[string]any
		if err := json.Unmarshal([]byte(s), &top); err != nil {
			return nil, pkgerrors.InvalidArgument("availability_json: must be a JSON object")
		}
		m.AvailabilityJSON = s
	}
	if in.PayoutLegalForm != nil {
		v := domain.NormalizePayoutLegalForm(*in.PayoutLegalForm)
		switch v {
		case "", domain.PayoutLegalFormIP, domain.PayoutLegalFormOOO, domain.PayoutLegalFormIndividual, domain.PayoutLegalFormSelfEmployed:
			m.PayoutLegalForm = v
		default:
			return nil, pkgerrors.InvalidArgument("invalid payout_legal_form")
		}
	}
	if in.YookassaSellerAccountID != nil {
		m.YookassaSellerAccountID = strings.TrimSpace(*in.YookassaSellerAccountID)
	}
	if in.PayoutLegalName != nil {
		m.PayoutLegalName = strings.TrimSpace(*in.PayoutLegalName)
	}
	if in.PayoutINN != nil {
		m.PayoutINN = normalizeDigits(*in.PayoutINN)
	}
	if in.PayoutKPP != nil {
		m.PayoutKPP = normalizeDigits(*in.PayoutKPP)
	}
	if in.PayoutOGRN != nil {
		m.PayoutOGRN = normalizeDigits(*in.PayoutOGRN)
	}
	if in.PayoutOGRNIP != nil {
		m.PayoutOGRNIP = normalizeDigits(*in.PayoutOGRNIP)
	}
	if in.PayoutBankName != nil {
		m.PayoutBankName = strings.TrimSpace(*in.PayoutBankName)
	}
	if in.PayoutBIK != nil {
		m.PayoutBIK = normalizeDigits(*in.PayoutBIK)
	}
	if in.PayoutSettlementAccount != nil {
		m.PayoutSettlementAccount = normalizeDigits(*in.PayoutSettlementAccount)
	}
	if in.PayoutCorrespondentAccount != nil {
		m.PayoutCorrespondentAccount = normalizeDigits(*in.PayoutCorrespondentAccount)
	}
	if in.PayoutVerificationStatus != nil {
		v := strings.TrimSpace(strings.ToLower(*in.PayoutVerificationStatus))
		switch v {
		case "":
			m.PayoutVerificationStatus = domain.PayoutVerificationUnverified
		case domain.PayoutVerificationUnverified, domain.PayoutVerificationPending, domain.PayoutVerificationVerified, domain.PayoutVerificationRejected:
			m.PayoutVerificationStatus = v
		default:
			return nil, pkgerrors.InvalidArgument("invalid payout_verification_status")
		}
	}
	if in.PayoutLegalForm != nil ||
		in.YookassaSellerAccountID != nil ||
		in.PayoutLegalName != nil ||
		in.PayoutINN != nil ||
		in.PayoutKPP != nil ||
		in.PayoutOGRN != nil ||
		in.PayoutOGRNIP != nil ||
		in.PayoutBankName != nil ||
		in.PayoutBIK != nil ||
		in.PayoutSettlementAccount != nil ||
		in.PayoutCorrespondentAccount != nil {
		if hasAnyPayoutData(m) {
			if err := m.ValidatePayoutProfile(); err != nil {
				return nil, pkgerrors.InvalidArgument(err.Error())
			}
		}
	}
	if in.ApplyTravelExcludeZones {
		m.TravelExcludeZones = in.TravelExcludeZones
	}

	if m.Status == domain.StatusActive {
		m.Status = domain.StatusPendingReview
		m.ModerationComment = ""
		m.ModeratedBy = nil
		m.ModeratedAt = nil
	}

	// Reject requests that explicitly set travel fields while the effective
	// work_format is venue. Silent normalisation (apply-then-clear) is
	// confusing: the client sends zones, gets back an empty list, and has no
	// idea why. Returning InvalidArgument here surfaces the conflict immediately
	// so the client can fix its logic rather than silently losing data.
	//
	// "Effective" means after in.WorkFormat has been applied to m above —
	// so switching to venue in the same request while also sending travel data
	// is caught too.
	if m.WorkFormat == domain.WorkFormatVenue {
		if in.TravelRadiusKm != nil && *in.TravelRadiusKm != 0 {
			return nil, pkgerrors.InvalidArgument("travel_radius_km не применимо для формата venue")
		}
		if in.TravelBaseLatitude != nil || in.TravelBaseLongitude != nil {
			return nil, pkgerrors.InvalidArgument("travel_base_lat/lng не применимо для формата venue")
		}
		if in.ApplyTravelExcludeZones && len(in.TravelExcludeZones) > 0 {
			return nil, pkgerrors.InvalidArgument("travel_exclude_zones не применимо для формата venue")
		}
	}

	// clearTravelFieldsForVenue runs after all input has been applied. It zeroes
	// any travel fields that may already be stored in the DB when the master
	// switches to venue format (in.WorkFormat = "venue" with no travel fields in
	// this request). After the explicit-conflict check above, reaching here means
	// the request either didn't touch travel fields or set them all to zero —
	// so clearing is a safe normalisation, not a silent data loss.
	clearTravelFieldsForVenue(m)

	if err := validateTravelBaseForProfile(m); err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	if err := validateTravelExcludeZonesForProfile(m); err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}

	if err := uc.repo.UpdateProfile(ctx, m); err != nil {
		return nil, err
	}

	if in.ApplyServicesReplace {
		if err := validateMasterServices(in.ServicesReplace); err != nil {
			return nil, err
		}
		if _, err := uc.repo.ReplaceServices(ctx, m.ID, in.ServicesReplace); err != nil {
			return nil, err
		}
	}

	// Re-read the full profile so the response is consistent: Photos may have
	// changed concurrently (AddMasterPhoto / DeleteMasterPhoto run outside this
	// call), and UpdateProfile sets UpdatedAt server-side via RETURNING which
	// would require an extra scan to propagate. A single GetByUserID costs two
	// batch queries (master row + associations) and is cheaper than building a
	// partial in-memory response that surprises callers with stale photo lists.
	return uc.repo.GetByUserID(ctx, userID)
}

func (uc *MasterUseCase) GetMyProfile(ctx context.Context, userID uuid.UUID) (*domain.Master, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master profile not found")
	}
	return m, nil
}

func (uc *MasterUseCase) SubmitForReview(ctx context.Context, userID uuid.UUID) (*domain.Master, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master profile not found")
	}
	switch m.Status {
	case domain.StatusDraft, domain.StatusNeedsRevision, domain.StatusRejected:
		// ok — validate then submit
	case domain.StatusPendingReview:
		// already submitted — idempotent
		return m, nil
	default:
		return nil, pkgerrors.InvalidArgument("profile cannot be submitted in current status: " + m.Status)
	}
	if err := validateReadyForReview(m); err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	// SubmitForReviewAtomic does UPDATE … WHERE status IN ('draft','needs_revision','rejected')
	// and inserts the history entry in one transaction, eliminating the race window
	// where two concurrent submits both read 'draft' and each write a history entry.
	// If the status changed between GetByUserID and here (duplicate submit), the UPDATE
	// touches 0 rows and ErrSubmitNotAllowed is returned — we re-read and return the
	// current state so the caller gets an idempotent success.
	if err := uc.repo.SubmitForReviewAtomic(ctx, m.ID, userID); err != nil {
		if errors.Is(err, domain.ErrSubmitNotAllowed) {
			// Race: another submit (or a moderator action) changed the status
			// between our read and the atomic UPDATE. Re-read current state.
			return uc.repo.GetByUserID(ctx, userID)
		}
		return nil, err
	}
	return uc.repo.GetByUserID(ctx, userID)
}

// normalizeRussianMobileDigits returns 11 digits starting with 7, or empty if invalid.
func normalizeRussianMobileDigits(phone string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(phone) {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	switch len(d) {
	case 11:
		if d[0] == '8' {
			return "7" + d[1:]
		}
		if d[0] == '7' {
			return d
		}
		return ""
	case 10:
		return "7" + d
	default:
		return ""
	}
}

func needsTravelBaseForFormat(wf string) bool {
	switch strings.TrimSpace(strings.ToLower(wf)) {
	case domain.WorkFormatMobile, domain.WorkFormatBoth:
		return true
	default:
		return false
	}
}

// clearTravelFieldsForVenue zeroes all travel-related fields when the master's
// work format is venue-only. Callers must invoke this after all input fields
// have been applied to m so that user-supplied travel data never reaches the DB
// for a venue profile, regardless of what the gRPC request contained.
func clearTravelFieldsForVenue(m *domain.Master) {
	if m.WorkFormat == domain.WorkFormatVenue {
		m.TravelBaseLatitude = nil
		m.TravelBaseLongitude = nil
		m.TravelRadiusKm = 0
		m.TravelExcludeZones = nil
	}
}

func validateTravelBaseForProfile(m *domain.Master) error {
	if !needsTravelBaseForFormat(m.WorkFormat) {
		return nil
	}
	if m.TravelRadiusKm <= 0 {
		// Радиус не задан: карта выезда для анкеты опциональна.
		return nil
	}
	if m.TravelBaseLatitude == nil || m.TravelBaseLongitude == nil {
		return fmt.Errorf("поставьте метку на карте (точка отсчёта километража)")
	}
	lat := *m.TravelBaseLatitude
	lon := *m.TravelBaseLongitude
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return fmt.Errorf("некорректные координаты метки на карте")
	}
	return nil
}

const (
	maxTravelExcludeZones = 20
	minExcludeRadiusKm    = 0.1
	maxExcludeRadiusKm    = 50.0
)

func validateTravelExcludeZones(zones []domain.MasterTravelExcludeZone) error {
	if len(zones) > maxTravelExcludeZones {
		return fmt.Errorf("не более %d зон, куда не выезжаете", maxTravelExcludeZones)
	}
	for i, z := range zones {
		if strings.TrimSpace(z.ID) == "" {
			return fmt.Errorf("зона %d: отсутствует id", i+1)
		}
		if z.Latitude < -90 || z.Latitude > 90 || z.Longitude < -180 || z.Longitude > 180 {
			return fmt.Errorf("зона %d: некорректные координаты", i+1)
		}
		if z.RadiusKm > 0 && (z.RadiusKm < minExcludeRadiusKm || z.RadiusKm > maxExcludeRadiusKm) {
			return fmt.Errorf("зона %d: радиус от %.1f до %.0f км", i+1, minExcludeRadiusKm, maxExcludeRadiusKm)
		}
	}
	return nil
}

func validateTravelExcludeZonesForProfile(m *domain.Master) error {
	if !needsTravelBaseForFormat(m.WorkFormat) {
		return nil
	}
	if err := validateTravelExcludeZones(m.TravelExcludeZones); err != nil {
		return err
	}
	return validateTravelExcludeZonesInsideTravelRadius(m)
}

// Исключения — круги внутри зоны выезда: центр_исключения + радиус_исключения не дальше travel_radius_km от метки.
func validateTravelExcludeZonesInsideTravelRadius(m *domain.Master) error {
	if len(m.TravelExcludeZones) == 0 {
		return nil
	}
	if m.TravelBaseLatitude == nil || m.TravelBaseLongitude == nil || m.TravelRadiusKm <= 0 {
		return nil
	}
	blat, blon := *m.TravelBaseLatitude, *m.TravelBaseLongitude
	R := float64(m.TravelRadiusKm)
	// Небольшой допуск: карта / ввод км дают погрешность относительно формулы на сфере.
	const epsKm = 0.05
	for i, z := range m.TravelExcludeZones {
		d := geo.HaversineKm(blat, blon, z.Latitude, z.Longitude)
		if d+z.RadiusKm > R+epsKm {
			return fmt.Errorf(
				"зона «куда не выезжаю» %d: круг должен целиком помещаться в зону выезда от метки (%d км по карте); уменьшите радиус зоны или перенесите центр ближе к метке",
				i+1, m.TravelRadiusKm,
			)
		}
	}
	return nil
}

func validateReadyForReview(m *domain.Master) error {
	if strings.TrimSpace(m.DisplayName) == "" {
		return fmt.Errorf("укажите имя или отображаемое имя")
	}
	if strings.TrimSpace(m.City) == "" {
		return fmt.Errorf("укажите город")
	}
	if m.Phone == "" {
		return fmt.Errorf("укажите полный номер телефона в формате +7 (999) 123-45-67")
	}
	bioTrim := strings.TrimSpace(m.Bio)
	if bioTrim == "" || len([]rune(bioTrim)) < 20 {
		return fmt.Errorf("описание должно быть не короче 20 символов")
	}
	if len(m.Services) == 0 {
		return fmt.Errorf("добавьте хотя бы одну услугу")
	}
	if err := m.ValidatePayoutProfile(); err != nil {
		return err
	}
	if err := validateTravelBaseForProfile(m); err != nil {
		return err
	}
	if err := validateTravelExcludeZonesForProfile(m); err != nil {
		return err
	}
	return nil
}

func normalizeDigits(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasAnyPayoutData(m *domain.Master) bool {
	return strings.TrimSpace(m.PayoutLegalForm) != "" ||
		strings.TrimSpace(m.YookassaSellerAccountID) != "" ||
		strings.TrimSpace(m.PayoutLegalName) != "" ||
		strings.TrimSpace(m.PayoutINN) != "" ||
		strings.TrimSpace(m.PayoutKPP) != "" ||
		strings.TrimSpace(m.PayoutOGRN) != "" ||
		strings.TrimSpace(m.PayoutOGRNIP) != "" ||
		strings.TrimSpace(m.PayoutBankName) != "" ||
		strings.TrimSpace(m.PayoutBIK) != "" ||
		strings.TrimSpace(m.PayoutSettlementAccount) != "" ||
		strings.TrimSpace(m.PayoutCorrespondentAccount) != ""
}

func (uc *MasterUseCase) ListPublic(ctx context.Context, params domain.ListPublicMastersParams) ([]domain.Master, int32, error) {
	return uc.repo.ListPublic(ctx, params)
}

func (uc *MasterUseCase) GetPublicBySlug(ctx context.Context, slug string) (*domain.Master, error) {
	m, err := uc.repo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if m == nil || m.Status != domain.StatusActive {
		return nil, pkgerrors.NotFound("master not found")
	}
	return m, nil
}

func (uc *MasterUseCase) ListForModeration(ctx context.Context, statusFilter string, limit, offset int32) ([]domain.Master, int32, error) {
	return uc.repo.ListByStatus(ctx, statusFilter, limit, offset)
}

func (uc *MasterUseCase) Moderate(ctx context.Context, masterID, moderatorID uuid.UUID, action, comment string) (*domain.Master, error) {
	action = strings.TrimSpace(strings.ToLower(action))
	comment = strings.TrimSpace(comment)
	// 1000 chars is enough for any legitimate moderation note and prevents
	// moderators from accidentally pasting personal data (PII/PD) into the
	// history log, which is stored indefinitely and visible to other admins.
	const maxCommentLen = 1000
	if len([]rune(comment)) > maxCommentLen {
		return nil, pkgerrors.InvalidArgument("comment must not exceed 1000 characters")
	}

	m, err := uc.repo.GetByID(ctx, masterID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master not found")
	}

	requireComment := action == "request_revision" || action == "reject" || action == "suspend"
	if requireComment && comment == "" {
		return nil, pkgerrors.InvalidArgument("comment is required for this action")
	}

	var newStatus string
	switch action {
	case "approve":
		switch m.Status {
		case domain.StatusPendingReview, domain.StatusSuspended:
			newStatus = domain.StatusActive
		default:
			return nil, pkgerrors.InvalidArgument("approve is not allowed from status " + m.Status)
		}
		// Guard: prevent approving a master whose payout profile is incomplete.
		// Without this check, the master becomes active but booking creation
		// fails with an opaque payout error — confusing for both the user and
		// the support team. Moderators must resolve payout issues before approve.
		if err := m.ValidatePayoutProfile(); err != nil {
			return nil, pkgerrors.InvalidArgument("cannot approve: payout profile is incomplete — " + err.Error())
		}
	case "request_revision":
		switch m.Status {
		case domain.StatusPendingReview, domain.StatusSuspended:
			newStatus = domain.StatusNeedsRevision
		default:
			return nil, pkgerrors.InvalidArgument("request_revision is not allowed from status " + m.Status)
		}
	case "reject":
		if m.Status != domain.StatusPendingReview {
			return nil, pkgerrors.InvalidArgument("reject is only allowed from pending_review")
		}
		newStatus = domain.StatusRejected
	case "suspend":
		if m.Status != domain.StatusActive {
			return nil, pkgerrors.InvalidArgument("suspend is only allowed from active")
		}
		newStatus = domain.StatusSuspended
	default:
		return nil, pkgerrors.InvalidArgument("unknown action: " + action)
	}

	// ModerateAtomic guards the UPDATE with WHERE status = m.Status so that if
	// a second moderator acted between our GetByID and this call, the UPDATE
	// touches 0 rows and ErrModerationConflict is returned instead of silently
	// overwriting the winning decision. Both status change and history entry are
	// written atomically — no partial writes, no duplicate history rows.
	if err := uc.repo.ModerateAtomic(ctx, masterID, m.Status, newStatus, comment, &moderatorID); err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			// Master deleted between GetByID and the UPDATE (concurrent admin delete).
			return nil, pkgerrors.NotFound("master not found")
		case errors.Is(err, domain.ErrModerationConflict):
			// Another moderator acted first. Tell the client to refresh.
			return nil, pkgerrors.Aborted("moderation conflict: the master status was changed by another moderator — please refresh and retry")
		}
		return nil, err
	}
	updated, err := uc.repo.GetByID(ctx, masterID)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		// Should not happen: UpdateStatusWithHistory succeeded so the row exists.
		// Guard anyway to prevent nil-pointer dereference in the proto marshaller.
		return nil, pkgerrors.NotFound("master not found after status update")
	}
	return updated, nil
}

func (uc *MasterUseCase) ListModerationHistory(ctx context.Context, masterID uuid.UUID, limit int32) ([]domain.ModerationHistoryEntry, error) {
	return uc.repo.ListModerationHistory(ctx, masterID, limit)
}

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
	payResp, err := uc.paymentClient.CreatePayment(ctx, &paymentv1.CreatePaymentRequest{
		BookingId:               b.ID.String(),
		Amount:                  totalPrice,
		Description:             fmt.Sprintf("Master booking %s", b.ID.String()),
		IdempotencyKey:          idemKey,
		CounterpartyType:        "master",
		CounterpartyId:          m.ID.String(),
		YookassaSellerAccountId: strings.TrimSpace(m.YookassaSellerAccountID),
	})
	if err != nil {
		if delErr := uc.repo.DeleteBooking(context.Background(), b.ID); delErr != nil {
			uc.log.Error().Err(delErr).Stringer("booking_id", b.ID).
				Msg("CreateBooking saga: CreatePayment failed AND DeleteBooking failed — booking is stale pending")
		}
		return nil, fmt.Errorf("create payment: %w", err)
	}

	// Step 2: persist the payment reference on the booking row.
	// On failure: the payment exists in YooKassa but the booking does not know
	// its payment_id. Delete the booking to avoid a permanently stale row.
	// The YooKassa payment will expire on its own TTL (~1 h for pending).
	// This residual risk is documented in docs/TECH_DEBT.md [BOOKING-ORPHAN-PAYMENT].
	b.PaymentID = strings.TrimSpace(payResp.GetId())
	b.PaymentURL = strings.TrimSpace(payResp.GetPaymentUrl())
	b.Status = "payment_pending"
	if err := uc.repo.SetBookingPayment(ctx, b.ID, b.PaymentID, b.PaymentURL, b.TotalPrice, b.Status); err != nil {
		uc.log.Error().Err(err).
			Stringer("booking_id", b.ID).
			Str("payment_id", b.PaymentID).
			Msg("CreateBooking saga: SetBookingPayment failed after CreatePayment — attempting booking deletion; payment may be orphaned in YooKassa")
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

// GetMasterUserIDsBatch returns map[masterID]userID for a set of master profile ids.
// Used by gRPC handlers to populate master_user_id on batch responses without N+1.
func (uc *MasterUseCase) GetMasterUserIDsBatch(ctx context.Context, masterIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	return uc.repo.GetMasterUserIDsByIDs(ctx, masterIDs)
}

// MasterOwnerUserID returns the platform user id for the master profile (venue owner account).
func (uc *MasterUseCase) MasterOwnerUserID(ctx context.Context, masterID uuid.UUID) (uuid.UUID, error) {
	m, err := uc.repo.GetByID(ctx, masterID)
	if err != nil {
		return uuid.Nil, err
	}
	if m == nil {
		return uuid.Nil, pkgerrors.NotFound("master not found")
	}
	return m.UserID, nil
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

func (uc *MasterUseCase) AddMasterPhoto(ctx context.Context, userID uuid.UUID, url string) (*domain.Master, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master profile not found")
	}
	cleanURL, err := normalizeMasterPhotoURL(m.ID, url)
	if err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	// The limit check (MaxMasterPhotos) is enforced inside the repo transaction
	// so concurrent uploads cannot race past it. CountPhotosByMaster is no longer
	// called here — the separate count+check pattern had a TOCTOU window.
	if _, err := uc.repo.AddMasterPhoto(ctx, m.ID, cleanURL); err != nil {
		if errors.Is(err, domain.ErrPhotoLimitReached) {
			return nil, pkgerrors.InvalidArgument(fmt.Sprintf("too many photos (max %d)", domain.MaxMasterPhotos))
		}
		return nil, err
	}
	return uc.repo.GetByUserID(ctx, userID)
}

func (uc *MasterUseCase) DeleteMasterPhoto(ctx context.Context, userID, photoID uuid.UUID) (string, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return "", err
	}
	if m == nil {
		return "", pkgerrors.NotFound("master profile not found")
	}
	u, err := uc.repo.DeleteMasterPhoto(ctx, m.ID, photoID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", pkgerrors.NotFound("photo not found")
		}
		return "", err
	}
	return u, nil
}

func (uc *MasterUseCase) SetMasterCoverPhoto(ctx context.Context, userID, photoID uuid.UUID) (*domain.Master, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master profile not found")
	}
	if err := uc.repo.SetMasterCoverPhoto(ctx, m.ID, photoID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, pkgerrors.NotFound("photo not found")
		}
		return nil, err
	}
	return uc.repo.GetByUserID(ctx, userID)
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
