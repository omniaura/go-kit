package fieldalign_test

import (
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"sort"
	"strings"
	"testing"

	"github.com/omniaura/go-kit/fieldalign"
)

// check parses and type-checks src as a single file and returns the
// computed fixes plus everything needed to apply them.
type checked struct {
	fset  *token.FileSet
	file  *ast.File
	src   []byte
	fixes []fieldalign.Fix
}

func check(t *testing.T, src string) *checked {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "a.go", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	conf := types.Config{Importer: importer.Default(), Sizes: types.SizesFor("gc", "amd64")}
	if _, err := conf.Check("p", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	unkeyed := fieldalign.UnkeyedStructs(info, []*ast.File{file})
	fixes := fieldalign.FileFixes(fset, file, []byte(src), info, conf.Sizes, unkeyed)
	return &checked{fset: fset, file: file, src: []byte(src), fixes: fixes}
}

// apply applies all fixable edits and returns the gofmt'd result.
func (c *checked) apply(t *testing.T) string {
	t.Helper()
	type edit struct {
		start, end int
		text       []byte
	}
	var edits []edit
	for i := range c.fixes {
		f := &c.fixes[i]
		if !f.Fixable() {
			continue
		}
		edits = append(edits, edit{
			start: c.fset.Position(f.EditPos).Offset,
			end:   c.fset.Position(f.EditEnd).Offset,
			text:  f.NewText,
		})
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := c.src
	for _, e := range edits {
		out = append(out[:e.start:e.start], append(e.text, out[e.end:]...)...)
	}
	formatted, err := format.Source(out)
	if err != nil {
		t.Fatalf("gofmt after apply: %v\n----\n%s", err, out)
	}
	return string(formatted)
}

func TestPreservesCommentsAndTags(t *testing.T) {
	src := `package p

// Config holds settings.
type Config struct {
	// Enabled toggles the feature.
	Enabled bool ` + "`json:\"enabled\"`" + `
	// Name is the display name.
	Name string ` + "`json:\"name\"`" + ` // trailing comment
	// Count is a counter.
	Count uint32 ` + "`json:\"count\"`" + `
}
`
	c := check(t, src)
	if len(c.fixes) != 1 {
		t.Fatalf("fixes = %d, want 1", len(c.fixes))
	}
	f := c.fixes[0]
	if !f.Fixable() {
		t.Fatalf("not fixable: %s", f.SkipReason)
	}
	if f.Name != "Config" {
		t.Errorf("Name = %q, want Config", f.Name)
	}
	if f.OldSize != 32 || f.NewSize != 24 {
		t.Errorf("sizes = %d -> %d, want 32 -> 24", f.OldSize, f.NewSize)
	}
	got := c.apply(t)
	want := `package p

// Config holds settings.
type Config struct {
	// Name is the display name.
	Name string ` + "`json:\"name\"`" + ` // trailing comment
	// Count is a counter.
	Count uint32 ` + "`json:\"count\"`" + `
	// Enabled toggles the feature.
	Enabled bool ` + "`json:\"enabled\"`" + `
}
`
	if got != want {
		t.Errorf("apply mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	// Idempotent: re-checking the output finds nothing.
	if again := check(t, got); len(again.fixes) != 0 {
		t.Errorf("second run found %d fixes, want 0: %v", len(again.fixes), again.fixes[0].Message)
	}
}

func TestMultiNameGroupStaysTogether(t *testing.T) {
	src := `package p

type T struct {
	a    bool
	X, Y string
	b    bool
}
`
	c := check(t, src)
	if len(c.fixes) != 1 {
		t.Fatalf("fixes = %d, want 1", len(c.fixes))
	}
	got := c.apply(t)
	if !strings.Contains(got, "X, Y string") {
		t.Errorf("multi-name group was split:\n%s", got)
	}
	if again := check(t, got); len(again.fixes) != 0 {
		t.Errorf("second run found fixes: %v", again.fixes[0].Message)
	}
}

func TestUnkeyedLiteralBlocksFix(t *testing.T) {
	src := `package p

type T struct {
	A bool
	B string
	C bool
}

var v = T{true, "x", false}
`
	c := check(t, src)
	if len(c.fixes) != 1 {
		t.Fatalf("fixes = %d, want 1", len(c.fixes))
	}
	f := c.fixes[0]
	if f.Fixable() {
		t.Fatal("fix offered despite unkeyed composite literal")
	}
	if !strings.Contains(f.SkipReason, "unkeyed") {
		t.Errorf("SkipReason = %q", f.SkipReason)
	}
}

func TestIgnoreDirective(t *testing.T) {
	src := `package p

// fieldalign:ignore matches on-disk layout
type T struct {
	A bool
	B string
	C bool
}
`
	c := check(t, src)
	if len(c.fixes) != 0 {
		t.Fatalf("fixes = %d, want 0 (ignored)", len(c.fixes))
	}
}

func TestSingleLineStruct(t *testing.T) {
	src := `package p

var x struct {
	A bool
	B string
	C bool
}

type m map[string]struct{ A bool; B string; C bool }
`
	c := check(t, src)
	if len(c.fixes) != 2 {
		t.Fatalf("fixes = %d, want 2", len(c.fixes))
	}
	got := c.apply(t)
	if again := check(t, got); len(again.fixes) != 0 {
		t.Errorf("second run found fixes: %v", again.fixes[0].Message)
	}
}

func TestNestedStructsConverge(t *testing.T) {
	src := `package p

type Outer struct {
	A bool
	Inner struct {
		X bool
		Y string
		Z bool
	}
	B string
	C bool
}
`
	// First pass fixes the outer struct only; the inner fix is dropped
	// as nested. A second pass over the output fixes the inner struct.
	c := check(t, src)
	fixable := 0
	for i := range c.fixes {
		if c.fixes[i].Fixable() {
			fixable++
		}
	}
	if fixable != 1 {
		t.Fatalf("fixable on pass 1 = %d, want 1 (outermost only)", fixable)
	}
	out1 := c.apply(t)
	c2 := check(t, out1)
	out2 := c2.apply(t)
	if again := check(t, out2); len(again.fixes) != 0 {
		t.Errorf("did not converge after 2 passes: %v", again.fixes[0].Message)
	}
}

func TestEmbeddedAndBlankFields(t *testing.T) {
	src := `package p

import "sync"

type T struct {
	a bool
	sync.Mutex
	_ [3]byte
	b string
}
`
	c := check(t, src)
	for i := range c.fixes {
		if !c.fixes[i].Fixable() {
			t.Fatalf("not fixable: %s", c.fixes[i].SkipReason)
		}
	}
	got := c.apply(t)
	if !strings.Contains(got, "sync.Mutex") {
		t.Errorf("embedded field lost:\n%s", got)
	}
	if again := check(t, got); len(again.fixes) != 0 {
		t.Errorf("second run found fixes: %v", again.fixes[0].Message)
	}
}

func TestBlankSeparatorLinesTravel(t *testing.T) {
	src := `package p

type T struct {
	// group one
	A bool
	B bool

	// group two
	S string
	P *int
}
`
	c := check(t, src)
	if len(c.fixes) != 1 {
		t.Fatalf("fixes = %d, want 1", len(c.fixes))
	}
	got := c.apply(t)
	// The pointerful fields move up (P first: zero trailing non-pointer
	// bytes), section comments travel with the field below them, and the
	// body starts cleanly after the brace.
	want := `package p

type T struct {
	P *int

	// group two
	S string
	// group one
	A bool
	B bool
}
`
	if got != want {
		t.Errorf("apply mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestMessageParityWithUpstream(t *testing.T) {
	src := `package p

type T struct {
	A bool
	B string
	C bool
}
`
	c := check(t, src)
	if len(c.fixes) != 1 {
		t.Fatalf("fixes = %d, want 1", len(c.fixes))
	}
	if got, want := c.fixes[0].Message, "struct of size 32 could be 24"; got != want {
		t.Errorf("Message = %q, want %q", got, want)
	}

	// Pointer-bytes-only diagnostic.
	src2 := `package p

type U struct {
	S string
	N uint64
	P *int
}
`
	c2 := check(t, src2)
	if len(c2.fixes) != 1 {
		t.Fatalf("fixes = %d, want 1", len(c2.fixes))
	}
	if !strings.Contains(c2.fixes[0].Message, "pointer bytes") {
		t.Errorf("Message = %q, want pointer bytes diagnostic", c2.fixes[0].Message)
	}
}
