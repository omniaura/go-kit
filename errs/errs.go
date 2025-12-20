package errs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unique"

	"github.com/rs/zerolog"
)

var (
	Unknown = NewFactory(http.StatusInternalServerError, "unknown error")
)

type Error struct {
	rspAnnotations []string
	ctx            context.Context
	event          *zerolog.Event
	message        errorMessage
	Status         uint16
	b              bytes.Buffer
}

type errorMessage struct {
	h unique.Handle[string]
}

// Is checks if the given error matches this error's factory (same status and message).
func (e *Error) Is(err error) bool {
	var other *Error
	if errors.As(err, &other) {
		return e.Status == other.Status && e.message == other.message
	}
	return false
}

// Not checks if the given error does not match this error's factory.
func (e *Error) Not(err error) bool {
	return !e.Is(err)
}

func AsError(ctx context.Context, err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		if e.ctx == nil {
			e.ctx = ctx
		}
		return e
	}
	return Unknown.New(ctx).Err(err)
}

func (e *Error) Message() string {
	var b strings.Builder
	msg := e.message.h.Value()
	msglen := len(msg)
	for i := range e.rspAnnotations {
		msglen += len(e.rspAnnotations[i]) + 2
	}
	b.Grow(msglen)
	b.WriteString(msg)
	for i := range e.rspAnnotations {
		b.WriteString(": ")
		b.WriteString(e.rspAnnotations[i])
	}
	return b.String()
}

func (e *Error) Error() string {
	return fmt.Sprintf("status code: %d, message: %s", e.Status, e.Message())
}

// Abort handles error responses in HTTP handlers.
//
// If e is nil:
//   - Does nothing
//   - Returns false
//
// If e is not nil:
//   - Writes the error to the response
//   - Logs the error using context's zerolog logger
//   - Returns true
//
// Common usage pattern:
//
//	if errSomething(ctx).AddError(err).Abort(w) {
//	    return
//	}
func (e *Error) Abort(w http.ResponseWriter) bool {
	if e == nil {
		return false
	}
	w.WriteHeader(int(e.Status))
	var buf bytes.Buffer
	e.marshalJSONBuffer(&buf)
	buf.WriteTo(w)
	msg := "request aborted: " + e.message.h.Value()
	e.event.Int("status", int(e.Status)).Msg(msg)
	return true
}
