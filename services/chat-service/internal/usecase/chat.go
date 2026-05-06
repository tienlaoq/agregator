package usecase

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/chat-service/internal/domain"
)

type Repo interface {
	domain.Repository
}

type ChatUseCase struct {
	repo Repo
}

func New(repo Repo) *ChatUseCase {
	return &ChatUseCase{repo: repo}
}

func normalizeParticipants(ids []string) ([]string, error) {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, s := range ids {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		u, err := uuid.Parse(s)
		if err != nil {
			return nil, pkgerrors.InvalidArgument("invalid participant_user_id")
		}
		s = u.String() // canonical lowercase UUID representation
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) < 2 {
		return nil, pkgerrors.InvalidArgument("at least 2 participants required")
	}
	slices.Sort(out)
	return out, nil
}

func (uc *ChatUseCase) EnsureThread(ctx context.Context, kind, refID string, participants []string) (*domain.Thread, error) {
	kind = strings.TrimSpace(kind)
	if kind != domain.ThreadKindVenueBooking && kind != domain.ThreadKindMasterBooking {
		return nil, pkgerrors.InvalidArgument("invalid thread kind")
	}
	refUUID, err := uuid.Parse(strings.TrimSpace(refID))
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid ref_id")
	}
	ps, err := normalizeParticipants(participants)
	if err != nil {
		return nil, err
	}
	return uc.repo.EnsureThread(ctx, kind, refUUID, ps)
}

func (uc *ChatUseCase) ListThreads(ctx context.Context, userID string, limit, offset int32) ([]domain.Thread, int32, error) {
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, 0, pkgerrors.InvalidArgument("invalid user_id")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return uc.repo.ListThreadsForUser(ctx, uid, limit, offset)
}

func isParticipant(t *domain.Thread, userID string) bool {
	for _, p := range t.ParticipantUserIDs {
		if p == userID {
			return true
		}
	}
	return false
}

func (uc *ChatUseCase) ListMessages(ctx context.Context, threadID, userID string, limit, offset int32) ([]domain.Message, int32, error) {
	tid, err := uuid.Parse(strings.TrimSpace(threadID))
	if err != nil {
		return nil, 0, pkgerrors.InvalidArgument("invalid thread_id")
	}
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, 0, pkgerrors.InvalidArgument("invalid user_id")
	}
	t, err := uc.repo.GetThreadByID(ctx, tid)
	if err != nil {
		return nil, 0, err
	}
	if t == nil {
		return nil, 0, pkgerrors.NotFound("thread not found")
	}
	if !isParticipant(t, uid.String()) {
		return nil, 0, pkgerrors.PermissionDenied("access denied")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return uc.repo.ListMessages(ctx, tid, limit, offset)
}

func (uc *ChatUseCase) SendMessage(ctx context.Context, threadID, userID, text, clientMsgID string) (*domain.Message, *domain.Thread, error) {
	tid, err := uuid.Parse(strings.TrimSpace(threadID))
	if err != nil {
		return nil, nil, pkgerrors.InvalidArgument("invalid thread_id")
	}
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, nil, pkgerrors.InvalidArgument("invalid user_id")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil, pkgerrors.InvalidArgument("message text is required")
	}
	if len([]rune(text)) > 3000 {
		return nil, nil, pkgerrors.InvalidArgument("message text too long")
	}
	t, err := uc.repo.GetThreadByID(ctx, tid)
	if err != nil {
		return nil, nil, err
	}
	if t == nil {
		return nil, nil, pkgerrors.NotFound("thread not found")
	}
	if !isParticipant(t, uid.String()) {
		return nil, nil, pkgerrors.PermissionDenied("access denied")
	}
	m, err := uc.repo.InsertMessage(ctx, tid, uid, text, clientMsgID)
	if err != nil {
		return nil, nil, err
	}
	t, err = uc.repo.GetThreadByID(ctx, tid)
	if err != nil {
		return nil, nil, err
	}
	return m, t, nil
}

func (uc *ChatUseCase) MarkRead(ctx context.Context, threadID, userID string) (*domain.Thread, error) {
	tid, err := uuid.Parse(strings.TrimSpace(threadID))
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid thread_id")
	}
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid user_id")
	}
	t, err := uc.repo.GetThreadByID(ctx, tid)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, pkgerrors.NotFound("thread not found")
	}
	if !isParticipant(t, uid.String()) {
		return nil, pkgerrors.PermissionDenied("access denied")
	}
	if err := uc.repo.MarkRead(ctx, tid, uid); err != nil {
		return nil, err
	}
	return uc.repo.GetThreadByID(ctx, tid)
}
