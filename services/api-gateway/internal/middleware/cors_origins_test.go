package middleware

import "testing"

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
