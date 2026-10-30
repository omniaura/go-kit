package errs

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog"
)

// Abort writes e to w and reports it. It returns false (and does nothing) when
// e is nil, so handlers read:
//
//	if validate(ctx, req).Abort(w) {
//		return
//	}
//
// The response body is the Public view rendered by the configured Encoder
// (JSON by default), with a Retry-After header when RetryAfter was set. The
// log line and the Sinks get everything, including the server-only parts.
func (e *Error) Abort(w http.ResponseWriter) bool {
	if e == nil {
		return false
	}
	pub := e.Public()
	cfg := current()
	body, encErr := cfg.encoder.Encode(pub)
	if encErr != nil {
		// The Public view is plain data; failing to encode it is a bug in a
		// custom Encoder (or an unencodable Param). Fall back to JSON, and if
		// even that fails, drop Params rather than send an empty body.
		body, encErr = JSON.Encode(pub)
		if encErr != nil {
			pub.Params = nil
			body, _ = JSON.Encode(pub)
		}
		w.Header().Set("Content-Type", JSON.ContentType())
	} else {
		w.Header().Set("Content-Type", cfg.encoder.ContentType())
	}
	if e.retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.FormatInt(int64((e.retryAfter+time.Second-1)/time.Second), 10))
	}
	w.WriteHeader(int(e.Status))
	_, err := w.Write(body)
	if e.clientCanceled() {
		// The request's own context was cancelled and that cancellation is
		// what failed: the client hung up (a navigation, an image scrolled
		// out of view). Behind a proxy or on HTTP/2 the write above usually
		// "succeeds" into a buffer anyway, so the write error cannot be what
		// decides this. Not a server fault: logged at info, never recorded.
		event := zerolog.Ctx(e.context()).Info().
			Int("status", int(e.Status)).Str("error_code", e.code)
		if err != nil {
			event = event.AnErr("write_error", err)
		}
		applyAttrs(event, e.logStack).Msg("client canceled the request")
		e.emitted = true
		return true
	}
	if err != nil {
		if ClientGone(err) {
			// The client left before hearing about it. The failed write is
			// only a warning. Whether the error itself is still a fault
			// depends on what caused it: a failure caused by the abandoned
			// request (context canceled) is not recorded, but an independent
			// fault — a database error whose 500 found no one listening — is
			// still logged and handed to the Sinks below.
			event := zerolog.Ctx(e.context()).Warn().Err(err).
				Int("status", int(e.Status)).Str("error_code", e.code)
			if e.causedByCancellation() {
				applyAttrs(event, e.logStack).Msg("client gone while writing error response")
				e.emitted = true
				return true
			}
			event.Msg("client gone while writing error response")
		} else {
			zerolog.Ctx(e.context()).Error().Err(err).Str("error_code", e.code).Msg("failed to write error response")
		}
	}
	e.emit("http", "request aborted: "+e.Message())
	return true
}

// clientCanceled reports whether the request was cancelled by its client and
// that cancellation is the whole story: every recorded cause is a
// cancellation, so an independent failure alongside it is still a fault.
//
// net/http cancels a request's context with a plain cancel when the client
// goes, so its Cause is exactly context.Canceled. A deadline, or a context an
// errgroup cancelled because a sibling failed (whose Cause is that failure),
// does not qualify.
func (e *Error) clientCanceled() bool {
	ctx := e.context()
	if !errors.Is(ctx.Err(), context.Canceled) || !errors.Is(context.Cause(ctx), context.Canceled) || len(e.causes) == 0 {
		return false
	}
	for _, c := range e.causes {
		if !errors.Is(c, context.Canceled) {
			return false
		}
	}
	return true
}

// causedByCancellation reports whether any recorded cause is the request's
// own cancellation. A deadline is deliberately not included: a server-side
// timeout is a fault.
func (e *Error) causedByCancellation() bool {
	for _, c := range e.causes {
		if errors.Is(c, context.Canceled) {
			return true
		}
	}
	return false
}

// Emit logs and records e without writing an HTTP response, and returns the
// Public view for the caller to send over its own transport (a websocket
// frame, an SSE event, a queue message). source names that transport in the
// record, e.g. "chat_stream".
func (e *Error) Emit(source string) Public {
	if e == nil {
		return Public{}
	}
	pub := e.Public()
	e.emit(source, "error emitted: "+e.Message())
	return pub
}

func (e *Error) emit(source, msg string) {
	if e.emitted {
		return
	}
	e.emitted = true
	if e.source == "" {
		e.source = source
	}
	severity := e.SeverityLevel()
	event := zerolog.Ctx(e.context()).WithLevel(severity.level()).
		Int("status", int(e.Status)).
		Str("error_code", e.code).
		Str("severity", severity.String())
	if ref := e.Ref(); ref != "" {
		event = event.Str("error_ref", ref)
	}
	if e.source != "" {
		event = event.Str("error_source", e.source)
	}
	if e.action != ActionNone {
		event = event.Str("action", string(e.action))
	}
	if e.retryAfter > 0 {
		event = event.Dur("retry_after", e.retryAfter)
	}
	if len(e.fields) > 0 {
		event = event.Interface("fields", e.fields)
	}
	if e.upstream != nil {
		event = event.Dict("upstream", e.upstream.dict())
	}
	event = applyAttrs(event, e.logStack)
	for _, fn := range e.logFns {
		fn(event)
	}
	event.Msg(msg)

	if e.shouldRecord(severity) {
		record(e.context(), e.recordFor(severity))
	}
}

func (e *Error) shouldRecord(severity Severity) bool {
	switch e.record {
	case recordAlways:
		return true
	case recordNever:
		return false
	default:
		return severity >= SeverityError
	}
}

// Source names where this error surfaced (e.g. "chat_stream") in the log and
// the record. Abort defaults it to "http".
func (e *Error) Source(source string) *Error {
	e.source = source
	return e
}

// Record forces (true) or suppresses (false) recording this occurrence.
func (e *Error) Record(record bool) *Error {
	if record {
		e.record = recordAlways
	} else {
		e.record = recordNever
	}
	return e
}
