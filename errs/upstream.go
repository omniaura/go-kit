package errs

import (
	"time"

	"github.com/rs/zerolog"
)

// Upstream is what a failed call to another service told us: kept server-side
// so the team can see exactly what the provider said, never sent to clients.
type Upstream struct {
	// Header holds selected response headers, e.g. the provider's request id.
	Header  map[string]string
	Service string
	Method  string
	URL     string // scheme, host and path only; callers strip query and userinfo
	Body    string // truncated
	Code    string // the provider's own error code/type, when decoded
	// Decoded is the error body decoded into the caller's error schema (the ErrRsp
	// of a net/hit request). Read it with Error.UpstreamAs. Kept out of JSON:
	// Body already carries the same bytes, truncated.
	Decoded  any `json:"-"`
	Status   int
	Duration time.Duration
}

// Upstream records what the upstream said. Server-only.
func (e *Error) Upstream(u Upstream) *Error {
	e.upstream = &u
	return e
}

// UpstreamAs returns the upstream's error body decoded as ErrRsp — the error schema
// of the net/hit request that failed — so Go code can branch on a provider's
// typed error without re-parsing Body.
//
//	if body, ok := e.UpstreamAs[anthropicError](); ok && body.Error.Type == "overloaded_error" { … }
func (e *Error) UpstreamAs[ErrRsp any]() (ErrRsp, bool) {
	var zero ErrRsp
	if e == nil || e.upstream == nil {
		return zero, false
	}
	v, ok := e.upstream.Decoded.(ErrRsp)
	return v, ok
}

// UpstreamInfo returns the recorded upstream response, if any.
func (e *Error) UpstreamInfo() (Upstream, bool) {
	if e.upstream == nil {
		return Upstream{}, false
	}
	return *e.upstream, true
}

func (u *Upstream) dict() *zerolog.Event {
	d := zerolog.Dict().
		Str("service", u.Service).
		Str("method", u.Method).
		Str("url", u.URL).
		Int("status", u.Status)
	if u.Code != "" {
		d = d.Str("code", u.Code)
	}
	if u.Body != "" {
		d = d.Str("body", u.Body)
	}
	if u.Duration > 0 {
		d = d.Dur("duration", u.Duration)
	}
	if len(u.Header) > 0 {
		d = d.Interface("header", u.Header)
	}
	return d
}
