package errs

import (
	"context"
	"errors"
	"unique"

	"github.com/rs/zerolog"
)

type ErrorFactory struct {
	message errorMessage
	status  uint16
	level   zerolog.Level
}

func NewFactory(statusCode int, msg string, opts ...optFunc) ErrorFactory {
	o := options{
		level: zerolog.ErrorLevel,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return ErrorFactory{
		message: errorMessage{h: unique.Make(msg)},
		status:  uint16(statusCode),
		level:   o.level,
	}
}

func (f ErrorFactory) New(ctx context.Context) *Error {
	event := zerolog.Ctx(ctx).WithLevel(f.level)
	return &Error{
		Status:  f.status,
		message: f.message,
		ctx:     ctx,
		event:   event,
	}
}

func (f ErrorFactory) Is(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Status == f.status && e.message == f.message
	}
	return false
}

func (f ErrorFactory) Not(err error) bool {
	return !f.Is(err)
}
