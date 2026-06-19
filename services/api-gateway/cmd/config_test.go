package main

import "testing"

func TestIsProductionEnv(t *testing.T) {
	tests := []struct {
		name string
		env  string // value for the ENV variable ("" = unset/empty)
		addr string // service address passed to isProductionEnv
		want bool
	}{
		// ENV=production is an explicit, address-independent signal.
		{"explicit production overrides loopback addr", "production", "localhost:50058", true},
		{"explicit production with empty addr", "production", "", true},
		{"explicit production is case-insensitive", "PRODUCTION", "localhost:50058", true},
		{"explicit production is trimmed", "  production  ", "127.0.0.1:50058", true},

		// Without ENV, a non-loopback address implies a real deployment.
		{"remote host implies production", "", "chat-service:50058", true},
		{"remote IP implies production", "", "10.0.0.5:50058", true},

		// Loopback / empty addresses are treated as local development.
		{"localhost is not production", "", "localhost:50058", false},
		{"127.0.0.1 is not production", "", "127.0.0.1:50058", false},
		{"ipv6 loopback without port is not production", "", "::1", false},
		{"empty addr is not production", "", "", false},
		{"non-production ENV value falls back to addr", "staging", "localhost:50058", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ENV", tt.env)
			if got := isProductionEnv(tt.addr); got != tt.want {
				t.Errorf("isProductionEnv(%q) with ENV=%q = %v, want %v", tt.addr, tt.env, got, tt.want)
			}
		})
	}
}
