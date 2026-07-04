package hit_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/omniaura/go-kit/net/hit"
	"github.com/redis/go-redis/v9"
)

type recordingCache struct {
	mu     sync.Mutex
	values map[string][]byte
	gets   []string
	sets   []string
}

func newRecordingCache() *recordingCache {
	return &recordingCache{values: make(map[string][]byte)}
}

func (c *recordingCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gets = append(c.gets, key)
	value, ok := c.values[key]
	if !ok {
		return nil, false, nil
	}
	out := append([]byte(nil), value...)
	return out, true, nil
}

func (c *recordingCache) Set(ctx context.Context, key string, value []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sets = append(c.sets, key)
	c.values[key] = append([]byte(nil), value...)
	return nil
}

func (c *recordingCache) lastSetKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sets) == 0 {
		return ""
	}
	return c.sets[len(c.sets)-1]
}

func (c *recordingCache) setCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sets)
}

type errorCache struct {
	getErr error
	setErr error
}

func (c errorCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return nil, false, c.getErr
}

func (c errorCache) Set(ctx context.Context, key string, value []byte) error {
	return c.setErr
}

func TestCacheKeyIncludesPathQueryAndConfiguredDimensions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: 1})
	}))
	defer server.Close()

	cache := newRecordingCache()
	var out testItem
	err := hit.POST[testItem](server.URL+"/v1/models?b=2").
		Query("a", "1").
		Header("Authorization", "Bearer secret").
		Header("X-Scope", "tenant-a").
		BodyString("payload").
		Cacheable(true).
		Cache(cache).
		Key(
			hit.Header("X-Scope"),
			hit.Field("provider", "openrouter"),
			hit.BodyHash(),
		).
		Do(context.Background(), &out)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	key := cache.lastSetKey()
	if !strings.Contains(key, "/v1/models?a=1&b=2") {
		t.Fatalf("cache key %q does not include normalized path/query", key)
	}
	for _, want := range []string{
		"field:provider=openrouter",
		"header:authorization=sha256:",
		"header:x-scope=sha256:",
		"|body=sha256:",
	} {
		if !strings.Contains(key, want) {
			t.Fatalf("cache key %q missing %q", key, want)
		}
	}
	for _, leaked := range []string{"secret", "payload"} {
		if strings.Contains(key, leaked) {
			t.Fatalf("cache key leaked sensitive value %q: %q", leaked, key)
		}
	}
}

func TestPathComposesWithConstructorURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: 1})
	}))
	defer server.Close()

	var out testItem
	if err := hit.GET[testItem](server.URL).Path("/v1/models").Do(context.Background(), &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

type fakeRedis struct {
	mu     sync.Mutex
	values map[string][]byte
	ttls   map[string]time.Duration
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{
		values: make(map[string][]byte),
		ttls:   make(map[string]time.Duration),
	}
}

func (r *fakeRedis) Get(ctx context.Context, key string) *redis.StringCmd {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(string(value), nil)
}

func (r *fakeRedis) Set(
	ctx context.Context,
	key string,
	value any,
	expiration time.Duration,
) *redis.StatusCmd {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch v := value.(type) {
	case []byte:
		r.values[key] = append([]byte(nil), v...)
	case string:
		r.values[key] = []byte(v)
	default:
		r.values[key] = []byte(fmt.Sprint(v))
	}
	r.ttls[key] = expiration
	return redis.NewStatusResult("OK", nil)
}

func TestRedisCacheAdapter(t *testing.T) {
	ctx := context.Background()
	fake := newFakeRedis()
	cache, err := hit.NewRedisCache(fake, hit.WithPrefix("hit:"), hit.WithTTL(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok, err := cache.Get(ctx, "models"); err != nil || ok {
		t.Fatalf("initial Get = ok %v err %v, want miss", ok, err)
	}
	if err := cache.Set(ctx, "models", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err := cache.Get(ctx, "models")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok || string(got) != `{"ok":true}` {
		t.Fatalf("Get = %q ok %v, want cached JSON", got, ok)
	}
	if fake.ttls["hit:models"] != time.Minute {
		t.Fatalf("ttl = %s, want %s", fake.ttls["hit:models"], time.Minute)
	}
}

func TestLayeredCacheBackfillsHotLayer(t *testing.T) {
	ctx := context.Background()
	hot, err := hit.NewMapCache(hit.WithTTL(20 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	warm, err := hit.NewMapCache(hit.WithTTL(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	cache := hit.NewLayeredCache(hot, warm)
	if err := cache.Set(ctx, "models", []byte("warm")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	time.Sleep(40 * time.Millisecond)

	if _, ok, err := hot.Get(ctx, "models"); err != nil || ok {
		t.Fatalf("hot layer before backfill = ok %v err %v, want expired miss", ok, err)
	}
	got, ok, err := cache.Get(ctx, "models")
	if err != nil {
		t.Fatalf("layered Get: %v", err)
	}
	if !ok || string(got) != "warm" {
		t.Fatalf("layered Get = %q ok %v, want warm hit", got, ok)
	}
	if got, ok, err := hot.Get(ctx, "models"); err != nil || !ok || string(got) != "warm" {
		t.Fatalf("hot layer after backfill = %q ok %v err %v", got, ok, err)
	}
}

func TestLayeredCacheContinuesAfterLayerError(t *testing.T) {
	ctx := context.Background()
	warm := newRecordingCache()
	if err := warm.Set(ctx, "models", []byte("warm")); err != nil {
		t.Fatal(err)
	}
	cache := hit.NewLayeredCache(errorCache{getErr: errors.New("hot unavailable")}, warm)

	got, ok, err := cache.Get(ctx, "models")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok || string(got) != "warm" {
		t.Fatalf("Get = %q ok %v, want warm hit", got, ok)
	}
}

func TestPlainGETDoesNotCoalesce(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: 1})
	}))
	defer server.Close()

	const n = 8
	start := make(chan struct{})
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			<-start
			var out testItem
			errs <- hit.GET[testItem](server.URL).Do(context.Background(), &out)
		}()
	}
	close(start)
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != n {
		t.Fatalf("server calls = %d, want %d", got, n)
	}
}

func TestCoalesceSuppressesConcurrentGETs(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: 1})
	}))
	defer server.Close()

	const n = 20
	start := make(chan struct{})
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			<-start
			var out testItem
			errs <- hit.GET[testItem](server.URL).Coalesce(true).Do(context.Background(), &out)
		}()
	}
	close(start)
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("server calls = %d, want 1", got)
	}
}

func TestCachedGETCoalescesByDefault(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: 1})
	}))
	defer server.Close()

	cache := newRecordingCache()
	const n = 8
	start := make(chan struct{})
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			<-start
			var out testItem
			errs <- hit.GET[testItem](server.URL).Cache(cache).Do(context.Background(), &out)
		}()
	}
	close(start)
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("server calls = %d, want 1", got)
	}
}

func TestMethodOverrideRecomputesCacheableDefault(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: int(atomic.LoadInt32(&calls))})
	}))
	defer server.Close()

	cache := newRecordingCache()
	for i := 0; i < 2; i++ {
		var out testItem
		err := hit.GET[testItem](server.URL).
			Cache(cache).
			Method(http.MethodPost).
			BodyString(`{}`).
			Do(context.Background(), &out)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("server calls = %d, want 2", got)
	}
}

func TestCacheGetErrorReturns(t *testing.T) {
	cacheErr := errors.New("cache down")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called after cache get error")
	}))
	defer server.Close()

	var out testItem
	err := hit.GET[testItem](server.URL).
		Cache(errorCache{getErr: cacheErr}).
		Do(context.Background(), &out)
	if err == nil || !strings.Contains(err.Error(), cacheErr.Error()) {
		t.Fatalf("Do error = %v, want cache error", err)
	}
}

func TestCacheSetErrorReturns(t *testing.T) {
	cacheErr := errors.New("cache write failed")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: 1})
	}))
	defer server.Close()

	var out testItem
	err := hit.GET[testItem](server.URL).
		Cache(errorCache{setErr: cacheErr}).
		Do(context.Background(), &out)
	if err == nil || !strings.Contains(err.Error(), cacheErr.Error()) {
		t.Fatalf("Do error = %v, want cache set error", err)
	}
}

func TestMalformedJSONDoesNotPopulateCache(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			_, _ = w.Write([]byte("not json"))
			return
		}
		_ = json.NewEncoder(w).Encode(testItem{ID: int(call)})
	}))
	defer server.Close()

	cache := newRecordingCache()
	var out testItem
	err := hit.GET[testItem](server.URL).
		Cache(cache).
		Fallback(testItem{ID: 99}).
		Do(context.Background(), &out)
	if err != nil {
		t.Fatalf("first Do: %v", err)
	}
	if out.ID != 99 {
		t.Fatalf("fallback out = %+v, want ID 99", out)
	}
	if got := cache.setCount(); got != 0 {
		t.Fatalf("cache writes after malformed JSON = %d, want 0", got)
	}

	out = testItem{}
	if err := hit.GET[testItem](server.URL).Cache(cache).Do(context.Background(), &out); err != nil {
		t.Fatalf("second Do: %v", err)
	}
	if out.ID != 2 {
		t.Fatalf("second out = %+v, want fresh valid response", out)
	}
}

func TestRateLimiterBlocksConcurrentCallers(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: 1})
	}))
	defer server.Close()

	limiter, err := hit.NewLimiter(1, 60*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			var out testItem
			errs <- hit.GET[testItem](server.URL).
				Coalesce(false).
				Rate(limiter).
				Do(context.Background(), &out)
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("server calls = %d, want 2", got)
	}
	if elapsed := time.Since(started); elapsed < 50*time.Millisecond {
		t.Fatalf("elapsed = %s, want limiter to block", elapsed)
	}
}

func TestNewLimiterRejectsZeroDerivedInterval(t *testing.T) {
	if _, err := hit.NewLimiter(2, time.Nanosecond); err == nil {
		t.Fatal("expected error")
	}
}

func TestSemaphoreGateBoundsConcurrency(t *testing.T) {
	var active int32
	var maxActive int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := atomic.AddInt32(&active, 1)
		for {
			max := atomic.LoadInt32(&maxActive)
			if now <= max || atomic.CompareAndSwapInt32(&maxActive, max, now) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: 1})
	}))
	defer server.Close()

	gate, err := hit.NewSemaphore(1)
	if err != nil {
		t.Fatal(err)
	}

	const n = 6
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			var out testItem
			errs <- hit.GET[testItem](server.URL).
				Coalesce(false).
				Gate(gate).
				Do(context.Background(), &out)
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Fatalf("max active = %d, want 1", got)
	}
}
