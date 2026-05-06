package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	"github.com/redis/go-redis/v9"
)

type ctxKey string

const (
	CtxUserID ctxKey = "user_id"
	CtxRole   ctxKey = "role"
	CtxEmail  ctxKey = "email"
)

func UserIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(CtxUserID).(string); ok {
		return v
	}
	return ""
}

func RoleFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(CtxRole).(string); ok {
		return v
	}
	return ""
}

func EmailFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(CtxEmail).(string); ok {
		return v
	}
	return ""
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
	// Browser WebSocket cannot set custom Authorization headers.
	qToken := strings.TrimSpace(r.URL.Query().Get("access_token"))
	if qToken != "" {
		return qToken, true, false
	}
	return "", false, false
}

// AuthOptional validates Bearer token when present and attaches user id/role to context; no header means anonymous.
// Malformed or invalid token returns 401.
func AuthOptional(authClient authv1.AuthServiceClient) func(http.Handler) http.Handler {
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
			resp, err := authClient.ValidateToken(r.Context(), &authv1.ValidateTokenRequest{
				AccessToken: token,
			})
			if err != nil || !resp.GetValid() {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}
			ctx := r.Context()
			ctx = context.WithValue(ctx, CtxUserID, resp.GetUserId())
			ctx = context.WithValue(ctx, CtxRole, resp.GetRole())
			ctx = context.WithValue(ctx, CtxEmail, resp.GetEmail())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func Auth(authClient authv1.AuthServiceClient, wsTicketRedis *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if wsTicketRedis != nil {
				if t := strings.TrimSpace(r.URL.Query().Get("ws_ticket")); t != "" {
					key := "chat:wst:" + t
					val, err := wsTicketRedis.GetDel(r.Context(), key).Result()
					if err == nil && val != "" {
						var payload struct {
							UserID string `json:"user_id"`
							Role   string `json:"role"`
							Email  string `json:"email"`
						}
						if json.Unmarshal([]byte(val), &payload) == nil && strings.TrimSpace(payload.UserID) != "" {
							ctx := r.Context()
							ctx = context.WithValue(ctx, CtxUserID, strings.TrimSpace(payload.UserID))
							ctx = context.WithValue(ctx, CtxRole, strings.TrimSpace(payload.Role))
							ctx = context.WithValue(ctx, CtxEmail, strings.TrimSpace(payload.Email))
							next.ServeHTTP(w, r.WithContext(ctx))
							return
						}
					}
				}
			}

			token, ok, malformed := bearerTokenFromRequest(r)
			if malformed {
				http.Error(w, `{"error":"invalid authorization header"}`, http.StatusUnauthorized)
				return
			}
			if !ok {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			resp, err := authClient.ValidateToken(r.Context(), &authv1.ValidateTokenRequest{
				AccessToken: token,
			})
			if err != nil || !resp.GetValid() {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, CtxUserID, resp.GetUserId())
			ctx = context.WithValue(ctx, CtxRole, resp.GetRole())
			ctx = context.WithValue(ctx, CtxEmail, resp.GetEmail())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := RoleFromCtx(r.Context())
			if _, ok := allowed[role]; !ok {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
