package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type userSvcMock struct {
	userv1.UserServiceClient
	user      *userv1.UserResponse
	getErr    error
	onUpdate  func(*userv1.UpdateUserRequest) (*userv1.UserResponse, error)
	deleteErr error
	deleted   bool
}

func (m *userSvcMock) GetUser(_ context.Context, in *userv1.GetUserRequest, _ ...grpc.CallOption) (*userv1.UserResponse, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.user != nil {
		return m.user, nil
	}
	return &userv1.UserResponse{Id: in.GetId()}, nil
}

func (m *userSvcMock) UpdateUser(_ context.Context, in *userv1.UpdateUserRequest, _ ...grpc.CallOption) (*userv1.UserResponse, error) {
	if m.onUpdate != nil {
		return m.onUpdate(in)
	}
	return &userv1.UserResponse{Id: in.GetId()}, nil
}

func (m *userSvcMock) DeleteUser(_ context.Context, _ *userv1.DeleteUserRequest, _ ...grpc.CallOption) (*userv1.DeleteUserResponse, error) {
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	m.deleted = true
	return &userv1.DeleteUserResponse{}, nil
}

type userAuthMock struct {
	authv1.AuthServiceClient
	err    error
	called bool
}

func (m *userAuthMock) DeleteAccount(_ context.Context, _ *authv1.DeleteAccountRequest, _ ...grpc.CallOption) (*authv1.DeleteAccountResponse, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}
	return &authv1.DeleteAccountResponse{}, nil
}

type userVenueMock struct {
	venuev1.VenueServiceClient
	err    error
	called bool
}

func (m *userVenueMock) SuspendVenuesByOwner(_ context.Context, _ *venuev1.SuspendVenuesByOwnerRequest, _ ...grpc.CallOption) (*venuev1.SuspendVenuesByOwnerResponse, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}
	return &venuev1.SuspendVenuesByOwnerResponse{}, nil
}

type userMasterMock struct {
	masterv1.MasterServiceClient
	err    error
	called bool
}

func (m *userMasterMock) SuspendMasterByUser(_ context.Context, _ *masterv1.SuspendMasterByUserRequest, _ ...grpc.CallOption) (*masterv1.SuspendMasterByUserResponse, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}
	return &masterv1.SuspendMasterByUserResponse{}, nil
}

func newUserHandlerWith(u *userSvcMock, a *userAuthMock, v *userVenueMock, m *userMasterMock) *UserHandler {
	return NewUserHandler(u, a, v, m)
}

// --- GetMe ---

func TestUserGetMe_Unauthorized(t *testing.T) {
	h := newUserHandlerWith(&userSvcMock{}, &userAuthMock{}, &userVenueMock{}, &userMasterMock{})
	rr := httptest.NewRecorder()
	h.GetMe(rr, httptest.NewRequest(http.MethodGet, "/me", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestUserGetMe_Success(t *testing.T) {
	u := &userSvcMock{user: &userv1.UserResponse{Id: "u1", Email: "a@b.c", Name: "Ann", Role: "client"}}
	h := newUserHandlerWith(u, &userAuthMock{}, &userVenueMock{}, &userMasterMock{})
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/me", nil)
	r = r.WithContext(middleware.WithUserID(r.Context(), "u1"))
	h.GetMe(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["email"] != "a@b.c" || body["name"] != "Ann" {
		t.Fatalf("body = %v", body)
	}
}

func TestUserGetMe_GRPCError(t *testing.T) {
	u := &userSvcMock{getErr: status.Error(codes.NotFound, "nope")}
	h := newUserHandlerWith(u, &userAuthMock{}, &userVenueMock{}, &userMasterMock{})
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/me", nil)
	r = r.WithContext(middleware.WithUserID(r.Context(), "u1"))
	h.GetMe(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- UpdateMe ---

func TestUserUpdateMe_Unauthorized(t *testing.T) {
	h := newUserHandlerWith(&userSvcMock{}, &userAuthMock{}, &userVenueMock{}, &userMasterMock{})
	rr := httptest.NewRecorder()
	h.UpdateMe(rr, httptest.NewRequest(http.MethodPatch, "/me", strings.NewReader(`{}`)))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestUserUpdateMe_PartialFields(t *testing.T) {
	var got *userv1.UpdateUserRequest
	u := &userSvcMock{onUpdate: func(in *userv1.UpdateUserRequest) (*userv1.UserResponse, error) {
		got = in
		return &userv1.UserResponse{Id: in.GetId(), Name: in.GetName()}, nil
	}}
	h := newUserHandlerWith(u, &userAuthMock{}, &userVenueMock{}, &userMasterMock{})
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/me", strings.NewReader(`{"name":"New","bio":"hi"}`))
	r = r.WithContext(middleware.WithUserID(r.Context(), "u1"))
	h.UpdateMe(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if got.Name == nil || got.GetName() != "New" {
		t.Fatalf("name not forwarded: %+v", got)
	}
	if got.Bio == nil || got.GetBio() != "hi" {
		t.Fatalf("bio not forwarded: %+v", got)
	}
	if got.Phone != nil || got.AvatarUrl != nil {
		t.Fatalf("unset fields should be nil: %+v", got)
	}
}

func TestUserUpdateMe_InvalidBody(t *testing.T) {
	h := newUserHandlerWith(&userSvcMock{}, &userAuthMock{}, &userVenueMock{}, &userMasterMock{})
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/me", strings.NewReader(`{bad`))
	r = r.WithContext(middleware.WithUserID(r.Context(), "u1"))
	h.UpdateMe(rr, r)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- DeleteMe ---

func deleteMeReq(userID, body string) *http.Request {
	r := httptest.NewRequest(http.MethodDelete, "/me", strings.NewReader(body))
	if userID != "" {
		r = r.WithContext(middleware.WithUserID(r.Context(), userID))
	}
	return r
}

func TestUserDeleteMe_Unauthorized(t *testing.T) {
	h := newUserHandlerWith(&userSvcMock{}, &userAuthMock{}, &userVenueMock{}, &userMasterMock{})
	rr := httptest.NewRecorder()
	h.DeleteMe(rr, deleteMeReq("", `{"confirmation":"a@b.c"}`))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestUserDeleteMe_ConfirmationRequired(t *testing.T) {
	h := newUserHandlerWith(&userSvcMock{}, &userAuthMock{}, &userVenueMock{}, &userMasterMock{})
	rr := httptest.NewRecorder()
	h.DeleteMe(rr, deleteMeReq("u1", `{"confirmation":"  "}`))
	if got := decodeCode(t, rr); got != apicatalog.GatewayAccountConfirmationRequired.Code {
		t.Fatalf("code = %s (status %d)", got, rr.Code)
	}
}

func TestUserDeleteMe_AdminForbidden(t *testing.T) {
	u := &userSvcMock{user: &userv1.UserResponse{Id: "u1", Email: "admin@x.io", Role: "admin"}}
	h := newUserHandlerWith(u, &userAuthMock{}, &userVenueMock{}, &userMasterMock{})
	rr := httptest.NewRecorder()
	h.DeleteMe(rr, deleteMeReq("u1", `{"confirmation":"admin@x.io"}`))
	if got := decodeCode(t, rr); got != apicatalog.GatewayAccountAdminSelfDeleteForbidden.Code {
		t.Fatalf("code = %s (status %d)", got, rr.Code)
	}
}

func TestUserDeleteMe_ConfirmationMismatch(t *testing.T) {
	u := &userSvcMock{user: &userv1.UserResponse{Id: "u1", Email: "real@x.io", Role: "client"}}
	h := newUserHandlerWith(u, &userAuthMock{}, &userVenueMock{}, &userMasterMock{})
	rr := httptest.NewRecorder()
	h.DeleteMe(rr, deleteMeReq("u1", `{"confirmation":"typo@x.io"}`))
	if got := decodeCode(t, rr); got != apicatalog.GatewayAccountConfirmationMismatch.Code {
		t.Fatalf("code = %s (status %d)", got, rr.Code)
	}
}

func TestUserDeleteMe_ClientSuccess_NoCascade(t *testing.T) {
	u := &userSvcMock{user: &userv1.UserResponse{Id: "u1", Email: "c@x.io", Role: "client"}}
	auth := &userAuthMock{}
	venue := &userVenueMock{}
	master := &userMasterMock{}
	h := newUserHandlerWith(u, auth, venue, master)
	rr := httptest.NewRecorder()
	// case-insensitive + trimmed confirmation match
	h.DeleteMe(rr, deleteMeReq("u1", `{"confirmation":"  C@X.IO  "}`))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !auth.called || !u.deleted {
		t.Fatalf("auth.called=%v deleted=%v", auth.called, u.deleted)
	}
	if venue.called || master.called {
		t.Fatalf("client role should not cascade: venue=%v master=%v", venue.called, master.called)
	}
}

func TestUserDeleteMe_VenueOwnerCascade(t *testing.T) {
	u := &userSvcMock{user: &userv1.UserResponse{Id: "u1", Email: "o@x.io", Role: "venue_owner"}}
	venue := &userVenueMock{}
	master := &userMasterMock{}
	auth := &userAuthMock{}
	h := newUserHandlerWith(u, auth, venue, master)
	rr := httptest.NewRecorder()
	h.DeleteMe(rr, deleteMeReq("u1", `{"confirmation":"o@x.io"}`))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rr.Code)
	}
	if !venue.called {
		t.Fatal("expected venue suspend cascade")
	}
	if master.called {
		t.Fatal("master cascade should not run for venue_owner")
	}
}

func TestUserDeleteMe_MasterCascade(t *testing.T) {
	u := &userSvcMock{user: &userv1.UserResponse{Id: "u1", Email: "m@x.io", Role: "master"}}
	master := &userMasterMock{}
	h := newUserHandlerWith(u, &userAuthMock{}, &userVenueMock{}, master)
	rr := httptest.NewRecorder()
	h.DeleteMe(rr, deleteMeReq("u1", `{"confirmation":"m@x.io"}`))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rr.Code)
	}
	if !master.called {
		t.Fatal("expected master suspend cascade")
	}
}

func TestUserDeleteMe_CascadeError_Aborts(t *testing.T) {
	u := &userSvcMock{user: &userv1.UserResponse{Id: "u1", Email: "o@x.io", Role: "venue_owner"}}
	venue := &userVenueMock{err: status.Error(codes.Unavailable, "down")}
	auth := &userAuthMock{}
	h := newUserHandlerWith(u, auth, venue, &userMasterMock{})
	rr := httptest.NewRecorder()
	h.DeleteMe(rr, deleteMeReq("u1", `{"confirmation":"o@x.io"}`))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rr.Code)
	}
	if auth.called || u.deleted {
		t.Fatal("must not revoke/delete after cascade failure")
	}
}

func TestUserDeleteMe_AuthRevokeError_Aborts(t *testing.T) {
	u := &userSvcMock{user: &userv1.UserResponse{Id: "u1", Email: "c@x.io", Role: "client"}}
	auth := &userAuthMock{err: status.Error(codes.Internal, "boom")}
	h := newUserHandlerWith(u, auth, &userVenueMock{}, &userMasterMock{})
	rr := httptest.NewRecorder()
	h.DeleteMe(rr, deleteMeReq("u1", `{"confirmation":"c@x.io"}`))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
	if u.deleted {
		t.Fatal("must not soft-delete after auth revoke failure")
	}
}

func TestUserDeleteMe_GetUserError(t *testing.T) {
	u := &userSvcMock{getErr: status.Error(codes.NotFound, "gone")}
	h := newUserHandlerWith(u, &userAuthMock{}, &userVenueMock{}, &userMasterMock{})
	rr := httptest.NewRecorder()
	h.DeleteMe(rr, deleteMeReq("u1", `{"confirmation":"x@x.io"}`))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestUserDeleteMe_InvalidBody(t *testing.T) {
	h := newUserHandlerWith(&userSvcMock{}, &userAuthMock{}, &userVenueMock{}, &userMasterMock{})
	rr := httptest.NewRecorder()
	h.DeleteMe(rr, deleteMeReq("u1", `{bad`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}
