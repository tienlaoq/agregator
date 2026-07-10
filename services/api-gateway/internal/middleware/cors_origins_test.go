package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginAllowed(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com")
	tests := []struct {
		name   string
		origin string // "" means header absent
		want   bool
	}{
		{"empty origin (non-browser) allowed", "", true},
		{"allowlisted host", "https://app.example.com", true},
		{"allowlisted host other path/port ignored, host matches", "https://app.example.com:443", false},
		{"localhost any port", "http://localhost:5173", true},
		{"foreign origin rejected", "https://evil.example.com", false},
		{"garbage origin rejected", "://nonsense", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/chat/ws", nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if got := OriginAllowed(r); got != tt.want {
				t.Fatalf("OriginAllowed(%q)=%v want %v", tt.origin, got, tt.want)
			}
		})
	}
}

func TestIsLocalLoopbackOrigin(t *testing.T) {
	tests := []struct {
		o    string
		want bool
	}{
		{"http://localhost:3001", true},
		{"http://localhost:5173", true},
		{"http://127.0.0.1:8080", true},
		{"http://[::1]:4321", true},
		{"https://evil.example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsLocalLoopbackOrigin(tt.o); got != tt.want {
			t.Fatalf("IsLocalLoopbackOrigin(%q)=%v want %v", tt.o, got, tt.want)
		}
	}
}
