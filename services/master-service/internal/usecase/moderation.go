package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

func (uc *MasterUseCase) ListForModeration(ctx context.Context, statusFilter string, limit, offset int32) ([]domain.Master, int32, error) {
	return uc.repo.ListByStatus(ctx, statusFilter, limit, offset)
}

// SuspendByUser suspends the master profile owned by userID (account-deletion
// cascade). Returns true when a profile was transitioned; a user with no
// profile is a normal no-op (false, nil).
func (uc *MasterUseCase) SuspendByUser(ctx context.Context, userID uuid.UUID) (bool, error) {
	return uc.repo.SuspendByUser(ctx, userID)
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
