package worker

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"time"

	"universal-defi-intelligence/internal/swap/domain"
)

// RetryPolicy is bounded exponential backoff with jitter.
//
// It only retries errors that are known to be safe to retry. A non-idempotent
// call such as an executable quote must be wrapped with NonRetryable so a
// duplicate provider record can never be created.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	// Jitter is the symmetric fraction applied to each delay (0.25 = +/-25%).
	Jitter float64

	rng func() float64
}

// DefaultRetryPolicy mirrors the handoff document: at most 3 attempts, a 1s
// base delay, a 30s cap, and +/-25% jitter.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   time.Second,
		MaxDelay:    30 * time.Second,
		Jitter:      0.25,
	}
}

// WithRand returns a copy that draws jitter from rng. Tests use it to make the
// backoff deterministic.
func (p RetryPolicy) WithRand(rng func() float64) RetryPolicy {
	p.rng = rng
	return p
}

// Delay returns the backoff before the given 1-based attempt, clamped to
// MaxDelay and jittered.
func (p RetryPolicy) Delay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	base := p.BaseDelay
	if base <= 0 {
		base = time.Second
	}
	maxDelay := p.MaxDelay
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}

	delay := float64(base) * math.Pow(2, float64(attempt-1))
	if delay > float64(maxDelay) {
		delay = float64(maxDelay)
	}
	if p.Jitter > 0 {
		rng := p.rng
		if rng == nil {
			rng = rand.Float64
		}
		// Map [0,1) to [-1,1) and scale by the jitter fraction.
		delta := (rng()*2 - 1) * p.Jitter
		delay *= 1 + delta
	}
	if delay < 0 {
		delay = 0
	}
	return time.Duration(delay)
}

// Do runs op, retrying only retryable failures up to MaxAttempts. A
// non-retryable error stops immediately. A provider Retry-After hint overrides
// the computed delay when it is longer.
func (p RetryPolicy) Do(ctx context.Context, op func(context.Context) error) error {
	attempts := p.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return lastErr
			}
			return err
		}

		err := op(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		if !Retryable(err) || attempt == attempts {
			return err
		}

		delay := p.Delay(attempt)
		if hint, ok := retryAfter(err); ok && hint > delay {
			delay = hint
		}
		if err := sleep(ctx, delay); err != nil {
			return lastErr
		}
	}
	return lastErr
}

// nonRetryableError marks an error as forbidden from retry.
type nonRetryableError struct{ err error }

func (e nonRetryableError) Error() string { return e.err.Error() }
func (e nonRetryableError) Unwrap() error { return e.err }

// NonRetryable wraps err so RetryPolicy.Do returns it without retrying.
func NonRetryable(err error) error {
	if err == nil {
		return nil
	}
	return nonRetryableError{err: err}
}

// IsNonRetryable reports whether err was wrapped by NonRetryable.
func IsNonRetryable(err error) bool {
	var target nonRetryableError
	return errors.As(err, &target)
}

// Retryable reports whether err is safe to retry. Context cancellation is never
// retryable; a timeout and transient provider/transport failures are.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	if IsNonRetryable(err) {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	switch domain.CodeOf(err) {
	case domain.CodeProviderRateLimited,
		domain.CodeProviderDelayed,
		domain.CodeRPCTimeout,
		domain.CodeInternal,
		domain.CodeInternalError:
		return true
	default:
		return false
	}
}

func retryAfter(err error) (time.Duration, bool) {
	if typed, ok := domain.AsError(err); ok && typed.RetryAfter > 0 {
		return typed.RetryAfter, true
	}
	return 0, false
}

func sleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
