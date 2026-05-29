package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgauth "github.com/tienlao/agregator/pkg/auth"
	"github.com/tienlao/agregator/pkg/roles"
)

// testKeyPair generates a fresh P-256 key pair for each test run.
func testKeyPair(t *testing.T) (*ecdsa.PrivateKey, *ecdsa.PublicKey) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return priv, &priv.PublicKey
}

// makeToken is a test helper that generates a signed ES256 token.
func makeToken(t *testing.T, priv *ecdsa.PrivateKey, userID, email, role string, ttl time.Duration) string {
	t.Helper()
	tok, err := pkgauth.GenerateAccessToken(priv, userID, email, role, ttl)
	require.NoError(t, err)
	return tok
}

// ── Auth middleware ──────────────────────────────────────────────────────────

func TestAuth_MissingHeader(t *testing.T) {
	_, pub := testKeyPair(t)
	h := Auth(zerolog.Nop(), pub, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "missing authorization header")
}

func TestAuth_InvalidFormat(t *testing.T) {
	_, pub := testKeyPair(t)
	h := Auth(zerolog.Nop(), pub, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "invalid")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid authorization header")
}

func TestAuth_InvalidToken(t *testing.T) {
	_, pub := testKeyPair(t)
	h := Auth(zerolog.Nop(), pub, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid or expired token")
}

func TestAuth_ExpiredToken(t *testing.T) {
	priv, pub := testKeyPair(t)
	tok := makeToken(t, priv, "uid", "e@x.com", roles.RoleUser, -time.Second)

	h := Auth(zerolog.Nop(), pub, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid or expired token")
}

func TestAuth_WrongKey(t *testing.T) {
	// Token signed with a different private key must be rejected.
	priv, _ := testKeyPair(t)
	_, wrongPub := testKeyPair(t)

	tok := makeToken(t, priv, "uid", "e@x.com", roles.RoleUser, time.Hour)

	h := Auth(zerolog.Nop(), wrongPub, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuth_ValidToken(t *testing.T) {
	priv, pub := testKeyPair(t)
	tok := makeToken(t, priv, "user-1", "a@b.c", roles.RoleUser, time.Hour)

	var uid, role, email string
	h := Auth(zerolog.Nop(), pub, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid = UserIDFromCtx(r.Context())
		role = RoleFromCtx(r.Context())
		email = EmailFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-1", uid)
	assert.Equal(t, roles.RoleUser, role)
	assert.Equal(t, "a@b.c", email)
}

func TestAuth_ValidTokenFromQueryParam(t *testing.T) {
	// Browser WebSocket API cannot set custom headers (no Authorization support),
	// so the gateway accepts ?access_token= exclusively on WebSocket upgrade
	// requests identified by the presence of "Upgrade: websocket" header.
	// This header is mandated by RFC 6455 §4.1 and is always sent by browsers
	// during the WS handshake — it is not something we add ourselves.
	priv, pub := testKeyPair(t)
	tok := makeToken(t, priv, "ws-user", "ws@b.c", roles.RoleUser, time.Hour)

	var uid string
	h := Auth(zerolog.Nop(), pub, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid = UserIDFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/chat/ws?access_token="+tok, nil)
	req.Header.Set("Upgrade", "websocket") // RFC 6455 §4.1 — browser always sends this
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ws-user", uid)
}

func TestAuth_QueryParamRejectedOnNonWS(t *testing.T) {
	// ?access_token= must NOT be accepted on regular HTTP (only WebSocket).
	priv, pub := testKeyPair(t)
	tok := makeToken(t, priv, "uid", "e@x.com", roles.RoleUser, time.Hour)

	h := Auth(zerolog.Nop(), pub, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run on non-WS request with query param token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/venues?access_token="+tok, nil)
	// No Upgrade: websocket header → must be rejected
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ── AuthOptional middleware ──────────────────────────────────────────────────

func TestAuthOptional_NoToken_PassesThrough(t *testing.T) {
	_, pub := testKeyPair(t)
	called := false
	h := AuthOptional(zerolog.Nop(), pub)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, UserIDFromCtx(req.Context())) // no claims attached
}

func TestAuthOptional_ValidToken_AttachesClaims(t *testing.T) {
	priv, pub := testKeyPair(t)
	tok := makeToken(t, priv, "opt-user", "opt@x.com", roles.RoleAdmin, time.Hour)

	var uid, role string
	h := AuthOptional(zerolog.Nop(), pub)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid = UserIDFromCtx(r.Context())
		role = RoleFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "opt-user", uid)
	assert.Equal(t, roles.RoleAdmin, role)
}

func TestAuthOptional_InvalidToken_Returns401(t *testing.T) {
	_, pub := testKeyPair(t)
	h := AuthOptional(zerolog.Nop(), pub)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer garbage")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ── RequireRole middleware ───────────────────────────────────────────────────

func TestRequireRole_Allowed(t *testing.T) {
	called := false
	h := RequireRole(roles.RoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	ctx := context.WithValue(context.Background(), ctxRole, roles.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequireRole_Forbidden(t *testing.T) {
	h := RequireRole(roles.RoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))

	ctx := context.WithValue(context.Background(), ctxRole, "guest")
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "forbidden")
}

func TestRequireRole_MultipleRoles(t *testing.T) {
	called := false
	h := RequireRole(roles.RoleAdmin, "partner", "owner")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	ctx := context.WithValue(context.Background(), ctxRole, "partner")
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ── Context helpers ──────────────────────────────────────────────────────────

func TestUserIDFromCtx(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxUserID, "u-42")
	assert.Equal(t, "u-42", UserIDFromCtx(ctx))
}

func TestUserIDFromCtx_Empty(t *testing.T) {
	assert.Empty(t, UserIDFromCtx(context.Background()))
}
