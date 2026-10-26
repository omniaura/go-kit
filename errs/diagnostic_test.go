package errs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/omniaura/go-kit/errs"
	"github.com/rs/zerolog"
)

// Every accidental formatting/serialization of the original cause fails fast.
type privateCause struct{ status int }

func (*privateCause) Error() string                { panic("raw Error called") }
func (*privateCause) String() string               { panic("raw String called") }
func (*privateCause) Format(fmt.State, rune)       { panic("raw Format called") }
func (*privateCause) MarshalJSON() ([]byte, error) { panic("raw JSON called") }
func (e *privateCause) HTTPStatus() int            { return e.status }

type sqlCause string

func (e sqlCause) Error() string    { panic("SQL Error called") }
func (e sqlCause) SQLState() string { return string(e) }

type vendorCause struct{ status int }

func (*vendorCause) Error() string { panic("vendor Error called") }

type cyclicCause struct{}

func (e *cyclicCause) Error() string { panic("cycle Error called") }
func (e *cyclicCause) Unwrap() error { return e }

type sliceCause []int            // non-comparable errors are legal
func (sliceCause) Error() string { panic("slice Error called") }

func TestDiagnoseWithoutFormatting(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		class string
	}{
		{"nil", nil, "none"},
		{"typed-nil", (*privateCause)(nil), "none"},
		{"unknown", errors.New(secret), "other"},
		{"non-comparable", sliceCause{1}, "other"},
		{"auth", &privateCause{403}, "http_auth"},
		{"limited", &privateCause{429}, "http_rate_limit"},
		{"server", &privateCause{503}, "http_server"},
		{"request", &privateCause{400}, "http_request"},
		{"invalid-status", &privateCause{999}, "other"},
		{"sql", sqlCause("23505"), "database_constraint"},
		{"sql-connection", sqlCause("08006"), "database_connection"},
		{"sql-other", sqlCause("40001"), "database"},
		{"bad-sql", sqlCause(secret), "other"},
		{"url", &url.Error{Op: secret, URL: secret, Err: errors.New(secret)}, "url_request"},
		{"timeout", &url.Error{Op: secret, URL: secret, Err: context.DeadlineExceeded}, "timeout"},
		{"joined-cancel", errors.Join(&privateCause{503}, context.DeadlineExceeded, context.Canceled), "canceled"},
		{"joined-timeout", errors.Join(&privateCause{503}, context.DeadlineExceeded), "timeout"},
		{"errs-upstream", errs.Unknown.New(t.Context()).Upstream(errs.Upstream{Status: 429, URL: secret, Body: secret, Decoded: &privateCause{}}), "http_rate_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := errs.Diagnose(tc.err)
			if d.Class != tc.class {
				t.Fatalf("class = %q, want %q", d.Class, tc.class)
			}
			b, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), secret) || strings.Contains(d.String(), secret) {
				t.Fatalf("leak: %s", b)
			}
			var logs bytes.Buffer
			logger := zerolog.New(&logs)
			logger.Error().Object("error", d).Msg("failed")
			slog.New(slog.NewJSONHandler(&logs, nil)).Error("failed", "error", d)
			if strings.Contains(logs.String(), secret) || !strings.Contains(logs.String(), tc.class) {
				t.Fatalf("logs: %s", logs.String())
			}
		})
	}
}

func TestDiagnoseCustomClassifierAndTree(t *testing.T) {
	vendor := &vendorCause{status: 429}
	// Do not use fmt.Errorf: that itself formats the hostile cause.
	cause := errors.Join(vendor, sqlCause("23505"))
	d := errs.Diagnose(cause, nil, func(err error) errs.Classification {
		if e, ok := err.(*vendorCause); ok {
			return errs.Classification{Status: e.status}
		}
		return errs.Classification{}
	})
	if d.Class != "http_rate_limit" || len(d.Causes) != 3 {
		t.Fatalf("diagnostic: %v", d)
	}
	if d.Causes[0].Parent != -1 || d.Causes[1].Parent != 0 || d.Causes[2].Parent != 0 || d.Causes[1].Status != 429 || d.Causes[2].SQLState != "23505" {
		t.Fatalf("lost tree/status/state: %v", d)
	}
	if errs.Diagnose(vendor).Class != "other" {
		t.Fatal("custom adapter leaked into global state")
	}
}

func TestDiagnoseBoundedTrees(t *testing.T) {
	wide := make([]error, 1000)
	for i := range wide {
		wide[i] = errors.New(secret)
	}
	for _, err := range []error{&cyclicCause{}, errors.Join(wide...)} {
		d := errs.Diagnose(err)
		if !d.Truncated || len(d.Causes) != 32 {
			t.Fatalf("not bounded: %v", d)
		}
	}
}

func TestAddPrivateErrorRetainsCauseWithoutDisclosure(t *testing.T) {
	for _, transport := range []string{"http", "stream"} {
		t.Run(transport, func(t *testing.T) {
			var logs bytes.Buffer
			logger := zerolog.New(&logs)
			ctx := logger.WithContext(errs.WithRef(t.Context(), "private-ref"))
			var records []errs.Record
			errs.AddSink(errs.SinkFunc(func(_ context.Context, r errs.Record) { records = append(records, r) }))
			t.Cleanup(errs.ResetSinks)
			original := &privateCause{503}
			e := errs.Unknown.New(ctx).AddPrivateError(nil).AddPrivateError(errors.Join(original, context.DeadlineExceeded))
			var got *privateCause
			if !errors.As(e, &got) || got != original || !errors.Is(e, context.DeadlineExceeded) {
				t.Fatal("cause lost")
			}
			d, ok := e.AttrAs[errs.Diagnostic]("error")
			if !ok || d.Class != "timeout" {
				t.Fatalf("diagnostic: %v", d)
			}
			rec := httptest.NewRecorder()
			if transport == "http" {
				e.Abort(rec)
			} else {
				e.Emit("stream")
			}
			if len(records) != 1 || records[0].Ref != "private-ref" {
				t.Fatalf("records: %v", records)
			}
			b, err := json.Marshal(records[0])
			if err != nil {
				t.Fatal(err)
			}
			for _, output := range []string{logs.String(), string(b), records[0].Detail(), e.Detail(), rec.Body.String()} {
				if strings.Contains(output, secret) || strings.Contains(output, "PANIC") {
					t.Fatalf("unsafe output: %s", output)
				}
			}
			if !strings.Contains(logs.String(), `"status":503`) || !strings.Contains(records[0].Detail(), "timeout") {
				t.Fatalf("lost diagnostics: %s / %s", logs.String(), records[0].Detail())
			}
		})
	}
}

func TestPrivateErrorPropagation(t *testing.T) {
	original := &privateCause{429}
	private := errs.Unknown.New(t.Context()).AddPrivateError(errors.Join(original, context.Canceled))
	var got *privateCause
	if !errors.As(private, &got) || got != original || !errors.Is(private, context.Canceled) {
		t.Fatal("cause lost")
	}
	for _, format := range []string{"%v", "%+v", "%s", "%q"} {
		out := fmt.Sprintf(format, private)
		if strings.Contains(out, "PANIC") {
			t.Fatalf("format %s: %s", format, out)
		}
	}
	wrapped := fmt.Errorf("operation failed: %w", private)
	if errs.AsError(t.Context(), wrapped) != private {
		t.Fatal("AsError lost private boundary")
	}
	e := errs.Unknown.New(t.Context()).AddError(wrapped)
	if !errors.Is(e, context.Canceled) {
		t.Fatal("legacy propagation lost cause")
	}
}
