package hit_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/omniaura/go-kit/errs"
	"github.com/omniaura/go-kit/net/hit"
)

// A provider's error body, as an SDK author would declare it.
type providerError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

var (
	errBadPrompt = errs.NewFactory(http.StatusBadRequest, "The request could not be processed.",
		errs.WithCode("bad_prompt"), errs.WithAction(errs.ActionFixInput))
	errOverloaded = errs.NewFactory(http.StatusServiceUnavailable, "The AI provider is overloaded.",
		errs.WithCode("provider_overloaded"), errs.WithAction(errs.ActionWait),
		errs.WithRetryPolicy(errs.FixedRetry(2, time.Millisecond)))
)

var providerErrors = hit.ErrorMap{
	Service: "provider",
	Status:  map[int]errs.ErrorFactory{529: errOverloaded},
	Classify: hit.DecodeErrorBody(func(status int, b providerError) (hit.Classified, bool) {
		if b.Error.Type == "invalid_request_error" {
			return hit.Classified{Factory: errBadPrompt, Code: b.Error.Type}, true
		}
		return hit.Classified{Code: b.Error.Type}, false
	}),
	KeepHeaders: []string{"X-Provider-Trace"},
}

func providerServer(t *testing.T, status int, typ string, headers map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"type": typ, "message": "org org_9f2 key sk-live-abc is over quota at https://internal.example"},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestErrorMapClassifiesTypedBody(t *testing.T) {
	srv := providerServer(t, http.StatusBadRequest, "invalid_request_error", map[string]string{"X-Provider-Trace": "tr_1"})
	var out testItem
	err := hit.POST[testItem](srv.URL+"/v1/messages?key=secret").Errors(providerErrors).Do(context.Background(), &out)

	e := errs.AsError(context.Background(), err)
	if !errBadPrompt.Is(e) || e.ActionHint() != errs.ActionFixInput {
		t.Fatalf("want errBadPrompt, got %s %s", e.Code(), e.Message())
	}
	up, _ := e.UpstreamInfo()
	if up.Service != "provider" || up.Code != "invalid_request_error" || up.Header["X-Provider-Trace"] != "tr_1" {
		t.Fatalf("upstream record incomplete: %+v", up)
	}
	if strings.Contains(up.URL, "secret") {
		t.Fatalf("query string kept on the record: %s", up.URL)
	}
	pub, _ := json.Marshal(e.Public())
	for _, leak := range []string{"sk-live", "org_9f2", "internal.example", "invalid_request_error"} {
		if strings.Contains(string(pub), leak) || strings.Contains(e.Error(), leak) {
			t.Fatalf("client view leaked %q: %s / %s", leak, pub, e.Error())
		}
	}
}

func TestErrorMapStatusAndFactoryRetry(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(529)
			return
		}
		_ = json.NewEncoder(w).Encode(testItem{ID: 7})
	}))
	t.Cleanup(srv.Close)

	var out testItem
	if err := hit.GET[testItem](srv.URL).Errors(providerErrors).Do(context.Background(), &out); err != nil {
		t.Fatalf("the factory's retry policy should have retried the 529: %v", err)
	}
	if out.ID != 7 || calls.Load() != 2 {
		t.Fatalf("out=%+v calls=%d", out, calls.Load())
	}
}

func TestUpstreamAuthFailureIsNotForwarded(t *testing.T) {
	srv := providerServer(t, http.StatusUnauthorized, "authentication_error", nil)
	var out testItem
	err := hit.GET[testItem](srv.URL).Errors(providerErrors).Do(context.Background(), &out)
	e := errs.AsError(context.Background(), err)
	if e.Status == http.StatusUnauthorized || !hit.ErrUpstream.Is(e) {
		t.Fatalf("an upstream 401 must not become our 401, got %d %s", e.Status, e.Code())
	}
}

func TestRetryAfterIsHonouredAndSurfaced(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	// A hint beyond MaxRetryAfter ends the loop at once and is passed on.
	policy := errs.FixedRetry(3, time.Millisecond)
	policy.MaxRetryAfter = 100 * time.Millisecond
	var out testItem
	started := time.Now()
	err := hit.GET[testItem](srv.URL).StatusRetry(http.StatusTooManyRequests, policy).Do(context.Background(), &out)
	if time.Since(started) > 500*time.Millisecond || calls.Load() != 1 {
		t.Fatalf("should not wait out a 1s hint: calls=%d took=%s", calls.Load(), time.Since(started))
	}
	e := errs.AsError(context.Background(), err)
	if !hit.ErrUpstreamRateLimited.Is(e) || e.RetryAfterHint() != time.Second || !e.Public().Retryable {
		t.Fatalf("want rate-limited with a 1s hint, got %s %s", e.Code(), e.RetryAfterHint())
	}
}

func TestTransportFailureIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	var out testItem
	err := hit.GET[testItem](url).Service("gone").Do(context.Background(), &out)
	e := errs.AsError(context.Background(), err)
	if !hit.ErrUpstreamUnavailable.Is(e) {
		t.Fatalf("want ErrUpstreamUnavailable, got %s", e.Code())
	}
	if up, _ := e.UpstreamInfo(); up.Service != "gone" {
		t.Fatalf("service not recorded: %+v", up)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = hit.GET[testItem](url).Do(ctx, &out)
	if hit.ErrUpstreamUnavailable.Is(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("a caller cancellation is not an upstream failure: %v", err)
	}
}
