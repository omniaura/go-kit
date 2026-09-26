# hit

`hit` is a fluent HTTP helper for JSON APIs. It keeps request construction close
to `net/http`, while adding provider-friendly cache keys, byte-oriented cache
adapters, duplicate request suppression, and blocking request limits.

## Request API

```go
var out ModelsResponse

err := hit.GET[hit.AnyError]("https://openrouter.ai").
	Path("/api/v1/models").
	Query("supported_parameters", "tools").
	Query("output_modalities", "text").
	Headers("Authorization", "Bearer "+apiKey).
	Do(ctx, &out)
```

Short fluent request methods are preferred: `Header`, `Headers`, `Body`, `JSON`,
`Query`, `Cache`, `Key`, `Rate`, and `Gate`. The older `WithHeader`,
`WithJSON`, `WithCache`, and similar methods remain as aliases.

## Cache Keys

The default cache key keeps the method plus URL path and query visible:

```text
hit:GET:https://openrouter.ai/api/v1/models?output_modalities=text&supported_parameters=tools
```

`Authorization` is included automatically when present, but hashed so tokens are
not leaked into map or Redis keys. Non-GET cacheable requests include a body hash
by default. Add more dimensions explicitly:

```go
req := hit.GET[hit.AnyError](baseURL).
	Path("/api/v1/models").
	Query("supported_parameters", "tools").
	Headers("Authorization", "Bearer "+apiKey).
	Key(
		hit.Header("X-Provider-Scope"),
		hit.Field("provider", "openrouter"),
		hit.BodyHash(),
	)
```

Use `CacheKey("literal-key")` only when the caller fully owns key stability and
isolation.

## Cache Backends

All caches implement a small key/bytes interface:

```go
type Cache interface {
	Get(context.Context, string) ([]byte, bool, error)
	Set(context.Context, string, []byte) error
}
```

In-memory map cache:

```go
hot, err := hit.NewMapCache(hit.WithTTL(30*time.Second), hit.WithSize(1024))
```

Redis cache:

```go
warm, err := hit.NewRedisCache(redisClient, hit.WithPrefix("hit:"), hit.WithTTL(5*time.Minute))
```

Layered cache with different TTLs:

```go
cache := hit.NewLayeredCache(hot, warm)

err := hit.GET[hit.AnyError](modelsURL).
	Cache(cache).
	Key(hit.Field("provider", "openrouter")).
	Do(ctx, &out)
```

The layered cache checks hot to cold, backfills hotter layers on a colder hit,
and writes through to every layer after a successful upstream fetch. This
supports L1/L2-style hot/warm/cold caching patterns.

## Concurrency And Limits

Cached requests coalesce concurrent duplicate calls with `singleflight` by
default. Plain uncached requests keep `net/http` behavior unless `Coalesce(true)`
is set. Concurrent waiters share the same upstream response bytes and decode
into their own output values.

Use a blocking request rate limiter for provider APIs that require N requests per
second:

```go
limiter, err := hit.RPS(5)

err = hit.GET[hit.AnyError](modelsURL).
	Rate(limiter).
	Do(ctx, &out)
```

Use a semaphore gate to cap concurrent in-flight requests:

```go
gate, err := hit.NewSemaphore(16)

err = hit.GET[hit.AnyError](detailsURL).
	Gate(gate).
	Do(ctx, &out)
```

Both `Rate` and `Gate` honor request context cancellation while callers wait.

## Two Schemas, Inferred: Response and Error

Every call has a success schema `Out` and an error schema `E`, and the types
come from the pointers you already pass:

```go
var out ModelsResponse
err := hit.GET[OpenRouterError](modelsURL).Do(ctx, &out) // E explicit, Out inferred from &out
```

- **`Out` is inferred at `Do(ctx, &out)`.** Go infers type arguments only from
  the arguments of the generic call itself, never backwards through a method
  chain. So `Out` belongs on the last call, which is a Go 1.27 generic method,
  and not on `GET`.
- **`In` (the request body) is inferred at `JSON(&req)`.** It takes a pointer,
  so the body is never copied on the way in. A request body type says nothing
  about the response type, so `JSON` cannot supply `Out`.
- **`E` is fixed once**, on the `Client`, or explicitly on a one-off
  `hit.GET[E](url)`. On any non-2xx response the body is decoded as `E`, passed
  to `Classify` as `*E`, copied into `ErrorInto(&apiErr)` if you asked for it,
  and kept on the returned `*errs.Error`:

```go
var apiErr OpenRouterError
err := hit.GET[OpenRouterError](modelsURL).ErrorInto(&apiErr).Do(ctx, &out)
// or, after the fact:
if body, ok := errs.AsError(ctx, err).UpstreamAs[OpenRouterError](); ok { … }
```

Use `hit.AnyError` (a `json.RawMessage`) for endpoints with no documented error
shape.

## Client: an SDK in a few declarations (Go 1.27)

Providers usually have **one** error shape for every endpoint and a
**different** response shape per endpoint. `hit.Client[E]` fixes `E` and the
SDK-wide configuration once. After `NewClient` no call site needs a type
argument:

```go
type apiError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

var anthropic = hit.NewClient[apiError]("https://api.anthropic.com").
	Service("anthropic").
	Header("anthropic-version", "2023-06-01").
	HTTPClient(sharedHTTPClient).
	Timeout(2 * time.Minute).
	Status(529, ErrOverloaded).
	Classify(func(status int, b *apiError, _ http.Header) (hit.Classified, bool) {
		if b.Error.Type == "invalid_request_error" {
			return hit.Classified{Factory: ErrBadRequest, Code: b.Error.Type}, true
		}
		return hit.Classified{Code: b.Error.Type}, false // keep the type, map by status
	}).
	StatusRetryPolicy(errs.TransientStatusRetry(errs.ExponentialRetry(3, 250*time.Millisecond, 4*time.Second)))

func CreateMessage(ctx context.Context, key string, req MessageRequest) (Message, error) {
	var out Message
	err := anthropic.POST("/v1/messages").Header("x-api-key", key).JSON(&req).Do(ctx, &out)
	return out, err
}

func ListModels(ctx context.Context, key string) (ModelList, error) {
	var out ModelList
	err := anthropic.GET("/v1/models").Header("x-api-key", key).Do(ctx, &out)
	return out, err
}
```

The Client holds the base URL, `*http.Client`, default headers, timeout, error
map, typed `Classify`, retry tables, `Rate` limiter, `Gate` and `Cache`. Each
call copies those settings into a fresh `Request`, so per-call builders never
change the Client, and one Client is safe to share across goroutines.

## Typed Errors

A failed call returns an `*errs.Error` that describes the failure from the
caller's side. Resolution order:

1. The typed `Classify` on the decoded `E`.
2. `ErrorMap.Status[code]`.
3. `ClientError` / `ServerError`.
4. The defaults:

| Upstream response | Error |
|---|---|
| 429 | `hit.ErrUpstreamRateLimited` (503, action `wait`, with any `Retry-After` hint) |
| 408, 5xx | `hit.ErrUpstreamUnavailable` (503, action `retry`) |
| any other non-2xx | `hit.ErrUpstream` (502) |
| no response (dial, TLS, timeout) | `hit.ErrUpstreamUnavailable` |

An upstream 401 means *your* credential is wrong. It must never reach your own
caller as a 401 that signs them out. The upstream's real status, the body
(truncated), the provider's error type, and request-id and rate-limit headers
are kept on `errs.Upstream`, alongside the decoded `E`. They go to the log line
and the error `Sink`s, never to the client. The URL is recorded without its
query string.

Retry precedence per status: `StatusRetry` on the request, then the chosen
factory's own `WithRetryPolicy`, then the request's `Retry`. A `Retry-After`
hint longer than the policy's backoff is honoured, up to `MaxRetryAfter`.
