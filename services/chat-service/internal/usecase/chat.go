package usecase

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"

	"github.com/google/uuid"
	pkgerrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/chat-service/internal/domain"
)

type Repo interface {
	domain.Repository
}

// Publisher publishes domain events asynchronously (P1#16).
// ctx is forwarded to the broker — a cancelled or timed-out context causes
// Publish to return immediately instead of blocking on a stalled broker.
// A nil implementation is safe — events are simply dropped.
type Publisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

// noopPublisher is used when NATS is not configured (tests, local dev).
type noopPublisher struct{}

func (noopPublisher) Publish(context.Context, string, []byte) error { return nil }

type ChatUseCase struct {
	repo Repo
	pub  Publisher
}

// New creates a ChatUseCase without NATS (tests / local dev).
func New(repo Repo) *ChatUseCase {
	return &ChatUseCase{repo: repo, pub: noopPublisher{}}
}

// NewWithPublisher creates a ChatUseCase that publishes domain events to NATS.
func NewWithPublisher(repo Repo, pub Publisher) *ChatUseCase {
	if pub == nil {
		pub = noopPublisher{}
	}
	return &ChatUseCase{repo: repo, pub: pub}
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

// EnsureThread creates or returns an existing chat thread for the given ref.
// actorUserID must be non-empty and must be present in participants — the service
// does not trust the caller to enforce access; it validates here so that even a
// rogue internal client cannot create threads on behalf of arbitrary users.
func (uc *ChatUseCase) EnsureThread(ctx context.Context, kind, refID, actorUserID string, participants []string) (*domain.Thread, error) {
	kind = strings.TrimSpace(kind)
	if kind != domain.ThreadKindVenueBooking && kind != domain.ThreadKindMasterBooking {
		return nil, pkgerrors.InvalidArgument("invalid thread kind")
	}
	refUUID, err := uuid.Parse(strings.TrimSpace(refID))
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid ref_id")
	}
	actorUID, err := uuid.Parse(strings.TrimSpace(actorUserID))
	if err != nil || actorUID == uuid.Nil {
		return nil, pkgerrors.InvalidArgument("invalid actor_user_id")
	}
	ps, err := normalizeParticipants(participants)
	if err != nil {
		return nil, err
	}
	// Verify the actor is actually one of the participants so that no caller
	// can open a thread they are not part of.
	actorStr := actorUID.String()
	found := false
	for _, p := range ps {
		if p == actorStr {
			found = true
			break
		}
	}
	if !found {
		return nil, pkgerrors.PermissionDenied("actor must be a thread participant")
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

// isParticipant checks membership. Both sides are compared after lower-casing so
// that an uppercase UUID from an auth-service response never causes a false miss
// (P1#12).
func isParticipant(t *domain.Thread, userID string) bool {
	uid := strings.ToLower(strings.TrimSpace(userID))
	for _, p := range t.ParticipantUserIDs {
		if strings.ToLower(p) == uid {
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
	// One GetThreadByID to verify membership before writing.
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
	// InsertMessage now returns the refreshed thread in the same transaction,
	// eliminating the second GetThreadByID and the race window (P1#8, P1#13).
	m, t, err := uc.repo.InsertMessage(ctx, tid, uid, text, clientMsgID)
	if err != nil {
		return nil, nil, err
	}

	// P1#16: publish a domain event so downstream services (email, push) can
	// consume it independently of the gateway.  Best-effort: a publish failure
	// does not roll back the message — the message is already persisted.
	uc.publishMessageCreated(ctx, m, t)

	return m, t, nil
}

// publishMessageCreated fires chat.message.created on NATS JetStream.
// Errors are logged but not returned to the caller.
func (uc *ChatUseCase) publishMessageCreated(ctx context.Context, m *domain.Message, t *domain.Thread) {
	payload, err := json.Marshal(map[string]any{
		"event":          "chat.message.created",
		"message_id":     m.ID.String(),
		"thread_id":      m.ThreadID.String(),
		"author_user_id": m.AuthorUserID.String(),
		"thread_kind":    t.Kind,
		"ref_id":         t.RefID.String(),
		"participants":   t.ParticipantUserIDs,
	})
	if err != nil {
		slog.ErrorContext(ctx, "chat: marshal message.created event failed", "err", err)
		return
	}
	if err := uc.pub.Publish(ctx, "chat.message.created", payload); err != nil {
		slog.ErrorContext(ctx, "chat: publish message.created failed", "message_id", m.ID, "err", err)
	}
}

// MarkRead records that userID has read up to the current last message in the
// thread and returns the refreshed thread state.
//
// N4 — watermark race: the watermark is resolved server-side as
// chat_threads.last_message_id at write time.  See repository.MarkRead for the
// full trade-off discussion.  When clients start sending the ID of the last
// message they actually rendered, thread that value through here and down to
// the repository to close the race without a schema change.
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
