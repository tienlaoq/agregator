package middleware

import (
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
		return http.TimeoutHandler(next, d, `{"error":"request timeout"}`)
	}
}
