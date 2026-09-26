package errs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

var (
	// Unknown is what AsError wraps a plain error in. Its message says nothing
	// about the cause, on purpose.
	Unknown = NewFactory(http.StatusInternalServerError, "unknown error")
)

// Error is one occurrence of an ErrorFactory, plus what was learned about it.
//
// Status is exported for callers that branch on it; every other field is set
// through the builder so the client/server split cannot be bypassed.
type Error struct {
	ctx      context.Context
	upstream *Upstream
	fields   map[string]string
	params   map[string]any
	message  errorMessage
	code     string
	action   Action
	ref      string
	logStack []Attr
	causes   []error
	logFns   []func(*zerolog.Event)
	retry    RetryPolicy
	// retryAfter is a server-provided "not before" hint for this occurrence,
	// e.g. an upstream's Retry-After header.
	retryAfter time.Duration
	source     string
	Status     uint16
	severity   Severity
	record     recordMode
	retrySet   bool
	emitted    bool
}

// AsError returns err as an *Error: itself if it (or its chain) already is
// one, otherwise Unknown carrying err as a server-only attribute. nil stays nil.
func AsError(ctx context.Context, err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		if e.ctx == nil {
			e.ctx = ctx
		}
		return e
	}
	return Unknown.New(ctx).AddError(err)
}

// Is reports whether err was made by the same factory as e.
func (e *Error) Is(err error) bool {
	var other *Error
	if errors.As(err, &other) {
		return e.Status == other.Status && e.message == other.message
	}
	return false
}

// Not is !Is.
func (e *Error) Not(err error) bool { return !e.Is(err) }

// Error implements error. It includes only client-safe parts, so an *Error
// that is later formatted into a response by mistake discloses nothing new.
func (e *Error) Error() string {
	return fmt.Sprintf("status code: %d, message: %s", e.Status, e.message.h.Value())
}

// ---- Client-facing builder -------------------------------------------------
//
// Everything set here is part of the response body. Nothing here accepts an
// error value.

// Action overrides the factory's Action for this occurrence.
func (e *Error) Action(action Action) *Error {
	e.action = action
	return e
}

// Field reports a problem with one input field, e.g. Field("email", "is
// required"). Fields are for form validation: name the field the client sent
// and describe the problem in words the user can act on.
func (e *Error) Field(name, problem string) *Error {
	if e.fields == nil {
		e.fields = make(map[string]string, 1)
	}
	e.fields[name] = problem
	if e.action == ActionNone {
		e.action = ActionFixInput
	}
	return e
}

// Param attaches a value the client may see, e.g. Param("limit", 100). Only
// pass values you would put in the message itself; never an error or a value
// derived from one.
func (e *Error) Param(key string, value any) *Error {
	if e.params == nil {
		e.params = make(map[string]any, 1)
	}
	e.params[key] = value
	return e
}

// RetryAfter tells the client (and any retry loop) not to try again before d.
// It sets the Retry-After header on Abort.
func (e *Error) RetryAfter(d time.Duration) *Error {
	if d > 0 {
		e.retryAfter = d
	}
	return e
}

// WithRetryPolicy overrides the factory's retry policy for this occurrence.
func (e *Error) WithRetryPolicy(policy RetryPolicy) *Error {
	e.retry = policy
	e.retrySet = true
	return e
}

// Severity overrides the severity for this occurrence.
func (e *Error) Severity(severity Severity) *Error {
	e.severity = severity
	return e
}

// ---- Server-only builder ---------------------------------------------------
//
// Everything set here goes to the log line and the Sinks, never the client.

// AddError records err as a cause: its text goes to the log and the Sinks, and
// errors.Is/As see it through Unwrap. It never reaches the response.
func (e *Error) AddError(err error) *Error {
	if err == nil {
		return e
	}
	e.causes = append(e.causes, err)
	return e.AddAttrs(String("error", err.Error()))
}

// AddErrors records several causes as error_0, error_1, ...
func (e *Error) AddErrors(errs ...error) *Error {
	for i, err := range errs {
		if err != nil {
			e.causes = append(e.causes, err)
			e.AddAttrs(String("error_"+strconv.Itoa(i), err.Error()))
		}
	}
	return e
}

// Unwrap exposes the causes to errors.Is and errors.As, so Go code can still
// branch on them (context.Canceled, a driver's sentinel). Error() stays safe.
func (e *Error) Unwrap() []error { return e.causes }

// AddAttrs records key/values.
func (e *Error) AddAttrs(attrs ...Attr) *Error {
	e.logStack = append(e.logStack, attrs...)
	return e
}

// AddMessage records a free-form note, e.g. why a policy rejected a request.
func (e *Error) AddMessage(msg string) *Error {
	return e.AddAttrs(String("detail", msg))
}

// Log adds fields to the log line directly, for zerolog types Attr does not
// cover. Fields added this way are logged but not recorded by Sinks.
func (e *Error) Log(fn func(*zerolog.Event)) *Error {
	e.logFns = append(e.logFns, fn)
	return e
}

// ---- Accessors -------------------------------------------------------------

// Message returns the client-facing message.
func (e *Error) Message() string { return e.message.h.Value() }

// Code returns the machine-readable code.
func (e *Error) Code() string { return e.code }

// Ref returns the reference this occurrence was (or will be) reported under.
func (e *Error) Ref() string {
	if e.ref == "" {
		e.ref = resolveRef(e.ctx)
	}
	return e.ref
}

// ActionHint returns the action the client should offer.
func (e *Error) ActionHint() Action { return e.action }

// SeverityLevel returns the effective severity.
func (e *Error) SeverityLevel() Severity {
	if e.severity == SeverityUnset {
		return SeverityForStatus(int(e.Status))
	}
	return e.severity
}

// RetryPolicy reports the attached policy and whether one was set at all.
// ok=false means "no opinion": the caller keeps its own default.
func (e *Error) RetryPolicy() (RetryPolicy, bool) {
	if e == nil || !e.retrySet {
		return RetryPolicy{}, false
	}
	return e.retry, true
}

// RetryAfterHint returns the RetryAfter delay, or 0.
func (e *Error) RetryAfterHint() time.Duration { return e.retryAfter }

// Retryable reports whether a client may retry: a retryable policy, a
// RetryAfter hint, or an ActionRetry/ActionWait.
func (e *Error) Retryable() bool {
	return (e.retrySet && e.retry.Retryable) || e.retryAfter > 0 ||
		e.action == ActionRetry || e.action == ActionWait
}

// Attrs returns the server-only attributes.
func (e *Error) Attrs() []Attr { return e.logStack }

// Detail renders the server-only attributes as one line, e.g. for a database
// column.
func (e *Error) Detail() string {
	var b strings.Builder
	for i, attr := range e.logStack {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(attr.Key)
		b.WriteByte('=')
		fmt.Fprint(&b, attr.Value)
	}
	return b.String()
}

func (e *Error) context() context.Context {
	if e.ctx == nil {
		return context.Background()
	}
	return e.ctx
}
