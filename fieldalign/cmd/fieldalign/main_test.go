package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeModule lays down a throwaway module and returns its dir.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files["go.mod"] = "module keyifytest\n\ngo 1.25\n"
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestKeyifyUnblocksTableTest(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a_test.go": `package keyifytest

import "testing"

func TestTable(t *testing.T) {
	tests := []struct {
		name string
		big  bool
		in   string
		want string
	}{
		{"lower", true, "A", "a"},
		{"upper", false, "b", "B"},
	}
	for _, tt := range tests {
		_ = tt
	}
}
`,
	})
	t.Chdir(dir)
	t.Setenv("GOWORK", "off")

	r := &runner{fix: true, keyify: true, generated: true, tests: true, maxPasses: 5}
	r.files = make(map[string]bool)
	if err := r.run([]string{"./..."}); err != nil {
		t.Fatalf("run: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(dir, "a_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	// Rows keyified (element order untouched — keyed elements bind by name).
	for _, want := range []string{
		`{name: "lower", big: true, in: "A", want: "a"}`,
		`{name: "upper", big: false, in: "b", want: "B"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing keyified row %q:\n%s", want, got)
		}
	}
	// Struct reordered: strings first, bool last.
	iName := strings.Index(got, "name string")
	iBig := strings.Index(got, "big")
	iWant := strings.Index(got, "want string")
	if iName == -1 || iBig == -1 || iWant == -1 || !(iName < iWant && iWant < iBig) {
		t.Errorf("struct fields not reordered (want name < want < big):\n%s", got)
	}

	// Idempotent: a second run rewrites nothing.
	r2 := &runner{fix: true, keyify: true, generated: true, tests: true, maxPasses: 5}
	r2.files = make(map[string]bool)
	if err := r2.run([]string{"./..."}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(r2.files) != 0 {
		t.Errorf("second run rewrote files: %v", r2.files)
	}
}

func TestKeyifyOffLeavesTableTestAlone(t *testing.T) {
	src := `package keyifytest

var tests = []struct {
	a bool
	b string
	c bool
}{
	{true, "x", false},
}
`
	dir := writeModule(t, map[string]string{"a.go": src})
	t.Chdir(dir)
	t.Setenv("GOWORK", "off")

	r := &runner{fix: true, generated: true, tests: true, maxPasses: 5}
	r.files = make(map[string]bool)
	if err := r.run([]string{"./..."}); err != nil {
		t.Fatalf("run: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != src {
		t.Errorf("file changed without -keyify:\n%s", out)
	}
	if r.unfixed != 1 {
		t.Errorf("unfixed = %d, want 1 (blocked by unkeyed literal)", r.unfixed)
	}
}

func TestKeyifySkipsBlankFieldStructs(t *testing.T) {
	src := `package keyifytest

var v = struct {
	a bool
	_ [6]byte
	b string
	c bool
}{true, [6]byte{}, "x", false}
`
	dir := writeModule(t, map[string]string{"a.go": src})
	t.Chdir(dir)
	t.Setenv("GOWORK", "off")

	r := &runner{fix: true, keyify: true, generated: true, tests: true, maxPasses: 5}
	r.files = make(map[string]bool)
	if err := r.run([]string{"./..."}); err != nil {
		t.Fatalf("run: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != src {
		t.Errorf("blank-field struct literal was rewritten:\n%s", out)
	}
}
