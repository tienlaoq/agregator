package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestOAuthHandler(t *testing.T, auth authv1.AuthServiceClient) *OAuthHandler {
	t.Helper()
	h, err := NewOAuthHandler(zerolog.Nop(), auth, OAuthConfig{
		VKClientID:     "vk-id",
		VKClientSecret: "vk-secret",
		YandexClientID: "ya-id",
		BaseURL:        "http://gw.local",
		FrontendURL:    "http://front.local",
	})
	if err != nil {
		t.Fatalf("NewOAuthHandler: %v", err)
	}
	return h
}

func cookieByName(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// --- Redirects ---

func TestVKRedirect_NotConfigured(t *testing.T) {
	h, _ := NewOAuthHandler(zerolog.Nop(), &mockAuthClient{}, OAuthConfig{BaseURL: "http://gw.local", FrontendURL: "http://front.local"})
	rr := httptest.NewRecorder()
	h.VKRedirect(rr, httptest.NewRequest(http.MethodGet, "/auth/vk", nil))
	if rr.Code == http.StatusTemporaryRedirect {
		t.Fatalf("expected not-configured error, got redirect")
	}
}

func TestVKRedirect_SetsCookiesAndRedirects(t *testing.T) {
	h := newTestOAuthHandler(t, &mockAuthClient{})
	rr := httptest.NewRecorder()
	h.VKRedirect(rr, httptest.NewRequest(http.MethodGet, "/auth/vk?next=/dashboard", nil))
	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d", rr.Code)
	}
	resp := rr.Result()
	if cookieByName(resp, "vk_oauth_state") == nil {
		t.Fatal("missing state cookie")
	}
	if cookieByName(resp, "vk_pkce_verifier") == nil {
		t.Fatal("missing PKCE cookie")
	}
	if cookieByName(resp, oauthNextCookie) == nil {
		t.Fatal("missing next cookie (valid next should be persisted)")
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "id.vk.com/authorize") || !strings.Contains(loc, "code_challenge_method=S256") {
		t.Fatalf("bad redirect location: %s", loc)
	}
}

func TestYandexRedirect_SetsStateAndRedirects(t *testing.T) {
	h := newTestOAuthHandler(t, &mockAuthClient{})
	rr := httptest.NewRecorder()
	h.YandexRedirect(rr, httptest.NewRequest(http.MethodGet, "/auth/yandex", nil))
	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d", rr.Code)
	}
	if cookieByName(rr.Result(), "yandex_oauth_state") == nil {
		t.Fatal("missing yandex state cookie")
	}
	if !strings.Contains(rr.Result().Header.Get("Location"), "oauth.yandex.ru/authorize") {
		t.Fatalf("bad location: %s", rr.Result().Header.Get("Location"))
	}
}

func TestYandexRedirect_NotConfigured(t *testing.T) {
	h, _ := NewOAuthHandler(zerolog.Nop(), &mockAuthClient{}, OAuthConfig{BaseURL: "http://gw.local", FrontendURL: "http://front.local"})
	rr := httptest.NewRecorder()
	h.YandexRedirect(rr, httptest.NewRequest(http.MethodGet, "/auth/yandex", nil))
	if rr.Code == http.StatusTemporaryRedirect {
		t.Fatalf("expected not-configured error, got redirect")
	}
}

// --- Callback error paths (no network needed) ---

func redirectsToLoginError(t *testing.T, rr *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d", rr.Code)
	}
	loc := rr.Result().Header.Get("Location")
	if !strings.Contains(loc, "/auth/login") || !strings.Contains(loc, "error="+wantCode) {
		t.Fatalf("location = %s, want error=%s", loc, wantCode)
	}
}

func TestVKCallback_StateMismatch(t *testing.T) {
	h := newTestOAuthHandler(t, &mockAuthClient{})
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/auth/vk/callback?state=abc", nil)
	r.AddCookie(&http.Cookie{Name: "vk_oauth_state", Value: "different"})
	h.VKCallback(rr, r)
	redirectsToLoginError(t, rr, string(oauthErrStateMismatch))
}

func TestVKCallback_MissingPKCE(t *testing.T) {
	h := newTestOAuthHandler(t, &mockAuthClient{})
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/auth/vk/callback?state=abc", nil)
	r.AddCookie(&http.Cookie{Name: "vk_oauth_state", Value: "abc"})
	// no vk_pkce_verifier cookie
	h.VKCallback(rr, r)
	redirectsToLoginError(t, rr, string(oauthErrMissingPKCE))
}

func TestVKCallback_Denied(t *testing.T) {
	h := newTestOAuthHandler(t, &mockAuthClient{})
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/auth/vk/callback?state=abc", nil) // no code
	r.AddCookie(&http.Cookie{Name: "vk_oauth_state", Value: "abc"})
	r.AddCookie(&http.Cookie{Name: "vk_pkce_verifier", Value: "verifier"})
	h.VKCallback(rr, r)
	redirectsToLoginError(t, rr, string(oauthErrDenied))
}

func TestYandexCallback_StateMismatch(t *testing.T) {
	h := newTestOAuthHandler(t, &mockAuthClient{})
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/auth/yandex/callback?state=abc", nil)
	r.AddCookie(&http.Cookie{Name: "yandex_oauth_state", Value: "nope"})
	h.YandexCallback(rr, r)
	redirectsToLoginError(t, rr, string(oauthErrStateMismatch))
}

func TestYandexCallback_Denied(t *testing.T) {
	h := newTestOAuthHandler(t, &mockAuthClient{})
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/auth/yandex/callback?state=abc", nil) // no code
	r.AddCookie(&http.Cookie{Name: "yandex_oauth_state", Value: "abc"})
	h.YandexCallback(rr, r)
	redirectsToLoginError(t, rr, string(oauthErrDenied))
}

// --- oauthLogin ---

func TestOAuthLogin_NoEmailRedirectsError(t *testing.T) {
	h := newTestOAuthHandler(t, &mockAuthClient{})
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/cb", nil)
	h.oauthLogin(rr, r, "vk", "123", "", false, "Ivan", "")
	redirectsToLoginError(t, rr, string(oauthErrNoEmail))
}

func TestOAuthLogin_AuthServiceError(t *testing.T) {
	auth := &mockAuthClient{OAuthLoginFn: func(context.Context, *authv1.OAuthLoginRequest, ...grpc.CallOption) (*authv1.OAuthLoginResponse, error) {
		return nil, status.Error(codes.PermissionDenied, "banned")
	}}
	h := newTestOAuthHandler(t, auth)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/cb", nil)
	h.oauthLogin(rr, r, "vk", "123", "a@b.c", true, "Ivan", "")
	redirectsToLoginError(t, rr, string(oauthErrAuthFailed))
}

func TestOAuthLogin_SuccessSetsRefreshCookieAndFragment(t *testing.T) {
	var gotReq *authv1.OAuthLoginRequest
	auth := &mockAuthClient{OAuthLoginFn: func(_ context.Context, in *authv1.OAuthLoginRequest, _ ...grpc.CallOption) (*authv1.OAuthLoginResponse, error) {
		gotReq = in
		return &authv1.OAuthLoginResponse{AccessToken: "acc-tok", RefreshToken: "ref-tok"}, nil
	}}
	h := newTestOAuthHandler(t, auth)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/cb", nil)
	r.AddCookie(&http.Cookie{Name: oauthNextCookie, Value: "/dashboard"})
	h.oauthLogin(rr, r, "yandex", "y1", "a@b.c", true, "Ivan", "http://av")

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d", rr.Code)
	}
	if gotReq.GetProvider() != "yandex" || gotReq.GetEmail() != "a@b.c" || !gotReq.GetEmailVerified() {
		t.Fatalf("login req = %+v", gotReq)
	}
	resp := rr.Result()
	rc := cookieByName(resp, "banya_refresh")
	if rc == nil || rc.Value != "ref-tok" || !rc.HttpOnly {
		t.Fatalf("refresh cookie = %+v", rc)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "/auth/callback") {
		t.Fatalf("location = %s", loc)
	}
	if !strings.Contains(loc, "access_token") {
		t.Fatalf("access token missing from fragment: %s", loc)
	}
	if !strings.Contains(loc, "next=") {
		t.Fatalf("validated next should ride in query: %s", loc)
	}
	// oauth_next cookie should be cleared (MaxAge<0)
	if nc := cookieByName(resp, oauthNextCookie); nc == nil || nc.MaxAge >= 0 {
		t.Fatalf("next cookie not cleared: %+v", nc)
	}
}

// --- helpers: generateState + cookie clears ---

func TestGenerateState_UniqueAndURLSafe(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		s, err := generateState()
		if err != nil {
			t.Fatalf("generateState: %v", err)
		}
		if s == "" || seen[s] {
			t.Fatalf("non-unique or empty state: %q", s)
		}
		seen[s] = true
		if strings.ContainsAny(s, "+/=") {
			t.Fatalf("state not base64url: %q", s)
		}
	}
}

func TestClearCookies_ExpireImmediately(t *testing.T) {
	h := newTestOAuthHandler(t, &mockAuthClient{})
	rr := httptest.NewRecorder()
	h.clearNextCookie(rr)
	h.clearPKCECookie(rr)
	resp := rr.Result()
	for _, name := range []string{oauthNextCookie, "vk_pkce_verifier"} {
		c := cookieByName(resp, name)
		if c == nil || c.MaxAge >= 0 {
			t.Fatalf("cookie %s not expired: %+v", name, c)
		}
	}
}

func TestSetPKCECookie_Attributes(t *testing.T) {
	h := newTestOAuthHandler(t, &mockAuthClient{})
	rr := httptest.NewRecorder()
	h.setPKCECookie(rr, "verifier-value")
	c := cookieByName(rr.Result(), "vk_pkce_verifier")
	if c == nil || c.Value != "verifier-value" || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("pkce cookie = %+v", c)
	}
}
