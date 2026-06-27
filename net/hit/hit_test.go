package hit_test

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/omniaura/go-kit/errs"
	"github.com/omniaura/go-kit/net/hit"
)

//go:embed testdata/*.json
var testFS embed.FS

type testItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type testEnvelope struct {
	Items []testItem `json:"items"`
}

func TestGET_JSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/things" {
			t.Errorf("expected /things, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: 1, Name: "one"})
	}))
	defer server.Close()

	var out testItem
	err := hit.GET[testItem](server.URL+"/things").
		WithHeader("Accept", "application/json").
		WithTimeout(5*time.Second).
		Do(context.Background(), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 1 || out.Name != "one" {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestGET_QueryParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("a") != "1" || q.Get("b") != "2" {
			t.Errorf("unexpected query: %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}))
	defer server.Close()

	var out map[string]string
	err := hit.GET[map[string]string](server.URL).
		WithQuery("a", "1").
		WithQueries(map[string]string{"b": "2"}).
		Do(context.Background(), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["ok"] != "true" {
		t.Fatalf("unexpected response: %v", out)
	}
}

func TestPOST_JSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json content-type, got %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		var in testItem
		if err := json.Unmarshal(body, &in); err != nil {
			t.Errorf("failed to decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: in.ID + 1, Name: in.Name})
	}))
	defer server.Close()

	reqBody := testItem{ID: 10, Name: "input"}
	var resp testItem
	err := hit.POST[testItem](server.URL).
		WithJSON(&reqBody).
		Do(context.Background(), &resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != 11 || resp.Name != "input" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestPOST_NotCacheableByDefault(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: calls})
	}))
	defer server.Close()

	cache, err := hit.NewMapCache()
	if err != nil {
		t.Fatal(err)
	}

	var out testItem
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		err := hit.POST[testItem](server.URL).
			WithCache(cache).
			Do(ctx, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if calls != 3 {
		t.Fatalf("expected 3 server calls, got %d", calls)
	}
}

func TestPOST_CacheableOptIn(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: calls})
	}))
	defer server.Close()

	cache, err := hit.NewMapCache()
	if err != nil {
		t.Fatal(err)
	}

	var out testItem
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		err := hit.POST[testItem](server.URL).
			WithCache(cache).
			Cacheable(true).
			Do(ctx, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("expected 1 server call, got %d", calls)
	}
	if out.ID != 1 {
		t.Fatalf("unexpected cached value: %+v", out)
	}
}

func TestGET_CacheDefault(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: calls})
	}))
	defer server.Close()

	cache, err := hit.NewMapCache()
	if err != nil {
		t.Fatal(err)
	}

	var out testItem
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		err := hit.GET[testItem](server.URL).
			WithCache(cache).
			Do(ctx, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("expected 1 server call, got %d", calls)
	}
	if out.ID != 1 {
		t.Fatalf("unexpected cached value: %+v", out)
	}
}

func TestGET_WithCacheKey(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: calls})
	}))
	defer server.Close()

	cache, err := hit.NewMapCache()
	if err != nil {
		t.Fatal(err)
	}

	var out testItem
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		err := hit.GET[testItem](server.URL).
			WithCache(cache).
			WithCacheKey("custom-key").
			Do(ctx, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("expected 1 server call, got %d", calls)
	}
}

func TestFallback_Static(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fallback := testItem{ID: 42, Name: "fallback"}
	var out testItem
	err := hit.GET[testItem](server.URL).
		WithFallback(&fallback).
		Do(context.Background(), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 42 || out.Name != "fallback" {
		t.Fatalf("unexpected fallback: %+v", out)
	}
}

func TestFallback_File(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "fallback.json")
	if err := os.WriteFile(path, []byte(`{"id":7,"name":"file"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out testItem
	err := hit.GET[testItem](server.URL).
		WithFallbackFile(path).
		Do(context.Background(), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 7 || out.Name != "file" {
		t.Fatalf("unexpected fallback: %+v", out)
	}
}

func TestFallback_FS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	var out testItem
	err := hit.GET[testItem](server.URL).
		WithFallbackFS(testFS, "testdata/fallback.json").
		Do(context.Background(), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 99 || out.Name != "fs" {
		t.Fatalf("unexpected fallback: %+v", out)
	}
}

func TestEmbedFS_Body(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var in testItem
		if err := json.Unmarshal(body, &in); err != nil {
			t.Errorf("failed to decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: in.ID * 2, Name: in.Name})
	}))
	defer server.Close()

	var out testItem
	err := hit.POST[testItem](server.URL).
		WithBodyFS(testFS, "testdata/body.json").
		Do(context.Background(), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 246 || out.Name != "embed" {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestBodyString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "hello" {
			t.Errorf("unexpected body: %q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: 1})
	}))
	defer server.Close()

	var out testItem
	err := hit.POST[testItem](server.URL).
		WithBodyString("hello").
		Do(context.Background(), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBaseURLAndPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/things/1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: 1})
	}))
	defer server.Close()

	var out testItem
	err := hit.GET[testItem]("").
		WithBaseURL(server.URL).
		WithPath("/things/1").
		Do(context.Background(), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestErrs_Non2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	var out testItem
	err := hit.GET[testItem](server.URL).
		Do(context.Background(), &out)
	if err == nil {
		t.Fatal("expected error")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *errs.Error, got %T", err)
	}
	if e.Status != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", e.Status)
	}
}

func TestErrs_Wrap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	var out testItem
	err := hit.GET[testItem](server.URL).
		Do(context.Background(), &out)
	if err == nil {
		t.Fatal("expected error")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *errs.Error, got %T", err)
	}
}

func TestNilOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	err := hit.GET[testItem](server.URL).Do(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil output pointer")
	}
}

func TestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var out testItem
	ctx := context.Background()
	err := hit.GET[testItem](server.URL).
		WithTimeout(1*time.Millisecond).
		Do(ctx, &out)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWithClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: 1})
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	var out testItem
	err := hit.GET[testItem](server.URL).
		WithClient(client).
		Do(context.Background(), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWithBodyReader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "reader body" {
			t.Errorf("unexpected body: %q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testItem{ID: 1})
	}))
	defer server.Close()

	var out testItem
	err := hit.POST[testItem](server.URL).
		WithBodyReader(strings.NewReader("reader body")).
		Do(context.Background(), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDoWithoutFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var out testItem
	err := hit.GET[testItem](server.URL).Do(context.Background(), &out)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var out testItem
	err := hit.GET[testItem](server.URL).Do(context.Background(), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
