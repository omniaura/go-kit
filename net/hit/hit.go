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
//   - ErrRsp, the JSON body of an error response, fixed when the request is made
//     (by its Client, or explicitly: hit.GET[apiError](url)). It is decoded on
//     every non-2xx response, handed to Classify, copied into ErrorInto's
//     target, and kept on the returned *errs.Error (UpstreamAs[ErrRsp]).
//   - Rsp, the JSON body of a 2xx response, inferred at the end of the chain
//     from the pointer handed to Do: .Do(ctx, &rsp).
//
// Go infers type arguments only from the arguments of the generic call
// itself, never backwards through a method chain, so Rsp lives on the
// terminal call (a Go 1.27 generic method) rather than on POST. The request
// body is typed the same way: .Body(&req) infers Req from the pointer, and
// its wire encoding comes from DataType (JSON unless the type says otherwise).
//
// Use AnyError when an endpoint has no documented error shape.
type Request[ErrRsp any] struct {
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
	classify      func(status int, body *ErrRsp, header http.Header) (Classified, bool)
	errorInto     *ErrRsp
	dataType      DataType
	responseType  DataType
	decodeAs      DataType
	service       string
}

// AnyError is the error schema for endpoints without a documented error
// shape: any JSON body decodes into it, and the raw bytes stay on the error.
type AnyError = json.RawMessage

func newRequest[ErrRsp any](method, rawURL string, cacheable bool) *Request[ErrRsp] {
	return &Request[ErrRsp]{
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
func GET[ErrRsp any](rawURL string) *Request[ErrRsp] {
	return newRequest[ErrRsp](http.MethodGet, rawURL, true)
}

// POST creates a POST request builder.
func POST[ErrRsp any](rawURL string) *Request[ErrRsp] {
	return newRequest[ErrRsp](http.MethodPost, rawURL, false)
}

// PUT creates a PUT request builder.
func PUT[ErrRsp any](rawURL string) *Request[ErrRsp] {
	return newRequest[ErrRsp](http.MethodPut, rawURL, false)
}

// PATCH creates a PATCH request builder.
func PATCH[ErrRsp any](rawURL string) *Request[ErrRsp] {
	return newRequest[ErrRsp](http.MethodPatch, rawURL, false)
}

// DELETE creates a DELETE request builder.
func DELETE[ErrRsp any](rawURL string) *Request[ErrRsp] {
	return newRequest[ErrRsp](http.MethodDelete, rawURL, false)
}

func (r *Request[ErrRsp]) setBuildErr(err error) {
	if err != nil && r.buildErr == nil {
		r.buildErr = err
	}
}

// Method sets the HTTP method.
func (r *Request[ErrRsp]) Method(method string) *Request[ErrRsp] {
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
func (r *Request[ErrRsp]) WithMethod(method string) *Request[ErrRsp] {
	return r.Method(method)
}

// URL sets the request URL, overriding base URL and path.
func (r *Request[ErrRsp]) URL(rawURL string) *Request[ErrRsp] {
	r.url = rawURL
	r.baseURL = ""
	r.path = ""
	return r
}

// WithURL sets the request URL, overriding base URL and path.
func (r *Request[ErrRsp]) WithURL(rawURL string) *Request[ErrRsp] {
	return r.URL(rawURL)
}

// BaseURL sets the base URL for Path composition.
func (r *Request[ErrRsp]) BaseURL(base string) *Request[ErrRsp] {
	r.baseURL = base
	r.url = ""
	return r
}

// WithBaseURL sets the base URL for Path composition.
func (r *Request[ErrRsp]) WithBaseURL(base string) *Request[ErrRsp] {
	return r.BaseURL(base)
}

// Path sets the request path for BaseURL composition.
func (r *Request[ErrRsp]) Path(path string) *Request[ErrRsp] {
	if r.url != "" && r.baseURL == "" {
		r.baseURL = r.url
	}
	r.path = path
	r.url = ""
	return r
}

// WithPath sets the request path for BaseURL composition.
func (r *Request[ErrRsp]) WithPath(path string) *Request[ErrRsp] {
	return r.Path(path)
}

// Query adds a query parameter.
func (r *Request[ErrRsp]) Query(key, value string) *Request[ErrRsp] {
	r.queries.Add(key, value)
	return r
}

// WithQuery adds a query parameter.
func (r *Request[ErrRsp]) WithQuery(key, value string) *Request[ErrRsp] {
	return r.Query(key, value)
}

// Queries adds multiple query parameters.
func (r *Request[ErrRsp]) Queries(queries map[string]string) *Request[ErrRsp] {
	for k, v := range queries {
		r.queries.Add(k, v)
	}
	return r
}

// WithQueries adds multiple query parameters.
func (r *Request[ErrRsp]) WithQueries(queries map[string]string) *Request[ErrRsp] {
	return r.Queries(queries)
}

// Header adds a request header.
func (r *Request[ErrRsp]) Header(key, value string) *Request[ErrRsp] {
	r.headers.Add(key, value)
	return r
}

// WithHeader adds a request header.
func (r *Request[ErrRsp]) WithHeader(key, value string) *Request[ErrRsp] {
	return r.Header(key, value)
}

// Headers adds request headers from key/value pairs.
func (r *Request[ErrRsp]) Headers(pairs ...string) *Request[ErrRsp] {
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
func (r *Request[ErrRsp]) WithHeaders(headers map[string]string) *Request[ErrRsp] {
	for k, v := range headers {
		r.headers.Add(k, v)
	}
	return r
}

// Body encodes req as the request body and sets Content-Type. The encoding is
// req's own DataType when *Req implements HasDataType, else the request's
// DataType (from its Client, or DataType), else JSON:
//
//	c.POST("/v1/messages").Body(&req).Do(ctx, &rsp)
//
// Body takes a pointer so Req is inferred from it and the value is never
// copied on the way in. That copy is real even on Go 1.27: a struct passed by
// value into an encoder's `any` parameter is boxed as a heap copy of the whole
// struct (a ~2.3 KB body measured 2945 B/op, 3 allocs by value vs 640 B/op,
// 1 alloc by pointer). A nil pointer clears the body.
func (r *Request[ErrRsp]) Body[Req any](req *Req) *Request[ErrRsp] {
	if req == nil {
		r.body = nil
		return r
	}
	return r.encodeBody(dataTypeOf(any(req), r.defaultDataType()), req)
}

// BodyAs encodes req with dt regardless of its type, for types you do not own:
//
//	c.POST("/oauth/token").BodyAs(hit.Form, &url.Values{"grant_type": {"client_credentials"}})
func (r *Request[ErrRsp]) BodyAs[Req any](dt DataType, req *Req) *Request[ErrRsp] {
	if req == nil {
		r.body = nil
		return r
	}
	return r.encodeBody(dt, req)
}

// WithBody is Body.
func (r *Request[ErrRsp]) WithBody[Req any](req *Req) *Request[ErrRsp] {
	return r.Body(req)
}

func (r *Request[ErrRsp]) encodeBody(dt DataType, v any) *Request[ErrRsp] {
	body, err := dt.Marshal(v)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.body = body
	r.headers.Set("Content-Type", dt.ContentType())
	return r
}

// RawBody sets the request body bytes as-is; set Content-Type yourself.
func (r *Request[ErrRsp]) RawBody(body []byte) *Request[ErrRsp] {
	r.body = cloneBytes(body)
	return r
}

// DataType sets the encoding used for the request body, the response and the
// error body whenever those types do not declare their own (HasDataType).
func (r *Request[ErrRsp]) DataType(dt DataType) *Request[ErrRsp] {
	r.dataType = dt
	return r
}

// ResponseAs decodes the 2xx response with dt regardless of Rsp's type, and
// asks for it with Accept.
func (r *Request[ErrRsp]) ResponseAs(dt DataType) *Request[ErrRsp] {
	r.responseType = dt
	return r
}

func (r *Request[ErrRsp]) responseCodec() DataType {
	if r.decodeAs != nil {
		return r.decodeAs
	}
	return r.defaultDataType()
}

func (r *Request[ErrRsp]) defaultDataType() DataType {
	if r.dataType != nil {
		return r.dataType
	}
	return JSON
}

// responseDataType resolves how a 2xx body decodes into rsp: ResponseAs, then
// rsp's own DataType, then the request default.
func (r *Request[ErrRsp]) responseDataType(rsp any) DataType {
	if r.responseType != nil {
		return r.responseType
	}
	return dataTypeOf(rsp, r.defaultDataType())
}

// BodyString sets the request body from a string.
func (r *Request[ErrRsp]) BodyString(body string) *Request[ErrRsp] {
	r.body = []byte(body)
	return r
}

// WithBodyString sets the request body from a string.
func (r *Request[ErrRsp]) WithBodyString(body string) *Request[ErrRsp] {
	return r.BodyString(body)
}

// BodyReader reads the provided reader and sets it as the request body.
func (r *Request[ErrRsp]) BodyReader(reader io.Reader) *Request[ErrRsp] {
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
func (r *Request[ErrRsp]) WithBodyReader(reader io.Reader) *Request[ErrRsp] {
	return r.BodyReader(reader)
}

// JSON is BodyAs(hit.JSON, req): JSON even when Req declares another
// DataType.
func (r *Request[ErrRsp]) JSON[Req any](req *Req) *Request[ErrRsp] {
	return r.BodyAs(JSON, req)
}

// WithJSON is JSON.
func (r *Request[ErrRsp]) WithJSON[Req any](req *Req) *Request[ErrRsp] {
	return r.JSON(req)
}

// BodyFS reads the named file from fsys and sets it as the request body.
func (r *Request[ErrRsp]) BodyFS(fsys fs.FS, name string) *Request[ErrRsp] {
	body, err := fs.ReadFile(fsys, name)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.body = body
	return r
}

// WithBodyFS reads the named file from fsys and sets it as the request body.
func (r *Request[ErrRsp]) WithBodyFS(fsys fs.FS, name string) *Request[ErrRsp] {
	return r.BodyFS(fsys, name)
}

// Timeout sets the request timeout.
func (r *Request[ErrRsp]) Timeout(d time.Duration) *Request[ErrRsp] {
	r.timeout = d
	return r
}

// WithTimeout sets the request timeout.
func (r *Request[ErrRsp]) WithTimeout(d time.Duration) *Request[ErrRsp] {
	return r.Timeout(d)
}

// Client overrides the default HTTP client.
func (r *Request[ErrRsp]) Client(c *http.Client) *Request[ErrRsp] {
	r.client = c
	return r
}

// WithClient overrides the default HTTP client.
func (r *Request[ErrRsp]) WithClient(c *http.Client) *Request[ErrRsp] {
	return r.Client(c)
}

// Fallback sets a static fallback response.
func (r *Request[ErrRsp]) Fallback(v any) *Request[ErrRsp] {
	if v == nil {
		r.fallback = nil
		return r
	}
	body, err := dataTypeOf(v, r.defaultDataType()).Marshal(v)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.fallback = body
	return r
}

// WithFallback sets a static fallback response.
func (r *Request[ErrRsp]) WithFallback(v any) *Request[ErrRsp] {
	return r.Fallback(v)
}

// FallbackFile reads the fallback response from a local file.
func (r *Request[ErrRsp]) FallbackFile(path string) *Request[ErrRsp] {
	body, err := os.ReadFile(path)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.fallback = body
	return r
}

// WithFallbackFile reads the fallback response from a local file.
func (r *Request[ErrRsp]) WithFallbackFile(path string) *Request[ErrRsp] {
	return r.FallbackFile(path)
}

// FallbackFS reads the fallback response from the provided filesystem.
func (r *Request[ErrRsp]) FallbackFS(fsys fs.FS, name string) *Request[ErrRsp] {
	body, err := fs.ReadFile(fsys, name)
	if err != nil {
		r.setBuildErr(err)
		return r
	}
	r.fallback = body
	return r
}

// WithFallbackFS reads the fallback response from the provided filesystem.
func (r *Request[ErrRsp]) WithFallbackFS(fsys fs.FS, name string) *Request[ErrRsp] {
	return r.FallbackFS(fsys, name)
}

// Cache sets the cache backend.
func (r *Request[ErrRsp]) Cache(cache Cache) *Request[ErrRsp] {
	r.cache = cache
	if !r.coalesceSet {
		r.coalesce = cache != nil && r.cacheable
	}
	return r
}

// WithCache sets the cache backend.
func (r *Request[ErrRsp]) WithCache(cache Cache) *Request[ErrRsp] {
	return r.Cache(cache)
}

// CacheKey overrides the derived cache key.
func (r *Request[ErrRsp]) CacheKey(key string) *Request[ErrRsp] {
	r.cacheKey = key
	return r
}

// WithCacheKey overrides the derived cache key.
func (r *Request[ErrRsp]) WithCacheKey(key string) *Request[ErrRsp] {
	return r.CacheKey(key)
}

// CacheSWR enables stale-while-revalidate reads when the cache supports it.
func (r *Request[ErrRsp]) CacheSWR() *Request[ErrRsp] {
	r.cacheSWR = true
	return r
}

// WithCacheSWR enables stale-while-revalidate reads when the cache supports it.
func (r *Request[ErrRsp]) WithCacheSWR() *Request[ErrRsp] {
	return r.CacheSWR()
}

// Key appends cache-key dimensions such as Header, BodyHash, or Field.
func (r *Request[ErrRsp]) Key(parts ...KeyPart) *Request[ErrRsp] {
	r.keyParts = append(r.keyParts, parts...)
	return r
}

// Cacheable toggles whether the request may use cache.
func (r *Request[ErrRsp]) Cacheable(v bool) *Request[ErrRsp] {
	r.cacheable = v
	r.cacheableSet = true
	if !r.coalesceSet {
		r.coalesce = v && r.cache != nil
	}
	return r
}

// Retry configures a fallback retry policy for request errors.
func (r *Request[ErrRsp]) Retry(policy errs.RetryPolicy) *Request[ErrRsp] {
	r.retryPolicy = policy
	r.retrySet = true
	return r
}

// WithRetryPolicy configures a fallback retry policy for request errors.
func (r *Request[ErrRsp]) WithRetryPolicy(policy errs.RetryPolicy) *Request[ErrRsp] {
	return r.Retry(policy)
}

// StatusRetry configures retry metadata for a specific HTTP status code.
func (r *Request[ErrRsp]) StatusRetry(status int, policy errs.RetryPolicy) *Request[ErrRsp] {
	if r.statusRetries == nil {
		r.statusRetries = make(errs.StatusRetryPolicy)
	}
	r.statusRetries[status] = policy
	return r
}

// WithStatusRetry configures retry metadata for a specific HTTP status code.
func (r *Request[ErrRsp]) WithStatusRetry(status int, policy errs.RetryPolicy) *Request[ErrRsp] {
	return r.StatusRetry(status, policy)
}

// StatusRetryPolicy configures status-code retry metadata.
func (r *Request[ErrRsp]) StatusRetryPolicy(policy errs.StatusRetryPolicy) *Request[ErrRsp] {
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
//	err := anthropic.POST("/v1/messages").Body(&req).ErrorInto(&apiErr).Do(ctx, &rsp)
//
// dst is left untouched when the call succeeds or the body is not an ErrRsp.
func (r *Request[ErrRsp]) ErrorInto(dst *ErrRsp) *Request[ErrRsp] {
	r.errorInto = dst
	return r
}

// Classify picks the factory for an error response from its decoded body.
// It runs before the ErrorMap's status mapping; returning false (optionally
// with a Code) falls through to it. Bodies that do not decode as ErrRsp skip it.
func (r *Request[ErrRsp]) Classify(fn func(status int, body *ErrRsp, header http.Header) (Classified, bool)) *Request[ErrRsp] {
	r.classify = fn
	return r
}

// Errors maps this request's failures to typed errors; see ErrorMap.
func (r *Request[ErrRsp]) Errors(m ErrorMap) *Request[ErrRsp] {
	r.errorMap = &m
	return r
}

// Service names the upstream on error records when no ErrorMap names it.
func (r *Request[ErrRsp]) Service(name string) *Request[ErrRsp] {
	r.service = name
	return r
}

// WithStatusRetryPolicy configures status-code retry metadata.
func (r *Request[ErrRsp]) WithStatusRetryPolicy(policy errs.StatusRetryPolicy) *Request[ErrRsp] {
	return r.StatusRetryPolicy(policy)
}

// Coalesce toggles duplicate concurrent request suppression.
// Cached requests coalesce by default; uncached requests do not.
func (r *Request[ErrRsp]) Coalesce(v bool) *Request[ErrRsp] {
	r.coalesce = v
	r.coalesceSet = true
	return r
}

// Group sets the singleflight group used for request coalescing.
func (r *Request[ErrRsp]) Group(group *singleflight.Group) *Request[ErrRsp] {
	r.group = group
	return r
}

// Rate sets a blocking request rate limiter.
func (r *Request[ErrRsp]) Rate(limiter RateLimiter) *Request[ErrRsp] {
	r.limiter = limiter
	return r
}

// Gate sets a concurrency gate.
func (r *Request[ErrRsp]) Gate(gate Gate) *Request[ErrRsp] {
	r.gate = gate
	return r
}

func (r *Request[ErrRsp]) buildURL() (string, error) {
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

func (r *Request[ErrRsp]) newHTTPRequest(ctx context.Context) (*http.Request, context.CancelFunc, error) {
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

func (r *Request[ErrRsp]) requestWithContext(ctx context.Context) (*http.Request, error) {
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

func (r *Request[ErrRsp]) cacheKeyValue(ctx context.Context, req *http.Request) (string, error) {
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

func (r *Request[ErrRsp]) httpClient() *http.Client {
	if r.client != nil {
		return r.client
	}
	return http.DefaultClient
}

// Do executes the request and decodes a 2xx JSON body into out. Rsp is
// inferred from the pointer: .Do(ctx, &rsp).
func (r *Request[ErrRsp]) Do[Rsp any](ctx context.Context, out *Rsp) error {
	if out == nil {
		return errs.AsError(ctx, fmt.Errorf("output pointer is nil"))
	}
	if r.buildErr != nil {
		return errs.AsError(ctx, r.buildErr)
	}
	// The response codec comes from *Rsp (HasDataType), ResponseAs, or the
	// request default, and is what we ask for when Accept is not set.
	r.decodeAs = r.responseDataType(any(out))
	if r.headers.Get("Accept") == "" {
		r.headers.Set("Accept", r.decodeAs.ContentType())
	}
	req, cancel, err := r.newHTTPRequest(ctx)
	if err != nil {
		return r.fail(ctx, out, errs.AsError(ctx, err))
	}
	defer cancel()

	// Bodies are checked against Rsp before they are cached or shared, so a
	// malformed response never poisons the cache for later callers.
	dt := r.decodeAs
	validate := func(body []byte) error { return validateAs[Rsp](dt, body) }

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

func (r *Request[ErrRsp]) loadBytes(ctx context.Context, key string, validate func([]byte) error) ([]byte, error) {
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

func (r *Request[ErrRsp]) fetchAndCache(ctx context.Context, key string, validate func([]byte) error) ([]byte, error) {
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

func (r *Request[ErrRsp]) wait(ctx context.Context) (func(), error) {
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

func (r *Request[ErrRsp]) executeBytesWithRetry(ctx context.Context) ([]byte, error) {
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

func (r *Request[ErrRsp]) executeBytes(req *http.Request) ([]byte, error) {
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

func (r *Request[ErrRsp]) serviceName(req *http.Request) string {
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
func (r *Request[ErrRsp]) transportError(req *http.Request, err error, took time.Duration) *errs.Error {
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

func (r *Request[ErrRsp]) error(ctx context.Context, err error) *errs.Error {
	e := errs.AsError(ctx, err)
	if r.retrySet {
		e.WithRetryPolicy(r.retryPolicy)
	}
	return e
}

// statusError maps a non-2xx response through the ErrorMap. The client-facing
// part comes from the chosen factory; the upstream's status, body, headers and
// own error code are kept server-side.
func (r *Request[ErrRsp]) statusError(req *http.Request, rsp *http.Response, body []byte, took time.Duration) *errs.Error {
	ctx := req.Context()
	var (
		decoded      ErrRsp
		hasDecoded   bool
		factory      errs.ErrorFactory
		providerCode string
		classified   bool
	)
	if len(body) > 0 && dataTypeOf(any(&decoded), r.defaultDataType()).Unmarshal(body, &decoded) == nil {
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
func (r *Request[ErrRsp]) retryPolicyForStatus(status int, factory errs.ErrorFactory) (errs.RetryPolicy, bool) {
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

func validateAs[Rsp any](dt DataType, body []byte) error {
	if len(body) == 0 {
		return nil
	}
	var out Rsp
	return dt.Unmarshal(body, &out)
}

func (r *Request[ErrRsp]) decode[Rsp any](ctx context.Context, out *Rsp, body []byte) error {
	if len(body) == 0 {
		return nil
	}
	if err := r.responseCodec().Unmarshal(body, out); err != nil {
		return r.fail(ctx, out, errs.AsError(ctx, err))
	}
	return nil
}

func (r *Request[ErrRsp]) fail[Rsp any](ctx context.Context, out *Rsp, err error) error {
	if len(r.fallback) == 0 {
		return err
	}
	if decodeErr := r.responseCodec().Unmarshal(r.fallback, out); decodeErr != nil {
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
