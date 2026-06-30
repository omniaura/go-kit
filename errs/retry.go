package errs

import (
	"context"
	"errors"
	"time"
)

// RetryPolicy describes how work that returned an error may be retried.
//
// MaxAttempts is the total number of attempts, including the first attempt.
// The zero value is not retryable.
type RetryPolicy struct {
	Retryable   bool
	MaxAttempts int
	Backoff     time.Duration
	MaxBackoff  time.Duration
	Exponential bool
}

// StatusRetryPolicy maps HTTP status codes to retry metadata.
type StatusRetryPolicy map[int]RetryPolicy

// NeverRetry returns a policy that prevents retry.
func NeverRetry() RetryPolicy {
	return RetryPolicy{}
}

// FixedRetry returns a retryable policy with a fixed delay between attempts.
func FixedRetry(maxAttempts int, backoff time.Duration) RetryPolicy {
	return RetryPolicy{
		Retryable:   true,
		MaxAttempts: maxAttempts,
		Backoff:     backoff,
	}
}

// ExponentialRetry returns a retryable policy whose delay doubles after each
// failed attempt until MaxBackoff is reached.
func ExponentialRetry(maxAttempts int, backoff, maxBackoff time.Duration) RetryPolicy {
	return RetryPolicy{
		Retryable:   true,
		MaxAttempts: maxAttempts,
		Backoff:     backoff,
		MaxBackoff:  maxBackoff,
		Exponential: true,
	}
}

// ForStatus returns the retry policy for status.
func (p StatusRetryPolicy) ForStatus(status int) (RetryPolicy, bool) {
	if p == nil {
		return RetryPolicy{}, false
	}
	policy, ok := p[status]
	return policy, ok
}

// Attempts returns the total number of attempts allowed by p.
func (p RetryPolicy) Attempts() int {
	if !p.Retryable {
		return 1
	}
	if p.MaxAttempts <= 1 {
		return 2
	}
	return p.MaxAttempts
}

// ShouldRetry reports whether another attempt should be made after attempt.
// attempt is one-based and represents the attempt that just failed.
func (p RetryPolicy) ShouldRetry(attempt int) bool {
	return p.Retryable && attempt < p.Attempts()
}

// Delay returns the delay before the next attempt after attempt failed.
func (p RetryPolicy) Delay(attempt int) time.Duration {
	if p.Backoff <= 0 {
		return 0
	}
	delay := p.Backoff
	if p.Exponential && attempt > 1 {
		for range attempt - 1 {
			if p.MaxBackoff > 0 && delay >= p.MaxBackoff {
				return p.MaxBackoff
			}
			delay *= 2
			if p.MaxBackoff > 0 && delay > p.MaxBackoff {
				return p.MaxBackoff
			}
		}
	}
	return delay
}

// Wait pauses according to p's delay for attempt, returning ctx.Err if the
// context is canceled first.
func (p RetryPolicy) Wait(ctx context.Context, attempt int) error {
	delay := p.Delay(attempt)
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

// RetryPolicyOf extracts retry metadata from err.
func RetryPolicyOf(err error) (RetryPolicy, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.RetryPolicy()
	}
	return RetryPolicy{}, false
}
