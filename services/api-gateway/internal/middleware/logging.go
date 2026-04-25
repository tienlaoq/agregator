package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

type wrappedWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *wrappedWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func traceIDFromCtx(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.HasTraceID() {
		return ""
	}
	return sc.TraceID().String()
}

func Logging(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &wrappedWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(ww, r)

			ev := log.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.statusCode).
				Dur("duration", time.Since(start)).
				Str("request_id", RequestIDFromCtx(r.Context()))
			if tid := traceIDFromCtx(r.Context()); tid != "" {
				ev = ev.Str("trace_id", tid)
			}
			ev.Msg("request")
		})
	}
}
