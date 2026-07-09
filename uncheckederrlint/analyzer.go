package uncheckederrlint

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const analyzerName = "uncheckederrlint"

// Analyzer reports discarded errors from JSON decoders, errgroup.Wait, and common write paths.
var Analyzer = &analysis.Analyzer{
	Name: analyzerName,
	Doc:  "check for discarded errors that should be handled or logged",
	URL:  "https://pkg.go.dev/github.com/omniaura/go-kit/uncheckederrlint",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if generated(file) {
			continue
		}

		ignores := newSuppressions(pass, file)
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.AssignStmt:
				checkAssignment(pass, ignores, node)
			case *ast.ExprStmt:
				if call, ok := node.X.(*ast.CallExpr); ok {
					reportDiscardedCall(pass, ignores, call)
				}
			}
			return true
		})
	}

	return nil, nil
}

func checkAssignment(pass *analysis.Pass, ignores suppressions, stmt *ast.AssignStmt) {
	if len(stmt.Rhs) != 1 {
		return
	}
	call, ok := stmt.Rhs[0].(*ast.CallExpr)
	if !ok || !hasBlankErrorResult(pass, stmt.Lhs, call) {
		return
	}
	reportDiscardedCall(pass, ignores, call)
}

func reportDiscardedCall(pass *analysis.Pass, ignores suppressions, call *ast.CallExpr) {
	message, ok := uncheckedMessage(pass, call)
	if !ok || ignores.ignored(call.Pos()) {
		return
	}
	pass.Report(analysis.Diagnostic{
		Pos:     call.Pos(),
		Message: message,
	})
}

func hasBlankErrorResult(pass *analysis.Pass, lhs []ast.Expr, call *ast.CallExpr) bool {
	resultType := pass.TypesInfo.Types[call].Type
	if len(lhs) == 1 && isBlank(lhs[0]) && isErrorType(resultType) {
		return true
	}

	results := resultTuple(resultType)
	if results == nil {
		return false
	}

	if results.Len() == len(lhs) {
		for i, target := range lhs {
			if isBlank(target) && isErrorType(results.At(i).Type()) {
				return true
			}
		}
		return false
	}

	return len(lhs) == 1 && isBlank(lhs[0]) && results.Len() == 1 && isErrorType(results.At(0).Type())
}

func resultTuple(t types.Type) *types.Tuple {
	if sig, ok := t.(*types.Signature); ok {
		return sig.Results()
	}
	tuple, _ := t.(*types.Tuple)
	return tuple
}

func isBlank(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "_"
}

func uncheckedMessage(pass *analysis.Pass, call *ast.CallExpr) (string, bool) {
	switch {
	case isJSONCodecCall(pass, call):
		return "discarded JSON encode/decode error should be handled or logged", true
	case isErrgroupWaitCall(pass, call):
		return "discarded errgroup.Wait error should be handled or use sync.WaitGroup when goroutines cannot fail", true
	case isWritePathCall(pass, call):
		return "discarded write-path error should be handled or logged", true
	default:
		return "", false
	}
}

func isJSONCodecCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if objectPackagePath(pass.TypesInfo.Uses[selector.Sel]) == "encoding/json" {
		switch selector.Sel.Name {
		case "Marshal", "MarshalIndent", "Unmarshal":
			return true
		}
	}
	switch selector.Sel.Name {
	case "Decode", "Encode":
		return objectPackagePath(selectedObject(pass, selector)) == "encoding/json"
	default:
		return false
	}
}

func isErrgroupWaitCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Wait" {
		return false
	}
	return namedPackagePath(deref(pass.TypesInfo.Types[selector.X].Type)) == "golang.org/x/sync/errgroup"
}

func isWritePathCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !hasErrorResult(pass.TypesInfo.Types[call].Type) {
		return false
	}
	name := selector.Sel.Name
	for _, prefix := range []string{"Claim", "Create", "Delete", "Grant", "Insert", "Revert", "Save", "Trigger", "Update", "Upsert"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func selectedObject(pass *analysis.Pass, selector *ast.SelectorExpr) types.Object {
	if selection := pass.TypesInfo.Selections[selector]; selection != nil {
		return selection.Obj()
	}
	return pass.TypesInfo.Uses[selector.Sel]
}

func objectPackagePath(obj types.Object) string {
	if obj == nil || obj.Pkg() == nil {
		return ""
	}
	return obj.Pkg().Path()
}

func namedPackagePath(t types.Type) string {
	named, ok := t.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return ""
	}
	return named.Obj().Pkg().Path()
}

func deref(t types.Type) types.Type {
	if ptr, ok := t.(*types.Pointer); ok {
		return ptr.Elem()
	}
	return t
}

func hasErrorResult(t types.Type) bool {
	if isErrorType(t) {
		return true
	}
	results := resultTuple(t)
	if results == nil {
		return false
	}
	for i := 0; i < results.Len(); i++ {
		if isErrorType(results.At(i).Type()) {
			return true
		}
	}
	return false
}

func isErrorType(t types.Type) bool {
	named, ok := t.(*types.Named)
	return ok && named.Obj() != nil && named.Obj().Name() == "error" && named.Obj().Pkg() == nil
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
