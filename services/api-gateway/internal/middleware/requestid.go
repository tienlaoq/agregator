package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type ctxRequestIDKey struct{}

var ctxRequestID = ctxRequestIDKey{}

const HeaderRequestID string = "X-Request-ID"

// WithRequestID returns a copy of ctx with the given request ID stored under
// the same key that the RequestID middleware uses. Intended for tests.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxRequestID, id)
}

func RequestIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxRequestID).(string); ok {
		return v
	}
	return ""
}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if id == "" {
			id = uuid.New().String()
		}
		ctx := context.WithValue(r.Context(), ctxRequestID, id)
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
