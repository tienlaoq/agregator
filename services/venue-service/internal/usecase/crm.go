package usecase

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
	"github.com/tienlao/agregator/services/venue-service/internal/repository"
)

const (
	maxCRMTaskTitleRunes = 200
	maxCRMTaskBodyRunes  = 4000
)

func (uc *VenueUseCase) ListForManagingUser(ctx context.Context, userID uuid.UUID) ([]domain.Venue, error) {
	return uc.repo.ListForManagingUser(ctx, userID)
}

func (uc *VenueUseCase) GetVenueManagementAccess(ctx context.Context, venueID, userID uuid.UUID) (string, error) {
	return uc.repo.GetVenueManagementAccess(ctx, venueID, userID)
}

func (uc *VenueUseCase) ensureVenueCRMMember(ctx context.Context, venueID, userID uuid.UUID) error {
	access, err := uc.repo.GetVenueManagementAccess(ctx, venueID, userID)
	if err != nil {
		return err
	}
	if access == "" {
		return pkgerrors.PermissionDenied("нет доступа к управлению заведением")
	}
	return nil
}

func (uc *VenueUseCase) ListVenueStaff(ctx context.Context, venueID, actorID uuid.UUID) ([]domain.VenueStaff, error) {
	if err := uc.ensureVenueCRMMember(ctx, venueID, actorID); err != nil {
		return nil, err
	}
	return uc.repo.ListVenueStaff(ctx, venueID)
}

func (uc *VenueUseCase) AddVenueStaff(ctx context.Context, venueID, actorID, targetUserID uuid.UUID, role string) error {
	v, err := uc.ensureVenueOwner(ctx, venueID, actorID)
	if err != nil {
		return err
	}
	if targetUserID == v.OwnerID {
		return pkgerrors.InvalidArgument("владелец не может быть в списке персонала")
	}
	role = strings.TrimSpace(strings.ToLower(role))
	if role != domain.VenueCRMStaffRoleManager && role != domain.VenueCRMStaffRoleStaff {
		return pkgerrors.InvalidArgument("роль должна быть manager или staff")
	}
	if err := uc.repo.AddVenueStaff(ctx, venueID, targetUserID, role, actorID); err != nil {
		return err
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return nil
}

func (uc *VenueUseCase) RemoveVenueStaff(ctx context.Context, venueID, actorID, targetUserID uuid.UUID) error {
	v, err := uc.ensureVenueOwner(ctx, venueID, actorID)
	if err != nil {
		return err
	}
	if targetUserID == v.OwnerID {
		return pkgerrors.InvalidArgument("нельзя удалить владельца")
	}
	if err := uc.repo.RemoveVenueStaff(ctx, venueID, targetUserID); err != nil {
		if errors.Is(err, repository.ErrVenueStaffNotFound) {
			return pkgerrors.NotFound("сотрудник не найден")
		}
		return err
	}
	uc.invalidateCache(ctx, venueID, v.Slug)
	return nil
}

func (uc *VenueUseCase) ListVenueCRMTasks(ctx context.Context, venueID, actorID uuid.UUID, status string) ([]domain.VenueCRMTask, error) {
	if err := uc.ensureVenueCRMMember(ctx, venueID, actorID); err != nil {
		return nil, err
	}
	status = strings.TrimSpace(strings.ToLower(status))
	if status != "" && status != domain.VenueCRMTaskStatusOpen && status != domain.VenueCRMTaskStatusDone {
		return nil, pkgerrors.InvalidArgument("неверный статус задачи")
	}
	return uc.repo.ListVenueCRMTasks(ctx, venueID, status)
}

func (uc *VenueUseCase) CreateVenueCRMTask(ctx context.Context, venueID, actorID uuid.UUID, title, body string, bookingID, assignee *uuid.UUID) (*domain.VenueCRMTask, error) {
	if err := uc.ensureVenueCRMMember(ctx, venueID, actorID); err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, pkgerrors.InvalidArgument("укажите заголовок задачи")
	}
	if utf8.RuneCountInString(title) > maxCRMTaskTitleRunes {
		return nil, pkgerrors.InvalidArgument("заголовок слишком длинный")
	}
	if utf8.RuneCountInString(body) > maxCRMTaskBodyRunes {
		return nil, pkgerrors.InvalidArgument("описание слишком длинное")
	}
	t := &domain.VenueCRMTask{
		VenueID:        venueID,
		BookingID:      bookingID,
		Title:          title,
		Body:           body,
		Status:         domain.VenueCRMTaskStatusOpen,
		AssigneeUserID: assignee,
		CreatedBy:      actorID,
	}
	if err := uc.repo.CreateVenueCRMTask(ctx, t); err != nil {
		return nil, err
	}
	v, _ := uc.repo.GetByID(ctx, venueID)
	if v != nil {
		uc.invalidateCache(ctx, venueID, v.Slug)
	}
	return t, nil
}

func (uc *VenueUseCase) CompleteVenueCRMTask(ctx context.Context, venueID, actorID, taskID uuid.UUID) error {
	if err := uc.ensureVenueCRMMember(ctx, venueID, actorID); err != nil {
		return err
	}
	ok, err := uc.repo.CompleteVenueCRMTask(ctx, venueID, taskID)
	if err != nil {
		return err
	}
	if !ok {
		return pkgerrors.NotFound("задача не найдена или уже закрыта")
	}
	v, _ := uc.repo.GetByID(ctx, venueID)
	if v != nil {
		uc.invalidateCache(ctx, venueID, v.Slug)
	}
	return nil
}
