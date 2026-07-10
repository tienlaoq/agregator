package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

// slotBlockMaxAdvanceDays bounds how far ahead a block may be placed. A year is
// generous for vacation planning while still rejecting obvious garbage dates.
const slotBlockMaxAdvanceDays = 365

// CreateSlotBlock adds a schedule block for the authenticated master. An empty
// timeFrom/timeTo pair means the whole day is blocked; otherwise both must be
// set with timeTo strictly after timeFrom.
func (uc *MasterUseCase) CreateSlotBlock(ctx context.Context, userID uuid.UUID, date, timeFrom, timeTo, note string) (*domain.MasterSlotBlock, error) {
	date = strings.TrimSpace(date)
	timeFrom = strings.TrimSpace(timeFrom)
	timeTo = strings.TrimSpace(timeTo)
	note = strings.TrimSpace(note)

	if len([]rune(note)) > domain.MaxSlotBlockNote {
		return nil, pkgerrors.InvalidArgument(fmt.Sprintf("note must not exceed %d characters", domain.MaxSlotBlockNote))
	}
	if err := validateSlotBlockDate(date); err != nil {
		return nil, err
	}
	if err := validateSlotBlockInterval(timeFrom, timeTo); err != nil {
		return nil, err
	}

	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master profile not found")
	}

	b := &domain.MasterSlotBlock{
		ID:       uuid.New(),
		MasterID: m.ID,
		Date:     date,
		TimeFrom: timeFrom,
		TimeTo:   timeTo,
		Note:     note,
	}
	if err := uc.repo.InsertSlotBlock(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

// ListSlotBlocks returns the authenticated master's upcoming blocks.
func (uc *MasterUseCase) ListSlotBlocks(ctx context.Context, userID uuid.UUID) ([]domain.MasterSlotBlock, error) {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, pkgerrors.NotFound("master profile not found")
	}
	return uc.repo.ListSlotBlocksByMaster(ctx, m.ID)
}

// DeleteSlotBlock removes a block owned by the authenticated master.
func (uc *MasterUseCase) DeleteSlotBlock(ctx context.Context, userID, blockID uuid.UUID) error {
	m, err := uc.repo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if m == nil {
		return pkgerrors.NotFound("master profile not found")
	}
	if err := uc.repo.DeleteSlotBlock(ctx, m.ID, blockID); err != nil {
		if err == domain.ErrNotFound {
			return pkgerrors.NotFound("block not found")
		}
		return err
	}
	return nil
}

// validateSlotBlockDate accepts YYYY-MM-DD from today (Moscow time, UTC+3) up to
// slotBlockMaxAdvanceDays ahead. Past dates are rejected — blocking a past day
// is meaningless.
func validateSlotBlockDate(date string) error {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return pkgerrors.InvalidArgument("invalid date: expected YYYY-MM-DD")
	}
	const moscowOffset = 3 * time.Hour
	nowMoscow := time.Now().UTC().Add(moscowOffset)
	today := time.Date(nowMoscow.Year(), nowMoscow.Month(), nowMoscow.Day(), 0, 0, 0, 0, time.UTC)
	if parsed.Before(today) {
		return pkgerrors.InvalidArgument("date must not be in the past")
	}
	if parsed.After(today.AddDate(0, 0, slotBlockMaxAdvanceDays)) {
		return pkgerrors.InvalidArgument(fmt.Sprintf("date must be within %d days from today", slotBlockMaxAdvanceDays))
	}
	return nil
}

// validateSlotBlockInterval enforces the whole-day-or-valid-range invariant:
// both empty (whole day) is allowed; otherwise both must parse as HH:MM with
// timeTo strictly after timeFrom.
func validateSlotBlockInterval(timeFrom, timeTo string) error {
	if timeFrom == "" && timeTo == "" {
		return nil
	}
	if timeFrom == "" || timeTo == "" {
		return pkgerrors.InvalidArgument("both time_from and time_to are required, or leave both empty for a whole-day block")
	}
	start, err := time.Parse("15:04", timeFrom)
	if err != nil {
		return pkgerrors.InvalidArgument("invalid time_from: expected HH:MM")
	}
	end, err := time.Parse("15:04", timeTo)
	if err != nil {
		return pkgerrors.InvalidArgument("invalid time_to: expected HH:MM")
	}
	if !end.After(start) {
		return pkgerrors.InvalidArgument("time_to must be later than time_from")
	}
	return nil
}
