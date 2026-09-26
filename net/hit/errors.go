package hit

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/omniaura/go-kit/errs"
)

// Default factories for a failed upstream call. They describe the failure from
// the point of view of the service making the call, which is what its own
// clients need: an upstream 401 means OUR credential is wrong, and must never
// reach our caller as a 401 that signs them out. The upstream's real status,
// body and request id are kept server-side on errs.Upstream.
var (
	ErrUpstream = errs.NewFactory(http.StatusBadGateway, "upstream request failed",
		errs.WithCode("upstream_error"))
	ErrUpstreamUnavailable = errs.NewFactory(http.StatusServiceUnavailable, "upstream service unavailable",
		errs.WithCode("upstream_unavailable"), errs.WithAction(errs.ActionRetry))
	ErrUpstreamRateLimited = errs.NewFactory(http.StatusServiceUnavailable, "upstream service is busy",
		errs.WithCode("upstream_rate_limited"), errs.WithAction(errs.ActionWait))
)

// maxUpstreamBody bounds how much of an error body is kept.
const maxUpstreamBody = 2048

// defaultKeptHeaders are response headers worth keeping on every upstream
// error: provider request ids for support tickets, and rate-limit state.
var defaultKeptHeaders = []string{
	"Retry-After", "X-Request-Id", "Request-Id", "Cf-Ray",
	"X-Ratelimit-Remaining-Requests", "X-Ratelimit-Reset-Requests",
}

// Response is a non-2xx upstream response, as seen by a Classify function.
type Response struct {
	Header http.Header
	Body   []byte
	Status int
}

// Classified is a Classify verdict: the factory to raise, and the provider's
// own error code/type for the server-side record.
type Classified struct {
	Factory errs.ErrorFactory
	Code    string
}

// ErrorMap turns an upstream's failures into typed errors. Declare one per
// provider next to its request helpers; with it, every call through hit returns
// an *errs.Error that already knows its retry policy, what the caller can do
// and what the provider said — the error half of a hand-rolled SDK.
//
//	var anthropicErrors = hit.ErrorMap{
//		Service: "anthropic",
//		Status:  map[int]errs.ErrorFactory{529: ErrOverloaded},
//		Classify: hit.DecodeErrorBody(func(status int, b anthropicError) (hit.Classified, bool) {
//			if b.Error.Type == "invalid_request_error" {
//				return hit.Classified{Factory: ErrBadPrompt, Code: b.Error.Type}, true
//			}
//			return hit.Classified{Code: b.Error.Type}, false
//		}),
//		KeepHeaders: []string{"Request-Id"},
//	}
//
// Resolution order for a non-2xx response: Classify, then Status, then
// ClientError/ServerError, then the package defaults (429 → ErrUpstreamRateLimited,
// 408/5xx → ErrUpstreamUnavailable, anything else → ErrUpstream).
type ErrorMap struct {
	Status map[int]errs.ErrorFactory
	// Classify inspects the response (typically decoding the provider's error
	// body) and picks a factory. Returning false with a Code keeps the code on
	// the record but falls through to the status mapping.
	Classify func(Response) (Classified, bool)
	Service  string
	// KeepHeaders adds response headers to keep on the upstream record.
	KeepHeaders []string
	ClientError errs.ErrorFactory
	ServerError errs.ErrorFactory
	// Transport is raised when no response arrived (dial, TLS, timeout).
	Transport errs.ErrorFactory
}

// DecodeErrorBody adapts a typed error-body decoder to ErrorMap.Classify. The
// body is decoded as E; bodies that are not JSON of that shape fall through.
func DecodeErrorBody[E any](fn func(status int, body E) (Classified, bool)) func(Response) (Classified, bool) {
	return func(rsp Response) (Classified, bool) {
		var body E
		if len(rsp.Body) == 0 || json.Unmarshal(rsp.Body, &body) != nil {
			return Classified{}, false
		}
		return fn(rsp.Status, body)
	}
}

func isSet(f errs.ErrorFactory) bool { return f.Status() != 0 }

func (m *ErrorMap) factoryFor(rsp Response) (errs.ErrorFactory, string) {
	var providerCode string
	if m != nil {
		if m.Classify != nil {
			c, ok := m.Classify(rsp)
			providerCode = c.Code
			if ok && isSet(c.Factory) {
				return c.Factory, providerCode
			}
		}
		if f, ok := m.Status[rsp.Status]; ok {
			return f, providerCode
		}
		if rsp.Status >= 500 && isSet(m.ServerError) {
			return m.ServerError, providerCode
		}
		if rsp.Status < 500 && isSet(m.ClientError) {
			return m.ClientError, providerCode
		}
	}
	switch {
	case rsp.Status == http.StatusTooManyRequests:
		return ErrUpstreamRateLimited, providerCode
	case rsp.Status == http.StatusRequestTimeout || rsp.Status >= 500:
		return ErrUpstreamUnavailable, providerCode
	default:
		return ErrUpstream, providerCode
	}
}

func (m *ErrorMap) transport() errs.ErrorFactory {
	if m != nil && isSet(m.Transport) {
		return m.Transport
	}
	return ErrUpstreamUnavailable
}

func (m *ErrorMap) keptHeaders(h http.Header) map[string]string {
	names := defaultKeptHeaders
	if m != nil && len(m.KeepHeaders) > 0 {
		names = append(append([]string(nil), defaultKeptHeaders...), m.KeepHeaders...)
	}
	var out map[string]string
	for _, name := range names {
		if v := h.Get(name); v != "" {
			if out == nil {
				out = make(map[string]string, 2)
			}
			out[http.CanonicalHeaderKey(name)] = v
		}
	}
	return out
}

// redactURL keeps scheme, host and path: query strings and userinfo carry keys.
func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	clean := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}
	return clean.String()
}

func truncate(body []byte) string {
	if len(body) <= maxUpstreamBody {
		return string(body)
	}
	return string(body[:maxUpstreamBody]) + "…"
}

// parseRetryAfter reads a Retry-After header: delta-seconds or an HTTP date.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}
