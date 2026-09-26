// Package hit provides an ergonomic HTTP client wrapper around net/http.
package hit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/omniaura/go-kit/errs"
	"golang.org/x/sync/singleflight"
)

var defaultFlights singleflight.Group

// Request is an HTTP request builder. Every call has two schemas:
//
//   - E, the JSON body of an error response, fixed when the request is made
//     (by its Client, or explicitly: hit.GET[apiError](url)). It is decoded on
//     every non-2xx response, handed to Classify, copied into ErrorInto's
//     target, and kept on the returned *errs.Error (UpstreamAs[E]).
//   - Out, the JSON body of a 2xx response, inferred at the end of the chain
//     from the pointer handed to Do: .Do(ctx, &out).
//
// Go infers type arguments only from the arguments of the generic call
// itself, never backwards through a method chain, so Out lives on the
// terminal call (a Go 1.27 generic method) rather than on POST. The request
// body is typed the same way: .JSON(&req) infers In from the pointer.
//
// Use AnyError when an endpoint has no documented error shape.
type Request[E any] struct {
	method        string
	url           string
	baseURL       string
	path          string
	queries       url.Values
	headers       http.Header
	body          []byte
	timeout       time.Duration
	client        *http.Client
	fallback      []byte
	cache         Cache
	cacheKey      string
	cacheable     bool
	cacheableSet  bool
	cacheSWR      bool
	keyParts      []KeyPart
	buildErr      error
	coalesce      bool
	coalesceSet   bool
	group         *singleflight.Group
	limiter       RateLimiter
	gate          Gate
	retryPolicy   errs.RetryPolicy
	retrySet      bool
	statusRetries errs.StatusRetryPolicy
	errorMap      *ErrorMap
	classify      func(status int, body *E, header http.Header) (Classified, bool)
	errorInto     *E
	service       string
}

// AnyError is the error schema for endpoints without a documented error
// shape: any JSON body decodes into it, and the raw bytes stay on the error.
type AnyError = json.RawMessage

func newRequest[E any](method, rawURL string, cacheable bool) *Request[E] {
	return &Request[E]{
		method:    method,
		url:       rawURL,
		queries:   make(url.Values),
		headers:   make(http.Header),
		cacheable: cacheable,
	}
}

func defaultCacheable(method string) bool {
	return strings.EqualFold(method, http.MethodGet)
}

// GET creates a GET request builder.
func GET[E any](rawURL string) *Request[E] {
	return newRequest[E](http.MethodGet, rawURL, true)
}

// POST creates a POST request builder.
func POST[E any](rawURL string) *Request[E] {
	return newRequest[E](http.MethodPost, rawURL, false)
}

// PUT creates a PUT request builder.
func PUT[E any](rawURL string) *Request[E] {
	return newRequest[E](http.MethodPut, rawURL, false)
}

// PATCH creates a PATCH request builder.
func PATCH[E any](rawURL string) *Request[E] {
	return newRequest[E](http.MethodPatch, rawURL, false)
}

// DELETE creates a DELETE request builder.
func DELETE[E any](rawURL string) *Request[E] {
	return newRequest[E](http.MethodDelete, rawURL, false)
}

func (r *Request[E]) setBuildErr(err error) {
	if err != nil && r.buildErr == nil {
		r.buildErr = err
	}
}

// Method sets the HTTP method.
func (r *Request[E]) Method(method string) *Request[E] {
	r.method = method
	if !r.cacheableSet {
		r.cacheable = defaultCacheable(method)
	}
	if !r.coalesceSet {
		r.coalesce = r.cache != nil && r.cacheable
	}
	return r
}

// WithMethod sets the HTTP method.
func (r *Request[E]) WithMethod(method string) *Request[E] {
	return r.Method(method)
}

// URL sets the request URL, overriding base URL and path.
func (r *Request[E]) URL(rawURL string) *Request[E] {
	r.url = rawURL
	r.baseURL = ""
	r.path = ""
	return r
}

// WithURL sets the request URL, overriding base URL and path.
func (r *Request[E]) WithURL(rawURL string) *Request[E] {
	return r.URL(rawURL)
}

// BaseURL sets the base URL for Path composition.
func (r *Request[E]) BaseURL(base string) *Request[E] {
	r.baseURL = base
	r.url = ""
	return r
}

// WithBaseURL sets the base URL for Path composition.
func (r *Request[E]) WithBaseURL(base string) *Request[E] {
	return r.BaseURL(base)
}

// Path sets the request path for BaseURL composition.
func (r *Request[E]) Path(path string) *Request[E] {
	if r.url != "" && r.baseURL == "" {
		r.baseURL = r.url
	}
	r.path = path
	r.url = ""
	return r
}

// WithPath sets the request path for BaseURL composition.
func (r *Request[E]) WithPath(path string) *Request[E] {
	return r.Path(path)
}

// Query adds a query parameter.
func (r *Request[E]) Query(key, value string) *Request[E] {
	r.queries.Add(key, value)
	return r
}

// WithQuery adds a query parameter.
func (r *Request[E]) WithQuery(key, value string) *Request[E] {
	return r.Query(key, value)
}

// Queries adds multiple query parameters.
func (r *Request[E]) Queries(queries map[string]string) *Request[E] {
	for k, v := range queries {
		r.queries.Add(k, v)
	}
	return r
}

// WithQueries adds multiple query parameters.
func (r *Request[E]) WithQueries(queries map[string]string) *Request[E] {
	return r.Queries(queries)
}

// Header adds a request header.
func (r *Request[E]) Header(key, value string) *Request[E] {
	r.headers.Add(key, value)
	return r
}

// WithHeader adds a request header.
func (r *Request[E]) WithHeader(key, value string) *Request[E] {
	return r.Header(key, value)
}

// Headers adds request headers from key/value pairs.
func (r *Request[E]) Headers(pairs ...string) *Request[E] {
	if len(pairs)%2 != 0 {
		r.setBuildErr(fmt.Errorf("headers requires key/value pairs"))
		return r
	}
	for i := 0; i < len(pairs); i += 2 {
		r.headers.Add(pairs[i], pairs[i+1])
	}
	return r
}

// WithHeaders adds multiple request headers.
func (r *Request[E]) WithHeaders(headers map[string]string) *Request[E] {
	for k, v := range headers {
		r.headers.Add(k, v)
	}
	return r
}

// Body sets the raw request body.
func (r *Request[E]) Body(body []byte) *Request[E] {
	r.body = cloneBytes(body)
	return r
}

// WithBody sets the raw request body.
func (r *Request[E]) WithBody(body []byte) *Request[E] {
	return r.Body(body)
}

// BodyString sets the request body from a string.
func (r *Request[E]) BodyString(body string) *Request[E] {
	r.body = []byte(body)
	return r
}

// WithBodyString sets the request body from a string.
func (r *Request[E]) WithBodyString(body string) *Request[E] {
	return r.BodyString(body)
}

// BodyReader reads the provided reader and sets it as the request body.
func (r *Request[E]) BodyReader(reader io.Reader) *Request[E] {
	if reader == nil {
		r.body = nil
		return r
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.body = body
	return r
}

// WithBodyReader reads the provided reader and sets it as the request body.
func (r *Request[E]) WithBodyReader(reader io.Reader) *Request[E] {
	return r.BodyReader(reader)
}

// JSON marshals v as JSON and sets Content-Type to application/json.
//
// JSON takes a pointer so In is inferred from it (.JSON(&req)) and the body
// is never copied on the way in. That copy is real even on Go 1.27: a struct
// passed by value into json.Marshal's `any` parameter is boxed as a heap copy
// of the whole struct (a ~2.3 KB body measured 2945 B/op, 3 allocs by value
// vs 640 B/op, 1 alloc by pointer). A nil pointer clears the body.
func (r *Request[E]) JSON[In any](in *In) *Request[E] {
	if in == nil {
		r.body = nil
		return r
	}
	body, err := json.Marshal(in)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.body = body
	r.headers.Set("Content-Type", "application/json")
	return r
}

// WithJSON marshals v as JSON and sets Content-Type to application/json.
func (r *Request[E]) WithJSON[In any](in *In) *Request[E] {
	return r.JSON(in)
}

// BodyFS reads the named file from fsys and sets it as the request body.
func (r *Request[E]) BodyFS(fsys fs.FS, name string) *Request[E] {
	body, err := fs.ReadFile(fsys, name)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.body = body
	return r
}

// WithBodyFS reads the named file from fsys and sets it as the request body.
func (r *Request[E]) WithBodyFS(fsys fs.FS, name string) *Request[E] {
	return r.BodyFS(fsys, name)
}

// Timeout sets the request timeout.
func (r *Request[E]) Timeout(d time.Duration) *Request[E] {
	r.timeout = d
	return r
}

// WithTimeout sets the request timeout.
func (r *Request[E]) WithTimeout(d time.Duration) *Request[E] {
	return r.Timeout(d)
}

// Client overrides the default HTTP client.
func (r *Request[E]) Client(c *http.Client) *Request[E] {
	r.client = c
	return r
}

// WithClient overrides the default HTTP client.
func (r *Request[E]) WithClient(c *http.Client) *Request[E] {
	return r.Client(c)
}

// Fallback sets a static fallback response.
func (r *Request[E]) Fallback(v any) *Request[E] {
	if v == nil {
		r.fallback = nil
		return r
	}
	body, err := json.Marshal(v)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.fallback = body
	return r
}

// WithFallback sets a static fallback response.
func (r *Request[E]) WithFallback(v any) *Request[E] {
	return r.Fallback(v)
}

// FallbackFile reads the fallback response from a local file.
func (r *Request[E]) FallbackFile(path string) *Request[E] {
	body, err := os.ReadFile(path)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.fallback = body
	return r
}

// WithFallbackFile reads the fallback response from a local file.
func (r *Request[E]) WithFallbackFile(path string) *Request[E] {
	return r.FallbackFile(path)
}

// FallbackFS reads the fallback response from the provided filesystem.
func (r *Request[E]) FallbackFS(fsys fs.FS, name string) *Request[E] {
	body, err := fs.ReadFile(fsys, name)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.fallback = body
	return r
}

// WithFallbackFS reads the fallback response from the provided filesystem.
func (r *Request[E]) WithFallbackFS(fsys fs.FS, name string) *Request[E] {
	return r.FallbackFS(fsys, name)
}

// Cache sets the cache backend.
func (r *Request[E]) Cache(cache Cache) *Request[E] {
	r.cache = cache
	if !r.coalesceSet {
		r.coalesce = cache != nil && r.cacheable
	}
	return r
}

// WithCache sets the cache backend.
func (r *Request[E]) WithCache(cache Cache) *Request[E] {
	return r.Cache(cache)
}

// CacheKey overrides the derived cache key.
func (r *Request[E]) CacheKey(key string) *Request[E] {
	r.cacheKey = key
	return r
}

// WithCacheKey overrides the derived cache key.
func (r *Request[E]) WithCacheKey(key string) *Request[E] {
	return r.CacheKey(key)
}

// CacheSWR enables stale-while-revalidate reads when the cache supports it.
func (r *Request[E]) CacheSWR() *Request[E] {
	r.cacheSWR = true
	return r
}

// WithCacheSWR enables stale-while-revalidate reads when the cache supports it.
func (r *Request[E]) WithCacheSWR() *Request[E] {
	return r.CacheSWR()
}

// Key appends cache-key dimensions such as Header, BodyHash, or Field.
func (r *Request[E]) Key(parts ...KeyPart) *Request[E] {
	r.keyParts = append(r.keyParts, parts...)
	return r
}

// Cacheable toggles whether the request may use cache.
func (r *Request[E]) Cacheable(v bool) *Request[E] {
	r.cacheable = v
	r.cacheableSet = true
	if !r.coalesceSet {
		r.coalesce = v && r.cache != nil
	}
	return r
}

// Retry configures a fallback retry policy for request errors.
func (r *Request[E]) Retry(policy errs.RetryPolicy) *Request[E] {
	r.retryPolicy = policy
	r.retrySet = true
	return r
}

// WithRetryPolicy configures a fallback retry policy for request errors.
func (r *Request[E]) WithRetryPolicy(policy errs.RetryPolicy) *Request[E] {
	return r.Retry(policy)
}

// StatusRetry configures retry metadata for a specific HTTP status code.
func (r *Request[E]) StatusRetry(status int, policy errs.RetryPolicy) *Request[E] {
	if r.statusRetries == nil {
		r.statusRetries = make(errs.StatusRetryPolicy)
	}
	r.statusRetries[status] = policy
	return r
}

// WithStatusRetry configures retry metadata for a specific HTTP status code.
func (r *Request[E]) WithStatusRetry(status int, policy errs.RetryPolicy) *Request[E] {
	return r.StatusRetry(status, policy)
}

// StatusRetryPolicy configures status-code retry metadata.
func (r *Request[E]) StatusRetryPolicy(policy errs.StatusRetryPolicy) *Request[E] {
	if len(policy) == 0 {
		r.statusRetries = nil
		return r
	}
	r.statusRetries = make(errs.StatusRetryPolicy, len(policy))
	for status, retry := range policy {
		r.statusRetries[status] = retry
	}
	return r
}

// ErrorInto decodes an error response's body into dst as well, for callers
// that want the provider's typed error next to the returned *errs.Error:
//
//	var apiErr apiError
//	err := anthropic.POST("/v1/messages").JSON(&req).ErrorInto(&apiErr).Do(ctx, &out)
//
// dst is left untouched when the call succeeds or the body is not an E.
func (r *Request[E]) ErrorInto(dst *E) *Request[E] {
	r.errorInto = dst
	return r
}

// Classify picks the factory for an error response from its decoded body.
// It runs before the ErrorMap's status mapping; returning false (optionally
// with a Code) falls through to it. Bodies that do not decode as E skip it.
func (r *Request[E]) Classify(fn func(status int, body *E, header http.Header) (Classified, bool)) *Request[E] {
	r.classify = fn
	return r
}

// Errors maps this request's failures to typed errors; see ErrorMap.
func (r *Request[E]) Errors(m ErrorMap) *Request[E] {
	r.errorMap = &m
	return r
}

// Service names the upstream on error records when no ErrorMap names it.
func (r *Request[E]) Service(name string) *Request[E] {
	r.service = name
	return r
}

// WithStatusRetryPolicy configures status-code retry metadata.
func (r *Request[E]) WithStatusRetryPolicy(policy errs.StatusRetryPolicy) *Request[E] {
	return r.StatusRetryPolicy(policy)
}

// Coalesce toggles duplicate concurrent request suppression.
// Cached requests coalesce by default; uncached requests do not.
func (r *Request[E]) Coalesce(v bool) *Request[E] {
	r.coalesce = v
	r.coalesceSet = true
	return r
}

// Group sets the singleflight group used for request coalescing.
func (r *Request[E]) Group(group *singleflight.Group) *Request[E] {
	r.group = group
	return r
}

// Rate sets a blocking request rate limiter.
func (r *Request[E]) Rate(limiter RateLimiter) *Request[E] {
	r.limiter = limiter
	return r
}

// Gate sets a concurrency gate.
func (r *Request[E]) Gate(gate Gate) *Request[E] {
	r.gate = gate
	return r
}

func (r *Request[E]) buildURL() (string, error) {
	var raw string
	if r.url != "" {
		raw = r.url
	} else {
		raw = strings.TrimRight(r.baseURL, "/") + "/" + strings.TrimLeft(r.path, "/")
	}
	if strings.TrimSpace(raw) == "" || raw == "/" {
		return "", fmt.Errorf("no URL configured")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if len(r.queries) > 0 {
		q := u.Query()
		for key, values := range r.queries {
			for _, v := range values {
				q.Add(key, v)
			}
		}
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func (r *Request[E]) newHTTPRequest(ctx context.Context) (*http.Request, context.CancelFunc, error) {
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		req, err := r.requestWithContext(ctx)
		if err != nil {
			cancel()
			return nil, nil, err
		}
		return req, cancel, nil
	}
	req, err := r.requestWithContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	return req, func() {}, nil
}

func (r *Request[E]) requestWithContext(ctx context.Context) (*http.Request, error) {
	u, err := r.buildURL()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, r.method, u, bytes.NewReader(r.body))
	if err != nil {
		return nil, err
	}
	for key, values := range r.headers {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	return req, nil
}

func (r *Request[E]) cacheKeyValue(ctx context.Context, req *http.Request) (string, error) {
	if r.cacheKey != "" {
		return r.cacheKey, nil
	}
	var b strings.Builder
	b.WriteString(baseCacheKey(r.method, req.URL))
	appendHeaderKeyPart(&b, req.Header, "Authorization")
	if len(r.body) > 0 && !strings.EqualFold(r.method, http.MethodGet) {
		appendHashedKeyPart(&b, "body", "", r.body)
	}
	for _, part := range r.keyParts {
		if part == nil {
			return "", keyPartError(part)
		}
		if err := part.appendCacheKey(ctx, req, r.body, &b); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

func (r *Request[E]) httpClient() *http.Client {
	if r.client != nil {
		return r.client
	}
	return http.DefaultClient
}

// Do executes the request and decodes a 2xx JSON body into out. Out is
// inferred from the pointer: .Do(ctx, &out).
func (r *Request[E]) Do[Out any](ctx context.Context, out *Out) error {
	if out == nil {
		return errs.AsError(ctx, fmt.Errorf("output pointer is nil"))
	}
	if r.buildErr != nil {
		return errs.AsError(ctx, r.buildErr)
	}
	req, cancel, err := r.newHTTPRequest(ctx)
	if err != nil {
		return r.fail(ctx, out, errs.AsError(ctx, err))
	}
	defer cancel()

	// Bodies are checked against Out before they are cached or shared, so a
	// malformed response never poisons the cache for later callers.
	validate := func(body []byte) error { return validateAs[Out](body) }

	key := ""
	if (r.cacheable && r.cache != nil) || r.coalesce {
		key, err = r.cacheKeyValue(ctx, req)
		if err != nil {
			return r.fail(ctx, out, errs.AsError(ctx, err))
		}
	}
	if r.cacheable && r.cache != nil {
		if r.cacheSWR {
			if cache, ok := r.cache.(SWRCache); ok {
				body, ok, err := cache.GetSWR(ctx, key, func(fetchCtx context.Context) ([]byte, error) {
					return r.loadBytes(fetchCtx, key, validate)
				})
				if err != nil {
					return r.fail(ctx, out, errs.AsError(ctx, err))
				}
				if ok {
					return r.decode(ctx, out, body)
				}
			}
		}
		body, ok, err := r.cache.Get(ctx, key)
		if err != nil {
			return r.fail(ctx, out, errs.AsError(ctx, err))
		}
		if ok {
			return r.decode(ctx, out, body)
		}
	}

	body, err := r.loadBytes(ctx, key, validate)
	if err != nil {
		return r.fail(ctx, out, err)
	}
	return r.decode(ctx, out, body)
}

func (r *Request[E]) loadBytes(ctx context.Context, key string, validate func([]byte) error) ([]byte, error) {
	if !r.coalesce {
		return r.fetchAndCache(ctx, key, validate)
	}
	group := r.group
	if group == nil {
		group = &defaultFlights
	}
	ch := group.DoChan(key, func() (any, error) {
		if r.cache != nil {
			body, ok, err := r.cache.Get(ctx, key)
			if err != nil {
				return nil, err
			}
			if ok {
				return body, nil
			}
		}
		return r.fetchAndCache(ctx, key, validate)
	})
	select {
	case result := <-ch:
		if result.Err != nil {
			return nil, result.Err
		}
		body, ok := result.Val.([]byte)
		if !ok {
			return nil, errs.AsError(ctx, fmt.Errorf("singleflight value type mismatch"))
		}
		return cloneBytes(body), nil
	case <-ctx.Done():
		return nil, errs.AsError(ctx, ctx.Err())
	}
}

func (r *Request[E]) fetchAndCache(ctx context.Context, key string, validate func([]byte) error) ([]byte, error) {
	body, err := r.executeBytesWithRetry(ctx)
	if err != nil {
		return nil, err
	}
	if err := validate(body); err != nil {
		return nil, r.error(ctx, err)
	}
	if r.cacheable && r.cache != nil && key != "" {
		if err := r.cache.Set(ctx, key, body); err != nil {
			return nil, r.error(ctx, err)
		}
	}
	return body, nil
}

func (r *Request[E]) wait(ctx context.Context) (func(), error) {
	if r.limiter != nil {
		if err := r.limiter.Wait(ctx); err != nil {
			return nil, err
		}
	}
	if r.gate == nil {
		return func() {}, nil
	}
	if err := r.gate.Wait(ctx); err != nil {
		return nil, err
	}
	return r.gate.Release, nil
}

func (r *Request[E]) executeBytesWithRetry(ctx context.Context) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		release, err := r.wait(ctx)
		if err != nil {
			return nil, r.error(ctx, err)
		}
		req, cancel, err := r.newHTTPRequest(ctx)
		if err != nil {
			release()
			return nil, r.error(ctx, err)
		}
		body, err := r.executeBytes(req)
		cancel()
		release()
		if err == nil {
			return body, nil
		}
		policy, ok := errs.RetryPolicyOf(err)
		if !ok {
			return nil, err
		}
		delay, retry := policy.Next(attempt, err)
		if !retry {
			return nil, err
		}
		if err := errs.Sleep(ctx, delay); err != nil {
			return nil, r.error(ctx, err)
		}
	}
}

func (r *Request[E]) executeBytes(req *http.Request) ([]byte, error) {
	started := time.Now()
	rsp, err := r.httpClient().Do(req)
	if err != nil {
		return nil, r.transportError(req, err, time.Since(started))
	}
	defer rsp.Body.Close()

	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		return nil, r.transportError(req, err, time.Since(started))
	}
	if rsp.StatusCode < http.StatusOK || rsp.StatusCode >= http.StatusMultipleChoices {
		return nil, r.statusError(req, rsp, body, time.Since(started))
	}
	return body, nil
}

func (r *Request[E]) serviceName(req *http.Request) string {
	if r.errorMap != nil && r.errorMap.Service != "" {
		return r.errorMap.Service
	}
	if r.service != "" {
		return r.service
	}
	return req.URL.Host
}

// transportError is a call that got no usable response: dial, TLS, timeout or
// a body cut short. The request's own retry policy applies.
func (r *Request[E]) transportError(req *http.Request, err error, took time.Duration) *errs.Error {
	ctx := req.Context()
	if errors.Is(err, context.Canceled) {
		// The caller gave up; that is not an upstream failure. A deadline is
		// different: the upstream was too slow, which is its failure.
		return r.error(ctx, err)
	}
	e := r.errorMap.transport().New(ctx).
		AddError(err).
		Upstream(errs.Upstream{
			Service:  r.serviceName(req),
			Method:   req.Method,
			URL:      redactURL(req.URL),
			Duration: took,
		})
	if r.retrySet {
		e.WithRetryPolicy(r.retryPolicy)
	}
	return e
}

func (r *Request[E]) error(ctx context.Context, err error) *errs.Error {
	e := errs.AsError(ctx, err)
	if r.retrySet {
		e.WithRetryPolicy(r.retryPolicy)
	}
	return e
}

// statusError maps a non-2xx response through the ErrorMap. The client-facing
// part comes from the chosen factory; the upstream's status, body, headers and
// own error code are kept server-side.
func (r *Request[E]) statusError(req *http.Request, rsp *http.Response, body []byte, took time.Duration) *errs.Error {
	ctx := req.Context()
	var (
		decoded      E
		hasDecoded   bool
		factory      errs.ErrorFactory
		providerCode string
		classified   bool
	)
	if len(body) > 0 && json.Unmarshal(body, &decoded) == nil {
		hasDecoded = true
		if r.classify != nil {
			var c Classified
			c, classified = r.classify(rsp.StatusCode, &decoded, rsp.Header)
			providerCode = c.Code
			classified = classified && isSet(c.Factory)
			factory = c.Factory
		}
	}
	if !classified {
		factory = r.errorMap.factoryFor(rsp.StatusCode)
	}
	upstream := errs.Upstream{
		Service:  r.serviceName(req),
		Method:   req.Method,
		URL:      redactURL(req.URL),
		Status:   rsp.StatusCode,
		Body:     truncate(body),
		Code:     providerCode,
		Header:   r.errorMap.keptHeaders(rsp.Header),
		Duration: took,
	}
	if hasDecoded {
		upstream.Decoded = decoded
		if r.errorInto != nil {
			*r.errorInto = decoded
		}
	}
	e := factory.New(ctx).Upstream(upstream)
	if after := parseRetryAfter(rsp.Header.Get("Retry-After"), time.Now()); after > 0 {
		e.RetryAfter(after)
	}
	if policy, ok := r.retryPolicyForStatus(rsp.StatusCode, factory); ok {
		e.WithRetryPolicy(policy)
	}
	return e
}

// retryPolicyForStatus: a per-status policy on the request wins, then the
// factory's own declared policy, then the request's fallback policy.
func (r *Request[E]) retryPolicyForStatus(status int, factory errs.ErrorFactory) (errs.RetryPolicy, bool) {
	if policy, ok := r.statusRetries.ForStatus(status); ok {
		return policy, true
	}
	if policy, ok := factory.RetryPolicy(); ok {
		return policy, true
	}
	if r.retrySet {
		return r.retryPolicy, true
	}
	return errs.RetryPolicy{}, false
}

func validateAs[Out any](body []byte) error {
	if len(body) == 0 {
		return nil
	}
	var out Out
	return json.Unmarshal(body, &out)
}

func (r *Request[E]) decode[Out any](ctx context.Context, out *Out, body []byte) error {
	if len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return r.fail(ctx, out, errs.AsError(ctx, err))
	}
	return nil
}

func (r *Request[E]) fail[Out any](ctx context.Context, out *Out, err error) error {
	if len(r.fallback) == 0 {
		return err
	}
	if decodeErr := json.Unmarshal(r.fallback, out); decodeErr != nil {
		return errs.AsError(ctx, decodeErr)
	}
	return nil
}

// TestServer returns the URL of an httptest.Server and panics on nil.
func TestServer(s *httptest.Server) string {
	if s == nil {
		panic("hit: nil test server")
	}
	return s.URL
}
