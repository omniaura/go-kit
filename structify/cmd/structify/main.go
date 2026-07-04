// Command structify rewrites functions with too many parameters to take a
// generated params struct, updating every caller across the loaded
// packages. See the package documentation for what is rewritten and what
// is conservatively skipped.
//
// Usage:
//
//	structify [flags] [packages]
//
//	-fix            rewrite files in place (default: report only)
//	-max-params N   flag functions with more than N inputs (default 4)
//	-generated      also structify functions declared in generated files
//	-tests          load test files so their call sites are rewritten (default true)
//	-v              list skipped functions with reasons
//
// Exit status: report mode exits 1 when candidates exist; -fix exits 0
// unless rewriting failed (skips are advisory, listed on stderr with -v).
package main

import (
	"flag"
	"fmt"
	"go/format"
	"os"

	"github.com/omniaura/go-kit/structify"
	"golang.org/x/tools/go/packages"
)

func main() {
	var (
		fix       = flag.Bool("fix", false, "rewrite files in place")
		maxParams = flag.Int("max-params", 4, "flag functions with more than this many input parameters")
		generated = flag.Bool("generated", false, "also structify functions declared in generated files")
		tests     = flag.Bool("tests", true, "load test files so their call sites are rewritten")
		verbose   = flag.Bool("v", false, "list skipped functions with reasons")
	)
	flag.Parse()
	patterns := flag.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo,
		Tests: *tests,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		fatal(err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		fatal(fmt.Errorf("packages contain errors"))
	}

	res, err := structify.Plan(pkgs, structify.Config{MaxParams: *maxParams, Generated: *generated})
	if err != nil {
		fatal(err)
	}

	if !*fix {
		for _, t := range res.Rewritten {
			fmt.Printf("%s: %s has %d input parameters; structify would generate %s and rewrite %d call sites\n",
				t.Pos, t.Name, t.NumParams, t.StructName, t.NumCallers)
		}
		for _, t := range res.Skipped {
			fmt.Printf("%s: %s has %d input parameters; cannot auto-fix: %s\n", t.Pos, t.Name, t.NumParams, t.SkipReason)
		}
		if len(res.Rewritten)+len(res.Skipped) > 0 {
			os.Exit(1)
		}
		return
	}

	for filename, edits := range res.Edits {
		src, err := os.ReadFile(filename)
		if err != nil {
			fatal(err)
		}
		out := structify.Apply(src, edits)
		formatted, err := format.Source(out)
		if err != nil {
			fatal(fmt.Errorf("%s: formatting after rewrite: %w", filename, err))
		}
		if err := os.WriteFile(filename, formatted, 0o644); err != nil {
			fatal(err)
		}
	}

	fmt.Printf("structify: rewrote %d functions (%d files touched)\n", len(res.Rewritten), len(res.Edits))
	for _, t := range res.Rewritten {
		fmt.Printf("  %-50s -> %s (%d callers)\n", t.Name, t.StructName, t.NumCallers)
	}
	if len(res.Skipped) > 0 {
		fmt.Printf("structify: %d functions skipped (run with -v for reasons)\n", len(res.Skipped))
		if *verbose {
			for _, t := range res.Skipped {
				fmt.Fprintf(os.Stderr, "  skip %s: %s: %s\n", t.Pos, t.Name, t.SkipReason)
			}
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "structify:", err)
	os.Exit(2)
}
