// Package hit provides an ergonomic HTTP client wrapper around net/http.
package hit

import (
	"bytes"
	"context"
	"encoding/json"
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

// Request is an HTTP request builder. Out is the expected JSON response type.
type Request[Out any] struct {
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
}

func newRequest[Out any](method, rawURL string, cacheable bool) *Request[Out] {
	return &Request[Out]{
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
func GET[Out any](rawURL string) *Request[Out] {
	return newRequest[Out](http.MethodGet, rawURL, true)
}

// POST creates a POST request builder.
func POST[Out any](rawURL string) *Request[Out] {
	return newRequest[Out](http.MethodPost, rawURL, false)
}

// PUT creates a PUT request builder.
func PUT[Out any](rawURL string) *Request[Out] {
	return newRequest[Out](http.MethodPut, rawURL, false)
}

// PATCH creates a PATCH request builder.
func PATCH[Out any](rawURL string) *Request[Out] {
	return newRequest[Out](http.MethodPatch, rawURL, false)
}

// DELETE creates a DELETE request builder.
func DELETE[Out any](rawURL string) *Request[Out] {
	return newRequest[Out](http.MethodDelete, rawURL, false)
}

func (r *Request[Out]) setBuildErr(err error) {
	if err != nil && r.buildErr == nil {
		r.buildErr = err
	}
}

// Method sets the HTTP method.
func (r *Request[Out]) Method(method string) *Request[Out] {
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
func (r *Request[Out]) WithMethod(method string) *Request[Out] {
	return r.Method(method)
}

// URL sets the request URL, overriding base URL and path.
func (r *Request[Out]) URL(rawURL string) *Request[Out] {
	r.url = rawURL
	r.baseURL = ""
	r.path = ""
	return r
}

// WithURL sets the request URL, overriding base URL and path.
func (r *Request[Out]) WithURL(rawURL string) *Request[Out] {
	return r.URL(rawURL)
}

// BaseURL sets the base URL for Path composition.
func (r *Request[Out]) BaseURL(base string) *Request[Out] {
	r.baseURL = base
	r.url = ""
	return r
}

// WithBaseURL sets the base URL for Path composition.
func (r *Request[Out]) WithBaseURL(base string) *Request[Out] {
	return r.BaseURL(base)
}

// Path sets the request path for BaseURL composition.
func (r *Request[Out]) Path(path string) *Request[Out] {
	if r.url != "" && r.baseURL == "" {
		r.baseURL = r.url
	}
	r.path = path
	r.url = ""
	return r
}

// WithPath sets the request path for BaseURL composition.
func (r *Request[Out]) WithPath(path string) *Request[Out] {
	return r.Path(path)
}

// Query adds a query parameter.
func (r *Request[Out]) Query(key, value string) *Request[Out] {
	r.queries.Add(key, value)
	return r
}

// WithQuery adds a query parameter.
func (r *Request[Out]) WithQuery(key, value string) *Request[Out] {
	return r.Query(key, value)
}

// Queries adds multiple query parameters.
func (r *Request[Out]) Queries(queries map[string]string) *Request[Out] {
	for k, v := range queries {
		r.queries.Add(k, v)
	}
	return r
}

// WithQueries adds multiple query parameters.
func (r *Request[Out]) WithQueries(queries map[string]string) *Request[Out] {
	return r.Queries(queries)
}

// Header adds a request header.
func (r *Request[Out]) Header(key, value string) *Request[Out] {
	r.headers.Add(key, value)
	return r
}

// WithHeader adds a request header.
func (r *Request[Out]) WithHeader(key, value string) *Request[Out] {
	return r.Header(key, value)
}

// Headers adds request headers from key/value pairs.
func (r *Request[Out]) Headers(pairs ...string) *Request[Out] {
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
func (r *Request[Out]) WithHeaders(headers map[string]string) *Request[Out] {
	for k, v := range headers {
		r.headers.Add(k, v)
	}
	return r
}

// Body sets the raw request body.
func (r *Request[Out]) Body(body []byte) *Request[Out] {
	r.body = cloneBytes(body)
	return r
}

// WithBody sets the raw request body.
func (r *Request[Out]) WithBody(body []byte) *Request[Out] {
	return r.Body(body)
}

// BodyString sets the request body from a string.
func (r *Request[Out]) BodyString(body string) *Request[Out] {
	r.body = []byte(body)
	return r
}

// WithBodyString sets the request body from a string.
func (r *Request[Out]) WithBodyString(body string) *Request[Out] {
	return r.BodyString(body)
}

// BodyReader reads the provided reader and sets it as the request body.
func (r *Request[Out]) BodyReader(reader io.Reader) *Request[Out] {
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
func (r *Request[Out]) WithBodyReader(reader io.Reader) *Request[Out] {
	return r.BodyReader(reader)
}

// JSON marshals v as JSON and sets Content-Type to application/json.
func (r *Request[Out]) JSON(v any) *Request[Out] {
	if v == nil {
		r.body = nil
		return r
	}
	body, err := json.Marshal(v)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.body = body
	r.headers.Set("Content-Type", "application/json")
	return r
}

// WithJSON marshals v as JSON and sets Content-Type to application/json.
func (r *Request[Out]) WithJSON(v any) *Request[Out] {
	return r.JSON(v)
}

// BodyFS reads the named file from fsys and sets it as the request body.
func (r *Request[Out]) BodyFS(fsys fs.FS, name string) *Request[Out] {
	body, err := fs.ReadFile(fsys, name)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.body = body
	return r
}

// WithBodyFS reads the named file from fsys and sets it as the request body.
func (r *Request[Out]) WithBodyFS(fsys fs.FS, name string) *Request[Out] {
	return r.BodyFS(fsys, name)
}

// Timeout sets the request timeout.
func (r *Request[Out]) Timeout(d time.Duration) *Request[Out] {
	r.timeout = d
	return r
}

// WithTimeout sets the request timeout.
func (r *Request[Out]) WithTimeout(d time.Duration) *Request[Out] {
	return r.Timeout(d)
}

// Client overrides the default HTTP client.
func (r *Request[Out]) Client(c *http.Client) *Request[Out] {
	r.client = c
	return r
}

// WithClient overrides the default HTTP client.
func (r *Request[Out]) WithClient(c *http.Client) *Request[Out] {
	return r.Client(c)
}

// Fallback sets a static fallback response.
func (r *Request[Out]) Fallback(v any) *Request[Out] {
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
func (r *Request[Out]) WithFallback(v any) *Request[Out] {
	return r.Fallback(v)
}

// FallbackFile reads the fallback response from a local file.
func (r *Request[Out]) FallbackFile(path string) *Request[Out] {
	body, err := os.ReadFile(path)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.fallback = body
	return r
}

// WithFallbackFile reads the fallback response from a local file.
func (r *Request[Out]) WithFallbackFile(path string) *Request[Out] {
	return r.FallbackFile(path)
}

// FallbackFS reads the fallback response from the provided filesystem.
func (r *Request[Out]) FallbackFS(fsys fs.FS, name string) *Request[Out] {
	body, err := fs.ReadFile(fsys, name)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.fallback = body
	return r
}

// WithFallbackFS reads the fallback response from the provided filesystem.
func (r *Request[Out]) WithFallbackFS(fsys fs.FS, name string) *Request[Out] {
	return r.FallbackFS(fsys, name)
}

// Cache sets the cache backend.
func (r *Request[Out]) Cache(cache Cache) *Request[Out] {
	r.cache = cache
	if !r.coalesceSet {
		r.coalesce = cache != nil && r.cacheable
	}
	return r
}

// WithCache sets the cache backend.
func (r *Request[Out]) WithCache(cache Cache) *Request[Out] {
	return r.Cache(cache)
}

// CacheKey overrides the derived cache key.
func (r *Request[Out]) CacheKey(key string) *Request[Out] {
	r.cacheKey = key
	return r
}

// WithCacheKey overrides the derived cache key.
func (r *Request[Out]) WithCacheKey(key string) *Request[Out] {
	return r.CacheKey(key)
}

// CacheSWR enables stale-while-revalidate reads when the cache supports it.
func (r *Request[Out]) CacheSWR() *Request[Out] {
	r.cacheSWR = true
	return r
}

// WithCacheSWR enables stale-while-revalidate reads when the cache supports it.
func (r *Request[Out]) WithCacheSWR() *Request[Out] {
	return r.CacheSWR()
}

// Key appends cache-key dimensions such as Header, BodyHash, or Field.
func (r *Request[Out]) Key(parts ...KeyPart) *Request[Out] {
	r.keyParts = append(r.keyParts, parts...)
	return r
}

// Cacheable toggles whether the request may use cache.
func (r *Request[Out]) Cacheable(v bool) *Request[Out] {
	r.cacheable = v
	r.cacheableSet = true
	if !r.coalesceSet {
		r.coalesce = v && r.cache != nil
	}
	return r
}

// Retry configures a fallback retry policy for request errors.
func (r *Request[Out]) Retry(policy errs.RetryPolicy) *Request[Out] {
	r.retryPolicy = policy
	r.retrySet = true
	return r
}

// WithRetryPolicy configures a fallback retry policy for request errors.
func (r *Request[Out]) WithRetryPolicy(policy errs.RetryPolicy) *Request[Out] {
	return r.Retry(policy)
}

// StatusRetry configures retry metadata for a specific HTTP status code.
func (r *Request[Out]) StatusRetry(status int, policy errs.RetryPolicy) *Request[Out] {
	if r.statusRetries == nil {
		r.statusRetries = make(errs.StatusRetryPolicy)
	}
	r.statusRetries[status] = policy
	return r
}

// WithStatusRetry configures retry metadata for a specific HTTP status code.
func (r *Request[Out]) WithStatusRetry(status int, policy errs.RetryPolicy) *Request[Out] {
	return r.StatusRetry(status, policy)
}

// StatusRetryPolicy configures status-code retry metadata.
func (r *Request[Out]) StatusRetryPolicy(policy errs.StatusRetryPolicy) *Request[Out] {
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

// WithStatusRetryPolicy configures status-code retry metadata.
func (r *Request[Out]) WithStatusRetryPolicy(policy errs.StatusRetryPolicy) *Request[Out] {
	return r.StatusRetryPolicy(policy)
}

// Coalesce toggles duplicate concurrent request suppression.
// Cached requests coalesce by default; uncached requests do not.
func (r *Request[Out]) Coalesce(v bool) *Request[Out] {
	r.coalesce = v
	r.coalesceSet = true
	return r
}

// Group sets the singleflight group used for request coalescing.
func (r *Request[Out]) Group(group *singleflight.Group) *Request[Out] {
	r.group = group
	return r
}

// Rate sets a blocking request rate limiter.
func (r *Request[Out]) Rate(limiter RateLimiter) *Request[Out] {
	r.limiter = limiter
	return r
}

// Gate sets a concurrency gate.
func (r *Request[Out]) Gate(gate Gate) *Request[Out] {
	r.gate = gate
	return r
}

func (r *Request[Out]) buildURL() (string, error) {
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

func (r *Request[Out]) newHTTPRequest(ctx context.Context) (*http.Request, context.CancelFunc, error) {
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

func (r *Request[Out]) requestWithContext(ctx context.Context) (*http.Request, error) {
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

func (r *Request[Out]) cacheKeyValue(ctx context.Context, req *http.Request) (string, error) {
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

func (r *Request[Out]) httpClient() *http.Client {
	if r.client != nil {
		return r.client
	}
	return http.DefaultClient
}

// Do executes the request and decodes the JSON response into out.
func (r *Request[Out]) Do(ctx context.Context, out *Out) error {
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
					return r.loadBytes(fetchCtx, key)
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

	body, err := r.loadBytes(ctx, key)
	if err != nil {
		return r.fail(ctx, out, err)
	}
	return r.decode(ctx, out, body)
}

func (r *Request[Out]) loadBytes(ctx context.Context, key string) ([]byte, error) {
	if !r.coalesce {
		return r.fetchAndCache(ctx, key)
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
		return r.fetchAndCache(ctx, key)
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

func (r *Request[Out]) fetchAndCache(ctx context.Context, key string) ([]byte, error) {
	body, err := r.executeBytesWithRetry(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.validate(ctx, body); err != nil {
		return nil, err
	}
	if r.cacheable && r.cache != nil && key != "" {
		if err := r.cache.Set(ctx, key, body); err != nil {
			return nil, r.error(ctx, err)
		}
	}
	return body, nil
}

func (r *Request[Out]) wait(ctx context.Context) (func(), error) {
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

func (r *Request[Out]) executeBytesWithRetry(ctx context.Context) ([]byte, error) {
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
		if !ok || !policy.ShouldRetry(attempt) {
			return nil, err
		}
		if err := policy.Wait(ctx, attempt); err != nil {
			return nil, r.error(ctx, err)
		}
	}
}

func (r *Request[Out]) executeBytes(req *http.Request) ([]byte, error) {
	rsp, err := r.httpClient().Do(req)
	if err != nil {
		return nil, r.error(req.Context(), err)
	}
	defer rsp.Body.Close()

	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		return nil, r.error(req.Context(), err)
	}
	if rsp.StatusCode < http.StatusOK || rsp.StatusCode >= http.StatusMultipleChoices {
		return nil, r.statusError(req.Context(), rsp.StatusCode, body)
	}
	return body, nil
}

func (r *Request[Out]) error(ctx context.Context, err error) *errs.Error {
	e := errs.AsError(ctx, err)
	if r.retrySet {
		e.WithRetryPolicy(r.retryPolicy)
	}
	return e
}

func (r *Request[Out]) statusError(ctx context.Context, status int, body []byte) *errs.Error {
	err := errs.NewFactory(status, "http request failed").
		New(ctx).
		Err(fmt.Errorf("status %d: %s", status, string(body)))
	if policy, ok := r.retryPolicyForStatus(status); ok {
		err.WithRetryPolicy(policy)
	}
	return err
}

func (r *Request[Out]) retryPolicyForStatus(status int) (errs.RetryPolicy, bool) {
	if policy, ok := r.statusRetries.ForStatus(status); ok {
		return policy, true
	}
	if r.retrySet {
		return r.retryPolicy, true
	}
	return errs.RetryPolicy{}, false
}

func (r *Request[Out]) validate(ctx context.Context, body []byte) error {
	if len(body) == 0 {
		return nil
	}
	var out Out
	if err := json.Unmarshal(body, &out); err != nil {
		return r.error(ctx, err)
	}
	return nil
}

func (r *Request[Out]) decode(ctx context.Context, out *Out, body []byte) error {
	if len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return r.fail(ctx, out, errs.AsError(ctx, err))
	}
	return nil
}

func (r *Request[Out]) fail(ctx context.Context, out *Out, err error) error {
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
