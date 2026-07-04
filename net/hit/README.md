# hit

`hit` is a fluent HTTP helper for JSON APIs. It keeps request construction close
to `net/http`, while adding provider-friendly cache keys, byte-oriented cache
adapters, duplicate request suppression, and blocking request limits.

## Request API

```go
var out ModelsResponse

err := hit.GET[ModelsResponse]("https://openrouter.ai").
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
req := hit.GET[ModelsResponse](baseURL).
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

err := hit.GET[ModelsResponse](modelsURL).
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

err = hit.GET[ModelsResponse](modelsURL).
	Rate(limiter).
	Do(ctx, &out)
```

Use a semaphore gate to cap concurrent in-flight requests:

```go
gate, err := hit.NewSemaphore(16)

err = hit.GET[ModelsResponse](detailsURL).
	Gate(gate).
	Do(ctx, &out)
```

Both `Rate` and `Gate` honor request context cancellation while callers wait.
