package errs

import (
	"strings"

	"github.com/rs/zerolog"
)

func (e *Error) Err(err error) *Error {
	e.rspAnnotations = append(e.rspAnnotations, err.Error())
	return e
}

func (e *Error) Strs(values []string) *Error {
	joined := strings.Join(values, ", ")
	e.rspAnnotations = append(e.rspAnnotations, joined)
	return e
}

func (e *Error) Log(f func(e *zerolog.Event)) *Error {
	f(e.event)
	return e
}
