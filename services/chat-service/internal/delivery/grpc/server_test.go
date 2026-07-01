package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	"github.com/tienlao/agregator/services/chat-service/internal/domain"
	"github.com/tienlao/agregator/services/chat-service/internal/usecase"
)

// mockRepo implements domain.Repository via function fields.
type mockRepo struct {
	EnsureThreadFunc  func(ctx context.Context, kind string, refID uuid.UUID, participants []string) (*domain.Thread, error)
	GetThreadByIDFunc func(ctx context.Context, threadID uuid.UUID) (*domain.Thread, error)
	ListThreadsFunc   func(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]domain.Thread, int32, error)
	ListMessagesFunc  func(ctx context.Context, threadID uuid.UUID, limit, offset int32) ([]domain.Message, int32, error)
	InsertMessageFunc func(ctx context.Context, threadID, authorUserID uuid.UUID, text, clientMsgID string) (*domain.Message, *domain.Thread, error)
	MarkReadFunc      func(ctx context.Context, threadID, userID uuid.UUID) error
}

func (m *mockRepo) EnsureThread(ctx context.Context, kind string, refID uuid.UUID, participants []string) (*domain.Thread, error) {
	return m.EnsureThreadFunc(ctx, kind, refID, participants)
}
func (m *mockRepo) GetThreadByID(ctx context.Context, threadID uuid.UUID) (*domain.Thread, error) {
	return m.GetThreadByIDFunc(ctx, threadID)
}
func (m *mockRepo) ListThreadsForUser(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]domain.Thread, int32, error) {
	return m.ListThreadsFunc(ctx, userID, limit, offset)
}
func (m *mockRepo) ListMessages(ctx context.Context, threadID uuid.UUID, limit, offset int32) ([]domain.Message, int32, error) {
	return m.ListMessagesFunc(ctx, threadID, limit, offset)
}
func (m *mockRepo) InsertMessage(ctx context.Context, threadID, authorUserID uuid.UUID, text, clientMsgID string) (*domain.Message, *domain.Thread, error) {
	return m.InsertMessageFunc(ctx, threadID, authorUserID, text, clientMsgID)
}
func (m *mockRepo) MarkRead(ctx context.Context, threadID, userID uuid.UUID) error {
	return m.MarkReadFunc(ctx, threadID, userID)
}

func newServer(repo *mockRepo) *Server { return NewServer(usecase.New(repo)) }

func wantCode(t *testing.T, err error, c codes.Code) {
	t.Helper()
	if status.Code(err) != c {
		t.Fatalf("status = %v, want %v (err: %v)", status.Code(err), c, err)
	}
}

// --- converters ---

func TestToThreadPB(t *testing.T) {
	if toThreadPB(nil) != nil {
		t.Fatal("toThreadPB(nil) must be nil")
	}
	id, ref := uuid.New(), uuid.New()
	lastMsg := uuid.New()
	lastAt := time.Unix(1700000000, 0)
	readMsg := uuid.New()
	th := &domain.Thread{
		ID: id, Kind: domain.ThreadKindVenueBooking, RefID: ref,
		ParticipantUserIDs: []string{"u1", "u2"},
		LastMessageID:      &lastMsg, LastMessageAt: &lastAt, UnreadCount: 3,
		ParticipantReads: []domain.ParticipantRead{{UserID: "u1", LastReadMessageID: &readMsg}, {UserID: "u2"}},
	}
	pb := toThreadPB(th)
	if pb.GetId() != id.String() || pb.GetRefId() != ref.String() || pb.GetUnreadCount() != 3 {
		t.Errorf("base fields mismatch: %+v", pb)
	}
	if pb.GetLastMessageId() != lastMsg.String() || pb.GetLastMessageAt() == nil {
		t.Error("optional last-message fields not mapped")
	}
	if len(pb.GetParticipantReads()) != 2 {
		t.Fatalf("participant reads = %d, want 2", len(pb.GetParticipantReads()))
	}
	if pb.GetParticipantReads()[0].GetLastReadMessageId() != readMsg.String() {
		t.Error("read watermark not mapped")
	}
	if pb.GetParticipantReads()[1].GetLastReadMessageId() != "" {
		t.Error("nil read watermark must map to empty string")
	}
}

func TestToMessagePB(t *testing.T) {
	if toMessagePB(nil) != nil {
		t.Fatal("toMessagePB(nil) must be nil")
	}
	id, tid, author := uuid.New(), uuid.New(), uuid.New()
	pb := toMessagePB(&domain.Message{ID: id, ThreadID: tid, AuthorUserID: author, Text: "привет", ClientMsgID: "c1", CreatedAt: time.Now()})
	if pb.GetId() != id.String() || pb.GetThreadId() != tid.String() || pb.GetText() != "привет" || pb.GetClientMsgId() != "c1" {
		t.Errorf("message mapping mismatch: %+v", pb)
	}
}

// --- EnsureThread ---

func TestEnsureThread(t *testing.T) {
	actor := uuid.NewString()
	other := uuid.NewString()
	ref := uuid.NewString()

	t.Run("success", func(t *testing.T) {
		var gotParticipants []string
		repo := &mockRepo{EnsureThreadFunc: func(_ context.Context, kind string, refID uuid.UUID, ps []string) (*domain.Thread, error) {
			gotParticipants = ps
			return &domain.Thread{ID: uuid.New(), Kind: kind, RefID: refID, ParticipantUserIDs: ps}, nil
		}}
		resp, err := newServer(repo).EnsureThread(context.Background(), &chatv1.EnsureThreadRequest{
			Kind: domain.ThreadKindVenueBooking, RefId: ref, ActorUserId: actor,
			ParticipantUserIds: []string{actor, other},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetThread() == nil || len(gotParticipants) != 2 {
			t.Errorf("expected a thread with 2 participants, got %+v", resp.GetThread())
		}
	})

	t.Run("invalid kind", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).EnsureThread(context.Background(), &chatv1.EnsureThreadRequest{
			Kind: "bogus", RefId: ref, ActorUserId: actor, ParticipantUserIds: []string{actor, other},
		})
		wantCode(t, err, codes.InvalidArgument)
	})

	t.Run("actor not a participant → PermissionDenied", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).EnsureThread(context.Background(), &chatv1.EnsureThreadRequest{
			Kind: domain.ThreadKindVenueBooking, RefId: ref, ActorUserId: actor,
			ParticipantUserIds: []string{other, uuid.NewString()},
		})
		wantCode(t, err, codes.PermissionDenied)
	})
}

// --- ListThreads ---

func TestListThreads(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mockRepo{ListThreadsFunc: func(context.Context, uuid.UUID, int32, int32) ([]domain.Thread, int32, error) {
			return []domain.Thread{{ID: uuid.New()}, {ID: uuid.New()}}, 2, nil
		}}
		resp, err := newServer(repo).ListThreads(context.Background(), &chatv1.ListThreadsRequest{UserId: uuid.NewString()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.GetThreads()) != 2 || resp.GetTotal() != 2 {
			t.Errorf("list mismatch: %+v", resp)
		}
	})

	t.Run("invalid user id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).ListThreads(context.Background(), &chatv1.ListThreadsRequest{UserId: "not-a-uuid"})
		wantCode(t, err, codes.InvalidArgument)
	})
}

// --- ListMessages / SendMessage membership ---

func threadWith(participant string) *domain.Thread {
	return &domain.Thread{ID: uuid.New(), ParticipantUserIDs: []string{participant, uuid.NewString()}}
}

func TestListMessages(t *testing.T) {
	user := uuid.NewString()
	tid := uuid.NewString()

	t.Run("success", func(t *testing.T) {
		repo := &mockRepo{
			GetThreadByIDFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) { return threadWith(user), nil },
			ListMessagesFunc: func(context.Context, uuid.UUID, int32, int32) ([]domain.Message, int32, error) {
				return []domain.Message{{ID: uuid.New(), Text: "hi"}}, 1, nil
			},
		}
		resp, err := newServer(repo).ListMessages(context.Background(), &chatv1.ListMessagesRequest{ThreadId: tid, UserId: user})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.GetMessages()) != 1 || resp.GetTotal() != 1 {
			t.Errorf("list mismatch: %+v", resp)
		}
	})

	t.Run("non-participant → PermissionDenied", func(t *testing.T) {
		repo := &mockRepo{GetThreadByIDFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) {
			return threadWith(uuid.NewString()), nil // user not in participants
		}}
		_, err := newServer(repo).ListMessages(context.Background(), &chatv1.ListMessagesRequest{ThreadId: tid, UserId: user})
		wantCode(t, err, codes.PermissionDenied)
	})
}

func TestSendMessage(t *testing.T) {
	user := uuid.NewString()
	tid := uuid.NewString()

	t.Run("success returns message and thread", func(t *testing.T) {
		repo := &mockRepo{
			GetThreadByIDFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) { return threadWith(user), nil },
			InsertMessageFunc: func(_ context.Context, threadID, author uuid.UUID, text, cmid string) (*domain.Message, *domain.Thread, error) {
				return &domain.Message{ID: uuid.New(), ThreadID: threadID, AuthorUserID: author, Text: text, ClientMsgID: cmid},
					&domain.Thread{ID: threadID}, nil
			},
		}
		resp, err := newServer(repo).SendMessage(context.Background(), &chatv1.SendMessageRequest{
			ThreadId: tid, UserId: user, Text: "  hello  ", ClientMsgId: "c1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetMessage().GetText() != "hello" {
			t.Errorf("text should be trimmed, got %q", resp.GetMessage().GetText())
		}
		if resp.GetThread() == nil {
			t.Error("thread must be returned")
		}
	})

	t.Run("empty text rejected", func(t *testing.T) {
		repo := &mockRepo{GetThreadByIDFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) { return threadWith(user), nil }}
		_, err := newServer(repo).SendMessage(context.Background(), &chatv1.SendMessageRequest{ThreadId: tid, UserId: user, Text: "   "})
		wantCode(t, err, codes.InvalidArgument)
	})
}

// --- MarkRead ---

func TestMarkRead(t *testing.T) {
	user := uuid.NewString()
	t.Run("success", func(t *testing.T) {
		repo := &mockRepo{
			GetThreadByIDFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) { return threadWith(user), nil },
			MarkReadFunc:      func(context.Context, uuid.UUID, uuid.UUID) error { return nil },
		}
		resp, err := newServer(repo).MarkRead(context.Background(), &chatv1.MarkReadRequest{ThreadId: uuid.NewString(), UserId: user})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetThread() == nil {
			t.Error("thread must be returned")
		}
	})

	t.Run("invalid thread id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).MarkRead(context.Background(), &chatv1.MarkReadRequest{ThreadId: "bad", UserId: user})
		wantCode(t, err, codes.InvalidArgument)
	})
}
