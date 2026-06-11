package middleware

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

// hijackableRecorder имитирует боевой ResponseWriter, поддерживающий Hijack.
type hijackableRecorder struct{ *httptest.ResponseRecorder }

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

// Регрессия: wrappedWriter обязан пробрасывать http.Hijacker, иначе gorilla
// отвечает 500 на каждый WebSocket-апгрейд при включённом request-логировании.
func TestLogging_preservesHijacker(t *testing.T) {
	t.Parallel()

	var sawHijacker bool
	h := Logging(zerolog.Nop())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, sawHijacker = w.(http.Hijacker)
	}))
	h.ServeHTTP(&hijackableRecorder{httptest.NewRecorder()},
		httptest.NewRequest(http.MethodGet, "/api/v2/chat/ws", http.NoBody))

	if !sawHijacker {
		t.Fatal("wrappedWriter must forward http.Hijacker — WebSocket upgrades return 500 otherwise")
	}
}
