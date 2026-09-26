package hit

import (
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/omniaura/go-kit/errs"
)

// Client is the configuration an SDK shares across every endpoint of one
// provider: the base URL, the *http.Client, default headers, the error map,
// retry tables, rate limiting, concurrency gates and caching — plus E, the
// provider's error schema. SDKs usually have one standardized error shape for
// every endpoint and a different response shape per endpoint, so E is fixed
// on the Client and Out is inferred per call from Do(ctx, &out) (a Go 1.27
// generic method) — no call site needs a type argument:
//
//	type apiError struct {
//		Error struct {
//			Type    string `json:"type"`
//			Message string `json:"message"`
//		} `json:"error"`
//	}
//
//	var anthropic = hit.NewClient[apiError]("https://api.anthropic.com").
//		Service("anthropic").
//		Header("anthropic-version", "2023-06-01").
//		Status(529, ErrOverloaded).
//		Classify(func(status int, b *apiError, _ http.Header) (hit.Classified, bool) {
//			if b.Error.Type == "invalid_request_error" {
//				return hit.Classified{Factory: ErrBadRequest, Code: b.Error.Type}, true
//			}
//			return hit.Classified{Code: b.Error.Type}, false
//		}).
//		StatusRetryPolicy(errs.TransientStatusRetry(errs.ExponentialRetry(3, 250*time.Millisecond, 4*time.Second)))
//
//	func CreateMessage(ctx context.Context, key string, req MessageRequest) (Message, error) {
//		var out Message
//		err := anthropic.POST("/v1/messages").Header("x-api-key", key).JSON(&req).Do(ctx, &out)
//		return out, err
//	}
//
// A Client is safe for concurrent use once configured: each call copies its
// settings into a fresh Request, which per-call builders then adjust.
type Client[E any] struct {
	statusRetries errs.StatusRetryPolicy
	headers       http.Header
	httpClient    *http.Client
	cache         Cache
	limiter       RateLimiter
	gate          Gate
	classify      func(status int, body *E, header http.Header) (Classified, bool)
	errorMap      ErrorMap
	baseURL       string
	retryPolicy   errs.RetryPolicy
	timeout       time.Duration
	retrySet      bool
}

// NewClient starts a Client for the API at baseURL whose error bodies decode
// as E. Use AnyError when the provider documents no error shape.
func NewClient[E any](baseURL string) *Client[E] {
	return &Client[E]{
		baseURL: strings.TrimRight(baseURL, "/"),
		headers: make(http.Header),
	}
}

// Service names the provider on error records and logs.
func (c *Client[E]) Service(name string) *Client[E] {
	c.errorMap.Service = name
	return c
}

// Header sets a header sent on every request.
func (c *Client[E]) Header(key, value string) *Client[E] {
	c.headers.Set(key, value)
	return c
}

// HTTPClient sets the *http.Client (transport, TLS, proxies) for every request.
func (c *Client[E]) HTTPClient(h *http.Client) *Client[E] {
	c.httpClient = h
	return c
}

// Timeout bounds every request.
func (c *Client[E]) Timeout(d time.Duration) *Client[E] {
	c.timeout = d
	return c
}

// Errors merges m into the Client's error map: Status entries are added (m
// wins on conflicts), KeepHeaders are appended, and non-zero Service,
// ClientError, ServerError and Transport replace what was set before — so
// Service/Status/Errors can be called in any order.
func (c *Client[E]) Errors(m ErrorMap) *Client[E] {
	for status, f := range m.Status {
		c.Status(status, f)
	}
	c.errorMap.KeepHeaders = append(c.errorMap.KeepHeaders, m.KeepHeaders...)
	if m.Service != "" {
		c.errorMap.Service = m.Service
	}
	if isSet(m.ClientError) {
		c.errorMap.ClientError = m.ClientError
	}
	if isSet(m.ServerError) {
		c.errorMap.ServerError = m.ServerError
	}
	if isSet(m.Transport) {
		c.errorMap.Transport = m.Transport
	}
	return c
}

// Status maps one upstream status to a factory.
func (c *Client[E]) Status(status int, factory errs.ErrorFactory) *Client[E] {
	if c.errorMap.Status == nil {
		c.errorMap.Status = make(map[int]errs.ErrorFactory)
	}
	c.errorMap.Status[status] = factory
	return c
}

// Classify picks a factory from the decoded error body; see Request.Classify.
func (c *Client[E]) Classify(fn func(status int, body *E, header http.Header) (Classified, bool)) *Client[E] {
	c.classify = fn
	return c
}

// Retry sets the fallback retry policy for every request.
func (c *Client[E]) Retry(policy errs.RetryPolicy) *Client[E] {
	c.retryPolicy = policy
	c.retrySet = true
	return c
}

// StatusRetryPolicy sets per-status retry policies for every request.
func (c *Client[E]) StatusRetryPolicy(policy errs.StatusRetryPolicy) *Client[E] {
	c.statusRetries = maps.Clone(policy)
	return c
}

// Rate applies a shared request-rate limiter to every request.
func (c *Client[E]) Rate(limiter RateLimiter) *Client[E] {
	c.limiter = limiter
	return c
}

// Gate applies a shared concurrency gate to every request.
func (c *Client[E]) Gate(gate Gate) *Client[E] {
	c.gate = gate
	return c
}

// Cache sets the cache used by cacheable requests (GET by default).
func (c *Client[E]) Cache(cache Cache) *Client[E] {
	c.cache = cache
	return c
}

// GET starts a GET of path. The response type is inferred at Do.
func (c *Client[E]) GET(path string) *Request[E] {
	return c.request(http.MethodGet, path)
}

// POST starts a POST of path. The response type is inferred at Do.
func (c *Client[E]) POST(path string) *Request[E] {
	return c.request(http.MethodPost, path)
}

// PUT starts a PUT of path. The response type is inferred at Do.
func (c *Client[E]) PUT(path string) *Request[E] {
	return c.request(http.MethodPut, path)
}

// PATCH starts a PATCH of path. The response type is inferred at Do.
func (c *Client[E]) PATCH(path string) *Request[E] {
	return c.request(http.MethodPatch, path)
}

// DELETE starts a DELETE of path. The response type is inferred at Do.
func (c *Client[E]) DELETE(path string) *Request[E] {
	return c.request(http.MethodDelete, path)
}

// request copies the Client's shared settings into a fresh Request so
// per-call builders never mutate the Client.
func (c *Client[E]) request(method, path string) *Request[E] {
	r := newRequest[E](method, "", defaultCacheable(method))
	r.baseURL = c.baseURL
	r.path = path
	r.headers = c.headers.Clone()
	r.client = c.httpClient
	r.timeout = c.timeout
	r.cache = c.cache
	r.limiter = c.limiter
	r.gate = c.gate
	r.classify = c.classify
	r.retryPolicy = c.retryPolicy
	r.retrySet = c.retrySet
	r.statusRetries = maps.Clone(c.statusRetries)
	em := c.errorMap
	em.Status = maps.Clone(c.errorMap.Status)
	em.KeepHeaders = append([]string(nil), c.errorMap.KeepHeaders...)
	r.errorMap = &em
	return r
}
