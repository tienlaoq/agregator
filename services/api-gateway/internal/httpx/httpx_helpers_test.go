package httpx

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── WriteJSON ────────────────────────────────────────────────────────────────

func TestWriteJSON_SerialisesWithHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteJSON(rr, http.StatusCreated, map[string]any{"ok": true, "n": 3})

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cl := rr.Header().Get("Content-Length"); cl == "" {
		t.Fatal("Content-Length should be set")
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("body = %v", body)
	}
}

// An un-serialisable value (NaN) must yield 500 with no partial success body.
func TestWriteJSON_UnserialisableValue_500(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteJSON(rr, http.StatusOK, map[string]any{"bad": math.NaN()})

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

// ── QueryInt ─────────────────────────────────────────────────────────────────

func TestQueryInt(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		def      int
		min      int
		max      int
		wantVal  int
		wantOK   bool
		wantCode int // expected HTTP status when !ok
	}{
		{"absent uses default", "", 7, 0, 100, 7, true, 0},
		{"empty uses default", "page=", 7, 0, 100, 7, true, 0},
		{"valid in range", "page=42", 0, 0, 100, 42, true, 0},
		{"at min boundary", "page=0", 5, 0, 100, 0, true, 0},
		{"at max boundary", "page=100", 5, 0, 100, 100, true, 0},
		{"non-numeric rejected", "page=abc", 5, 0, 100, 0, false, http.StatusBadRequest},
		{"below min rejected", "page=-1", 5, 0, 100, 0, false, http.StatusBadRequest},
		{"above max rejected", "page=101", 5, 0, 100, 0, false, http.StatusBadRequest},
		{"max=0 disables upper bound", "page=999999", 5, 0, 0, 999999, true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/?"+tc.query, nil)
			rr := httptest.NewRecorder()
			got, ok := QueryInt(rr, r, "page", tc.def, tc.min, tc.max)
			if got != tc.wantVal || ok != tc.wantOK {
				t.Fatalf("QueryInt = (%d,%v), want (%d,%v)", got, ok, tc.wantVal, tc.wantOK)
			}
			if !tc.wantOK && rr.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rr.Code, tc.wantCode)
			}
		})
	}
}

// ── GRPCErrorToHTTP ──────────────────────────────────────────────────────────

func TestGRPCErrorToHTTP_MapsCodes(t *testing.T) {
	tests := []struct {
		name     string
		code     codes.Code
		wantHTTP int
		wantCode string
	}{
		{"not found", codes.NotFound, 404, "GATEWAY.UPSTREAM.NOT_FOUND"},
		{"invalid argument", codes.InvalidArgument, 400, "GATEWAY.UPSTREAM.INVALID_ARGUMENT"},
		{"permission denied", codes.PermissionDenied, 403, "GATEWAY.UPSTREAM.PERMISSION_DENIED"},
		// Unmapped code falls back to UPSTREAM.UNKNOWN (500).
		{"unmapped data loss falls back", codes.DataLoss, 500, "GATEWAY.UPSTREAM.UNKNOWN"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			GRPCErrorToHTTP(rr, status.Error(tc.code, "upstream detail"))

			if rr.Code != tc.wantHTTP {
				t.Fatalf("status = %d, want %d", rr.Code, tc.wantHTTP)
			}
			var body struct{ Code, Error string }
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", body.Code, tc.wantCode)
			}
			// The upstream detail message should be surfaced.
			if body.Error != "upstream detail" {
				t.Fatalf("error = %q, want upstream detail", body.Error)
			}
		})
	}
}

func TestGRPCErrorToHTTP_NilError(t *testing.T) {
	rr := httptest.NewRecorder()
	GRPCErrorToHTTP(rr, nil)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

// A status with an empty message must fall back to err.Error() rather than
// emitting a blank detail.
func TestGRPCErrorToHTTP_EmptyMessageFallsBack(t *testing.T) {
	rr := httptest.NewRecorder()
	GRPCErrorToHTTP(rr, status.Error(codes.NotFound, ""))

	var body struct{ Error string }
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if body.Error == "" {
		t.Fatal("error message should fall back to err.Error(), got empty")
	}
}
