package usecase

import (
	"context"
	"errors"
	"strings"
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

// CRMUseCase orchestrates staff and CRM task operations.
type CRMUseCase struct {
	repo domain.Repository
}

func New(repo domain.Repository) *CRMUseCase {
	return &CRMUseCase{repo: repo}
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

func (uc *CRMUseCase) ensureMember(ctx context.Context, venueID, userID uuid.UUID) error {
	access, err := uc.repo.GetManagementAccess(ctx, venueID, userID)
	if err != nil {
		return err
	}
	if access == "" {
		return pkgerrors.PermissionDenied("нет доступа к управлению заведением")
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
	if err := uc.ensureMember(ctx, venueID, actorID); err != nil {
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
	if err := uc.ensureMember(ctx, venueID, actorID); err != nil {
		return nil, err
	}
	status = strings.TrimSpace(strings.ToLower(status))
	if status != "" && status != domain.TaskStatusOpen && status != domain.TaskStatusDone {
		return nil, pkgerrors.InvalidArgument("неверный статус задачи")
	}
	return uc.repo.ListTasks(ctx, venueID, status)
}

func (uc *CRMUseCase) CreateTask(ctx context.Context, venueID, actorID uuid.UUID, title, body string, bookingID, assignee *uuid.UUID) (*domain.Task, error) {
	if err := uc.ensureMember(ctx, venueID, actorID); err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, pkgerrors.InvalidArgument("укажите заголовок задачи")
	}
	if utf8.RuneCountInString(title) > maxTaskTitleRunes {
		return nil, pkgerrors.InvalidArgument("заголовок слишком длинный")
	}
	body = strings.TrimSpace(body)
	if utf8.RuneCountInString(body) > maxTaskBodyRunes {
		return nil, pkgerrors.InvalidArgument("описание слишком длинное")
	}
	// Assignee must manage this venue; actor was already verified above.
	if assignee != nil && *assignee != actorID {
		access, err := uc.repo.GetManagementAccess(ctx, venueID, *assignee)
		if err != nil {
			return nil, err
		}
		if access == "" {
			return nil, pkgerrors.InvalidArgument("исполнитель не входит в персонал заведения")
		}
	}
	t := &domain.Task{
		VenueID:        venueID,
		BookingID:      bookingID,
		Title:          title,
		Body:           body,
		Status:         domain.TaskStatusOpen,
		AssigneeUserID: assignee,
		CreatedBy:      actorID,
	}
	if err := uc.repo.CreateTask(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (uc *CRMUseCase) CompleteTask(ctx context.Context, venueID, actorID, taskID uuid.UUID) error {
	if err := uc.ensureMember(ctx, venueID, actorID); err != nil {
		return err
	}
	ok, err := uc.repo.CompleteTask(ctx, venueID, taskID)
	if err != nil {
		return err
	}
	if !ok {
		return pkgerrors.NotFound("задача не найдена или уже закрыта")
	}
	return nil
}
