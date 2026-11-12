package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"universal-defi-intelligence/internal/swap/domain"
)

func fixedRand(value float64) func() float64 {
	return func() float64 { return value }
}

func TestDelayClampsAndJitters(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   time.Second,
		MaxDelay:    30 * time.Second,
		Jitter:      0.25,
	}

	noJitter := policy.WithRand(fixedRand(0.5))
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{10, 30 * time.Second}, // clamped
		{0, time.Second},       // attempt < 1 normalized to 1
	}
	for _, tc := range cases {
		if got := noJitter.Delay(tc.attempt); got != tc.want {
			t.Fatalf("Delay(%d) = %s, want %s", tc.attempt, got, tc.want)
		}
	}

	if got := policy.WithRand(fixedRand(1)).Delay(1); got != 1250*time.Millisecond {
		t.Fatalf("Delay with +jitter = %s, want 1.25s", got)
	}
	if got := policy.WithRand(fixedRand(0)).Delay(1); got != 750*time.Millisecond {
		t.Fatalf("Delay with -jitter = %s, want 0.75s", got)
	}
}

func TestRetryableClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"rate limited", domain.New(domain.CodeProviderRateLimited, ""), true},
		{"delayed", domain.New(domain.CodeProviderDelayed, ""), true},
		{"rpc timeout", domain.New(domain.CodeRPCTimeout, ""), true},
		{"internal", domain.New(domain.CodeInternal, ""), true},
		{"auth failed", domain.New(domain.CodeProviderAuthFailed, ""), false},
		{"unknown status", domain.New(domain.CodeUnknownProviderStatus, ""), false},
		{"context canceled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, true},
		{"non retryable wrapper", NonRetryable(domain.New(domain.CodeProviderRateLimited, "")), false},
	}
	for _, tc := range cases {
		if got := Retryable(tc.err); got != tc.want {
			t.Fatalf("%s: Retryable = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDoRetriesRetryableThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	policy := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}.
		WithRand(fixedRand(0.5))

	err := policy.Do(context.Background(), func(context.Context) error {
		if calls.Add(1) < 3 {
			return domain.New(domain.CodeProviderRateLimited, "slow down")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do returned %v, want nil", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("calls = %d, want 3", got)
	}
}

func TestDoStopsOnNonRetryable(t *testing.T) {
	var calls atomic.Int32
	policy := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond}.WithRand(fixedRand(0.5))

	err := policy.Do(context.Background(), func(context.Context) error {
		calls.Add(1)
		return domain.New(domain.CodeProviderAuthFailed, "bad token")
	})
	if !domain.IsCode(err, domain.CodeProviderAuthFailed) {
		t.Fatalf("Do error = %v, want PROVIDER_AUTH_FAILED", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}

func TestDoNeverRetriesWrappedCall(t *testing.T) {
	var calls atomic.Int32
	policy := RetryPolicy{MaxAttempts: 5, BaseDelay: time.Millisecond}.WithRand(fixedRand(0.5))

	err := policy.Do(context.Background(), func(context.Context) error {
		calls.Add(1)
		return NonRetryable(domain.New(domain.CodeProviderRateLimited, "executable quote"))
	})
	if !domain.IsCode(err, domain.CodeProviderRateLimited) {
		t.Fatalf("Do error = %v, want PROVIDER_RATE_LIMITED", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1 (non-retryable must run once)", got)
	}
}

func TestDoHonorsRetryAfterHint(t *testing.T) {
	hint := 40 * time.Millisecond
	err := domain.New(domain.CodeProviderRateLimited, "slow down").WithRetryAfter(hint)
	got, ok := retryAfter(err)
	if !ok || got != hint {
		t.Fatalf("retryAfter = (%s, %v), want (%s, true)", got, ok, hint)
	}

	var calls atomic.Int32
	policy := RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond}.WithRand(fixedRand(0.5))
	start := time.Now()
	_ = policy.Do(context.Background(), func(context.Context) error {
		if calls.Add(1) == 1 {
			return err
		}
		return nil
	})
	if elapsed := time.Since(start); elapsed < hint {
		t.Fatalf("Do elapsed = %s, want at least the %s Retry-After hint", elapsed, hint)
	}
}

func TestDoStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	policy := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second}.WithRand(fixedRand(0.5))

	var calls atomic.Int32
	err := policy.Do(ctx, func(context.Context) error {
		calls.Add(1)
		return domain.New(domain.CodeProviderRateLimited, "slow")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Do error = %v, want context.Canceled", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("calls = %d, want 0 when the context is already cancelled", got)
	}
}

func TestIsNonRetryable(t *testing.T) {
	if IsNonRetryable(nil) {
		t.Fatal("nil reported as non-retryable")
	}
	if !IsNonRetryable(NonRetryable(errors.New("x"))) {
		t.Fatal("wrapped error not reported as non-retryable")
	}
	if IsNonRetryable(errors.New("plain")) {
		t.Fatal("plain error reported as non-retryable")
	}
}
