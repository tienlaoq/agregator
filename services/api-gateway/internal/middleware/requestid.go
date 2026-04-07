package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

const (
	CtxRequestID ctxKey    = "request_id"
	HeaderRequestID string = "X-Request-ID"
)

func RequestIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(CtxRequestID).(string); ok {
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
		ctx := context.WithValue(r.Context(), CtxRequestID, id)
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
