package middleware

import (
	"context"
	"net/http"
	"time"
)

// RequestTimeout wraps the next handler in an [http.TimeoutHandler] so that
// any REST handler that does not finish within d returns 503 to the client and
// cancels its context.
//
// Do NOT apply this middleware to WebSocket upgrade routes: the HTTP/1.1
// upgrade handshake succeeds immediately, but the underlying TCP connection is
// then held open for the lifetime of the WebSocket session — a deadline would
// close that connection after d seconds.
//
// Typical usage in a chi router:
//
//	// REST routes — protected
//	r.Group(func(r chi.Router) {
//	    r.Use(middleware.RequestTimeout(60 * time.Second))
//	    r.Get("/venues", venueHandler.List)
//	    // ...
//	})
//
//	// WebSocket routes — no timeout middleware
//	r.Get("/chat/ws", chatHandler.WS)
func RequestTimeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		timeoutHandler := http.TimeoutHandler(next, d, `{"error":"request timeout"}`)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// http.TimeoutHandler on its own writes 503 when the timer fires but
			// does NOT cancel the request context — the handler goroutine keeps
			// running and any downstream gRPC call it made will not abort.
			// Wrapping the request with a context.WithTimeout propagates the
			// cancellation so handlers observing r.Context().Done() unblock and
			// downstream RPCs are aborted promptly.
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			timeoutHandler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
