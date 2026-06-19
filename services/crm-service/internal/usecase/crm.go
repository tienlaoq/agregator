package usecase

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"

	"github.com/tienlao/agregator/services/crm-service/internal/domain"
	"github.com/tienlao/agregator/services/crm-service/internal/repository"
)

const (
	maxTaskTitleRunes = 200
	maxTaskBodyRunes  = 4000
)

// CRMUseCase orchestrates staff, CRM tasks and the guest projection.
type CRMUseCase struct {
	repo         domain.Repository
	vipThreshold int64 // guest total_spent at/above which the VIP segment applies; <=0 disables it
}

// Option configures a CRMUseCase at construction.
type Option func(*CRMUseCase)

// WithVIPThreshold sets the spend threshold (same unit as booking total_price)
// for the VIP guest segment. Zero or negative disables the VIP segment.
func WithVIPThreshold(v int64) Option {
	return func(uc *CRMUseCase) { uc.vipThreshold = v }
}

func New(repo domain.Repository, opts ...Option) *CRMUseCase {
	uc := &CRMUseCase{repo: repo}
	for _, o := range opts {
		o(uc)
	}
	return uc
}

func (uc *CRMUseCase) GetManagementAccess(ctx context.Context, venueID, userID uuid.UUID) (string, error) {
	return uc.repo.GetManagementAccess(ctx, venueID, userID)
}

func (uc *CRMUseCase) BatchGetManagementAccess(ctx context.Context, userID uuid.UUID, venueIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	return uc.repo.BatchGetManagementAccess(ctx, userID, venueIDs)
}

func (uc *CRMUseCase) ListManagedVenues(ctx context.Context, userID uuid.UUID) ([]domain.ManagedVenue, error) {
	return uc.repo.ListManagedVenues(ctx, userID)
}

// memberAccess returns the actor's management role for the venue, or
// PermissionDenied when the actor has no access. The role lets callers branch on
// capabilities (see domain.Can).
func (uc *CRMUseCase) memberAccess(ctx context.Context, venueID, userID uuid.UUID) (string, error) {
	access, err := uc.repo.GetManagementAccess(ctx, venueID, userID)
	if err != nil {
		return "", err
	}
	if access == "" {
		return "", pkgerrors.PermissionDenied("нет доступа к управлению заведением")
	}
	return access, nil
}

// ensureCanManageTasks authorizes task mutations that require CapManageAnyTask
// (owner|manager). Staff and non-members are denied.
func (uc *CRMUseCase) ensureCanManageTasks(ctx context.Context, venueID, actorID uuid.UUID) error {
	access, err := uc.repo.GetManagementAccess(ctx, venueID, actorID)
	if err != nil {
		return err
	}
	if !domain.Can(access, domain.CapManageAnyTask) {
		return pkgerrors.PermissionDenied("недостаточно прав для изменения задачи")
	}
	return nil
}

// validateTaskContent trims and bounds the title and body shared by create/update.
func validateTaskContent(title, body string) (string, string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", "", pkgerrors.InvalidArgument("укажите заголовок задачи")
	}
	if utf8.RuneCountInString(title) > maxTaskTitleRunes {
		return "", "", pkgerrors.InvalidArgument("заголовок слишком длинный")
	}
	body = strings.TrimSpace(body)
	if utf8.RuneCountInString(body) > maxTaskBodyRunes {
		return "", "", pkgerrors.InvalidArgument("описание слишком длинное")
	}
	return title, body, nil
}

// validateAssignee ensures that a non-nil assignee (other than the actor, who is
// already verified as a member) also manages the venue.
func (uc *CRMUseCase) validateAssignee(ctx context.Context, venueID, actorID uuid.UUID, assignee *uuid.UUID) error {
	if assignee == nil || *assignee == actorID {
		return nil
	}
	access, err := uc.repo.GetManagementAccess(ctx, venueID, *assignee)
	if err != nil {
		return err
	}
	if access == "" {
		return pkgerrors.InvalidArgument("исполнитель не входит в персонал заведения")
	}
	return nil
}

// ensureOwner verifies that actorID owns the venue. Returns the owner UUID
// from the venues row so callers can compare it against a target user.
func (uc *CRMUseCase) ensureOwner(ctx context.Context, venueID, actorID uuid.UUID) (uuid.UUID, error) {
	owner, err := uc.repo.VenueOwnerID(ctx, venueID)
	if err != nil {
		if errors.Is(err, repository.ErrVenueNotFound) {
			return uuid.Nil, pkgerrors.NotFound("заведение не найдено")
		}
		return uuid.Nil, err
	}
	if owner != actorID {
		return uuid.Nil, pkgerrors.PermissionDenied("только владелец может управлять персоналом")
	}
	return owner, nil
}

func (uc *CRMUseCase) ListStaff(ctx context.Context, venueID, actorID uuid.UUID) ([]domain.StaffMember, error) {
	if _, err := uc.memberAccess(ctx, venueID, actorID); err != nil {
		return nil, err
	}
	return uc.repo.ListStaff(ctx, venueID)
}

func (uc *CRMUseCase) AddStaff(ctx context.Context, venueID, actorID, targetUserID uuid.UUID, role string) error {
	owner, err := uc.ensureOwner(ctx, venueID, actorID)
	if err != nil {
		return err
	}
	if targetUserID == owner {
		return pkgerrors.InvalidArgument("владелец не может быть в списке персонала")
	}
	role = strings.TrimSpace(strings.ToLower(role))
	if role != domain.StaffRoleManager && role != domain.StaffRoleStaff {
		return pkgerrors.InvalidArgument("роль должна быть manager или staff")
	}
	return uc.repo.AddStaff(ctx, venueID, targetUserID, role, actorID)
}

func (uc *CRMUseCase) RemoveStaff(ctx context.Context, venueID, actorID, targetUserID uuid.UUID) error {
	owner, err := uc.ensureOwner(ctx, venueID, actorID)
	if err != nil {
		return err
	}
	if targetUserID == owner {
		return pkgerrors.InvalidArgument("нельзя удалить владельца")
	}
	if err := uc.repo.RemoveStaff(ctx, venueID, targetUserID); err != nil {
		if errors.Is(err, repository.ErrStaffNotFound) {
			return pkgerrors.NotFound("сотрудник не найден")
		}
		return err
	}
	return nil
}

func (uc *CRMUseCase) ListTasks(ctx context.Context, venueID, actorID uuid.UUID, status string) ([]domain.Task, error) {
	if _, err := uc.memberAccess(ctx, venueID, actorID); err != nil {
		return nil, err
	}
	status = strings.TrimSpace(strings.ToLower(status))
	if status != "" && status != domain.TaskStatusOpen && status != domain.TaskStatusDone && status != domain.TaskStatusCancelled {
		return nil, pkgerrors.InvalidArgument("неверный статус задачи")
	}
	return uc.repo.ListTasks(ctx, venueID, status)
}

func (uc *CRMUseCase) CreateTask(ctx context.Context, venueID, actorID uuid.UUID, title, body string, bookingID, assignee *uuid.UUID, priority string, dueAt *time.Time) (*domain.Task, error) {
	if _, err := uc.memberAccess(ctx, venueID, actorID); err != nil {
		return nil, err
	}
	title, body, err := validateTaskContent(title, body)
	if err != nil {
		return nil, err
	}
	prio, ok := domain.NormalizePriority(priority)
	if !ok {
		return nil, pkgerrors.InvalidArgument("приоритет должен быть low, normal или high")
	}
	if err := uc.validateAssignee(ctx, venueID, actorID, assignee); err != nil {
		return nil, err
	}
	t := &domain.Task{
		VenueID:        venueID,
		BookingID:      bookingID,
		Title:          title,
		Body:           body,
		Status:         domain.TaskStatusOpen,
		Priority:       prio,
		AssigneeUserID: assignee,
		DueAt:          dueAt,
		CreatedBy:      actorID,
	}
	if err := uc.repo.CreateTask(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTask edits a task's content. Manager/owner only (CapManageAnyTask).
func (uc *CRMUseCase) UpdateTask(ctx context.Context, venueID, actorID, taskID uuid.UUID, title, body string, assignee *uuid.UUID, priority string, dueAt *time.Time) (*domain.Task, error) {
	if err := uc.ensureCanManageTasks(ctx, venueID, actorID); err != nil {
		return nil, err
	}
	title, body, err := validateTaskContent(title, body)
	if err != nil {
		return nil, err
	}
	prio, ok := domain.NormalizePriority(priority)
	if !ok {
		return nil, pkgerrors.InvalidArgument("приоритет должен быть low, normal или high")
	}
	if err := uc.validateAssignee(ctx, venueID, actorID, assignee); err != nil {
		return nil, err
	}
	t := &domain.Task{
		ID:             taskID,
		VenueID:        venueID,
		Title:          title,
		Body:           body,
		Priority:       prio,
		AssigneeUserID: assignee,
		DueAt:          dueAt,
	}
	if err := uc.repo.UpdateTask(ctx, t); err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			return nil, pkgerrors.NotFound("задача не найдена")
		}
		return nil, err
	}
	return t, nil
}

func (uc *CRMUseCase) CompleteTask(ctx context.Context, venueID, actorID, taskID uuid.UUID) error {
	access, err := uc.memberAccess(ctx, venueID, actorID)
	if err != nil {
		return err
	}
	// Staff may close only their own tasks; managers and owners may close any.
	if !domain.Can(access, domain.CapManageAnyTask) {
		t, gerr := uc.repo.GetTask(ctx, venueID, taskID)
		if gerr != nil {
			if errors.Is(gerr, repository.ErrTaskNotFound) {
				return pkgerrors.NotFound("задача не найдена")
			}
			return gerr
		}
		if !t.OwnedBy(actorID) {
			return pkgerrors.PermissionDenied("можно закрывать только свои задачи")
		}
	}
	ok, err := uc.repo.CompleteTask(ctx, venueID, taskID, actorID)
	if err != nil {
		return err
	}
	if !ok {
		return pkgerrors.NotFound("задача не найдена или уже закрыта")
	}
	return nil
}

// ReopenTask transitions a done task back to open. Manager/owner only.
func (uc *CRMUseCase) ReopenTask(ctx context.Context, venueID, actorID, taskID uuid.UUID) (*domain.Task, error) {
	if err := uc.ensureCanManageTasks(ctx, venueID, actorID); err != nil {
		return nil, err
	}
	t, err := uc.repo.ReopenTask(ctx, venueID, taskID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			return nil, pkgerrors.NotFound("задача не найдена или не закрыта")
		}
		return nil, err
	}
	return t, nil
}

// CancelTask soft-deletes a task. Manager/owner only.
func (uc *CRMUseCase) CancelTask(ctx context.Context, venueID, actorID, taskID uuid.UUID) error {
	if err := uc.ensureCanManageTasks(ctx, venueID, actorID); err != nil {
		return err
	}
	ok, err := uc.repo.CancelTask(ctx, venueID, taskID)
	if err != nil {
		return err
	}
	if !ok {
		return pkgerrors.NotFound("задача не найдена")
	}
	return nil
}

// recentGuestBookings caps how many bookings the 360 timeline returns.
const recentGuestBookings = 20

// GuestView pairs a stored guest profile with its computed segment labels.
type GuestView struct {
	Profile  domain.GuestProfile
	Segments []string
}

func (uc *CRMUseCase) toGuestView(p domain.GuestProfile, now time.Time) GuestView {
	return GuestView{Profile: p, Segments: domain.ComputeSegments(&p, now, uc.vipThreshold)}
}

// ListGuests returns the venue's guest profiles (any member may view). Segments
// are computed per row from the configured VIP threshold and the current time.
func (uc *CRMUseCase) ListGuests(ctx context.Context, venueID, actorID uuid.UUID, segment, sort string, limit, offset int) ([]GuestView, int, error) {
	if _, err := uc.memberAccess(ctx, venueID, actorID); err != nil {
		return nil, 0, err
	}
	if segment != "" && !domain.ValidGuestSegment(segment) {
		return nil, 0, pkgerrors.InvalidArgument("неизвестный сегмент")
	}
	// VIP filtering is meaningless without a configured threshold.
	if segment == domain.SegmentVIP && uc.vipThreshold <= 0 {
		return []GuestView{}, 0, nil
	}
	profiles, total, err := uc.repo.ListGuests(ctx, venueID, domain.GuestListParams{
		Segment:      segment,
		Sort:         sort,
		Limit:        limit,
		Offset:       offset,
		VIPThreshold: uc.vipThreshold,
	})
	if err != nil {
		return nil, 0, err
	}
	now := time.Now()
	views := make([]GuestView, len(profiles))
	for i := range profiles {
		views[i] = uc.toGuestView(profiles[i], now)
	}
	return views, total, nil
}

// GetGuest returns one guest's profile with computed segments plus their recent
// bookings (the Customer 360 card). Any member may view.
func (uc *CRMUseCase) GetGuest(ctx context.Context, venueID, actorID, userID uuid.UUID) (*GuestView, []domain.GuestBookingSummary, error) {
	if _, err := uc.memberAccess(ctx, venueID, actorID); err != nil {
		return nil, nil, err
	}
	p, err := uc.repo.GetGuestProfile(ctx, venueID, userID)
	if err != nil {
		if errors.Is(err, repository.ErrGuestNotFound) {
			return nil, nil, pkgerrors.NotFound("гость не найден")
		}
		return nil, nil, err
	}
	bookings, err := uc.repo.ListGuestBookings(ctx, venueID, userID, recentGuestBookings)
	if err != nil {
		return nil, nil, err
	}
	view := uc.toGuestView(*p, time.Now())
	return &view, bookings, nil
}
