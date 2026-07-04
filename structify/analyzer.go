package structify

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports functions whose inputs should be grouped into a params
// struct: more than five substantive input parameters. A leading
// context.Context is boilerplate, not an input, and is not counted.
//
// The analyzer only diagnoses; rewriting requires whole-program knowledge
// of every caller, which is cmd/structify's job.
var Analyzer = &analysis.Analyzer{
	Name: analyzerName,
	Doc: "check for functions whose inputs should be grouped into a struct; " +
		"fix mechanically with the structify command; " +
		"suppress with //lint:ignore structify <reason>",
	URL: "https://pkg.go.dev/github.com/omniaura/go-kit/structify",
	Run: runAnalyzer,
}

const analyzerMaxParams = 5

func runAnalyzer(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		tf := pass.Fset.File(file.FileStart)
		if tf == nil || ast.IsGenerated(file) || strings.HasSuffix(tf.Name(), "_test.go") {
			continue
		}
		sup := parseSuppressions(pass.Fset, file)
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.FuncDecl:
				if n.Body == nil {
					return true
				}
				count := substantiveParamCount(pass.TypesInfo, n.Type.Params)
				if count <= analyzerMaxParams || sup.ignored(pass.Fset, n.Name.Pos()) {
					return true
				}
				kind, suggestion := "function", "move inputs into an input struct and make the behavior a method"
				if n.Recv != nil {
					kind, suggestion = "method", "move inputs into a request/options struct"
				}
				pass.Reportf(n.Name.Pos(), "%s %s has %d non-context input parameters; %s", kind, n.Name.Name, count, suggestion)
			case *ast.FuncLit:
				count := substantiveParamCount(pass.TypesInfo, n.Type.Params)
				if count <= analyzerMaxParams || sup.ignored(pass.Fset, n.Type.Func) {
					return true
				}
				pass.Reportf(n.Type.Func, "function literal has %d non-context input parameters; move inputs into an input struct before wiring this behavior", count)
			}
			return true
		})
	}
	return nil, nil
}
