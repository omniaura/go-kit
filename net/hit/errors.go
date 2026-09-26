package hit

import (
	"math"
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

// Classified is a Classify verdict: the factory to raise, and the provider's
// own error code/type for the server-side record.
type Classified struct {
	Factory errs.ErrorFactory
	Code    string
}

// ErrorMap turns an upstream's failures into typed errors by status. Declare
// one per provider, usually on a Client; the typed Classify (on the Client or
// Request) decides first from the decoded error body, then:
//
//	Status[code] → ClientError / ServerError → package defaults
//	(429 → ErrUpstreamRateLimited, 408/5xx → ErrUpstreamUnavailable,
//	anything else → ErrUpstream)
type ErrorMap struct {
	Status  map[int]errs.ErrorFactory
	Service string
	// KeepHeaders adds response headers to keep on the upstream record.
	KeepHeaders []string
	ClientError errs.ErrorFactory
	ServerError errs.ErrorFactory
	// Transport is raised when no response arrived (dial, TLS, timeout).
	Transport errs.ErrorFactory
}

func isSet(f errs.ErrorFactory) bool { return f.Status() != 0 }

func (m *ErrorMap) factoryFor(status int) errs.ErrorFactory {
	if m != nil {
		if f, ok := m.Status[status]; ok {
			return f
		}
		if status >= 500 && isSet(m.ServerError) {
			return m.ServerError
		}
		if status < 500 && isSet(m.ClientError) {
			return m.ClientError
		}
	}
	switch {
	case status == http.StatusTooManyRequests:
		return ErrUpstreamRateLimited
	case status == http.StatusRequestTimeout || status >= 500:
		return ErrUpstreamUnavailable
	default:
		return ErrUpstream
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
		// Clamp before multiplying: a huge value would overflow int64 and
		// wrap to a negative or arbitrary delay.
		if int64(secs) > int64(math.MaxInt64/int64(time.Second)) {
			return time.Duration(math.MaxInt64)
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
