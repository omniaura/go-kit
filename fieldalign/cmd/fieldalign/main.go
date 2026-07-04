// Command fieldalign reports structs whose fields could be reordered to
// use less memory, and (with -fix) rewrites them in place, preserving
// comments, tags, and multi-name field groups.
//
// Unlike `fieldalignment -fix` (x/tools) or a multichecker's -fix flag,
// this command can also rewrite generated files — the go/analysis driver
// refuses to touch them — which makes it suitable to run at the end of a
// code-generation pipeline so sqlc/templ/protoc output ends up optimally
// packed too.
//
// Usage:
//
//	fieldalign [flags] [packages]
//
//	-fix            rewrite files in place (default: report only)
//	-generated      also rewrite generated files (default true)
//	-tests          include test files (default true)
//	-summary        print a memory-savings summary (default true with -fix)
//	-max-passes N   fixpoint iterations for nested structs (default 5)
//
// Exit status is 1 if any suboptimal struct was found (or, with -fix,
// left unfixable), 0 otherwise.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"sort"

	"github.com/omniaura/go-kit/fieldalign"
	"golang.org/x/tools/go/packages"
)

func main() {
	var (
		fix       = flag.Bool("fix", false, "rewrite files in place")
		generated = flag.Bool("generated", true, "also rewrite generated files")
		tests     = flag.Bool("tests", true, "include test files")
		summary   = flag.Bool("summary", true, "print a memory-savings summary after -fix")
		maxPasses = flag.Int("max-passes", 5, "fixpoint iterations for nested structs")
	)
	flag.Parse()
	patterns := flag.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	r := &runner{
		fix:       *fix,
		generated: *generated,
		tests:     *tests,
		maxPasses: *maxPasses,
	}
	if err := r.run(patterns); err != nil {
		fmt.Fprintln(os.Stderr, "fieldalign:", err)
		os.Exit(2)
	}
	if *fix && *summary {
		r.printSummary()
	}
	if r.found {
		if !*fix {
			os.Exit(1)
		}
		if r.unfixed > 0 {
			os.Exit(1)
		}
	}
}

type savings struct {
	pos       token.Position
	name      string
	oldSize   int64
	newSize   int64
	oldPtrs   int64
	newPtrs   int64
	skip      string
	generated bool
}

type runner struct {
	fix       bool
	generated bool
	tests     bool
	maxPasses int

	found   bool
	unfixed int
	fixed   []savings
	skipped []savings
	files   map[string]bool // files rewritten
}

func (r *runner) run(patterns []string) error {
	r.files = make(map[string]bool)
	for pass := 1; pass <= r.maxPasses; pass++ {
		changed, err := r.onePass(patterns, pass)
		if err != nil {
			return err
		}
		if !r.fix || !changed {
			return nil
		}
	}
	return nil
}

func (r *runner) onePass(patterns []string, pass int) (changed bool, err error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedTypesSizes,
		Tests: r.tests,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return false, err
	}
	if packages.PrintErrors(pkgs) > 0 {
		return false, fmt.Errorf("packages contain errors")
	}

	// Struct types constructed with unkeyed composite literals anywhere
	// in the loaded set must not be reordered.
	unkeyed := make(map[*types.Struct]bool)
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p.TypesInfo == nil {
			return
		}
		for st := range fieldalign.UnkeyedStructs(p.TypesInfo, p.Syntax) {
			unkeyed[st] = true
		}
	})

	seen := make(map[string]bool) // a file can appear in pkg and pkg.test
	for _, p := range pkgs {
		for i, file := range p.Syntax {
			filename := p.CompiledGoFiles[i]
			if seen[filename] {
				continue
			}
			seen[filename] = true
			isGen := ast.IsGenerated(file)
			if isGen && !r.generated {
				continue
			}
			src, err := os.ReadFile(filename)
			if err != nil {
				return changed, err
			}
			fixes := fieldalign.FileFixes(p.Fset, file, src, p.TypesInfo, p.TypesSizes, unkeyed)
			if len(fixes) == 0 {
				continue
			}
			r.found = true
			if !r.fix {
				for i := range fixes {
					pos := p.Fset.Position(fixes[i].Pos)
					fmt.Printf("%s: %s\n", pos, fixes[i].Message)
				}
				continue
			}
			didEdit, err := r.applyFixes(p.Fset, filename, src, fixes, pass, isGen)
			if err != nil {
				return changed, err
			}
			changed = changed || didEdit
		}
	}
	return changed, nil
}

func (r *runner) applyFixes(fset *token.FileSet, filename string, src []byte, fixes []fieldalign.Fix, pass int, isGen bool) (bool, error) {
	type edit struct {
		start, end int
		text       []byte
	}
	var edits []edit
	for i := range fixes {
		fix := &fixes[i]
		pos := fset.Position(fix.Pos)
		sv := savings{
			pos: pos, name: fix.Name,
			oldSize: fix.OldSize, newSize: fix.NewSize,
			oldPtrs: fix.OldPtrBytes, newPtrs: fix.NewPtrBytes,
			generated: isGen,
		}
		if !fix.Fixable() {
			// Nested-struct skips resolve on a later pass; only report
			// what is still unfixable on the final look.
			if pass == 1 && fix.SkipReason != "" {
				sv.skip = fix.SkipReason
				r.skipped = append(r.skipped, sv)
				if fix.SkipReason != "nested in another struct being rewritten; re-run to fix" {
					r.unfixed++
					fmt.Fprintf(os.Stderr, "fieldalign: %s: cannot fix %s: %s\n", pos, structLabel(fix.Name), fix.SkipReason)
				}
			}
			continue
		}
		r.fixed = append(r.fixed, sv)
		edits = append(edits, edit{
			start: fset.Position(fix.EditPos).Offset,
			end:   fset.Position(fix.EditEnd).Offset,
			text:  fix.NewText,
		})
	}
	if len(edits) == 0 {
		return false, nil
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := src
	for _, e := range edits {
		out = append(out[:e.start:e.start], append(e.text, out[e.end:]...)...)
	}
	formatted, err := format.Source(out)
	if err != nil {
		return false, fmt.Errorf("%s: formatting after rewrite: %w", filename, err)
	}
	if err := os.WriteFile(filename, formatted, 0o644); err != nil {
		return false, err
	}
	r.files[filename] = true
	return true, nil
}

func (r *runner) printSummary() {
	if len(r.fixed) == 0 {
		fmt.Println("fieldalign: all structs already optimally ordered")
		return
	}
	var savedBytes, savedPtrs int64
	var sized int
	for _, s := range r.fixed {
		savedBytes += s.oldSize - s.newSize
		savedPtrs += s.oldPtrs - s.newPtrs
		if s.oldSize != s.newSize {
			sized++
		}
	}
	fmt.Printf("fieldalign: reordered %d structs across %d files\n", len(r.fixed), len(r.files))
	fmt.Printf("  per-instance size savings: %d bytes across %d shrunk structs\n", savedBytes, sized)
	fmt.Printf("  GC pointer-bytes reduced:  %d bytes\n", savedPtrs)

	top := make([]savings, len(r.fixed))
	copy(top, r.fixed)
	sort.Slice(top, func(i, j int) bool {
		return top[i].oldSize-top[i].newSize > top[j].oldSize-top[j].newSize
	})
	n := min(len(top), 15)
	fmt.Println("  top savers (per instance):")
	for _, s := range top[:n] {
		if s.oldSize == s.newSize {
			break
		}
		fmt.Printf("    %-60s %s %d -> %d bytes (-%d)\n",
			fmt.Sprintf("%s:%d", s.pos.Filename, s.pos.Line), structLabel(s.name), s.oldSize, s.newSize, s.oldSize-s.newSize)
	}
	if r.unfixed > 0 {
		fmt.Printf("  %d structs left unfixed (see stderr; unkeyed literals or unusual layouts)\n", r.unfixed)
	}
}

func structLabel(name string) string {
	if name == "" {
		return "anonymous struct"
	}
	return "struct " + name
}
