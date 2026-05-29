package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/rs/zerolog"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
)

// Recoverer returns a middleware that catches panics, logs them via zerolog
// (with request_id, trace_id, and full stack trace), and responds with
// catalog error GATEWAY.UPSTREAM.INTERNAL (HTTP 500).
//
// chi's built-in chimw.Recoverer also does not expose the stack trace in the
// HTTP response body — that part is fine. However it has two shortcomings for
// this codebase:
//
//  1. It calls PrintPrettyStack which writes to os.Stderr, bypassing zerolog.
//     In production, stderr is often aggregated separately from structured
//     logs, so panics can disappear from alerting or appear without
//     request_id/trace_id correlation.
//
//  2. It responds with an empty HTTP 500 body, breaking the JSON error
//     contract that all other gateway endpoints follow.
//
// This implementation fixes both: panics are logged as structured zerolog
// events (same pipeline, same fields) and the response is always a JSON
// catalog entry, consistent with the rest of the API.
func Recoverer(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					// Capture the stack before doing anything else.
					stack := debug.Stack()

					rid := RequestIDFromCtx(r.Context())
					tid := traceIDFromCtx(r.Context())

					ev := log.Error().
						Str("request_id", rid).
						Str("method", r.Method).
						Str("path", r.URL.Path).
						Str("panic", fmt.Sprintf("%v", rec)).
						Bytes("stack", stack)
					if tid != "" {
						ev = ev.Str("trace_id", tid)
					}
					ev.Msg("panic recovered")

					// Respond with a generic catalog error — no stack trace in the body.
					apicatalog.GatewayUpstreamInternal.Write(w)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
