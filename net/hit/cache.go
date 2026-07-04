package hit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/omniaura/go-kit/mapcache"
	"github.com/redis/go-redis/v9"
)

// Cache is a byte-oriented key/value cache for HTTP response bodies.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte) error
}

// SWRCache supports stale-while-revalidate reads.
type SWRCache interface {
	GetSWR(ctx context.Context, key string, up func(context.Context) ([]byte, error)) ([]byte, bool, error)
}

type cacheOptions struct {
	ttl    time.Duration
	size   *int
	prefix string
}

// CacheOpt configures hit cache adapters.
type CacheOpt func(*cacheOptions) error

// WithTTL sets the cache adapter TTL. A zero TTL means values do not expire.
func WithTTL(ttl time.Duration) CacheOpt {
	return func(o *cacheOptions) error {
		if ttl < 0 {
			return fmt.Errorf("ttl less than 0: %d", ttl)
		}
		o.ttl = ttl
		return nil
	}
}

// TTL sets the cache adapter TTL. A zero TTL means values do not expire.
func TTL(ttl time.Duration) CacheOpt {
	return WithTTL(ttl)
}

// WithSize preallocates the in-memory map cache.
func WithSize(size int) CacheOpt {
	return func(o *cacheOptions) error {
		if size < 0 {
			return fmt.Errorf("size less than 0: %d", size)
		}
		o.size = &size
		return nil
	}
}

// Size preallocates the in-memory map cache.
func Size(size int) CacheOpt {
	return WithSize(size)
}

// WithPrefix adds a cache-local key prefix.
func WithPrefix(prefix string) CacheOpt {
	return func(o *cacheOptions) error {
		o.prefix = prefix
		return nil
	}
}

// Prefix adds a cache-local key prefix.
func Prefix(prefix string) CacheOpt {
	return WithPrefix(prefix)
}

func parseCacheOptions(opts []CacheOpt) (cacheOptions, error) {
	var o cacheOptions
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return cacheOptions{}, err
		}
	}
	return o, nil
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out
}

// MapCache adapts go-kit's mapcache to hit's byte cache contract.
type MapCache struct {
	mc     *mapcache.MapCache[string, []byte]
	prefix string
}

// NewMapCache creates an in-memory cache backed by mapcache.
func NewMapCache(opts ...CacheOpt) (*MapCache, error) {
	o, err := parseCacheOptions(opts)
	if err != nil {
		return nil, err
	}
	mapOpts := make([]mapcache.OptFunc, 0, 2)
	mapOpts = append(mapOpts, mapcache.WithTTL(o.ttl))
	if o.size != nil {
		mapOpts = append(mapOpts, mapcache.WithSize(*o.size))
	}
	mc, err := mapcache.New[string, []byte](mapOpts...)
	if err != nil {
		return nil, err
	}
	return &MapCache{mc: mc, prefix: o.prefix}, nil
}

// Map creates an in-memory cache backed by mapcache.
func Map(opts ...CacheOpt) (*MapCache, error) {
	return NewMapCache(opts...)
}

func (c *MapCache) key(key string) string {
	if c.prefix == "" {
		return key
	}
	return c.prefix + key
}

// Get implements Cache.
func (c *MapCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if c == nil || c.mc == nil {
		return nil, false, nil
	}
	item, ok := c.mc.Peek(c.key(key))
	if !ok {
		return nil, false, nil
	}
	return cloneBytes(item.V), true, nil
}

// GetSWR implements SWRCache.
func (c *MapCache) GetSWR(
	ctx context.Context,
	key string,
	up func(context.Context) ([]byte, error),
) ([]byte, bool, error) {
	if c == nil || c.mc == nil {
		return nil, false, nil
	}
	value, err := c.mc.GetSWR(ctx, c.key(key), func(fetchCtx context.Context) ([]byte, error) {
		body, err := up(fetchCtx)
		return cloneBytes(body), err
	})
	if err != nil {
		return nil, false, err
	}
	return cloneBytes(value), true, nil
}

// Set implements Cache.
func (c *MapCache) Set(ctx context.Context, key string, value []byte) error {
	if c == nil || c.mc == nil {
		return nil
	}
	c.mc.Set(c.key(key), cloneBytes(value))
	return nil
}

// RedisClient is the subset of go-redis used by RedisCache.
type RedisClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
}

// RedisCache adapts Redis to hit's byte cache contract.
type RedisCache struct {
	client RedisClient
	ttl    time.Duration
	prefix string
}

// NewRedisCache creates a Redis-backed cache adapter.
func NewRedisCache(client RedisClient, opts ...CacheOpt) (*RedisCache, error) {
	if client == nil {
		return nil, errors.New("redis client must not be nil")
	}
	o, err := parseCacheOptions(opts)
	if err != nil {
		return nil, err
	}
	return &RedisCache{client: client, ttl: o.ttl, prefix: o.prefix}, nil
}

// Redis creates a Redis-backed cache adapter.
func Redis(client RedisClient, opts ...CacheOpt) (*RedisCache, error) {
	return NewRedisCache(client, opts...)
}

func (c *RedisCache) key(key string) string {
	if c.prefix == "" {
		return key
	}
	return c.prefix + key
}

// Get implements Cache.
func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if c == nil || c.client == nil {
		return nil, false, nil
	}
	value, err := c.client.Get(ctx, c.key(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return cloneBytes(value), true, nil
}

// Set implements Cache.
func (c *RedisCache) Set(ctx context.Context, key string, value []byte) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Set(ctx, c.key(key), cloneBytes(value), c.ttl).Err()
}

// LayeredCache checks caches from hot to cold and backfills hotter layers.
type LayeredCache struct {
	caches []Cache
}

// NewLayeredCache creates a cache chain similar to CPU L1/L2 caches.
func NewLayeredCache(caches ...Cache) *LayeredCache {
	out := make([]Cache, 0, len(caches))
	for _, cache := range caches {
		if cache != nil {
			out = append(out, cache)
		}
	}
	return &LayeredCache{caches: out}
}

// Layered creates a cache chain similar to CPU L1/L2 caches.
func Layered(caches ...Cache) *LayeredCache {
	return NewLayeredCache(caches...)
}

// Get implements Cache.
func (c *LayeredCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if c == nil {
		return nil, false, nil
	}
	var lastErr error
	for i, cache := range c.caches {
		value, ok, err := cache.Get(ctx, key)
		if err != nil {
			lastErr = err
			continue
		}
		if !ok {
			continue
		}
		for j := 0; j < i; j++ {
			_ = c.caches[j].Set(ctx, key, value)
		}
		return cloneBytes(value), true, nil
	}
	return nil, false, lastErr
}

// Set implements Cache.
func (c *LayeredCache) Set(ctx context.Context, key string, value []byte) error {
	if c == nil {
		return nil
	}
	var firstErr error
	for _, cache := range c.caches {
		if err := cache.Set(ctx, key, value); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
