package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
)

var errBoom = errors.New("boom")

// wantCode asserts the gRPC status code carried by err. codes.OK means "no error".
func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("status code = %v, want %v (err=%v)", got, want, err)
	}
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	long := func(n int) string { return strings.Repeat("x", n) }

	tests := []struct {
		name                   string
		userID                 uuid.UUID
		typ, title, body, data string
		wantCode               codes.Code
	}{
		{name: "nil user", userID: uuid.Nil, typ: "t", title: "hi", wantCode: codes.InvalidArgument},
		{name: "empty type", userID: userID, typ: "  ", title: "hi", wantCode: codes.InvalidArgument},
		{name: "type too long", userID: userID, typ: long(maxTypeRunes + 1), title: "hi", wantCode: codes.InvalidArgument},
		{name: "empty title", userID: userID, typ: "t", title: "   ", wantCode: codes.InvalidArgument},
		{name: "title too long", userID: userID, typ: "t", title: long(maxTitleRunes + 1), wantCode: codes.InvalidArgument},
		{name: "body too long", userID: userID, typ: "t", title: "hi", body: long(maxBodyRunes + 1), wantCode: codes.InvalidArgument},
		{name: "data too long", userID: userID, typ: "t", title: "hi", data: long(maxDataBytes + 1), wantCode: codes.InvalidArgument},
		{name: "valid", userID: userID, typ: "booking", title: "hi", wantCode: codes.OK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := New(&mockRepo{})
			_, err := uc.Create(ctx, tt.userID, tt.typ, tt.title, tt.body, tt.data)
			wantCode(t, err, tt.wantCode)
		})
	}
}

func TestCreate_trimsAndForwards(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	var got *domain.Notification
	repo := &mockRepo{
		CreateFunc: func(_ context.Context, n *domain.Notification) (*domain.Notification, error) {
			got = n
			n.ID = uuid.New()
			return n, nil
		},
	}
	out, err := New(repo).Create(ctx, userID, "  booking ", " Привет ", " body ", ` {"k":"v"} `)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got == nil {
		t.Fatal("repo.Create not called")
	}
	if got.Type != "booking" || got.Title != "Привет" || got.Body != "body" || got.Data != `{"k":"v"}` {
		t.Fatalf("fields not trimmed: %+v", got)
	}
	if got.UserID != userID {
		t.Fatalf("user id = %v, want %v", got.UserID, userID)
	}
	if out.ID == uuid.Nil {
		t.Fatal("expected populated notification returned")
	}
}

func TestCreate_repoError(t *testing.T) {
	repo := &mockRepo{
		CreateFunc: func(context.Context, *domain.Notification) (*domain.Notification, error) {
			return nil, errBoom
		},
	}
	_, err := New(repo).Create(context.Background(), uuid.New(), "t", "title", "", "")
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("nil user", func(t *testing.T) {
		_, _, _, err := New(&mockRepo{}).List(ctx, uuid.Nil, 10, 0, false)
		wantCode(t, err, codes.InvalidArgument)
	})

	t.Run("normalizes limit and offset", func(t *testing.T) {
		tests := []struct {
			name               string
			inLimit, inOffset  int32
			wantLimit, wantOff int32
		}{
			{name: "zero limit -> default", inLimit: 0, wantLimit: defaultLimit},
			{name: "negative limit -> default", inLimit: -5, wantLimit: defaultLimit},
			{name: "over max -> capped", inLimit: maxLimit + 50, wantLimit: maxLimit},
			{name: "negative offset -> zero", inLimit: 10, inOffset: -3, wantLimit: 10, wantOff: 0},
			{name: "in range passes through", inLimit: 25, inOffset: 5, wantLimit: 25, wantOff: 5},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var gotLimit, gotOff int32
				repo := &mockRepo{
					ListFunc: func(_ context.Context, _ uuid.UUID, limit, offset int32, _ bool) ([]domain.Notification, int32, error) {
						gotLimit, gotOff = limit, offset
						return nil, 0, nil
					},
				}
				if _, _, _, err := New(repo).List(ctx, userID, tt.inLimit, tt.inOffset, false); err != nil {
					t.Fatalf("List: %v", err)
				}
				if gotLimit != tt.wantLimit || gotOff != tt.wantOff {
					t.Fatalf("limit/offset = %d/%d, want %d/%d", gotLimit, gotOff, tt.wantLimit, tt.wantOff)
				}
			})
		}
	})

	t.Run("success returns list, total, unread", func(t *testing.T) {
		repo := &mockRepo{
			ListFunc: func(context.Context, uuid.UUID, int32, int32, bool) ([]domain.Notification, int32, error) {
				return []domain.Notification{{ID: uuid.New()}}, 7, nil
			},
			UnreadCountFunc: func(context.Context, uuid.UUID) (int32, error) { return 3, nil },
		}
		list, total, unread, err := New(repo).List(ctx, userID, 10, 0, true)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 1 || total != 7 || unread != 3 {
			t.Fatalf("got len=%d total=%d unread=%d", len(list), total, unread)
		}
	})

	t.Run("list error", func(t *testing.T) {
		repo := &mockRepo{
			ListFunc: func(context.Context, uuid.UUID, int32, int32, bool) ([]domain.Notification, int32, error) {
				return nil, 0, errBoom
			},
		}
		_, _, _, err := New(repo).List(ctx, userID, 10, 0, false)
		if !errors.Is(err, errBoom) {
			t.Fatalf("err = %v, want errBoom", err)
		}
	})

	t.Run("unread count error", func(t *testing.T) {
		repo := &mockRepo{
			UnreadCountFunc: func(context.Context, uuid.UUID) (int32, error) { return 0, errBoom },
		}
		_, _, _, err := New(repo).List(ctx, userID, 10, 0, false)
		if !errors.Is(err, errBoom) {
			t.Fatalf("err = %v, want errBoom", err)
		}
	})
}

func TestUnreadCount(t *testing.T) {
	ctx := context.Background()

	t.Run("nil user", func(t *testing.T) {
		_, err := New(&mockRepo{}).UnreadCount(ctx, uuid.Nil)
		wantCode(t, err, codes.InvalidArgument)
	})

	t.Run("forwards repo value", func(t *testing.T) {
		repo := &mockRepo{UnreadCountFunc: func(context.Context, uuid.UUID) (int32, error) { return 9, nil }}
		got, err := New(repo).UnreadCount(ctx, uuid.New())
		if err != nil || got != 9 {
			t.Fatalf("got %d, err %v", got, err)
		}
	})
}

func TestMarkRead(t *testing.T) {
	ctx := context.Background()
	userID, nID := uuid.New(), uuid.New()

	tests := []struct {
		name   string
		userID uuid.UUID
		nID    uuid.UUID
	}{
		{name: "nil user", userID: uuid.Nil, nID: nID},
		{name: "nil notification", userID: userID, nID: uuid.Nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(&mockRepo{}).MarkRead(ctx, tt.userID, tt.nID)
			wantCode(t, err, codes.InvalidArgument)
		})
	}

	t.Run("mark error", func(t *testing.T) {
		repo := &mockRepo{MarkReadFunc: func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return false, errBoom }}
		_, err := New(repo).MarkRead(ctx, userID, nID)
		if !errors.Is(err, errBoom) {
			t.Fatalf("err = %v, want errBoom", err)
		}
	})

	t.Run("success returns refreshed count", func(t *testing.T) {
		called := false
		repo := &mockRepo{
			MarkReadFunc:    func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { called = true; return true, nil },
			UnreadCountFunc: func(context.Context, uuid.UUID) (int32, error) { return 2, nil },
		}
		got, err := New(repo).MarkRead(ctx, userID, nID)
		if err != nil || got != 2 || !called {
			t.Fatalf("got %d err %v called %v", got, err, called)
		}
	})
}

func TestMarkAllRead(t *testing.T) {
	ctx := context.Background()

	t.Run("nil user", func(t *testing.T) {
		_, err := New(&mockRepo{}).MarkAllRead(ctx, uuid.Nil)
		wantCode(t, err, codes.InvalidArgument)
	})

	t.Run("mark error", func(t *testing.T) {
		repo := &mockRepo{MarkAllReadFunc: func(context.Context, uuid.UUID) error { return errBoom }}
		_, err := New(repo).MarkAllRead(ctx, uuid.New())
		if !errors.Is(err, errBoom) {
			t.Fatalf("err = %v, want errBoom", err)
		}
	})

	t.Run("success returns count", func(t *testing.T) {
		repo := &mockRepo{UnreadCountFunc: func(context.Context, uuid.UUID) (int32, error) { return 0, nil }}
		got, err := New(repo).MarkAllRead(ctx, uuid.New())
		if err != nil || got != 0 {
			t.Fatalf("got %d err %v", got, err)
		}
	})
}
