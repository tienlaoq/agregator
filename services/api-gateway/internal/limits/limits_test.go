package limits

import (
	"testing"
	"time"
)

func TestIntLimit(t *testing.T) {
	tests := []struct {
		name string
		env  string // value to set for GATEWAY_LIMIT_TEST_INT; "" means unset
		def  int
		want int
	}{
		{"unset uses default", "", 42, 42},
		{"valid override", "100", 42, 100},
		{"zero override", "0", 42, 0},
		{"negative override", "-5", 42, -5},
		{"unparseable falls back to default", "abc", 42, 42},
		{"empty string uses default", "", 7, 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("GATEWAY_LIMIT_TEST_INT", tc.env)
			}
			if got := intLimit("TEST_INT", tc.def); got != tc.want {
				t.Fatalf("intLimit = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDurLimit(t *testing.T) {
	tests := []struct {
		name string
		env  string
		def  time.Duration
		want time.Duration
	}{
		{"unset uses default", "", time.Minute, time.Minute},
		{"valid seconds override", "30s", time.Minute, 30 * time.Second},
		{"valid minutes override", "2m", time.Minute, 2 * time.Minute},
		{"unparseable falls back to default", "notaduration", 5 * time.Second, 5 * time.Second},
		{"plain integer is not a duration", "100", 5 * time.Second, 5 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("GATEWAY_LIMIT_TEST_DUR", tc.env)
			}
			if got := durLimit("TEST_DUR", tc.def); got != tc.want {
				t.Fatalf("durLimit = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCompiledDefaults asserts the package-level defaults resolved at init time
// hold sane, non-zero values and respect the documented invariants.
func TestCompiledDefaults(t *testing.T) {
	if ChatSendRateMax <= 0 {
		t.Errorf("ChatSendRateMax must be positive, got %d", ChatSendRateMax)
	}
	if ChatSendRateWindow <= 0 {
		t.Errorf("ChatSendRateWindow must be positive, got %v", ChatSendRateWindow)
	}
	// Documented invariant: PongWait must exceed PingInterval for at least one
	// full round-trip before a dead-connection close.
	if ChatWSPongWait <= ChatWSPingInterval {
		t.Errorf("ChatWSPongWait (%v) must exceed ChatWSPingInterval (%v)",
			ChatWSPongWait, ChatWSPingInterval)
	}
	if JSONMaxBodyBytes <= 0 {
		t.Errorf("JSONMaxBodyBytes must be positive, got %d", JSONMaxBodyBytes)
	}
	if PhotoMaxBytes <= 0 {
		t.Errorf("PhotoMaxBytes must be positive, got %d", PhotoMaxBytes)
	}
}
