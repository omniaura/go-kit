package structify_test

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/omniaura/go-kit/structify"
	"golang.org/x/tools/go/packages"
)

// loadModule writes files into a temp module and loads it.
func loadModule(t *testing.T, files map[string]string) (string, []*packages.Package) {
	t.Helper()
	dir := t.TempDir()
	files["go.mod"] = "module example.com/m\n\ngo 1.25\n"
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, load(t, dir)
}

func load(t *testing.T, dir string) []*packages.Package {
	t.Helper()
	cfg := &packages.Config{
		Dir: dir,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo,
		Tests: true,
		Env:   append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off"),
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatal(err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatal("test module has errors before rewrite")
	}
	return pkgs
}

// plan is a convenience wrapper.
func plan(t *testing.T, pkgs []*packages.Package) *structify.Result {
	t.Helper()
	res, err := structify.Plan(pkgs, structify.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// applyAll writes the rewritten files back and returns their contents.
// It fails the test if the rewritten module no longer type-checks —
// the strongest guarantee a signature-changing refactor can offer.
func applyAll(t *testing.T, dir string, res *structify.Result) map[string]string {
	t.Helper()
	out := map[string]string{}
	for filename, edits := range res.Edits {
		src, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		formatted, err := format.Source(structify.Apply(src, edits))
		if err != nil {
			t.Fatalf("gofmt after rewrite of %s: %v", filename, err)
		}
		if err := os.WriteFile(filename, formatted, 0o644); err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(dir, filename)
		out[rel] = string(formatted)
	}
	// Reload: must type-check cleanly and be a fixpoint.
	pkgs := load(t, dir)
	again := plan(t, pkgs)
	if len(again.Rewritten) != 0 {
		t.Fatalf("not idempotent: second pass wants to rewrite %s", again.Rewritten[0].Name)
	}
	return out
}

func mustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("output missing %q:\n%s", want, got)
	}
}

func TestFuncWithCtxAndCallers(t *testing.T) {
	dir, pkgs := loadModule(t, map[string]string{
		"a/a.go": `package a

import "context"

// CreateUser makes a user.
func CreateUser(ctx context.Context, name, email string, age int, admin bool) string {
	_ = ctx
	if admin {
		return name + email
	}
	return name
}

func caller(ctx context.Context) string {
	return CreateUser(ctx, "n", "e", 41+1, true)
}
`,
		"b/b.go": `package b

import (
	"context"

	"example.com/m/a"
)

func Use() string {
	return a.CreateUser(context.Background(), "x", "y", 7, false)
}
`,
	})
	res := plan(t, pkgs)
	if len(res.Rewritten) != 1 || len(res.Skipped) != 0 {
		t.Fatalf("rewritten=%d skipped=%d, want 1/0 (%+v)", len(res.Rewritten), len(res.Skipped), res.Skipped)
	}
	tgt := res.Rewritten[0]
	if tgt.StructName != "CreateUserParams" || tgt.NumCallers != 2 {
		t.Fatalf("target = %+v", tgt)
	}
	out := applyAll(t, dir, res)

	a := out["a/a.go"]
	mustContain(t, a, "// CreateUserParams bundles the parameters of CreateUser.")
	mustContain(t, a, "type CreateUserParams struct {")
	mustContain(t, a, "Name  string")
	mustContain(t, a, "Admin bool")
	mustContain(t, a, "func CreateUser(ctx context.Context, arg CreateUserParams) string {")
	mustContain(t, a, "return arg.Name + arg.Email")
	mustContain(t, a, `CreateUser(ctx, CreateUserParams{Name: "n", Email: "e", Age: 41 + 1, Admin: true})`)

	b := out["b/b.go"]
	mustContain(t, b, `a.CreateUser(context.Background(), a.CreateUserParams{Name: "x", Email: "y", Age: 7, Admin: false})`)
}

func TestMethodCrossPackageCaller(t *testing.T) {
	dir, pkgs := loadModule(t, map[string]string{
		"svc/svc.go": `package svc

type Client struct{}

func (c *Client) Upload(bucket, key, contentType string, size int64, public bool) error {
	_ = bucket + key + contentType
	_, _ = size, public
	return nil
}
`,
		"use/use.go": `package use

import "example.com/m/svc"

func Do(c *svc.Client) error {
	return c.Upload("b", "k", "text/plain", 42, true)
}
`,
	})
	res := plan(t, pkgs)
	if len(res.Rewritten) != 1 {
		t.Fatalf("rewritten=%d skipped=%+v", len(res.Rewritten), res.Skipped)
	}
	out := applyAll(t, dir, res)
	mustContain(t, out["svc/svc.go"], "func (c *Client) Upload(arg UploadParams) error {")
	mustContain(t, out["use/use.go"], `c.Upload(svc.UploadParams{Bucket: "b", Key: "k", ContentType: "text/plain", Size: 42, Public: true})`)
}

func TestTestFileCallersRewritten(t *testing.T) {
	dir, pkgs := loadModule(t, map[string]string{
		"a/a.go": `package a

func Sum(a1, a2, a3, a4, a5 int) int { return a1 + a2 + a3 + a4 + a5 }
`,
		"a/a_test.go": `package a

import "testing"

func TestSum(t *testing.T) {
	if Sum(1, 2, 3, 4, 5) != 15 {
		t.Fatal("nope")
	}
}
`,
	})
	res := plan(t, pkgs)
	if len(res.Rewritten) != 1 || res.Rewritten[0].NumCallers != 1 {
		t.Fatalf("res=%+v skipped=%+v", res.Rewritten, res.Skipped)
	}
	out := applyAll(t, dir, res)
	mustContain(t, out["a/a_test.go"], "Sum(SumParams{A1: 1, A2: 2, A3: 3, A4: 4, A5: 5})")
}

func TestNestedStructifiedCalls(t *testing.T) {
	// outer's params are used as arguments to inner — the ident rewrites
	// happen inside spans that the caller rewrite wraps with insertions.
	dir, pkgs := loadModule(t, map[string]string{
		"a/a.go": `package a

func inner(p, q, r, s, u int) int { return p + q + r + s + u }

func outer(v, w, x, y, z int) int {
	return inner(v, w, x, y, z)
}

func kick() int { return outer(1, 2, 3, 4, 5) }
`,
	})
	res := plan(t, pkgs)
	if len(res.Rewritten) != 2 {
		t.Fatalf("rewritten=%d skipped=%+v", len(res.Rewritten), res.Skipped)
	}
	out := applyAll(t, dir, res)
	mustContain(t, out["a/a.go"], "return inner(innerParams{P: arg.V, Q: arg.W, R: arg.X, S: arg.Y, U: arg.Z})")
	mustContain(t, out["a/a.go"], "outer(outerParams{V: 1, W: 2, X: 3, Y: 4, Z: 5})")
}

func TestBodyShadowingPreserved(t *testing.T) {
	dir, pkgs := loadModule(t, map[string]string{
		"a/a.go": `package a

func F(name, email, city, zip, state string) string {
	out := name
	{
		name := "shadow"
		out += name
	}
	return out + email + city + zip + state
}

var _ = F("a", "b", "c", "d", "e")
`,
	})
	res := plan(t, pkgs)
	if len(res.Rewritten) != 1 {
		t.Fatalf("skipped=%+v", res.Skipped)
	}
	out := applyAll(t, dir, res)
	mustContain(t, out["a/a.go"], `name := "shadow"`)
	mustContain(t, out["a/a.go"], "out += name") // shadowed use untouched
	mustContain(t, out["a/a.go"], "out := arg.Name")
}

func TestInitialisms(t *testing.T) {
	dir, pkgs := loadModule(t, map[string]string{
		"a/a.go": `package a

func Fetch(userID, url, id, apiKey, dbName string) string {
	return userID + url + id + apiKey + dbName
}

var _ = Fetch("u", "l", "i", "k", "d")
`,
	})
	res := plan(t, pkgs)
	if len(res.Rewritten) != 1 {
		t.Fatalf("skipped=%+v", res.Skipped)
	}
	out := applyAll(t, dir, res)
	for _, want := range []string{"UserID", "URL", "ID", "ApiKey", "DbName"} {
		if !regexp.MustCompile(`\t` + want + `\s+string`).MatchString(out["a/a.go"]) {
			t.Errorf("missing field %s:\n%s", want, out["a/a.go"])
		}
	}
}

func TestStructNameCollisionUsesRecvPrefix(t *testing.T) {
	dir, pkgs := loadModule(t, map[string]string{
		"a/a.go": `package a

// UploadParams is already taken.
type UploadParams struct{ X int }

type Store struct{}

func (s *Store) Upload(bucket, key, kind string, size int64, public bool) error {
	_ = bucket + key + kind
	_, _ = size, public
	return nil
}

var _ = (&Store{}).Upload("b", "k", "t", 1, true)
`,
	})
	res := plan(t, pkgs)
	if len(res.Rewritten) != 1 {
		t.Fatalf("skipped=%+v", res.Skipped)
	}
	if got := res.Rewritten[0].StructName; got != "StoreUploadParams" {
		t.Fatalf("StructName = %q, want StoreUploadParams", got)
	}
	applyAll(t, dir, res)
}

func TestSkips(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		reason string
	}{
		{
			name: "variadic",
			src: `package a
func F(a1, a2, a3, a4 int, rest ...int) int { return a1 + a2 + a3 + a4 + len(rest) }
var _ = F(1, 2, 3, 4, 5)
`,
			reason: "variadic",
		},
		{
			name: "func value",
			src: `package a
func F(a1, a2, a3, a4, a5 int) int { return a1 }
var G = F
`,
			reason: "function value",
		},
		{
			name: "interface method",
			src: `package a
type Uploader interface {
	Upload(bucket, key, kind string, size int64, public bool) error
}
type S struct{}
func (S) Upload(bucket, key, kind string, size int64, public bool) error { return nil }
var _ Uploader = S{}
`,
			reason: "interface",
		},
		{
			name: "multi-value call",
			src: `package a
func five() (int, int, int, int, int) { return 1, 2, 3, 4, 5 }
func F(a1, a2, a3, a4, a5 int) int { return a1 }
var _ = F(five())
`,
			reason: "multi-value",
		},
		{
			name: "blank param",
			src: `package a
func F(a1, a2, a3, a4 int, _ string) int { return a1 }
var _ = F(1, 2, 3, 4, "x")
`,
			reason: "blank",
		},
		{
			name: "generic",
			src: `package a
func F[T any](a1, a2, a3, a4, a5 T) T { return a1 }
var _ = F(1, 2, 3, 4, 5)
`,
			reason: "generic",
		},
		{
			name: "param reused on := left side",
			src: `package a
func F(cfg, b, c, d, e string) (string, error) {
	cfg, err := cfg+"x", error(nil)
	return cfg + b + c + d + e, err
}
var _, _ = F("a", "b", "c", "d", "e")
`,
			reason: "reassigned with :=",
		},
		{
			name: "compiler directive",
			src: `package a
//go:noinline
func F(a1, a2, a3, a4, a5 int) int { return a1 }
var _ = F(1, 2, 3, 4, 5)
`,
			reason: "directive",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, pkgs := loadModule(t, map[string]string{"a/a.go": tc.src})
			res := plan(t, pkgs)
			if len(res.Rewritten) != 0 {
				t.Fatalf("expected skip, got rewrite %+v", res.Rewritten[0])
			}
			if len(res.Skipped) != 1 {
				t.Fatalf("skipped=%d, want 1", len(res.Skipped))
			}
			if !strings.Contains(res.Skipped[0].SkipReason, tc.reason) {
				t.Fatalf("reason %q does not mention %q", res.Skipped[0].SkipReason, tc.reason)
			}
			if len(res.Edits) != 0 {
				t.Fatalf("skip must produce no edits, got %d files", len(res.Edits))
			}
		})
	}
}

func TestSkipCallerInGeneratedFile(t *testing.T) {
	_, pkgs := loadModule(t, map[string]string{
		"a/a.go": `package a

func F(a1, a2, a3, a4, a5 int) int { return a1 }
`,
		"a/gen.go": `// Code generated by testgen. DO NOT EDIT.

package a

var _ = F(1, 2, 3, 4, 5)
`,
	})
	res := plan(t, pkgs)
	if len(res.Rewritten) != 0 || len(res.Skipped) != 1 {
		t.Fatalf("rewritten=%d skipped=%d", len(res.Rewritten), len(res.Skipped))
	}
	if !strings.Contains(res.Skipped[0].SkipReason, "generated") {
		t.Fatalf("reason = %q", res.Skipped[0].SkipReason)
	}
}

func TestSuppressionsBothSpellings(t *testing.T) {
	_, pkgs := loadModule(t, map[string]string{
		"a/a.go": `package a

//lint:ignore funcparamlint legacy spelling
func F(a1, a2, a3, a4, a5 int) int { return a1 }

// structify:ignore native spelling
func G(a1, a2, a3, a4, a5 int) int { return a1 }

var _ = F(1, 2, 3, 4, 5)
var _ = G(1, 2, 3, 4, 5)
`,
	})
	res := plan(t, pkgs)
	if len(res.Rewritten) != 0 || len(res.Skipped) != 0 {
		t.Fatalf("suppressed funcs were processed: rewritten=%d skipped=%d", len(res.Rewritten), len(res.Skipped))
	}
}

func TestArgNameAvoidsCollision(t *testing.T) {
	dir, pkgs := loadModule(t, map[string]string{
		"a/a.go": `package a

func F(name, email, city, zip, state string) string {
	arg := "local"
	return arg + name + email + city + zip + state
}

var _ = F("a", "b", "c", "d", "e")
`,
	})
	res := plan(t, pkgs)
	if len(res.Rewritten) != 1 {
		t.Fatalf("skipped=%+v", res.Skipped)
	}
	out := applyAll(t, dir, res)
	mustContain(t, out["a/a.go"], "func F(arg0 FParams) string {")
	mustContain(t, out["a/a.go"], `arg := "local"`)
	mustContain(t, out["a/a.go"], "return arg + arg0.Name")
}

func TestReportTargetsWithoutFix(t *testing.T) {
	_, pkgs := loadModule(t, map[string]string{
		"a/a.go": `package a

func F(a1, a2, a3, a4, a5 int) int { return a1 }
var _ = F(1, 2, 3, 4, 5)
`,
	})
	res := plan(t, pkgs)
	if len(res.Rewritten) != 1 {
		t.Fatal("expected one target")
	}
	tgt := res.Rewritten[0]
	if tgt.NumParams != 5 || tgt.NumCallers != 1 || tgt.StructName != "FParams" {
		t.Fatalf("target = %+v", tgt)
	}
	msg := fmt.Sprintf("%s: %d params -> %s", tgt.Name, tgt.NumParams, tgt.StructName)
	if msg == "" {
		t.Fatal("unreachable")
	}
}
