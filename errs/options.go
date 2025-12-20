package errs

import "github.com/rs/zerolog"

type options struct {
	level zerolog.Level
}

type optFunc func(*options)

func WithLevel(level zerolog.Level) optFunc {
	return func(o *options) {
		o.level = level
	}
}
