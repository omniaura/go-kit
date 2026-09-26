package errs

import (
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
		// custom Encoder. Fall back to the default rather than send nothing.
		body, _ = JSON.Encode(pub)
		w.Header().Set("Content-Type", JSON.ContentType())
	} else {
		w.Header().Set("Content-Type", cfg.encoder.ContentType())
	}
	if e.retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.FormatInt(int64((e.retryAfter+time.Second-1)/time.Second), 10))
	}
	w.WriteHeader(int(e.Status))
	if _, err := w.Write(body); err != nil {
		event := zerolog.Ctx(e.context()).Error()
		if ClientGone(err) {
			event = zerolog.Ctx(e.context()).Warn()
		}
		event.Err(err).Str("error_code", e.code).Msg("failed to write error response")
	}
	e.emit("http", "request aborted: "+e.Message())
	return true
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
