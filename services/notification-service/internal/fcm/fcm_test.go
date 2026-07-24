package fcm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
)

// senderTo builds a Sender pointed at a test server, bypassing New/OAuth.
func senderTo(url string, client *http.Client) *Sender {
	return &Sender{projectID: "test", endpoint: url, client: client}
}

// TestPush_PrunesOnlyOn404 guards the one subtle rule: a token is pruned only on
// 404 UNREGISTERED. A 400 (possible payload bug on our side) or any transient
// failure must NOT prune, or a code bug would silently disable push for everyone.
func TestPush_PrunesOnlyOn404(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		wantPruned bool
		wantErr    bool
	}{
		{"unregistered 404 -> prune, no error", http.StatusNotFound, true, false},
		{"malformed 400 -> keep, error", http.StatusBadRequest, false, true},
		{"transient 500 -> keep, error", http.StatusInternalServerError, false, true},
		{"ok 200 -> keep, no error", http.StatusOK, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			s := senderTo(srv.URL, srv.Client())
			n := &domain.Notification{ID: uuid.New(), Type: "booking", Title: "hi", Body: "b"}
			invalid, err := s.Push(context.Background(), []string{"tok"}, n)

			if (len(invalid) > 0) != tt.wantPruned {
				t.Fatalf("invalid = %v, wantPruned = %v", invalid, tt.wantPruned)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestPush_PerTokenClassification verifies a mixed batch: only the 404 token is
// pruned, every token is still attempted, and the first error is surfaced
// without aborting the loop. The token is echoed in the request body, so the
// server routes the status by inspecting it.
func TestPush_PerTokenClassification(t *testing.T) {
	var attempted int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		attempted++
		switch {
		case strings.Contains(string(body), `"token":"dead"`):
			w.WriteHeader(http.StatusNotFound)
		case strings.Contains(string(body), `"token":"bad"`):
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	s := senderTo(srv.URL, srv.Client())
	n := &domain.Notification{ID: uuid.New(), Title: "hi"}
	invalid, err := s.Push(context.Background(), []string{"good", "dead", "bad"}, n)

	if attempted != 3 {
		t.Fatalf("attempted %d tokens, want all 3", attempted)
	}
	if len(invalid) != 1 || invalid[0] != "dead" {
		t.Fatalf("invalid = %v, want [dead]", invalid)
	}
	if err == nil {
		t.Fatal("expected the 400 token's error to surface")
	}
}

// TestPush_IncludesPayloadOnlyWhenSet checks the data map always carries type
// and notification_id, and includes the raw payload only when Data is non-empty.
func TestPush_IncludesPayloadOnlyWhenSet(t *testing.T) {
	capture := func(n *domain.Notification) string {
		var body string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		s := senderTo(srv.URL, srv.Client())
		if _, err := s.Push(context.Background(), []string{"tok"}, n); err != nil {
			t.Fatalf("Push: %v", err)
		}
		return body
	}

	id := uuid.New()
	withData := capture(&domain.Notification{ID: id, Type: "booking", Title: "hi", Data: `{"venue_id":"v1"}`})
	for _, want := range []string{`"type":"booking"`, id.String(), "payload", "venue_id"} {
		if !strings.Contains(withData, want) {
			t.Fatalf("body missing %q: %s", want, withData)
		}
	}

	noData := capture(&domain.Notification{ID: id, Type: "booking", Title: "hi"})
	if strings.Contains(noData, "payload") {
		t.Fatalf("payload key should be absent when Data empty: %s", noData)
	}
}
