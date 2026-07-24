package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	notificationv1 "github.com/tienlao/agregator/gen/go/notification/v1"
	pkgerrors "github.com/tienlao/agregator/pkg/errors"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
	"github.com/tienlao/agregator/services/notification-service/internal/usecase"
)

var errBoom = errors.New("boom")

// mockRepo is a hand-written func-field mock of domain.Repository. Unset fields
// return zero values, so each test wires only the methods it exercises.
type mockRepo struct {
	CreateFunc      func(ctx context.Context, n *domain.Notification) (*domain.Notification, error)
	ListFunc        func(ctx context.Context, userID uuid.UUID, limit, offset int32, unreadOnly bool) ([]domain.Notification, int32, error)
	UnreadCountFunc func(ctx context.Context, userID uuid.UUID) (int32, error)
	MarkReadFunc    func(ctx context.Context, userID, notificationID uuid.UUID) (bool, error)
	MarkAllReadFunc func(ctx context.Context, userID uuid.UUID) error

	SaveDeviceTokenFunc    func(ctx context.Context, userID uuid.UUID, token, platform string) error
	DeleteDeviceTokenFunc  func(ctx context.Context, userID uuid.UUID, token string) error
	ListDeviceTokensFunc   func(ctx context.Context, userID uuid.UUID) ([]string, error)
	DeleteDeviceTokensFunc func(ctx context.Context, tokens []string) error
}

func (m *mockRepo) Create(ctx context.Context, n *domain.Notification) (*domain.Notification, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, n)
	}
	return n, nil
}

func (m *mockRepo) List(ctx context.Context, userID uuid.UUID, limit, offset int32, unreadOnly bool) ([]domain.Notification, int32, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, userID, limit, offset, unreadOnly)
	}
	return nil, 0, nil
}

func (m *mockRepo) UnreadCount(ctx context.Context, userID uuid.UUID) (int32, error) {
	if m.UnreadCountFunc != nil {
		return m.UnreadCountFunc(ctx, userID)
	}
	return 0, nil
}

func (m *mockRepo) MarkRead(ctx context.Context, userID, notificationID uuid.UUID) (bool, error) {
	if m.MarkReadFunc != nil {
		return m.MarkReadFunc(ctx, userID, notificationID)
	}
	return false, nil
}

func (m *mockRepo) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	if m.MarkAllReadFunc != nil {
		return m.MarkAllReadFunc(ctx, userID)
	}
	return nil
}

func (m *mockRepo) SaveDeviceToken(ctx context.Context, userID uuid.UUID, token, platform string) error {
	if m.SaveDeviceTokenFunc != nil {
		return m.SaveDeviceTokenFunc(ctx, userID, token, platform)
	}
	return nil
}

func (m *mockRepo) DeleteDeviceToken(ctx context.Context, userID uuid.UUID, token string) error {
	if m.DeleteDeviceTokenFunc != nil {
		return m.DeleteDeviceTokenFunc(ctx, userID, token)
	}
	return nil
}

func (m *mockRepo) ListDeviceTokens(ctx context.Context, userID uuid.UUID) ([]string, error) {
	if m.ListDeviceTokensFunc != nil {
		return m.ListDeviceTokensFunc(ctx, userID)
	}
	return nil, nil
}

func (m *mockRepo) DeleteDeviceTokens(ctx context.Context, tokens []string) error {
	if m.DeleteDeviceTokensFunc != nil {
		return m.DeleteDeviceTokensFunc(ctx, tokens)
	}
	return nil
}

func newServer(repo *mockRepo) *Server {
	return NewServer(usecase.New(repo, nil, zerolog.Nop()), zerolog.Nop())
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("status code = %v, want %v (err=%v)", got, want, err)
	}
}

func TestParseUUIDArg(t *testing.T) {
	valid := uuid.New()
	got, err := parseUUIDArg(valid.String(), "user_id")
	if err != nil || got != valid {
		t.Fatalf("got %v err %v, want %v", got, err, valid)
	}

	_, err = parseUUIDArg("not-a-uuid", "user_id")
	wantCode(t, err, codes.InvalidArgument)
}

func TestToPB(t *testing.T) {
	if toPB(nil) != nil {
		t.Fatal("toPB(nil) should be nil")
	}

	id, userID := uuid.New(), uuid.New()
	created := time.Now().UTC().Truncate(time.Second)

	t.Run("unread", func(t *testing.T) {
		out := toPB(&domain.Notification{
			ID: id, UserID: userID, Type: "booking", Title: "hi",
			Body: "b", Data: "{}", CreatedAt: created,
		})
		if out.GetId() != id.String() || out.GetUserId() != userID.String() {
			t.Fatalf("id/userid mismatch: %+v", out)
		}
		if out.GetType() != "booking" || out.GetTitle() != "hi" || out.GetBody() != "b" || out.GetData() != "{}" {
			t.Fatalf("field mismatch: %+v", out)
		}
		if out.GetRead() || out.GetReadAt() != nil {
			t.Fatal("expected unread with nil ReadAt")
		}
		if !out.GetCreatedAt().AsTime().Equal(created) {
			t.Fatalf("created_at = %v, want %v", out.GetCreatedAt().AsTime(), created)
		}
	})

	t.Run("read", func(t *testing.T) {
		readAt := created.Add(time.Hour)
		out := toPB(&domain.Notification{ID: id, UserID: userID, CreatedAt: created, ReadAt: &readAt})
		if !out.GetRead() {
			t.Fatal("expected Read=true")
		}
		if !out.GetReadAt().AsTime().Equal(readAt) {
			t.Fatalf("read_at = %v, want %v", out.GetReadAt().AsTime(), readAt)
		}
	})
}

func TestPassthroughOrInternal(t *testing.T) {
	s := newServer(&mockRepo{})

	// Status error passes through unchanged.
	in := pkgerrors.InvalidArgument("bad")
	if got := s.passthroughOrInternal(in, "msg"); got != in {
		t.Fatalf("expected passthrough, got %v", got)
	}

	// Plain error becomes Internal.
	wantCode(t, s.passthroughOrInternal(errBoom, "msg"), codes.Internal)
}

func TestCreate(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid user id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).Create(ctx, &notificationv1.CreateRequest{UserId: "bad"})
		wantCode(t, err, codes.InvalidArgument)
	})

	t.Run("usecase validation passthrough", func(t *testing.T) {
		// Empty type triggers usecase InvalidArgument; must not be masked as Internal.
		req := &notificationv1.CreateRequest{UserId: uuid.New().String(), Title: "hi"}
		_, err := newServer(&mockRepo{}).Create(ctx, req)
		wantCode(t, err, codes.InvalidArgument)
	})

	t.Run("repo error becomes internal", func(t *testing.T) {
		repo := &mockRepo{CreateFunc: func(context.Context, *domain.Notification) (*domain.Notification, error) { return nil, errBoom }}
		req := &notificationv1.CreateRequest{UserId: uuid.New().String(), Type: "t", Title: "hi"}
		_, err := newServer(repo).Create(ctx, req)
		wantCode(t, err, codes.Internal)
	})

	t.Run("success", func(t *testing.T) {
		id := uuid.New()
		repo := &mockRepo{CreateFunc: func(_ context.Context, n *domain.Notification) (*domain.Notification, error) {
			n.ID = id
			return n, nil
		}}
		req := &notificationv1.CreateRequest{UserId: uuid.New().String(), Type: "booking", Title: "hi", Body: "b"}
		resp, err := newServer(repo).Create(ctx, req)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if resp.GetNotification().GetId() != id.String() {
			t.Fatalf("id = %s, want %s", resp.GetNotification().GetId(), id)
		}
	})
}

func TestList(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid user id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).List(ctx, &notificationv1.ListRequest{UserId: "bad"})
		wantCode(t, err, codes.InvalidArgument)
	})

	t.Run("repo error becomes internal", func(t *testing.T) {
		repo := &mockRepo{ListFunc: func(context.Context, uuid.UUID, int32, int32, bool) ([]domain.Notification, int32, error) {
			return nil, 0, errBoom
		}}
		_, err := newServer(repo).List(ctx, &notificationv1.ListRequest{UserId: uuid.New().String()})
		wantCode(t, err, codes.Internal)
	})

	t.Run("success maps results", func(t *testing.T) {
		repo := &mockRepo{
			ListFunc: func(context.Context, uuid.UUID, int32, int32, bool) ([]domain.Notification, int32, error) {
				return []domain.Notification{{ID: uuid.New()}, {ID: uuid.New()}}, 5, nil
			},
			UnreadCountFunc: func(context.Context, uuid.UUID) (int32, error) { return 4, nil },
		}
		resp, err := newServer(repo).List(ctx, &notificationv1.ListRequest{UserId: uuid.New().String(), Limit: 10})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(resp.GetNotifications()) != 2 || resp.GetTotal() != 5 || resp.GetUnreadCount() != 4 {
			t.Fatalf("got %d items total=%d unread=%d", len(resp.GetNotifications()), resp.GetTotal(), resp.GetUnreadCount())
		}
	})
}

func TestGetUnreadCount(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid user id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).GetUnreadCount(ctx, &notificationv1.GetUnreadCountRequest{UserId: "bad"})
		wantCode(t, err, codes.InvalidArgument)
	})

	t.Run("success", func(t *testing.T) {
		repo := &mockRepo{UnreadCountFunc: func(context.Context, uuid.UUID) (int32, error) { return 6, nil }}
		resp, err := newServer(repo).GetUnreadCount(ctx, &notificationv1.GetUnreadCountRequest{UserId: uuid.New().String()})
		if err != nil || resp.GetUnreadCount() != 6 {
			t.Fatalf("got %d err %v", resp.GetUnreadCount(), err)
		}
	})
}

func TestMarkRead(t *testing.T) {
	ctx := context.Background()
	good := uuid.New().String()

	t.Run("invalid user id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).MarkRead(ctx, &notificationv1.MarkReadRequest{UserId: "bad", NotificationId: good})
		wantCode(t, err, codes.InvalidArgument)
	})

	t.Run("invalid notification id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).MarkRead(ctx, &notificationv1.MarkReadRequest{UserId: good, NotificationId: "bad"})
		wantCode(t, err, codes.InvalidArgument)
	})

	t.Run("success", func(t *testing.T) {
		repo := &mockRepo{UnreadCountFunc: func(context.Context, uuid.UUID) (int32, error) { return 1, nil }}
		resp, err := newServer(repo).MarkRead(ctx, &notificationv1.MarkReadRequest{UserId: good, NotificationId: good})
		if err != nil || resp.GetUnreadCount() != 1 {
			t.Fatalf("got %d err %v", resp.GetUnreadCount(), err)
		}
	})
}

func TestMarkAllRead(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid user id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).MarkAllRead(ctx, &notificationv1.MarkAllReadRequest{UserId: "bad"})
		wantCode(t, err, codes.InvalidArgument)
	})

	t.Run("repo error becomes internal", func(t *testing.T) {
		repo := &mockRepo{MarkAllReadFunc: func(context.Context, uuid.UUID) error { return errBoom }}
		_, err := newServer(repo).MarkAllRead(ctx, &notificationv1.MarkAllReadRequest{UserId: uuid.New().String()})
		wantCode(t, err, codes.Internal)
	})

	t.Run("success", func(t *testing.T) {
		repo := &mockRepo{UnreadCountFunc: func(context.Context, uuid.UUID) (int32, error) { return 0, nil }}
		resp, err := newServer(repo).MarkAllRead(ctx, &notificationv1.MarkAllReadRequest{UserId: uuid.New().String()})
		if err != nil || resp.GetUnreadCount() != 0 {
			t.Fatalf("got %d err %v", resp.GetUnreadCount(), err)
		}
	})
}
