// Package hit provides an ergonomic, builder-pattern HTTP client wrapper
// around Go's standard net/http library. It supports JSON request/response
// bodies, fallback responses, embedded filesystem bodies, and pluggable
// caching with mapcache as the default backend.
package hit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"time"

	"github.com/omniaura/go-kit/errs"
	"github.com/omniaura/go-kit/mapcache"
)

// Cache is the interface implemented by caching backends.
type Cache interface {
	// Get returns the cached value for key, calling up to populate the cache
	// on a miss or expiration.
	Get(ctx context.Context, key string, up func() (any, error)) (any, error)
}

// SWRCache is implemented by caches that support stale-while-revalidate reads.
type SWRCache interface {
	GetSWR(ctx context.Context, key string, up func(context.Context) (any, error)) (any, error)
}

// MapCache is a Cache implementation backed by mapcache.MapCache.
type MapCache struct {
	mc *mapcache.MapCache[string, any]
}

// NewMapCache creates a new in-memory cache backed by mapcache.
func NewMapCache(opts ...mapcache.OptFunc) (*MapCache, error) {
	mc, err := mapcache.New[string, any](opts...)
	if err != nil {
		return nil, err
	}
	return &MapCache{mc: mc}, nil
}

// Get implements Cache.
func (c *MapCache) Get(ctx context.Context, key string, up func() (any, error)) (any, error) {
	return c.mc.Get(key, up)
}

// GetSWR implements SWRCache.
func (c *MapCache) GetSWR(
	ctx context.Context,
	key string,
	up func(context.Context) (any, error),
) (any, error) {
	return c.mc.GetSWR(ctx, key, up)
}

// Request is an HTTP request builder. All fields are private; configuration
// happens through the builder methods. The type parameter Out is the expected
// response type.
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
	cacheSWR      bool
	retryPolicy   errs.RetryPolicy
	retrySet      bool
	statusRetries errs.StatusRetryPolicy
}

// GET creates a new GET request builder for the given output type.
func GET[Out any](rawURL string) *Request[Out] {
	return &Request[Out]{
		method:    http.MethodGet,
		url:       rawURL,
		queries:   make(url.Values),
		headers:   make(http.Header),
		cacheable: true,
	}
}

// POST creates a new POST request builder for the given output type.
func POST[Out any](rawURL string) *Request[Out] {
	return &Request[Out]{
		method:    http.MethodPost,
		url:       rawURL,
		queries:   make(url.Values),
		headers:   make(http.Header),
		cacheable: false,
	}
}

// PUT creates a new PUT request builder for the given output type.
func PUT[Out any](rawURL string) *Request[Out] {
	return &Request[Out]{
		method:    http.MethodPut,
		url:       rawURL,
		queries:   make(url.Values),
		headers:   make(http.Header),
		cacheable: false,
	}
}

// PATCH creates a new PATCH request builder for the given output type.
func PATCH[Out any](rawURL string) *Request[Out] {
	return &Request[Out]{
		method:    http.MethodPatch,
		url:       rawURL,
		queries:   make(url.Values),
		headers:   make(http.Header),
		cacheable: false,
	}
}

// DELETE creates a new DELETE request builder for the given output type.
func DELETE[Out any](rawURL string) *Request[Out] {
	return &Request[Out]{
		method:    http.MethodDelete,
		url:       rawURL,
		queries:   make(url.Values),
		headers:   make(http.Header),
		cacheable: false,
	}
}

// WithMethod sets the HTTP method.
func (r *Request[Out]) WithMethod(method string) *Request[Out] {
	r.method = method
	return r
}

// WithURL sets the request URL, overriding any base URL and path.
func (r *Request[Out]) WithURL(rawURL string) *Request[Out] {
	r.url = rawURL
	r.baseURL = ""
	r.path = ""
	return r
}

// WithBaseURL sets the base URL. The final URL is built from baseURL + path
// when no explicit URL is set.
func (r *Request[Out]) WithBaseURL(base string) *Request[Out] {
	r.baseURL = base
	r.url = ""
	return r
}

// WithPath sets the request path. Use with WithBaseURL to compose URLs.
func (r *Request[Out]) WithPath(path string) *Request[Out] {
	r.path = path
	r.url = ""
	return r
}

// WithQuery adds a query parameter.
func (r *Request[Out]) WithQuery(key, value string) *Request[Out] {
	r.queries.Add(key, value)
	return r
}

// WithQueries adds multiple query parameters.
func (r *Request[Out]) WithQueries(queries map[string]string) *Request[Out] {
	for k, v := range queries {
		r.queries.Add(k, v)
	}
	return r
}

// WithHeader adds a request header.
func (r *Request[Out]) WithHeader(key, value string) *Request[Out] {
	r.headers.Add(key, value)
	return r
}

// WithHeaders adds multiple request headers.
func (r *Request[Out]) WithHeaders(headers map[string]string) *Request[Out] {
	for k, v := range headers {
		r.headers.Add(k, v)
	}
	return r
}

// WithBody sets the raw request body.
func (r *Request[Out]) WithBody(body []byte) *Request[Out] {
	r.body = body
	return r
}

// WithBodyString sets the request body from a string.
func (r *Request[Out]) WithBodyString(body string) *Request[Out] {
	r.body = []byte(body)
	return r
}

// WithBodyReader reads the provided reader and sets it as the request body.
func (r *Request[Out]) WithBodyReader(reader io.Reader) *Request[Out] {
	if reader == nil {
		r.body = nil
		return r
	}
	body, _ := io.ReadAll(reader)
	r.body = body
	return r
}

// WithJSON marshals v as JSON and sets it as the request body, also setting
// the Content-Type header to application/json.
func (r *Request[Out]) WithJSON(v any) *Request[Out] {
	if v == nil {
		r.body = nil
		return r
	}
	body, err := json.Marshal(v)
	if err != nil {
		// Store the error as a sentinel body so Do can report it.
		r.body = []byte("{}")
		return r
	}
	r.body = body
	r.headers.Set("Content-Type", "application/json")
	return r
}

// WithBodyFS reads the named file from fsys and sets it as the request body.
func (r *Request[Out]) WithBodyFS(fsys fs.FS, name string) *Request[Out] {
	body, err := fs.ReadFile(fsys, name)
	if err != nil {
		r.body = nil
		return r
	}
	r.body = body
	return r
}

// WithTimeout sets the request timeout.
func (r *Request[Out]) WithTimeout(d time.Duration) *Request[Out] {
	r.timeout = d
	return r
}

// WithClient overrides the default http.Client.
func (r *Request[Out]) WithClient(c *http.Client) *Request[Out] {
	r.client = c
	return r
}

// WithFallback sets a static fallback response. The value is marshaled to JSON
// and unmarshaled into the output on request failure.
func (r *Request[Out]) WithFallback(v any) *Request[Out] {
	if v == nil {
		r.fallback = nil
		return r
	}
	body, err := json.Marshal(v)
	if err != nil {
		r.fallback = nil
		return r
	}
	r.fallback = body
	return r
}

// WithFallbackFile reads the fallback response from a local file.
func (r *Request[Out]) WithFallbackFile(path string) *Request[Out] {
	body, err := os.ReadFile(path)
	if err != nil {
		r.fallback = nil
		return r
	}
	r.fallback = body
	return r
}

// WithFallbackFS reads the fallback response from the provided filesystem.
func (r *Request[Out]) WithFallbackFS(fsys fs.FS, name string) *Request[Out] {
	body, err := fs.ReadFile(fsys, name)
	if err != nil {
		r.fallback = nil
		return r
	}
	r.fallback = body
	return r
}

// WithCache sets the cache backend. When set, GET requests are cached by
// default. Use Cacheable to opt POST/PUT/PATCH/DELETE into caching.
func (r *Request[Out]) WithCache(cache Cache) *Request[Out] {
	r.cache = cache
	return r
}

// WithCacheKey overrides the derived cache key.
func (r *Request[Out]) WithCacheKey(key string) *Request[Out] {
	r.cacheKey = key
	return r
}

// WithCacheSWR enables stale-while-revalidate reads when the configured cache
// implements SWRCache.
func (r *Request[Out]) WithCacheSWR() *Request[Out] {
	r.cacheSWR = true
	return r
}

// Cacheable toggles whether the request may be cached. GET requests are
// cacheable by default; other methods are not.
func (r *Request[Out]) Cacheable(v bool) *Request[Out] {
	r.cacheable = v
	return r
}

// WithRetryPolicy configures a fallback retry policy for request errors.
func (r *Request[Out]) WithRetryPolicy(policy errs.RetryPolicy) *Request[Out] {
	r.retryPolicy = policy
	r.retrySet = true
	return r
}

// WithStatusRetry configures retry metadata for a specific HTTP status code.
func (r *Request[Out]) WithStatusRetry(status int, policy errs.RetryPolicy) *Request[Out] {
	if r.statusRetries == nil {
		r.statusRetries = make(errs.StatusRetryPolicy)
	}
	r.statusRetries[status] = policy
	return r
}

// WithStatusRetryPolicy configures status-code retry metadata.
func (r *Request[Out]) WithStatusRetryPolicy(policy errs.StatusRetryPolicy) *Request[Out] {
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

// buildURL returns the final URL string with query parameters applied.
func (r *Request[Out]) buildURL() (string, error) {
	var raw string
	if r.url != "" {
		raw = r.url
	} else {
		raw = r.baseURL + r.path
	}
	if raw == "" {
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

// cacheKey returns the cache key for this request.
func (r *Request[Out]) cacheKeyValue() string {
	if r.cacheKey != "" {
		return r.cacheKey
	}
	u, err := r.buildURL()
	if err != nil {
		u = ""
	}
	h := sha256.New()
	h.Write([]byte(r.method))
	h.Write([]byte("|"))
	h.Write([]byte(u))
	h.Write([]byte("|"))
	h.Write(r.body)
	return hex.EncodeToString(h.Sum(nil))
}

// httpClient returns the configured or default HTTP client.
func (r *Request[Out]) httpClient() *http.Client {
	if r.client != nil {
		return r.client
	}
	return http.DefaultClient
}

// Do executes the request and decodes the JSON response into out.
//
// If a fallback response is configured and the request fails or returns a
// non-2xx status, out is populated from the fallback and a nil error is
// returned unless fallback decoding also fails.
func (r *Request[Out]) Do(ctx context.Context, out *Out) error {
	if out == nil {
		return errs.AsError(ctx, fmt.Errorf("output pointer is nil"))
	}

	u, err := r.buildURL()
	if err != nil {
		return r.fail(ctx, out, errs.AsError(ctx, err))
	}

	if r.cache != nil && r.cacheable {
		key := r.cacheKeyValue()
		cached, err := r.cached(ctx, key, u)
		if err != nil {
			return err
		}
		if cached == nil {
			return nil
		}
		// The cached value is the decoded output value.
		if v, ok := cached.(Out); ok {
			*out = v
			return nil
		}
		return r.fail(ctx, out, errs.AsError(ctx, fmt.Errorf("cache type mismatch")))
	}

	_, err = r.fetch(ctx, u, out)
	return err
}

func (r *Request[Out]) cached(ctx context.Context, key, u string) (any, error) {
	fetch := func(fetchCtx context.Context) (any, error) {
		var value Out
		return r.fetch(fetchCtx, u, &value)
	}
	if r.cacheSWR {
		if cache, ok := r.cache.(SWRCache); ok {
			return cache.GetSWR(ctx, key, fetch)
		}
	}
	return r.cache.Get(ctx, key, func() (any, error) {
		return fetch(ctx)
	})
}

func (r *Request[Out]) fetch(ctx context.Context, u string, out *Out) (any, error) {
	if _, err := r.executeWithRetry(ctx, u, out); err != nil {
		if err := r.fail(ctx, out, err); err != nil {
			return nil, err
		}
	}
	return *out, nil
}

func (r *Request[Out]) executeWithRetry(ctx context.Context, u string, out *Out) (any, error) {
	for attempt := 1; ; attempt++ {
		value, err := r.execute(ctx, u, out)
		if err == nil {
			return value, nil
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

// execute performs one HTTP request and decodes the response. It returns the
// decoded value so it can be stored in caches that store any values.
func (r *Request[Out]) execute(ctx context.Context, u string, out *Out) (any, error) {
	req, err := http.NewRequestWithContext(ctx, r.method, u, bytes.NewReader(r.body))
	if err != nil {
		return nil, r.error(ctx, err)
	}
	for key, values := range r.headers {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	client := r.httpClient()
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}

	rsp, err := client.Do(req)
	if err != nil {
		return nil, r.error(ctx, err)
	}
	defer rsp.Body.Close()

	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		return nil, r.error(ctx, err)
	}

	if rsp.StatusCode < 200 || rsp.StatusCode >= 300 {
		return nil, r.statusError(ctx, rsp.StatusCode, body)
	}

	if len(body) == 0 {
		return *out, nil
	}

	if err := json.Unmarshal(body, out); err != nil {
		return nil, r.error(ctx, err)
	}
	return *out, nil
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

// fail attempts to populate out from the configured fallback. If no fallback
// is configured, it returns err unchanged.
func (r *Request[Out]) fail(ctx context.Context, out *Out, err error) error {
	if len(r.fallback) == 0 {
		return err
	}
	if decodeErr := json.Unmarshal(r.fallback, out); decodeErr != nil {
		return errs.AsError(ctx, decodeErr)
	}
	return nil
}

// TestServer is a convenience helper that returns the URL of an httptest.Server
// for use in tests. It panics if the server is nil.
func TestServer(s *httptest.Server) string {
	if s == nil {
		panic("hit: nil test server")
	}
	return s.URL
}
