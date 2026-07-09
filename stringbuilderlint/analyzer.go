package stringbuilderlint

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const analyzerName = "stringbuilderlint"

// Analyzer reports string concatenation with more than two parts.
var Analyzer = &analysis.Analyzer{
	Name: analyzerName,
	Doc:  "check for string concatenation with more than two parts that should use strings.Builder",
	URL:  "https://pkg.go.dev/github.com/omniaura/go-kit/stringbuilderlint",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if generated(file) {
			continue
		}

		parents := parentMap(file)
		ignores := newSuppressions(pass, file)
		ast.Inspect(file, func(node ast.Node) bool {
			expr, ok := node.(*ast.BinaryExpr)
			if !ok || expr.Op != token.ADD {
				return true
			}
			if !isString(pass.TypesInfo.Types[expr].Type) {
				return true
			}
			if isStringConcatParent(pass, parents, expr) {
				return true
			}
			if pass.TypesInfo.Types[expr].Value != nil && pass.TypesInfo.Types[expr].Value.Kind() == constant.String {
				return true
			}
			parts := countStringConcatParts(pass, expr)
			if parts <= 2 {
				return true
			}
			if ignores.ignored(expr.Pos()) {
				return true
			}

			pass.Reportf(expr.Pos(), "string concatenation with %d parts should use strings.Builder", parts)
			return true
		})
	}

	return nil, nil
}

func parentMap(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

func isStringConcatParent(pass *analysis.Pass, parents map[ast.Node]ast.Node, node ast.Node) bool {
	parent := parents[node]
	if paren, ok := parent.(*ast.ParenExpr); ok {
		parent = parents[paren]
	}
	expr, ok := parent.(*ast.BinaryExpr)
	return ok && expr.Op == token.ADD && isString(pass.TypesInfo.Types[expr].Type)
}

func countStringConcatParts(pass *analysis.Pass, expr ast.Expr) int {
	if paren, ok := expr.(*ast.ParenExpr); ok {
		return countStringConcatParts(pass, paren.X)
	}
	binary, ok := expr.(*ast.BinaryExpr)
	if !ok || binary.Op != token.ADD || !isString(pass.TypesInfo.Types[binary].Type) {
		return 1
	}
	return countStringConcatParts(pass, binary.X) + countStringConcatParts(pass, binary.Y)
}

func isString(t types.Type) bool {
	if t == nil {
		return false
	}
	basic, ok := t.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.String
}

func generated(file *ast.File) bool {
	return ast.IsGenerated(file)
}

type suppressions struct {
	pass *analysis.Pass
	line map[int]bool
	file bool
}

func newSuppressions(pass *analysis.Pass, file *ast.File) suppressions {
	s := suppressions{
		pass: pass,
		line: make(map[int]bool),
	}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			kind, ok := directiveApplies(comment.Text)
			if !ok {
				continue
			}
			switch kind {
			case "lint:ignore":
				s.line[pass.Fset.PositionFor(comment.End(), false).Line+1] = true
			case "lint:file-ignore":
				s.file = true
			}
		}
	}
	return s
}

func (s suppressions) ignored(pos token.Pos) bool {
	return s.file || s.line[s.pass.Fset.PositionFor(pos, false).Line]
}

func directiveApplies(raw string) (string, bool) {
	fields := strings.Fields(commentText(raw))
	if len(fields) < 3 {
		return "", false
	}
	kind := fields[0]
	if kind != "lint:ignore" && kind != "lint:file-ignore" {
		return "", false
	}
	for analyzer := range strings.SplitSeq(fields[1], ",") {
		analyzer = strings.TrimSpace(analyzer)
		if analyzer == analyzerName || analyzer == "all" || analyzer == "*" {
			return kind, true
		}
	}
	return "", false
}

func commentText(raw string) string {
	switch {
	case strings.HasPrefix(raw, "//"):
		return strings.TrimSpace(strings.TrimPrefix(raw, "//"))
	case strings.HasPrefix(raw, "/*") && strings.HasSuffix(raw, "*/"):
		raw = strings.TrimPrefix(raw, "/*")
		raw = strings.TrimSuffix(raw, "*/")
		return strings.TrimSpace(raw)
	default:
		return strings.TrimSpace(raw)
	}
}
