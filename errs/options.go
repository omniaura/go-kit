package errs

import "github.com/rs/zerolog"

type options struct {
	level    zerolog.Level
	retry    RetryPolicy
	retrySet bool
}

type optFunc func(*options)

func WithLevel(level zerolog.Level) optFunc {
	return func(o *options) {
		o.level = level
	}
}

func WithRetryPolicy(policy RetryPolicy) optFunc {
	return func(o *options) {
		o.retry = policy
		o.retrySet = true
	}
}
