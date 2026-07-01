package adapter

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc"

	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/services/auth-service/internal/domain"
)

// fakeUserServiceClient implements userv1.UserServiceClient. Only CreateUser and
// GetUser are exercised by the adapter; the embedded interface satisfies the
// rest and will panic if an unexpected method is called.
type fakeUserServiceClient struct {
	userv1.UserServiceClient

	createReq *userv1.CreateUserRequest
	getReq    *userv1.GetUserRequest
	resp      *userv1.UserResponse
	err       error
}

func (f *fakeUserServiceClient) CreateUser(_ context.Context, in *userv1.CreateUserRequest, _ ...grpc.CallOption) (*userv1.UserResponse, error) {
	f.createReq = in
	return f.resp, f.err
}

func (f *fakeUserServiceClient) GetUser(_ context.Context, in *userv1.GetUserRequest, _ ...grpc.CallOption) (*userv1.UserResponse, error) {
	f.getReq = in
	return f.resp, f.err
}

func TestCreateUser_MapsRequestAndResponse(t *testing.T) {
	fake := &fakeUserServiceClient{
		resp: &userv1.UserResponse{Id: "u1", Email: "a@b.com", Role: "master"},
	}
	a := NewUserClientAdapter(fake)

	got, err := a.CreateUser(context.Background(), domain.CreateUserInput{
		ID:    "u1",
		Email: "a@b.com",
		Phone: "+1000",
		Name:  "Alice",
		Role:  "master",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Request fields are forwarded verbatim.
	if fake.createReq.GetId() != "u1" || fake.createReq.GetEmail() != "a@b.com" ||
		fake.createReq.GetPhone() != "+1000" || fake.createReq.GetName() != "Alice" ||
		fake.createReq.GetRole() != "master" {
		t.Errorf("forwarded request mismatch: %+v", fake.createReq)
	}

	// Response is mapped onto domain.UserRecord (Phone/Name are not echoed back).
	want := domain.UserRecord{ID: "u1", Email: "a@b.com", Role: "master"}
	if *got != want {
		t.Errorf("mapped record = %+v, want %+v", *got, want)
	}
}

func TestCreateUser_WrapsError(t *testing.T) {
	fake := &fakeUserServiceClient{err: errors.New("boom")}
	a := NewUserClientAdapter(fake)

	_, err := a.CreateUser(context.Background(), domain.CreateUserInput{ID: "u1"})
	if err == nil || !strings.Contains(err.Error(), "user-service CreateUser") {
		t.Fatalf("want wrapped CreateUser error, got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("wrapped error must preserve cause, got %v", err)
	}
}

func TestGetUser_MapsRequestAndResponse(t *testing.T) {
	fake := &fakeUserServiceClient{
		resp: &userv1.UserResponse{Id: "u9", Email: "x@y.com", Role: "user"},
	}
	a := NewUserClientAdapter(fake)

	got, err := a.GetUser(context.Background(), "u9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.getReq.GetId() != "u9" {
		t.Errorf("GetUser request id = %q, want u9", fake.getReq.GetId())
	}
	want := domain.UserRecord{ID: "u9", Email: "x@y.com", Role: "user"}
	if *got != want {
		t.Errorf("mapped record = %+v, want %+v", *got, want)
	}
}

func TestGetUser_WrapsError(t *testing.T) {
	fake := &fakeUserServiceClient{err: errors.New("down")}
	a := NewUserClientAdapter(fake)

	_, err := a.GetUser(context.Background(), "u9")
	if err == nil || !strings.Contains(err.Error(), "user-service GetUser") {
		t.Fatalf("want wrapped GetUser error, got %v", err)
	}
}
