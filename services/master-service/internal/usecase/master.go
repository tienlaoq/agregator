package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/pkg/geo"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

type MasterRepo interface {
	Insert(ctx context.Context, m *domain.Master) error
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.Master, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Master, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Master, error)
	UpdateProfile(ctx context.Context, m *domain.Master) error
	UpdateStatus(ctx context.Context, masterID uuid.UUID, status, comment string, moderatedBy *uuid.UUID) error
	ListByStatus(ctx context.Context, statusFilter string, limit, offset int32) ([]domain.Master, int32, error)
	ListPublic(ctx context.Context, params domain.ListPublicMastersParams) ([]domain.Master, int32, error)
	ReplaceServices(ctx context.Context, masterID uuid.UUID, items []domain.MasterServiceUpsert) error
	InsertModerationHistory(ctx context.Context, e *domain.ModerationHistoryEntry) error
	ListModerationHistory(ctx context.Context, masterID uuid.UUID, limit int32) ([]domain.ModerationHistoryEntry, error)
	InsertBooking(ctx context.Context, b *domain.MasterBooking) error
	ListBookingsByMaster(ctx context.Context, masterID uuid.UUID, statusFilter string) ([]domain.MasterBooking, error)
	NewSlug(ctx context.Context, displayName string) (string, error)

	CountPhotosByMaster(ctx context.Context, masterID uuid.UUID) (int32, error)
	AddMasterPhoto(ctx context.Context, masterID uuid.UUID, url string) (*domain.MasterPhoto, error)
	DeleteMasterPhoto(ctx context.Context, masterID, photoID uuid.UUID) (deletedURL string, err error)
	SetMasterCoverPhoto(ctx context.Context, masterID, photoID uuid.UUID) error
}

type MasterUseCase struct {
	repo MasterRepo
}

func NewMasterUseCase(repo MasterRepo) *MasterUseCase {
	return &MasterUseCase{repo: repo}
}

const maxMasterPhotos int32 = 12

func masterPhotoURLPrefix(masterID uuid.UUID) string {
	return fmt.Sprintf("/api/v1/uploads/masters/%s/", masterID.String())
}

func validateMasterPhotoURL(masterID uuid.UUID, url string) error {
	url = strings.TrimSpace(url)
	if url == "" || strings.Contains(url, "..") {
		return fmt.Errorf("invalid photo url")
	}
	want := masterPhotoURLPrefix(masterID)
	if !strings.HasPrefix(url, want) {
		return fmt.Errorf("photo url must be under master upload path")
	}
	return nil
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
	slug, err := uc.repo.NewSlug(ctx, displayName)
	if err != nil {
		return nil, err
	}
	m := &domain.Master{
		ID:               uuid.New(),
		UserID:           userID,
		Slug:             slug,
		DisplayName:      displayName,
		Bio:              "",
		Phone:            "",
		City:             "",
		WorkFormat:       domain.WorkFormatBoth,
		Specializations:  []string{},
		Status:           domain.StatusDraft,
		AvailabilityJSON: "{}",
	}
	if err := uc.repo.Insert(ctx, m); err != nil {
		return nil, err
	}
	out, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type UpdateMasterInput struct {
	DisplayName      *string
	Bio              *string
	Phone            *string
	City             *string
	WorkFormat       *string
	TravelRadiusKm         *int32
	TravelBaseLatitude     *float64
	TravelBaseLongitude    *float64
	ExperienceYears        *int32
	HourlyRate             *int64
	AvailabilityJSON       *string
	ApplyServicesReplace   bool
	ServicesReplace        []domain.MasterServiceUpsert
	ApplySpecializations   bool
	Specializations        []string
	PayoutLegalForm        *string
	ApplyTravelExcludeZones bool
	TravelExcludeZones      []domain.MasterTravelExcludeZone
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
		m.Phone = strings.TrimSpace(*in.Phone)
	}
	if in.City != nil {
		m.City = strings.TrimSpace(*in.City)
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
		m.AvailabilityJSON = s
	}
	if in.PayoutLegalForm != nil {
		v := strings.TrimSpace(strings.ToLower(*in.PayoutLegalForm))
		// До миграции 005 в БД было «gph»; в UI и proto — «individual».
		if v == "gph" {
			v = domain.PayoutLegalFormIndividual
		}
		switch v {
		case "", domain.PayoutLegalFormIP, domain.PayoutLegalFormOOO, domain.PayoutLegalFormIndividual, domain.PayoutLegalFormSelfEmployed:
			m.PayoutLegalForm = v
		default:
			return nil, pkgerrors.InvalidArgument("invalid payout_legal_form")
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

	if m.WorkFormat == domain.WorkFormatVenue {
		m.TravelBaseLatitude = nil
		m.TravelBaseLongitude = nil
		m.TravelRadiusKm = 0
		m.TravelExcludeZones = nil
	}

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
		if err := uc.repo.ReplaceServices(ctx, m.ID, in.ServicesReplace); err != nil {
			return nil, err
		}
	}

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
		// ok
	case domain.StatusPendingReview:
		return m, nil
	default:
		return nil, pkgerrors.InvalidArgument("profile cannot be submitted in current status: " + m.Status)
	}
	if err := validateReadyForReview(m); err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	old := m.Status
	m.Status = domain.StatusPendingReview
	m.ModerationComment = ""
	m.ModeratedBy = nil
	m.ModeratedAt = nil
	if err := uc.repo.UpdateProfile(ctx, m); err != nil {
		return nil, err
	}
	_ = uc.repo.InsertModerationHistory(ctx, &domain.ModerationHistoryEntry{
		MasterID:  m.ID,
		OldStatus: old,
		NewStatus: domain.StatusPendingReview,
		Comment:   "submitted by master",
		ChangedBy: userID,
	})
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

func validateTravelBaseForProfile(m *domain.Master) error {
	if !needsTravelBaseForFormat(m.WorkFormat) {
		return nil
	}
	if m.TravelRadiusKm <= 0 {
		return fmt.Errorf("укажите расстояние в километрах от метки на карте")
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
		if z.RadiusKm < minExcludeRadiusKm || z.RadiusKm > maxExcludeRadiusKm {
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
	if normalizeRussianMobileDigits(m.Phone) == "" {
		return fmt.Errorf("укажите полный номер телефона в формате +7 (999) 123-45-67")
	}
	bioTrim := strings.TrimSpace(m.Bio)
	if bioTrim == "" || len([]rune(bioTrim)) < 20 {
		return fmt.Errorf("описание должно быть не короче 20 символов")
	}
	if len(m.Services) == 0 {
		return fmt.Errorf("добавьте хотя бы одну услугу")
	}
	plf := strings.TrimSpace(strings.ToLower(m.PayoutLegalForm))
	if plf == "gph" {
		plf = domain.PayoutLegalFormIndividual
	}
	switch plf {
	case domain.PayoutLegalFormIP, domain.PayoutLegalFormOOO, domain.PayoutLegalFormIndividual, domain.PayoutLegalFormSelfEmployed:
	default:
		return fmt.Errorf("укажите форму получения выплат: ИП, ООО, физическое лицо или самозанятость")
	}
	if err := validateTravelBaseForProfile(m); err != nil {
		return err
	}
	if err := validateTravelExcludeZonesForProfile(m); err != nil {
		return err
	}
	return nil
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

	old := m.Status
	if err := uc.repo.UpdateStatus(ctx, masterID, newStatus, comment, &moderatorID); err != nil {
		return nil, err
	}
	_ = uc.repo.InsertModerationHistory(ctx, &domain.ModerationHistoryEntry{
		MasterID:  masterID,
		OldStatus: old,
		NewStatus: newStatus,
		Comment:   comment,
		ChangedBy: moderatorID,
	})
	return uc.repo.GetByID(ctx, masterID)
}

func (uc *MasterUseCase) ListModerationHistory(ctx context.Context, masterID uuid.UUID, limit int32) ([]domain.ModerationHistoryEntry, error) {
	return uc.repo.ListModerationHistory(ctx, masterID, limit)
}

func (uc *MasterUseCase) CreateBooking(ctx context.Context, clientUserID uuid.UUID, masterSlug string, serviceID *uuid.UUID, date, timeFrom, timeTo, comment string) (*domain.MasterBooking, error) {
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
	}
	if err := uc.repo.InsertBooking(ctx, b); err != nil {
		return nil, err
	}
	list, err := uc.repo.ListBookingsByMaster(ctx, m.ID, "")
	if err != nil {
		return b, nil
	}
	for i := range list {
		if list[i].ID == b.ID {
			return &list[i], nil
		}
	}
	return b, nil
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

func (uc *MasterUseCase) AddMasterPhoto(ctx context.Context, userID uuid.UUID, url string) (*domain.Master, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master profile not found")
	}
	if err := validateMasterPhotoURL(m.ID, url); err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	n, err := uc.repo.CountPhotosByMaster(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	if n >= maxMasterPhotos {
		return nil, pkgerrors.InvalidArgument("too many photos (max 12)")
	}
	if _, err := uc.repo.AddMasterPhoto(ctx, m.ID, strings.TrimSpace(url)); err != nil {
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
		if errors.Is(err, pgx.ErrNoRows) {
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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pkgerrors.NotFound("photo not found")
		}
		return nil, err
	}
	return uc.repo.GetByUserID(ctx, userID)
}
