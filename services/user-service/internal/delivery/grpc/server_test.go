package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/services/user-service/internal/domain"
	"github.com/tienlao/agregator/services/user-service/internal/usecase"
)

// mockRepo implements domain.UserRepository via function fields (mirrors
// usecase/mock_test.go). Defaults follow the real repo contract: reads return
// ErrNotFound, writes succeed.
type mockRepo struct {
	CreateFunc     func(ctx context.Context, u *domain.User) error
	GetByIDFunc    func(ctx context.Context, id string) (*domain.User, error)
	GetBatchFunc   func(ctx context.Context, ids []string) (map[string]*domain.User, error)
	GetByEmailFunc func(ctx context.Context, email string) (*domain.User, error)
	UpdateFunc     func(ctx context.Context, u *domain.User) error
	SoftDeleteFunc func(ctx context.Context, id string) error
}

func (m *mockRepo) Create(ctx context.Context, u *domain.User) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, u)
	}
	return nil
}
func (m *mockRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, domain.ErrNotFound
}
func (m *mockRepo) GetBatch(ctx context.Context, ids []string) (map[string]*domain.User, error) {
	if m.GetBatchFunc != nil {
		return m.GetBatchFunc(ctx, ids)
	}
	return map[string]*domain.User{}, nil
}
func (m *mockRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if m.GetByEmailFunc != nil {
		return m.GetByEmailFunc(ctx, email)
	}
	return nil, domain.ErrNotFound
}
func (m *mockRepo) Update(ctx context.Context, u *domain.User) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, u)
	}
	return nil
}
func (m *mockRepo) SoftDelete(ctx context.Context, id string) error {
	if m.SoftDeleteFunc != nil {
		return m.SoftDeleteFunc(ctx, id)
	}
	return nil
}

func newServer(repo domain.UserRepository) *UserServer {
	return NewUserServer(usecase.NewUserUseCase(repo), zerolog.Nop())
}

func wantCode(t *testing.T, err error, code codes.Code) {
	t.Helper()
	if status.Code(err) != code {
		t.Fatalf("status code = %v, want %v (err: %v)", status.Code(err), code, err)
	}
}

func strptr(s string) *string { return &s }

// --- CreateUser ---

func TestCreateUser_Validation(t *testing.T) {
	s := newServer(&mockRepo{})
	tests := []struct {
		name string
		req  *userv1.CreateUserRequest
	}{
		{"missing id", &userv1.CreateUserRequest{Email: "a@b.com", Name: "A"}},
		{"missing email", &userv1.CreateUserRequest{Id: "u1", Name: "A"}},
		{"missing name", &userv1.CreateUserRequest{Id: "u1", Email: "a@b.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.CreateUser(context.Background(), tt.req)
			wantCode(t, err, codes.InvalidArgument)
		})
	}
}

func TestCreateUser_InvalidRole(t *testing.T) {
	s := newServer(&mockRepo{})
	_, err := s.CreateUser(context.Background(), &userv1.CreateUserRequest{
		Id: "u1", Email: "a@b.com", Name: "A", Role: "superadmin",
	})
	wantCode(t, err, codes.InvalidArgument)
}

func TestCreateUser_RepoErrorIsInternal(t *testing.T) {
	repo := &mockRepo{CreateFunc: func(context.Context, *domain.User) error {
		return errors.New("db down")
	}}
	_, err := newServer(repo).CreateUser(context.Background(), &userv1.CreateUserRequest{
		Id: "u1", Email: "a@b.com", Name: "A",
	})
	wantCode(t, err, codes.Internal)
}

func TestCreateUser_Success_DefaultsRoleAndNormalisesEmail(t *testing.T) {
	var captured *domain.User
	repo := &mockRepo{CreateFunc: func(_ context.Context, u *domain.User) error {
		captured = u
		return nil
	}}
	resp, err := newServer(repo).CreateUser(context.Background(), &userv1.CreateUserRequest{
		Id: "u1", Email: "  A@B.COM  ", Name: "Alice", // role omitted
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Role != "user" {
		t.Errorf("role default = %q, want user", captured.Role)
	}
	if captured.Email != "a@b.com" {
		t.Errorf("email normalised = %q, want a@b.com", captured.Email)
	}
	if resp.GetId() != "u1" || resp.GetEmail() != "a@b.com" || resp.GetRole() != "user" {
		t.Errorf("response mismatch: %+v", resp)
	}
}

// --- GetUser ---

func TestGetUser(t *testing.T) {
	t.Run("missing id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).GetUser(context.Background(), &userv1.GetUserRequest{})
		wantCode(t, err, codes.InvalidArgument)
	})
	t.Run("not found", func(t *testing.T) {
		// Default GetByID returns ErrNotFound.
		_, err := newServer(&mockRepo{}).GetUser(context.Background(), &userv1.GetUserRequest{Id: "u1"})
		wantCode(t, err, codes.NotFound)
	})
	t.Run("internal", func(t *testing.T) {
		repo := &mockRepo{GetByIDFunc: func(context.Context, string) (*domain.User, error) {
			return nil, errors.New("boom")
		}}
		_, err := newServer(repo).GetUser(context.Background(), &userv1.GetUserRequest{Id: "u1"})
		wantCode(t, err, codes.Internal)
	})
	t.Run("success", func(t *testing.T) {
		repo := &mockRepo{GetByIDFunc: func(_ context.Context, id string) (*domain.User, error) {
			return &domain.User{ID: id, Email: "a@b.com", Name: "A", Role: "master"}, nil
		}}
		resp, err := newServer(repo).GetUser(context.Background(), &userv1.GetUserRequest{Id: "u1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetId() != "u1" || resp.GetRole() != "master" {
			t.Errorf("response mismatch: %+v", resp)
		}
	})
}

// --- GetUsersBatch ---

func TestGetUsersBatch(t *testing.T) {
	t.Run("empty ids returns empty map", func(t *testing.T) {
		resp, err := newServer(&mockRepo{}).GetUsersBatch(context.Background(), &userv1.GetUsersBatchRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.GetUsers()) != 0 {
			t.Errorf("expected empty map, got %d", len(resp.GetUsers()))
		}
	})
	t.Run("over 200 rejected", func(t *testing.T) {
		ids := make([]string, 201)
		_, err := newServer(&mockRepo{}).GetUsersBatch(context.Background(), &userv1.GetUsersBatchRequest{Ids: ids})
		wantCode(t, err, codes.InvalidArgument)
	})
	t.Run("success maps results", func(t *testing.T) {
		repo := &mockRepo{GetBatchFunc: func(_ context.Context, ids []string) (map[string]*domain.User, error) {
			return map[string]*domain.User{"u1": {ID: "u1", Name: "A"}}, nil
		}}
		resp, err := newServer(repo).GetUsersBatch(context.Background(), &userv1.GetUsersBatchRequest{Ids: []string{"u1", "u2"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.GetUsers()) != 1 || resp.GetUsers()["u1"].GetName() != "A" {
			t.Errorf("response mismatch: %+v", resp.GetUsers())
		}
	})
	t.Run("repo error is internal", func(t *testing.T) {
		repo := &mockRepo{GetBatchFunc: func(context.Context, []string) (map[string]*domain.User, error) {
			return nil, errors.New("boom")
		}}
		_, err := newServer(repo).GetUsersBatch(context.Background(), &userv1.GetUsersBatchRequest{Ids: []string{"u1"}})
		wantCode(t, err, codes.Internal)
	})
}

// --- GetUserByEmail ---

func TestGetUserByEmail(t *testing.T) {
	t.Run("missing email", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).GetUserByEmail(context.Background(), &userv1.GetUserByEmailRequest{})
		wantCode(t, err, codes.InvalidArgument)
	})
	t.Run("not found", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).GetUserByEmail(context.Background(), &userv1.GetUserByEmailRequest{Email: "x@y.com"})
		wantCode(t, err, codes.NotFound)
	})
	t.Run("success normalises lookup", func(t *testing.T) {
		var lookedUp string
		repo := &mockRepo{GetByEmailFunc: func(_ context.Context, email string) (*domain.User, error) {
			lookedUp = email
			return &domain.User{ID: "u1", Email: email}, nil
		}}
		_, err := newServer(repo).GetUserByEmail(context.Background(), &userv1.GetUserByEmailRequest{Email: "  A@B.COM "})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if lookedUp != "a@b.com" {
			t.Errorf("usecase must normalise email before lookup, got %q", lookedUp)
		}
	})
}

// --- UpdateUser ---

func TestUpdateUser(t *testing.T) {
	t.Run("missing id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).UpdateUser(context.Background(), &userv1.UpdateUserRequest{})
		wantCode(t, err, codes.InvalidArgument)
	})
	t.Run("not found", func(t *testing.T) {
		// Default GetByID → ErrNotFound, so Update surfaces NotFound.
		_, err := newServer(&mockRepo{}).UpdateUser(context.Background(), &userv1.UpdateUserRequest{Id: "u1"})
		wantCode(t, err, codes.NotFound)
	})
	t.Run("success applies patch", func(t *testing.T) {
		repo := &mockRepo{
			GetByIDFunc: func(_ context.Context, id string) (*domain.User, error) {
				return &domain.User{ID: id, Name: "Old", Phone: "111"}, nil
			},
		}
		resp, err := newServer(repo).UpdateUser(context.Background(), &userv1.UpdateUserRequest{
			Id: "u1", Name: strptr("New"), Bio: strptr("hello"),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetName() != "New" || resp.GetBio() != "hello" {
			t.Errorf("patch not applied: %+v", resp)
		}
		if resp.GetPhone() != "111" {
			t.Errorf("nil patch field must be left unchanged, got phone %q", resp.GetPhone())
		}
	})
}

// --- DeleteUser ---

func TestDeleteUser(t *testing.T) {
	t.Run("missing id", func(t *testing.T) {
		_, err := newServer(&mockRepo{}).DeleteUser(context.Background(), &userv1.DeleteUserRequest{})
		wantCode(t, err, codes.InvalidArgument)
	})
	t.Run("not found", func(t *testing.T) {
		repo := &mockRepo{SoftDeleteFunc: func(context.Context, string) error { return domain.ErrNotFound }}
		_, err := newServer(repo).DeleteUser(context.Background(), &userv1.DeleteUserRequest{Id: "u1"})
		wantCode(t, err, codes.NotFound)
	})
	t.Run("success", func(t *testing.T) {
		var deleted string
		repo := &mockRepo{SoftDeleteFunc: func(_ context.Context, id string) error { deleted = id; return nil }}
		_, err := newServer(repo).DeleteUser(context.Background(), &userv1.DeleteUserRequest{Id: "u1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deleted != "u1" {
			t.Errorf("SoftDelete called with %q, want u1", deleted)
		}
	})
}
