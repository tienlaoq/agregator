package domain

import "testing"

func TestIsValidRole(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{"user", true},
		{"venue_owner", true},
		{"master", true},
		{"admin", true},
		{"", false}, // empty is NOT valid — callers must default first
		{"superadmin", false},
		{"User", false},  // case-sensitive
		{" user", false}, // not trimmed
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			if got := IsValidRole(tt.role); got != tt.want {
				t.Errorf("IsValidRole(%q) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}
