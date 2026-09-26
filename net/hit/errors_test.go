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

// A provider's error body, as an SDK author would declare it once for every
// endpoint.
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

// provider is the SDK: one Client carrying the error schema, error mapping
// and shared headers; endpoints pick their own response type per call.
func provider(baseURL string) *hit.Client[providerError] {
	return hit.NewClient[providerError](baseURL).
		Service("provider").
		Header("X-Sdk", "test").
		Status(529, errOverloaded).
		Classify(func(status int, b *providerError, _ http.Header) (hit.Classified, bool) {
			if b.Error.Type == "invalid_request_error" {
				return hit.Classified{Factory: errBadPrompt, Code: b.Error.Type}, true
			}
			return hit.Classified{Code: b.Error.Type}, false
		}).
		Errors(hit.ErrorMap{KeepHeaders: []string{"X-Provider-Trace"}})
}

func providerServer(t *testing.T, status int, typ string, headers map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Sdk") != "test" {
			t.Errorf("client header missing on %s", r.URL.Path)
		}
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

func TestClientClassifiesTypedErrorBody(t *testing.T) {
	srv := providerServer(t, http.StatusBadRequest, "invalid_request_error", map[string]string{"X-Provider-Trace": "tr_1"})
	var out testItem
	err := provider(srv.URL).POST("/v1/messages").Query("key", "secret").Do(context.Background(), &out)

	e := errs.AsError(context.Background(), err)
	if !errBadPrompt.Is(e) || e.ActionHint() != errs.ActionFixInput {
		t.Fatalf("want errBadPrompt, got %s %s", e.Code(), e.Message())
	}
	// Go code reads the provider's typed error through the error value.
	body, ok := e.UpstreamAs[providerError]()
	if !ok || body.Error.Type != "invalid_request_error" {
		t.Fatalf("UpstreamAs = %+v, %v", body, ok)
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

func TestClientEndpointsHaveTheirOwnResponseTypes(t *testing.T) {
	type user struct {
		Name string `json:"name"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/items/1":
			_ = json.NewEncoder(w).Encode(testItem{ID: 1, Name: "one"})
		case "/users/me":
			_ = json.NewEncoder(w).Encode(user{Name: "ada"})
		}
	}))
	t.Cleanup(srv.Close)
	c := provider(srv.URL)

	var item testItem
	var me user
	if err := c.GET("/items/1").Do(context.Background(), &item); err != nil || item.Name != "one" {
		t.Fatalf("item = %+v, %v", item, err)
	}
	if err := c.GET("users/me").Do(context.Background(), &me); err != nil || me.Name != "ada" {
		t.Fatalf("me = %+v, %v", me, err)
	}
}

func TestClientRequestsDoNotMutateTheClient(t *testing.T) {
	var seen atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.Header.Get("X-Call"))
		_ = json.NewEncoder(w).Encode(testItem{})
	}))
	t.Cleanup(srv.Close)
	c := provider(srv.URL)
	var out testItem
	_ = c.GET("/a").Header("X-Call", "first").Do(context.Background(), &out)
	_ = c.GET("/b").Do(context.Background(), &out)
	if got := seen.Load(); got != "" {
		t.Fatalf("a per-call header leaked into the next call: %q", got)
	}
}

func TestStatusMapAndFactoryRetry(t *testing.T) {
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
	if err := provider(srv.URL).GET("/").Do(context.Background(), &out); err != nil {
		t.Fatalf("the factory's retry policy should have retried the 529: %v", err)
	}
	if out.ID != 7 || calls.Load() != 2 {
		t.Fatalf("out=%+v calls=%d", out, calls.Load())
	}
}

func TestUpstreamAuthFailureIsNotForwarded(t *testing.T) {
	srv := providerServer(t, http.StatusUnauthorized, "authentication_error", nil)
	var out testItem
	err := provider(srv.URL).GET("/").Do(context.Background(), &out)
	e := errs.AsError(context.Background(), err)
	if e.Status == http.StatusUnauthorized || !hit.ErrUpstream.Is(e) {
		t.Fatalf("an upstream 401 must not become our 401, got %d %s", e.Status, e.Code())
	}
	if up, _ := e.UpstreamInfo(); up.Code != "authentication_error" {
		t.Fatalf("an unclassified body still records the provider code, got %q", up.Code)
	}
}

func TestUndecodableErrorBodySkipsClassify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>bad gateway</html>"))
	}))
	t.Cleanup(srv.Close)
	var out testItem
	err := provider(srv.URL).GET("/").Do(context.Background(), &out)
	e := errs.AsError(context.Background(), err)
	if !hit.ErrUpstreamUnavailable.Is(e) {
		t.Fatalf("want status mapping for an HTML 502, got %s", e.Code())
	}
	if _, ok := e.UpstreamAs[providerError](); ok {
		t.Fatal("an HTML body must not decode as the error schema")
	}
	if up, _ := e.UpstreamInfo(); !strings.Contains(up.Body, "bad gateway") {
		t.Fatalf("raw body not kept: %q", up.Body)
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
	err := hit.GET[hit.AnyError](srv.URL).StatusRetry(http.StatusTooManyRequests, policy).Do(context.Background(), &out)
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
	err := hit.GET[hit.AnyError](url).Service("gone").Do(context.Background(), &out)
	e := errs.AsError(context.Background(), err)
	if !hit.ErrUpstreamUnavailable.Is(e) {
		t.Fatalf("want ErrUpstreamUnavailable, got %s", e.Code())
	}
	if up, _ := e.UpstreamInfo(); up.Service != "gone" {
		t.Fatalf("service not recorded: %+v", up)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = hit.GET[hit.AnyError](url).Do(ctx, &out)
	if hit.ErrUpstreamUnavailable.Is(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("a caller cancellation is not an upstream failure: %v", err)
	}
}

// The whole chain is inferred: no type arguments after NewClient. In comes
// from JSON(&req), Out from Do(ctx, &out), and E is the Client's.
func TestInferredChainWithErrorInto(t *testing.T) {
	type messageRequest struct {
		Prompt string `json:"prompt"`
	}
	type message struct {
		Text string `json:"text"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in messageRequest
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Prompt == "fail" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"type": "invalid_request_error", "message": "bad"}})
			return
		}
		_ = json.NewEncoder(w).Encode(message{Text: "echo:" + in.Prompt})
	}))
	t.Cleanup(srv.Close)
	c := provider(srv.URL)

	req := messageRequest{Prompt: "hi"}
	var out message
	if err := c.POST("/v1/messages").JSON(&req).Do(context.Background(), &out); err != nil || out.Text != "echo:hi" {
		t.Fatalf("out = %+v, err = %v", out, err)
	}

	req.Prompt = "fail"
	var apiErr providerError
	err := c.POST("/v1/messages").JSON(&req).ErrorInto(&apiErr).Do(context.Background(), &out)
	if !errBadPrompt.Is(err) {
		t.Fatalf("want errBadPrompt, got %v", err)
	}
	if apiErr.Error.Type != "invalid_request_error" {
		t.Fatalf("ErrorInto did not receive the typed body: %+v", apiErr)
	}
}
