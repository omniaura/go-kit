package errs

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"reflect"

	"github.com/rs/zerolog"
)

// Classification is an allowlisted description of one error. Empty fields mean
// no opinion. Class must be an application-defined constant, never error text.
// Status is an HTTP status; SQLState must be five uppercase letters/digits.
type Classification struct {
	Class    string `json:"class,omitempty"`
	Status   int    `json:"status,omitempty"`
	SQLState string `json:"sqlstate,omitempty"`
}

// Classifier adapts a vendor error without adding a vendor dependency to errs.
// It receives ONE node, not the whole error tree: use a type assertion, not
// errors.As/Is. Return only audited metadata, never messages, URLs, request IDs,
// bodies or arbitrary provider codes. Classifiers are trusted application code.
type Classifier func(error) Classification

// DiagnosticCause describes one node of an error tree without its values.
type DiagnosticCause struct {
	Classification
	Type   string `json:"type"`
	Parent int    `json:"parent"` // -1 for the root; otherwise an index in Causes
}

// Diagnostic is a message-free snapshot for logs, metrics and durable records.
// It contains no original error, so JSON, fmt, slog and custom sinks cannot
// accidentally invoke an error's Error, String, Format or MarshalJSON methods.
// Class is the first recognized class in breadth-first order, with cancellation
// and then timeouts taking precedence. Unrecognized errors have class "other";
// nil has class "none". Causes includes wrappers and all branches of joins.
type Diagnostic struct {
	Class     string            `json:"class"`
	Causes    []DiagnosticCause `json:"causes,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
}

const maxDiagnosticCauses = 32

// Diagnose inspects at most 32 error nodes. It never formats an error or reads
// its fields reflectively. Standard support covers context cancellation,
// net.Error timeouts, URL errors, SQLState() string and HTTPStatus()/StatusCode()
// int interfaces, and errs.Error. Explicit classifiers run before the standard
// status/SQL/URL adapters; context and timeout detection always take precedence.
// Custom Unwrap, Timeout and classifier methods must be safe and non-blocking.
func Diagnose(err error, classifiers ...Classifier) Diagnostic {
	d := Diagnostic{Class: "none"}
	if nilError(err) {
		return d
	}
	type pending struct {
		err    error
		parent int
	}
	queue := []pending{{err, -1}}
	d.Class = "other"
	for i := 0; i < len(queue); i++ {
		item := queue[i]
		c := classifyDiagnostic(item.err, classifiers)
		d.Causes = append(d.Causes, DiagnosticCause{
			Classification: c, Type: reflect.TypeOf(item.err).String(), Parent: item.parent,
		})
		if c.Class == "canceled" || (c.Class == "timeout" && d.Class != "canceled") || d.Class == "other" && c.Class != "" {
			d.Class = c.Class
		}
		var children []error
		switch e := item.err.(type) {
		case interface{ Unwrap() []error }:
			children = e.Unwrap()
		case interface{ Unwrap() error }:
			children = []error{e.Unwrap()}
		}
		// Bound both work and allocation, even for cycles or enormous joins.
		if len(children) > maxDiagnosticCauses {
			children = children[:maxDiagnosticCauses]
			d.Truncated = true
		}
		for _, child := range children {
			if nilError(child) {
				continue
			}
			if len(queue) == maxDiagnosticCauses {
				d.Truncated = true
				break
			}
			queue = append(queue, pending{child, i})
		}
	}
	return d
}

func nilError(err error) bool {
	if err == nil {
		return true
	}
	v := reflect.ValueOf(err)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func classifyDiagnostic(err error, classifiers []Classifier) Classification {
	switch err {
	case context.Canceled:
		return Classification{Class: "canceled"}
	case context.DeadlineExceeded:
		return Classification{Class: "timeout"}
	}
	// Standard wrapper Timeout methods recurse into their child. Inspect the
	// child via our bounded queue instead, so cyclic URL/net wrappers stay safe.
	switch err.(type) {
	case *url.Error, *net.OpError:
	default:
		if e, ok := err.(net.Error); ok && e.Timeout() {
			return Classification{Class: "timeout"}
		}
	}
	for _, classify := range classifiers {
		if classify == nil {
			continue
		}
		if c := normalizeClassification(classify(err)); c != (Classification{}) {
			return c
		}
	}
	c := Classification{}
	switch e := err.(type) {
	case *Error:
		c.Status = int(e.Status)
		if e.upstream != nil {
			c.Status = e.upstream.Status
		}
	case interface{ HTTPStatus() int }:
		c.Status = e.HTTPStatus()
	case interface{ StatusCode() int }:
		c.Status = e.StatusCode()
	}
	if e, ok := err.(interface{ SQLState() string }); ok {
		c.SQLState = e.SQLState()
	}
	c = normalizeClassification(c)
	if c.Class == "" {
		if _, ok := err.(*url.Error); ok {
			c.Class = "url_request"
		}
	}
	return c
}

func normalizeClassification(c Classification) Classification {
	if c.Status < 100 || c.Status > 599 {
		c.Status = 0
	}
	if !validSQLState(c.SQLState) {
		c.SQLState = ""
	}
	if c.Class != "" {
		return c
	}
	switch {
	case c.Status == 401 || c.Status == 403:
		c.Class = "http_auth"
	case c.Status == 429:
		c.Class = "http_rate_limit"
	case c.Status >= 500:
		c.Class = "http_server"
	case c.Status >= 400:
		c.Class = "http_request"
	case c.SQLState != "":
		c.Class = "database"
		switch c.SQLState[:2] {
		case "23":
			c.Class = "database_constraint"
		case "08":
			c.Class = "database_connection"
		}
	}
	return c
}

func validSQLState(code string) bool {
	if len(code) != 5 {
		return false
	}
	for _, c := range code {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

// String renders only the snapshot; no raw errors are retained in it.
func (d Diagnostic) String() string {
	b, _ := json.Marshal(d)
	return string(b)
}

// MarshalZerologObject supports logger.Error().Object("error", Diagnose(err)).
func (d Diagnostic) MarshalZerologObject(e *zerolog.Event) {
	e.Str("class", d.Class).Interface("causes", d.Causes)
	if d.Truncated {
		e.Bool("truncated", true)
	}
}

// AddPrivateError retains the original cause for errors.Is/As, but logs and
// records only Diagnose's message-free snapshot. nil is a no-op. Unlike
// AddError, it never calls err.Error(). Other builder fields (AddAttrs,
// AddMessage, Upstream, Log) remain explicit independent disclosure choices.
func (e *Error) AddPrivateError(err error, classifiers ...Classifier) *Error {
	if nilError(err) {
		return e
	}
	e.causes = append(e.causes, err)
	return e.AddAttrs(Any("error", Diagnose(err, classifiers...)))
}
