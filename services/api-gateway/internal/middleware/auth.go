package middleware

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	pkgauth "github.com/tienlao/agregator/pkg/auth"
	"github.com/tienlao/agregator/pkg/roles"
)

// Context keys use unexported empty-struct types so they are package-unique by
// construction. Unlike type ctxKey string, two packages that both define
// type ctxKey string can collide; two packages with separate unexported struct
// types cannot share a key even if the type names happen to be identical.
type (
	ctxUserIDKey struct{}
	ctxRoleKey   struct{}
	ctxEmailKey  struct{}
)

var (
	ctxUserID = ctxUserIDKey{}
	ctxRole   = ctxRoleKey{}
	ctxEmail  = ctxEmailKey{}
)

func UserIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxUserID).(string); ok {
		return v
	}
	return ""
}

// WithUserID returns a copy of ctx with userID stored under the same key that
// the Auth middleware uses. Intended for tests that need to bypass the full
// JWT flow but still exercise handlers that call UserIDFromCtx.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxUserID, userID)
}

// WithRole returns a copy of ctx with role stored under the same key that
// the Auth middleware uses. Intended for tests alongside WithUserID.
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, ctxRole, role)
}

// WithEmail returns a copy of ctx with email stored under the same key that
// the Auth middleware uses. Intended for tests alongside WithUserID.
func WithEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, ctxEmail, email)
}

func RoleFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxRole).(string); ok {
		return v
	}
	return ""
}

func EmailFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxEmail).(string); ok {
		return v
	}
	return ""
}

// isWebSocketUpgrade reports whether r is a WebSocket upgrade request.
// Browser WebSocket API cannot set custom headers, so ws-only paths use
// query-parameter auth instead of Authorization header.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func bearerTokenFromRequest(r *http.Request) (token string, found bool, malformed bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header != "" {
		t, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || strings.TrimSpace(t) == "" {
			return "", false, true
		}
		return strings.TrimSpace(t), true, false
	}
	// ?access_token= is accepted ONLY for WebSocket upgrade requests.
	// Allowing it on regular HTTP requests would expose the token in:
	//   - proxy/CDN access logs (full URL is logged)
	//   - Referer headers (if the page loads external resources)
	//   - browser history
	// The ws_ticket mechanism (Redis one-time token) is preferred for WebSocket
	// auth; ?access_token= is kept as a fallback for environments without Redis.
	if isWebSocketUpgrade(r) {
		if qToken := strings.TrimSpace(r.URL.Query().Get("access_token")); qToken != "" {
			return qToken, true, false
		}
	}
	return "", false, false
}

// claimsToCtx attaches JWT claims to the request context.
func claimsToCtx(ctx context.Context, claims *pkgauth.Claims) context.Context {
	ctx = context.WithValue(ctx, ctxUserID, claims.UserID)
	ctx = context.WithValue(ctx, ctxRole, claims.Role)
	ctx = context.WithValue(ctx, ctxEmail, claims.Email)
	return ctx
}

// validateLocal verifies the ES256 token signature and expiry locally using the
// EC public key. Returns nil claims + false on any failure.
//
// pubKey should never be nil in production — the gateway validates it is set
// during startup (LoadConfig → Validate). A nil key here indicates a
// programming error (e.g. the key was not wired through to the middleware), so
// we log at error level rather than silently returning 401.
func validateLocal(pubKey *ecdsa.PublicKey, token string, log zerolog.Logger) (*pkgauth.Claims, bool) {
	if pubKey == nil {
		log.Error().Msg("validateLocal: JWT public key is nil — JWT_EC_PUBLIC_KEY not wired to Auth middleware")
		return nil, false
	}
	claims, err := pkgauth.ValidateAccessToken(pubKey, token)
	if err != nil {
		return nil, false
	}
	return claims, true
}

// AuthOptional validates Bearer token when present and attaches user id/role
// to context; no header means anonymous. Malformed or invalid token returns
// 401.
//
// pubKey is the ES256 ECDSA public key loaded at startup from JWT_EC_PUBLIC_KEY.
// Token verification happens entirely in-process (no gRPC call). Callers must
// ensure the key is set in production via a fatal check in main.
func AuthOptional(log zerolog.Logger, pubKey *ecdsa.PublicKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok, malformed := bearerTokenFromRequest(r)
			if malformed {
				http.Error(w, `{"error":"invalid authorization header"}`, http.StatusUnauthorized)
				return
			}
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			claims, valid := validateLocal(pubKey, token, log)
			if !valid {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(claimsToCtx(r.Context(), claims)))
		})
	}
}

// Auth validates the Bearer token (or ws_ticket for WebSocket upgrades) and
// requires authentication. Unauthenticated requests receive 401.
//
// Token verification is local (ES256 with pubKey) — no gRPC call to
// auth-service on the hot path. This eliminates auth-service as a latency
// bottleneck and single point of failure for every gateway request.
//
// ws_ticket path is unchanged: one-time token fetched from Redis (already
// issued by auth-service at ticket issuance time, bound to origin).
func Auth(log zerolog.Logger, pubKey *ecdsa.PublicKey, wsTicketRedis *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// ── ws_ticket fast path (WebSocket upgrade) ─────────────────────
			if wsTicketRedis != nil {
				if t := strings.TrimSpace(r.URL.Query().Get("ws_ticket")); t != "" {
					key := "chat:wst:" + t
					val, err := wsTicketRedis.GetDel(r.Context(), key).Result()
					if err == nil && val != "" {
						var payload struct {
							UserID string `json:"user_id"`
							Role   string `json:"role"`
							Email  string `json:"email"`
							Origin string `json:"origin"`
						}
						if json.Unmarshal([]byte(val), &payload) == nil && strings.TrimSpace(payload.UserID) != "" {
							requestOrigin := strings.TrimSpace(r.Header.Get("Origin"))
							storedOrigin := strings.TrimSpace(payload.Origin)
							if storedOrigin != "" && !strings.EqualFold(storedOrigin, requestOrigin) {
								log.Warn().
									Str("stored_origin", storedOrigin).
									Str("request_origin", requestOrigin).
									Str("user_id", payload.UserID).
									Msg("chat: ws_ticket origin mismatch, rejecting")
								http.Error(w, `{"error":"invalid ws_ticket"}`, http.StatusUnauthorized)
								return
							}
							ctx := r.Context()
							ctx = context.WithValue(ctx, ctxUserID, strings.TrimSpace(payload.UserID))
							ctx = context.WithValue(ctx, ctxRole, strings.TrimSpace(payload.Role))
							ctx = context.WithValue(ctx, ctxEmail, strings.TrimSpace(payload.Email))
							next.ServeHTTP(w, r.WithContext(ctx))
							return
						}
					}
				}
			}

			// ── Bearer token path ────────────────────────────────────────────
			token, ok, malformed := bearerTokenFromRequest(r)
			if malformed {
				http.Error(w, `{"error":"invalid authorization header"}`, http.StatusUnauthorized)
				return
			}
			if !ok {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			claims, valid := validateLocal(pubKey, token, log)
			if !valid {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r.WithContext(claimsToCtx(r.Context(), claims)))
		})
	}
}

// RequireRole returns a middleware that allows only requests whose context role
// matches one of the provided roles constants. Always pass roles.Role*
// constants from the pkg/roles package — never raw string literals.
func RequireRole(allowed ...roles.Role) func(http.Handler) http.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		set[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := RoleFromCtx(r.Context())
			if _, ok := set[role]; !ok {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
