package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/chat-service/internal/domain"
)

// fnRepo is a flexible function-field mock of domain.Repository for the paths
// the struct-based mockRepo (chat_test.go) doesn't exercise.
type fnRepo struct {
	GetThreadFunc     func(ctx context.Context, id uuid.UUID) (*domain.Thread, error)
	ListThreadsFunc   func(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]domain.Thread, int32, error)
	ListMessagesFunc  func(ctx context.Context, threadID uuid.UUID, limit, offset int32) ([]domain.Message, int32, error)
	InsertMessageFunc func(ctx context.Context, threadID, author uuid.UUID, text, cmid string) (*domain.Message, *domain.Thread, error)
	MarkReadFunc      func(ctx context.Context, threadID, userID uuid.UUID) error
}

func (r *fnRepo) EnsureThread(context.Context, string, uuid.UUID, []string) (*domain.Thread, error) {
	return nil, errors.New("EnsureThread not stubbed")
}
func (r *fnRepo) GetThreadByID(ctx context.Context, id uuid.UUID) (*domain.Thread, error) {
	return r.GetThreadFunc(ctx, id)
}
func (r *fnRepo) ListThreadsForUser(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]domain.Thread, int32, error) {
	return r.ListThreadsFunc(ctx, userID, limit, offset)
}
func (r *fnRepo) ListMessages(ctx context.Context, threadID uuid.UUID, limit, offset int32) ([]domain.Message, int32, error) {
	return r.ListMessagesFunc(ctx, threadID, limit, offset)
}
func (r *fnRepo) InsertMessage(ctx context.Context, threadID, author uuid.UUID, text, cmid string) (*domain.Message, *domain.Thread, error) {
	return r.InsertMessageFunc(ctx, threadID, author, text, cmid)
}
func (r *fnRepo) MarkRead(ctx context.Context, threadID, userID uuid.UUID) error {
	return r.MarkReadFunc(ctx, threadID, userID)
}

type fnPub struct {
	calls int
	err   error
}

func (p *fnPub) Publish(context.Context, string, []byte) error {
	p.calls++
	return p.err
}

func threadWith(member string) *domain.Thread {
	return &domain.Thread{ID: uuid.New(), ParticipantUserIDs: []string{member, uuid.NewString()}}
}

func code(err error) codes.Code { return status.Code(err) }

func TestListThreads_ClampsAndValidates(t *testing.T) {
	ctx := context.Background()

	_, _, err := New(&fnRepo{}).ListThreads(ctx, "not-a-uuid", 10, 0)
	if code(err) != codes.InvalidArgument {
		t.Fatalf("invalid user_id: got %v", code(err))
	}

	var gotLimit, gotOffset int32
	repo := &fnRepo{ListThreadsFunc: func(_ context.Context, _ uuid.UUID, limit, offset int32) ([]domain.Thread, int32, error) {
		gotLimit, gotOffset = limit, offset
		return []domain.Thread{{ID: uuid.New()}}, 1, nil
	}}
	// limit>200 and negative offset are clamped to (50, 0).
	list, total, err := New(repo).ListThreads(ctx, uuid.NewString(), 9999, -5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 || total != 1 {
		t.Errorf("result mismatch: len=%d total=%d", len(list), total)
	}
	if gotLimit != 50 || gotOffset != 0 {
		t.Errorf("clamp mismatch: limit=%d offset=%d, want 50/0", gotLimit, gotOffset)
	}
}

func TestListMessages_Paths(t *testing.T) {
	ctx := context.Background()
	user := uuid.NewString()
	tid := uuid.NewString()

	t.Run("invalid thread id", func(t *testing.T) {
		_, _, err := New(&fnRepo{}).ListMessages(ctx, "bad", user, 10, 0)
		if code(err) != codes.InvalidArgument {
			t.Fatalf("got %v", code(err))
		}
	})
	t.Run("thread not found", func(t *testing.T) {
		repo := &fnRepo{GetThreadFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) { return nil, nil }}
		_, _, err := New(repo).ListMessages(ctx, tid, user, 10, 0)
		if code(err) != codes.NotFound {
			t.Fatalf("got %v", code(err))
		}
	})
	t.Run("non-participant denied", func(t *testing.T) {
		repo := &fnRepo{GetThreadFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) {
			return threadWith(uuid.NewString()), nil
		}}
		_, _, err := New(repo).ListMessages(ctx, tid, user, 10, 0)
		if code(err) != codes.PermissionDenied {
			t.Fatalf("got %v", code(err))
		}
	})
	t.Run("success", func(t *testing.T) {
		repo := &fnRepo{
			GetThreadFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) { return threadWith(user), nil },
			ListMessagesFunc: func(context.Context, uuid.UUID, int32, int32) ([]domain.Message, int32, error) {
				return []domain.Message{{ID: uuid.New()}}, 1, nil
			},
		}
		list, total, err := New(repo).ListMessages(ctx, tid, user, 0, -1) // clamps too
		if err != nil || len(list) != 1 || total != 1 {
			t.Fatalf("unexpected: %v len=%d total=%d", err, len(list), total)
		}
	})
}

func TestMarkRead_Paths(t *testing.T) {
	ctx := context.Background()
	user := uuid.NewString()
	tid := uuid.NewString()

	t.Run("invalid user id", func(t *testing.T) {
		_, err := New(&fnRepo{}).MarkRead(ctx, tid, "bad")
		if code(err) != codes.InvalidArgument {
			t.Fatalf("got %v", code(err))
		}
	})
	t.Run("non-participant denied", func(t *testing.T) {
		repo := &fnRepo{GetThreadFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) {
			return threadWith(uuid.NewString()), nil
		}}
		_, err := New(repo).MarkRead(ctx, tid, user)
		if code(err) != codes.PermissionDenied {
			t.Fatalf("got %v", code(err))
		}
	})
	t.Run("success returns refreshed thread", func(t *testing.T) {
		var markCalled bool
		repo := &fnRepo{
			GetThreadFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) { return threadWith(user), nil },
			MarkReadFunc:  func(context.Context, uuid.UUID, uuid.UUID) error { markCalled = true; return nil },
		}
		th, err := New(repo).MarkRead(ctx, tid, user)
		if err != nil || th == nil {
			t.Fatalf("unexpected: %v thread=%v", err, th)
		}
		if !markCalled {
			t.Error("repo.MarkRead must be called")
		}
	})
}

func TestSendMessage_EdgeCases(t *testing.T) {
	ctx := context.Background()
	user := uuid.NewString()
	tid := uuid.NewString()

	t.Run("text too long", func(t *testing.T) {
		long := strings.Repeat("я", 3001)
		_, _, err := New(&fnRepo{}).SendMessage(ctx, tid, user, long, "")
		if code(err) != codes.InvalidArgument {
			t.Fatalf("got %v", code(err))
		}
	})
	t.Run("thread not found", func(t *testing.T) {
		repo := &fnRepo{GetThreadFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) { return nil, nil }}
		_, _, err := New(repo).SendMessage(ctx, tid, user, "hi", "")
		if code(err) != codes.NotFound {
			t.Fatalf("got %v", code(err))
		}
	})
	t.Run("insert error propagates", func(t *testing.T) {
		repo := &fnRepo{
			GetThreadFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) { return threadWith(user), nil },
			InsertMessageFunc: func(context.Context, uuid.UUID, uuid.UUID, string, string) (*domain.Message, *domain.Thread, error) {
				return nil, nil, errors.New("db down")
			},
		}
		_, _, err := New(repo).SendMessage(ctx, tid, user, "hi", "")
		if err == nil {
			t.Fatal("expected insert error to propagate")
		}
	})
	t.Run("publish failure does not fail the send", func(t *testing.T) {
		th := threadWith(user)
		repo := &fnRepo{
			GetThreadFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) { return th, nil },
			InsertMessageFunc: func(_ context.Context, threadID, author uuid.UUID, text, cmid string) (*domain.Message, *domain.Thread, error) {
				return &domain.Message{ID: uuid.New(), ThreadID: threadID, AuthorUserID: author, Text: text}, th, nil
			},
		}
		pub := &fnPub{err: errors.New("nats down")}
		m, tr, err := NewWithPublisher(repo, pub).SendMessage(ctx, tid, user, "hi", "c1")
		if err != nil {
			t.Fatalf("publish failure must not fail SendMessage: %v", err)
		}
		if m == nil || tr == nil {
			t.Error("message and thread must be returned")
		}
		if pub.calls != 1 {
			t.Errorf("publisher should be invoked once, got %d", pub.calls)
		}
	})
}

func TestNewWithPublisher_NilUsesNoop(t *testing.T) {
	// A nil publisher must be replaced by the no-op so SendMessage never panics.
	user := uuid.NewString()
	th := threadWith(user)
	repo := &fnRepo{
		GetThreadFunc: func(context.Context, uuid.UUID) (*domain.Thread, error) { return th, nil },
		InsertMessageFunc: func(_ context.Context, threadID, author uuid.UUID, text, cmid string) (*domain.Message, *domain.Thread, error) {
			return &domain.Message{ID: uuid.New(), ThreadID: threadID}, th, nil
		},
	}
	_, _, err := NewWithPublisher(repo, nil).SendMessage(context.Background(), uuid.NewString(), user, "hi", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
