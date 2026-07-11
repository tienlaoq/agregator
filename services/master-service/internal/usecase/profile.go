package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/pkg/geo"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

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

// validateMasterCredentials checks the certificate/award list supplied to
// ReplaceCredentials before it reaches the repository. Limits mirror the domain
// constants and the CHECK constraints in migration 021 so the application and DB
// layers stay in sync. It returns the normalised items (trimmed strings, default
// kind) so the caller persists exactly what was validated.
func validateMasterCredentials(items []domain.MasterCredentialUpsert) ([]domain.MasterCredentialUpsert, error) {
	if int32(len(items)) > domain.MaxCredentialsPerMaster {
		return nil, pkgerrors.InvalidArgument(fmt.Sprintf(
			"too many credentials: %d provided, max %d", len(items), domain.MaxCredentialsPerMaster,
		))
	}
	out := make([]domain.MasterCredentialUpsert, 0, len(items))
	for i, it := range items {
		kind := strings.TrimSpace(it.Kind)
		if kind == "" {
			kind = domain.CredentialKindCertificate
		}
		if kind != domain.CredentialKindCertificate && kind != domain.CredentialKindAward {
			return nil, pkgerrors.InvalidArgument(fmt.Sprintf("credential[%d]: invalid kind %q", i, kind))
		}
		title := strings.TrimSpace(it.Title)
		if title == "" {
			return nil, pkgerrors.InvalidArgument(fmt.Sprintf("credential[%d]: title is required", i))
		}
		if len([]rune(title)) > domain.MaxCredentialTitle {
			return nil, pkgerrors.InvalidArgument(fmt.Sprintf(
				"credential[%d]: title exceeds %d characters", i, domain.MaxCredentialTitle,
			))
		}
		issuer := strings.TrimSpace(it.Issuer)
		if len([]rune(issuer)) > domain.MaxCredentialIssuer {
			return nil, pkgerrors.InvalidArgument(fmt.Sprintf(
				"credential[%d]: issuer exceeds %d characters", i, domain.MaxCredentialIssuer,
			))
		}
		if it.Year != 0 && (it.Year < domain.MinCredentialYear || it.Year > domain.MaxCredentialYear) {
			return nil, pkgerrors.InvalidArgument(fmt.Sprintf(
				"credential[%d]: year must be between %d and %d", i, domain.MinCredentialYear, domain.MaxCredentialYear,
			))
		}
		out = append(out, domain.MasterCredentialUpsert{
			Kind:      kind,
			Title:     title,
			Issuer:    issuer,
			Year:      it.Year,
			SortOrder: int32(i),
		})
	}
	return out, nil
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
	ApplyCredentialsReplace    bool
	CredentialsReplace         []domain.MasterCredentialUpsert
	ApplySpecializations       bool
	Specializations            []string
	PayoutLegalForm            *string
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

	// Validate the replaceable sub-payloads BEFORE the first write. Both are pure
	// functions of the input, so running them here means an invalid services or
	// credentials list is rejected without UpdateProfile having already persisted
	// the scalar fields and flipped status active → pending_review.
	// validateMasterCredentials also returns the normalised items to persist.
	var normalizedCreds []domain.MasterCredentialUpsert
	if in.ApplyServicesReplace {
		if err := validateMasterServices(in.ServicesReplace); err != nil {
			return nil, err
		}
	}
	if in.ApplyCredentialsReplace {
		var err error
		normalizedCreds, err = validateMasterCredentials(in.CredentialsReplace)
		if err != nil {
			return nil, err
		}
	}

	// Single transaction: the master row plus any services/credentials replace
	// commit together or not at all, so a failure mid-save can't leave the profile
	// flipped to pending_review with stale associations.
	if err := uc.repo.UpdateProfileWithAssociations(ctx, m,
		in.ApplyServicesReplace, in.ServicesReplace,
		in.ApplyCredentialsReplace, normalizedCreds,
	); err != nil {
		return nil, err
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
