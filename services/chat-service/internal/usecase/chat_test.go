package usecase

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tienlao/agregator/services/chat-service/internal/domain"
)

type mockRepo struct {
	thread           *domain.Thread
	lastClientMsgID string
}

func (m *mockRepo) EnsureThread(ctx context.Context, kind string, refID uuid.UUID, participantUserIDs []string) (*domain.Thread, error) {
	t := &domain.Thread{
		ID:                 uuid.New(),
		Kind:               kind,
		RefID:              refID,
		ParticipantUserIDs: participantUserIDs,
	}
	m.thread = t
	return t, nil
}
func (m *mockRepo) GetThreadByID(ctx context.Context, threadID uuid.UUID) (*domain.Thread, error) {
	return m.thread, nil
}
func (m *mockRepo) ListThreadsForUser(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]domain.Thread, int32, error) {
	return nil, 0, nil
}
func (m *mockRepo) ListMessages(ctx context.Context, threadID uuid.UUID, limit, offset int32) ([]domain.Message, int32, error) {
	return nil, 0, nil
}
func (m *mockRepo) InsertMessage(ctx context.Context, threadID, authorUserID uuid.UUID, text, clientMsgID string) (*domain.Message, error) {
	m.lastClientMsgID = clientMsgID
	return &domain.Message{
		ID: uuid.New(), ThreadID: threadID, AuthorUserID: authorUserID, Text: text, ClientMsgID: clientMsgID, CreatedAt: time.Now(),
	}, nil
}
func (m *mockRepo) MarkRead(ctx context.Context, threadID, userID uuid.UUID) error { return nil }

func TestEnsureThread_SortsParticipants(t *testing.T) {
	r := &mockRepo{}
	uc := New(r)
	a := uuid.New().String()
	b := uuid.New().String()
	_, err := uc.EnsureThread(context.Background(), domain.ThreadKindVenueBooking, uuid.New().String(), []string{b, a, b})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.thread.ParticipantUserIDs) != 2 {
		t.Fatalf("want 2 participants, got %d", len(r.thread.ParticipantUserIDs))
	}
	if !slices.IsSorted(r.thread.ParticipantUserIDs) {
		t.Fatalf("participants are not sorted: %#v", r.thread.ParticipantUserIDs)
	}
	if !slices.Contains(r.thread.ParticipantUserIDs, a) || !slices.Contains(r.thread.ParticipantUserIDs, b) {
		t.Fatalf("participants are not sorted/deduped: %#v", r.thread.ParticipantUserIDs)
	}
}

func TestSendMessage_DeniesNonParticipant(t *testing.T) {
	r := &mockRepo{
		thread: &domain.Thread{
			ID:                 uuid.New(),
			ParticipantUserIDs: []string{uuid.New().String(), uuid.New().String()},
		},
	}
	uc := New(r)
	_, _, err := uc.SendMessage(context.Background(), r.thread.ID.String(), uuid.New().String(), "hello", "")
	if err == nil {
		t.Fatal("expected permission error")
	}
}

func TestSendMessage_PassesClientMsgID(t *testing.T) {
	uid := uuid.New().String()
	r := &mockRepo{
		thread: &domain.Thread{
			ID:                 uuid.New(),
			ParticipantUserIDs: []string{uid, uuid.New().String()},
		},
	}
	uc := New(r)
	_, _, err := uc.SendMessage(context.Background(), r.thread.ID.String(), uid, "hello", "msg-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.lastClientMsgID != "msg-123" {
		t.Fatalf("client msg id not passed, got %q", r.lastClientMsgID)
	}
}
