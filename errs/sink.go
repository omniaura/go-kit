package errs

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Record is what a Sink keeps about an error: the client view and the
// server-only detail side by side, under the same Ref.
type Record struct {
	Time       time.Time
	Upstream   *Upstream
	Fields     map[string]string
	Params     map[string]any
	Ref        string
	Code       string
	Message    string
	Source     string
	Action     Action
	Attrs      []Attr
	RetryAfter time.Duration
	Status     int
	Severity   Severity
	Retryable  bool
}

// Detail renders Attrs as one line.
func (r Record) Detail() string {
	var b strings.Builder
	for i, attr := range r.Attrs {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(attr.Key)
		b.WriteByte('=')
		fmt.Fprint(&b, attr.Value)
	}
	return b.String()
}

// Sink keeps error records somewhere durable: a database table, an audit log,
// an alerting pipe. Record runs on the goroutine that raised the error, so it
// must not block; buffer and write asynchronously.
type Sink interface {
	Record(ctx context.Context, r Record)
}

// SinkFunc adapts a function to Sink.
type SinkFunc func(ctx context.Context, r Record)

// Record implements Sink.
func (f SinkFunc) Record(ctx context.Context, r Record) { f(ctx, r) }

// Encoder renders the Public view as an HTTP response body.
type Encoder interface {
	ContentType() string
	Encode(Public) ([]byte, error)
}

type config struct {
	encoder Encoder
	refFunc func(context.Context) string
	sinks   []Sink
}

var (
	configMu sync.Mutex
	cfg      atomic.Pointer[config]
)

func init() {
	cfg.Store(&config{encoder: JSON, refFunc: defaultRef})
}

func current() *config { return cfg.Load() }

func update(fn func(*config)) {
	configMu.Lock()
	defer configMu.Unlock()
	next := *cfg.Load()
	next.sinks = append([]Sink(nil), next.sinks...)
	fn(&next)
	cfg.Store(&next)
}

// SetEncoder sets how Abort renders response bodies. JSON is the default;
// ProblemJSON renders RFC 9457 problem details.
func SetEncoder(enc Encoder) {
	if enc == nil {
		enc = JSON
	}
	update(func(c *config) { c.encoder = enc })
}

// AddSink registers a Sink for every recorded error.
func AddSink(s Sink) {
	if s == nil {
		return
	}
	update(func(c *config) { c.sinks = append(c.sinks, s) })
}

// ResetSinks unregisters every Sink. Tests use it to restore global state.
func ResetSinks() { update(func(c *config) { c.sinks = nil }) }

// SetRefFunc sets how an error finds its reference when none was attached with
// WithRef. The default reads zerolog's hlog request id.
func SetRefFunc(fn func(context.Context) string) {
	if fn == nil {
		fn = defaultRef
	}
	update(func(c *config) { c.refFunc = fn })
}

func record(ctx context.Context, r Record) {
	for _, s := range current().sinks {
		s.Record(ctx, r)
	}
}

func (e *Error) recordFor(severity Severity) Record {
	return Record{
		Time:       time.Now(),
		Upstream:   e.upstream,
		Fields:     e.fields,
		Params:     e.params,
		Ref:        e.Ref(),
		Code:       e.code,
		Message:    e.Message(),
		Source:     e.source,
		Action:     e.action,
		Attrs:      e.logStack,
		RetryAfter: e.retryAfter,
		Status:     int(e.Status),
		Severity:   severity,
		Retryable:  e.Retryable(),
	}
}
