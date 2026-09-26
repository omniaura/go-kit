package errs

import (
	"context"
	"errors"
	"net/http"
	"unique"

	"github.com/rs/zerolog"
)

// ErrorFactory declares one kind of error: its status, the fixed message a
// client sees, and the answers to "when can you retry", "what can you do" and
// "how serious is it". Declare factories at package level; New makes an
// instance per occurrence.
type ErrorFactory struct {
	message  errorMessage
	code     string
	action   Action
	retry    RetryPolicy
	status   uint16
	severity Severity
	record   recordMode
	retrySet bool
}

type errorMessage struct {
	h unique.Handle[string]
}

type recordMode uint8

const (
	recordDefault recordMode = iota
	recordAlways
	recordNever
)

type options struct {
	code     string
	action   Action
	retry    RetryPolicy
	severity Severity
	record   recordMode
	retrySet bool
}

// Option declares optional behaviour for every error a factory makes.
type Option func(*options)

// NewFactory declares an error kind. msg is what clients see, verbatim — it is
// the only text that reaches the wire, so write it for the person reading it.
func NewFactory(statusCode int, msg string, opts ...Option) ErrorFactory {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	status := httpStatus(statusCode)
	if o.code == "" {
		o.code = DefaultCode(int(status))
	}
	return ErrorFactory{
		message:  errorMessage{h: unique.Make(msg)},
		code:     o.code,
		action:   o.action,
		status:   status,
		severity: o.severity,
		record:   o.record,
		retry:    o.retry,
		retrySet: o.retrySet,
	}
}

// WithCode gives a factory a stable machine-readable code. Clients branch on
// codes, never on message text. lower_snake_case; never change a shipped code.
func WithCode(code string) Option { return func(o *options) { o.code = code } }

// WithAction declares what the user can do about every error of this kind.
func WithAction(action Action) Option { return func(o *options) { o.action = action } }

// WithSeverity overrides the status-derived severity.
func WithSeverity(severity Severity) Option {
	return func(o *options) { o.severity = severity }
}

// WithLevel overrides the severity by log level. Kept for callers that think in
// zerolog levels; WithSeverity is the primary form.
func WithLevel(level zerolog.Level) Option {
	return func(o *options) { o.severity = severityForLevel(level) }
}

// WithRecord forces (true) or suppresses (false) handing errors of this kind to
// the Sinks. By default an error is recorded when its severity is Error or
// worse.
func WithRecord(record bool) Option {
	return func(o *options) {
		if record {
			o.record = recordAlways
		} else {
			o.record = recordNever
		}
	}
}

// WithRetryPolicy declares the retry behaviour of every error a factory makes.
func WithRetryPolicy(policy RetryPolicy) Option {
	return func(o *options) {
		o.retry = policy
		o.retrySet = true
	}
}

// New makes an instance of the error for one occurrence.
func (f ErrorFactory) New(ctx context.Context) *Error {
	return &Error{
		ctx:      ctx,
		Status:   f.status,
		message:  f.message,
		code:     f.code,
		action:   f.action,
		severity: f.severity,
		record:   f.record,
		retry:    f.retry,
		retrySet: f.retrySet,
	}
}

// Status returns the HTTP status of errors of this kind.
func (f ErrorFactory) Status() int { return int(f.status) }

// Code returns the machine-readable code of errors of this kind.
func (f ErrorFactory) Code() string { return f.code }

// Message returns the client-facing message of errors of this kind.
func (f ErrorFactory) Message() string { return f.message.h.Value() }

// RetryPolicy returns the declared policy and whether one was declared.
func (f ErrorFactory) RetryPolicy() (RetryPolicy, bool) { return f.retry, f.retrySet }

// Is reports whether err (or any error in its chain) was made by f.
func (f ErrorFactory) Is(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Status == f.status && e.message == f.message
	}
	return false
}

// Not is !Is.
func (f ErrorFactory) Not(err error) bool { return !f.Is(err) }

// httpStatus narrows a status code for storage. Anything outside the HTTP range
// is a programming error in a factory declaration; 500 is the honest answer.
func httpStatus(code int) uint16 {
	if code < 100 || code > 599 {
		return http.StatusInternalServerError
	}
	return uint16(code)
}

// DefaultCode is the code a factory without WithCode reports: the status
// class, which is all a client can safely infer.
func DefaultCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusPaymentRequired:
		return "payment_required"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusRequestEntityTooLarge:
		return "too_large"
	case http.StatusUnprocessableEntity:
		return "unprocessable"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case 499:
		return "client_closed"
	case http.StatusBadGateway:
		return "upstream_error"
	case http.StatusServiceUnavailable:
		return "unavailable"
	case http.StatusGatewayTimeout:
		return "timeout"
	}
	switch {
	case status >= 500:
		return "internal_error"
	case status >= 400:
		return "request_failed"
	default:
		return "error"
	}
}
