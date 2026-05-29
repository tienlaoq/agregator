package yookassa

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ── backoff ───────────────────────────────────────────────────────────────────

func TestBackoff_StaysWithinBounds(t *testing.T) {
	t.Parallel()
	for attempt := range 10 {
		d := backoff(attempt)
		// Never negative.
		if d < 0 {
			t.Errorf("attempt %d: backoff %v < 0", attempt, d)
		}
		// Never exceeds retryMax + 25% jitter headroom.
		limit := retryMax + retryMax/4
		if d > limit {
			t.Errorf("attempt %d: backoff %v > limit %v", attempt, d, limit)
		}
	}
}

func TestBackoff_GrowsWithAttempt(t *testing.T) {
	t.Parallel()
	// Average over several samples to smooth jitter.
	avg := func(attempt int) time.Duration {
		const n = 100
		var total time.Duration
		for range n {
			total += backoff(attempt)
		}
		return total / n
	}
	a0, a1, a2 := avg(0), avg(1), avg(2)
	if a1 <= a0 {
		t.Errorf("avg backoff(1)=%v should be > backoff(0)=%v", a1, a0)
	}
	if a2 <= a1 {
		t.Errorf("avg backoff(2)=%v should be > backoff(1)=%v", a2, a1)
	}
}

// ── isPermanent ───────────────────────────────────────────────────────────────

func TestIsPermanent_4xxIsTrue(t *testing.T) {
	t.Parallel()
	for _, code := range []int{400, 401, 403, 404, 422} {
		if !isPermanent(&apiError{statusCode: code}) {
			t.Errorf("expected isPermanent=true for HTTP %d", code)
		}
	}
}

func TestIsPermanent_5xxIsFalse(t *testing.T) {
	t.Parallel()
	for _, code := range []int{500, 502, 503, 504} {
		if isPermanent(&apiError{statusCode: code}) {
			t.Errorf("expected isPermanent=false for HTTP %d", code)
		}
	}
}

func TestIsPermanent_PlainErrorIsFalse(t *testing.T) {
	t.Parallel()
	if isPermanent(errors.New("network timeout")) {
		t.Error("expected isPermanent=false for plain error")
	}
}

// ── retryDo ───────────────────────────────────────────────────────────────────

func TestRetryDo_SuccessOnFirstAttempt(t *testing.T) {
	t.Parallel()
	calls := 0
	err := retryDo(context.Background(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRetryDo_RetriesTransientError(t *testing.T) {
	t.Parallel()
	calls := 0
	transient := &apiError{statusCode: 503, op: "test"}
	err := retryDo(context.Background(), func() error {
		calls++
		if calls < retryAttempts {
			return transient
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if calls != retryAttempts {
		t.Errorf("expected %d calls, got %d", retryAttempts, calls)
	}
}

func TestRetryDo_StopsOnPermanentError(t *testing.T) {
	t.Parallel()
	calls := 0
	permanent := &apiError{statusCode: 422, op: "test"}
	err := retryDo(context.Background(), func() error {
		calls++
		return permanent
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retry on 4xx), got %d", calls)
	}
}

func TestRetryDo_ExhaustsAllAttempts(t *testing.T) {
	t.Parallel()
	calls := 0
	err := retryDo(context.Background(), func() error {
		calls++
		return &apiError{statusCode: 500, op: "test"}
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if calls != retryAttempts {
		t.Errorf("expected %d calls, got %d", retryAttempts, calls)
	}
}

func TestRetryDo_RespectsContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	calls := 0
	err := retryDo(ctx, func() error {
		calls++
		return &apiError{statusCode: 500, op: "test"}
	})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	// With a pre-cancelled context the first attempt is skipped entirely.
	if calls > 1 {
		t.Errorf("expected at most 1 call with cancelled context, got %d", calls)
	}
}

// ── apiError ──────────────────────────────────────────────────────────────────

func TestAPIError_Message(t *testing.T) {
	t.Parallel()
	e := &apiError{statusCode: 404, body: "not found", op: "Capture"}
	got := e.Error()
	want := "yookassa Capture: HTTP 404: not found"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ── providerKey ───────────────────────────────────────────────────────────────

func TestProviderKey_Deterministic(t *testing.T) {
	t.Parallel()
	k1 := providerKey("some-internal-key")
	k2 := providerKey("some-internal-key")
	if k1 != k2 {
		t.Errorf("providerKey is not deterministic: %q vs %q", k1, k2)
	}
}

func TestProviderKey_DifferentInputsProduceDifferentKeys(t *testing.T) {
	t.Parallel()
	k1 := providerKey("key-aaa")
	k2 := providerKey("key-bbb")
	if k1 == k2 {
		t.Errorf("different inputs produced identical provider keys: %q", k1)
	}
}

func TestProviderKey_DoesNotExposeInternalKey(t *testing.T) {
	t.Parallel()
	internal := "sha256-derived-booking-user-key"
	pk := providerKey(internal)
	if pk == internal {
		t.Error("provider key must not be identical to internal key")
	}
}

func TestProviderKey_ValidUUID(t *testing.T) {
	t.Parallel()
	pk := providerKey("any-key")
	// UUID format: 8-4-4-4-12
	if len(pk) != 36 {
		t.Errorf("expected UUID length 36, got %d: %q", len(pk), pk)
	}
	for i, c := range pk {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				t.Errorf("expected '-' at position %d, got %q", i, c)
			}
		}
	}
}

// ── kopecksToStr ──────────────────────────────────────────────────────────────

func TestKopecksToStr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kopecks int64
		want    string
	}{
		{0, "0.00"},
		{1, "0.01"},
		{99, "0.99"},
		{100, "1.00"},
		{150, "1.50"},
		{150050, "1500.50"},
		// Exact round-number amounts must not gain a fractional part.
		{100000, "1000.00"},
		// Large amount: 9 999 999 roubles 99 kopecks.
		// float64 represents this exactly, but values above 2^53 kopecks would
		// silently lose precision — the integer path is correct for all int64 values.
		{999999999, "9999999.99"},
	}
	for _, tc := range cases {
		got := kopecksToStr(tc.kopecks)
		if got != tc.want {
			t.Errorf("kopecksToStr(%d) = %q, want %q", tc.kopecks, got, tc.want)
		}
	}
}
