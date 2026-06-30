package errs_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/omniaura/go-kit/errs"
)

func TestRetryPolicy(t *testing.T) {
	t.Run("fixed retry", func(t *testing.T) {
		policy := errs.FixedRetry(3, 10*time.Millisecond)

		if policy.Attempts() != 3 {
			t.Fatalf("expected 3 attempts, got %d", policy.Attempts())
		}
		if !policy.ShouldRetry(1) {
			t.Fatal("expected retry after first attempt")
		}
		if policy.ShouldRetry(3) {
			t.Fatal("did not expect retry after max attempts")
		}
		if got := policy.Delay(2); got != 10*time.Millisecond {
			t.Fatalf("expected fixed delay, got %s", got)
		}
	})

	t.Run("exponential retry caps delay", func(t *testing.T) {
		policy := errs.ExponentialRetry(5, 500*time.Millisecond, 1200*time.Millisecond)

		if got := policy.Delay(1); got != 500*time.Millisecond {
			t.Fatalf("expected first delay 500ms, got %s", got)
		}
		if got := policy.Delay(2); got != time.Second {
			t.Fatalf("expected second delay 1s, got %s", got)
		}
		if got := policy.Delay(3); got != 1200*time.Millisecond {
			t.Fatalf("expected capped delay 1.2s, got %s", got)
		}
	})

	t.Run("zero value is not retryable", func(t *testing.T) {
		var policy errs.RetryPolicy
		if policy.ShouldRetry(1) {
			t.Fatal("zero value should not retry")
		}
		if policy.Attempts() != 1 {
			t.Fatalf("expected one attempt, got %d", policy.Attempts())
		}
	})
}

func TestRetryPolicyOf(t *testing.T) {
	policy := errs.ExponentialRetry(5, 500*time.Millisecond, 2*time.Second)
	err := errs.NewFactory(
		http.StatusTooManyRequests,
		"rate limited",
		errs.WithRetryPolicy(policy),
	).New(context.Background())

	got, ok := errs.RetryPolicyOf(err)
	if !ok {
		t.Fatal("expected retry policy")
	}
	if got != policy {
		t.Fatalf("expected %+v, got %+v", policy, got)
	}

	if _, ok := errs.RetryPolicyOf(errors.New("plain")); ok {
		t.Fatal("did not expect retry metadata on plain error")
	}
}

func TestStatusRetryPolicy(t *testing.T) {
	policies := errs.StatusRetryPolicy{
		http.StatusBadRequest:         errs.NeverRetry(),
		http.StatusTooManyRequests:    errs.ExponentialRetry(6, 500*time.Millisecond, 5*time.Second),
		http.StatusServiceUnavailable: errs.FixedRetry(3, time.Second),
	}

	if policy, ok := policies.ForStatus(http.StatusBadRequest); !ok || policy.Retryable {
		t.Fatalf("expected 400 to be explicitly non-retryable, got %+v", policy)
	}
	if policy, ok := policies.ForStatus(http.StatusTooManyRequests); !ok || !policy.Retryable {
		t.Fatalf("expected 429 retry policy, got %+v", policy)
	}
	if _, ok := policies.ForStatus(http.StatusNotFound); ok {
		t.Fatal("did not expect policy for 404")
	}
}
