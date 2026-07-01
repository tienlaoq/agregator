package supportstore

import "testing"

// New must return a nil *Store (not a non-nil wrapper around a nil pool) when no
// pool is supplied, so callers can use a simple `if store == nil` disabled check.
func TestNew_NilPoolReturnsNilStore(t *testing.T) {
	if s := New(nil); s != nil {
		t.Fatalf("New(nil) = %v, want nil", s)
	}
}

func TestNotifyStatusConstants(t *testing.T) {
	// These strings are persisted to the notify_status column and consumed by
	// the admin UI; pin them so an accidental rename is caught.
	cases := map[string]string{
		NotifyPending: "pending",
		NotifyOK:      "ok",
		NotifyFailed:  "failed",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("notify status constant = %q, want %q", got, want)
		}
	}
}
