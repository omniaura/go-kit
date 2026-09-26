package errs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/omniaura/go-kit/errs"
	"github.com/omniaura/go-kit/errs/validation"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

const secret = `pq: relation "billing_ledger" does not exist; dial tcp 10.0.3.7:5432`

// serve runs h behind hlog's request-id middleware and captures the log.
func serve(t *testing.T, h http.HandlerFunc) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var logs bytes.Buffer
	logger := zerolog.New(&logs)
	chain := hlog.NewHandler(logger)(hlog.RequestIDHandler("req_id", "Request-Id")(h))
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	return rec, logs.String()
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) errs.Public {
	t.Helper()
	var p errs.Public
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return p
}

func TestAbortSplitsClientBodyFromServerDetail(t *testing.T) {
	rec, logs := serve(t, func(w http.ResponseWriter, r *http.Request) {
		errs.Unknown.New(r.Context()).
			AddError(errors.New(secret)).
			AddAttrs(errs.String("user", "u_123")).
			Abort(w)
	})
	body := rec.Body.String()
	for _, leak := range []string{"billing_ledger", "10.0.3.7", "u_123"} {
		if strings.Contains(body, leak) {
			t.Fatalf("body leaked %q: %s", leak, body)
		}
		if !strings.Contains(logs, leak) {
			t.Fatalf("log lost %q: %s", leak, logs)
		}
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	p := decode(t, rec)
	if p.Message != "unknown error" || p.Code != "internal_error" || p.Status != 500 {
		t.Fatalf("unexpected public view: %+v", p)
	}
	if p.Ref == "" || p.Ref != rec.Header().Get("Request-Id") {
		t.Fatalf("ref %q should be the request id %q", p.Ref, rec.Header().Get("Request-Id"))
	}
	if !strings.Contains(logs, `"error_ref":"`+p.Ref+`"`) || !strings.Contains(logs, `"level":"error"`) {
		t.Fatalf("log line missing ref or error level: %s", logs)
	}
}

func TestClientBuilderShapesBody(t *testing.T) {
	limited := errs.NewFactory(http.StatusTooManyRequests, "Slow down a little.",
		errs.WithCode("slow_down"), errs.WithAction(errs.ActionWait))
	rec, logs := serve(t, func(w http.ResponseWriter, r *http.Request) {
		limited.New(r.Context()).Param("limit", 60).RetryAfter(1500 * time.Millisecond).Abort(w)
	})
	p := decode(t, rec)
	if p.Code != "slow_down" || p.Action != errs.ActionWait || !p.Retryable || p.RetryAfterMs != 1500 {
		t.Fatalf("unexpected public view: %+v", p)
	}
	if p.Params["limit"] != float64(60) {
		t.Fatalf("params = %v", p.Params)
	}
	if got := rec.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want 2 (rounded up)", got)
	}
	if !strings.Contains(logs, `"level":"warn"`) {
		t.Fatalf("a 4xx should log at warn: %s", logs)
	}
}

func TestFieldImpliesFixInput(t *testing.T) {
	rec, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		validation.CheckEmptyStringFields(r.Context(), "email", "", "name", "Ada", "title", "").Abort(w)
	})
	p := decode(t, rec)
	if p.Action != errs.ActionFixInput || p.Fields["email"] != "is required" || p.Fields["title"] != "is required" {
		t.Fatalf("unexpected public view: %+v", p)
	}
	if _, ok := p.Fields["name"]; ok {
		t.Fatalf("a present field was reported: %+v", p.Fields)
	}
}

func TestSinksReceiveServerFaultsOnce(t *testing.T) {
	var got []errs.Record
	errs.AddSink(errs.SinkFunc(func(_ context.Context, r errs.Record) { got = append(got, r) }))
	t.Cleanup(errs.ResetSinks)

	serve(t, func(w http.ResponseWriter, r *http.Request) {
		errs.NewFactory(404, "not found").New(r.Context()).Abort(w)
	})
	if len(got) != 0 {
		t.Fatalf("a 404 is not a fault worth keeping: %+v", got)
	}

	audited := errs.NewFactory(http.StatusForbidden, "not allowed", errs.WithRecord(true))
	serve(t, func(w http.ResponseWriter, r *http.Request) { audited.New(r.Context()).Abort(w) })
	if len(got) != 1 || got[0].Status != 403 {
		t.Fatalf("WithRecord(true) should record a 403: %+v", got)
	}

	rec, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		e := errs.Unknown.New(r.Context()).AddError(errors.New(secret)).Upstream(errs.Upstream{Service: "db", Status: 0})
		e.Abort(w)
		e.Abort(httptest.NewRecorder()) // a second write must not record twice
	})
	if len(got) != 2 {
		t.Fatalf("want 2 records, got %d", len(got))
	}
	r := got[1]
	if r.Ref != rec.Header().Get("Request-Id") || r.Source != "http" || r.Severity != errs.SeverityError {
		t.Fatalf("unexpected record: %+v", r)
	}
	if !strings.Contains(r.Detail(), "billing_ledger") || r.Upstream == nil || r.Upstream.Service != "db" {
		t.Fatalf("record lost server-only detail: %+v", r)
	}
}

func TestEmitForNonHTTPTransports(t *testing.T) {
	var got []errs.Record
	errs.AddSink(errs.SinkFunc(func(_ context.Context, r errs.Record) { got = append(got, r) }))
	t.Cleanup(errs.ResetSinks)

	ctx, ref := errs.WithNewRef(context.Background())
	pub := errs.NewFactory(http.StatusBadGateway, "The AI provider failed. Try again.",
		errs.WithCode("provider_failed"), errs.WithAction(errs.ActionRetry)).
		New(ctx).AddError(errors.New(secret)).Emit("chat_stream")
	if pub.Ref != ref || pub.Code != "provider_failed" || !pub.Retryable {
		t.Fatalf("unexpected public view: %+v", pub)
	}
	if b, _ := json.Marshal(pub); strings.Contains(string(b), "billing_ledger") {
		t.Fatalf("public view leaked detail: %s", b)
	}
	if len(got) != 1 || got[0].Source != "chat_stream" || got[0].Ref != ref {
		t.Fatalf("unexpected records: %+v", got)
	}
}

func TestProblemJSONEncoder(t *testing.T) {
	errs.SetEncoder(errs.ProblemEncoder{TypeBase: "https://errors.example.com/"})
	t.Cleanup(func() { errs.SetEncoder(nil) })
	rec, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		errs.NewFactory(http.StatusConflict, "Already running.", errs.WithCode("already_running")).New(r.Context()).Abort(w)
	})
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	var p map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p["type"] != "https://errors.example.com/already_running" || p["title"] != "Already running." || p["status"] != float64(409) || p["ref"] == "" {
		t.Fatalf("unexpected problem: %v", p)
	}
}

func TestAsError(t *testing.T) {
	ctx := context.Background()
	if errs.AsError(ctx, nil) != nil {
		t.Fatal("AsError(nil) must be nil so Abort is a no-op")
	}
	e := errs.AsError(ctx, errors.New(secret))
	if !errs.Unknown.Is(e) || strings.Contains(e.Error(), "billing_ledger") {
		t.Fatalf("AsError should wrap as Unknown without exposing the cause: %v", e)
	}
	if !strings.Contains(e.Detail(), "billing_ledger") {
		t.Fatalf("AsError lost the cause server-side: %q", e.Detail())
	}
	wrapped := errors.Join(errors.New("context"), e)
	if errs.AsError(ctx, wrapped) != e {
		t.Fatal("AsError should find an *Error in the chain")
	}
}

func TestSeverityOverrides(t *testing.T) {
	quiet := errs.NewFactory(http.StatusServiceUnavailable, "draining", errs.WithSeverity(errs.SeverityInfo))
	_, logs := serve(t, func(w http.ResponseWriter, r *http.Request) { quiet.New(r.Context()).Abort(w) })
	if !strings.Contains(logs, `"level":"info"`) {
		t.Fatalf("WithSeverity(Info) should log at info: %s", logs)
	}
	_, logs = serve(t, func(w http.ResponseWriter, r *http.Request) {
		errs.NewFactory(499, "client closed").New(r.Context()).Abort(w)
	})
	if !strings.Contains(logs, `"level":"info"`) {
		t.Fatalf("499 should log at info: %s", logs)
	}
}

func TestCausesUnwrapButStayOffTheWire(t *testing.T) {
	sentinel := errors.New(secret)
	e := errs.Unknown.New(context.Background()).AddError(sentinel)
	if !errors.Is(e, sentinel) {
		t.Fatal("errors.Is should see an added cause")
	}
	if strings.Contains(e.Error(), "billing_ledger") {
		t.Fatalf("Error() leaked the cause: %s", e.Error())
	}
}
