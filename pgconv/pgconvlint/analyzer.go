// Package pgconvlint reports manual pgtype conversions that should use pgconv.
package pgconvlint

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/omniaura/go-kit/pgconv/internal/lintignore"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/astutil"
)

const (
	analyzerName = "pgconvlint"
	pgtypePath   = "github.com/jackc/pgx/v5/pgtype"
	pgencodePath = "github.com/omniaura/go-kit/pgconv/pgencode"
	pgdecodePath = "github.com/omniaura/go-kit/pgconv/pgdecode"
)

// Analyzer checks for manual pgtype <-> regular Go value conversions.
var Analyzer = &analysis.Analyzer{
	Name: analyzerName,
	Doc:  "check for manual pgtype conversions that should use pgencode or pgdecode; suppress with //lint:ignore pgconvlint <reason>",
	URL:  "https://pkg.go.dev/github.com/omniaura/go-kit/pgconv/pgconvlint",
	Run:  run,
}

type reporter struct {
	pass    *analysis.Pass
	file    *ast.File
	ignores lintignore.Suppressions
}

type pgtypeRule struct {
	valueFields  []string
	encodeFunc   string
	encodeMethod string
	decodeFunc   string
	// zeroIsNull is true when the encode builder exposes a ZeroIsNull() method,
	// i.e. a `Valid: v != 0` composite literal can be rewritten as
	// pgencode.Fn(v).ZeroIsNull().Method(). Applies to the integer and float
	// builders.
	zeroIsNull bool
}

type pgconvUse struct {
	kind      string
	qualifier string
}

type tinyWrapper struct {
	name      string
	kind      string
	qualifier string
	params    []string
	expr      ast.Expr
}

type encodeReplacement struct {
	text    string
	display string
}

var pgtypeRules = map[string]pgtypeRule{
	"Text": {
		valueFields:  []string{"String"},
		encodeFunc:   "String",
		encodeMethod: "Text",
		decodeFunc:   "Text",
	},
	"Bool": {
		valueFields:  []string{"Bool"},
		encodeFunc:   "Bool",
		encodeMethod: "Bool",
		decodeFunc:   "Bool",
	},
	"Int2": {
		valueFields:  []string{"Int16"},
		encodeFunc:   "Int16",
		encodeMethod: "Int2",
		decodeFunc:   "Int2",
		zeroIsNull:   true,
	},
	"Int4": {
		valueFields:  []string{"Int32"},
		encodeFunc:   "Int32",
		encodeMethod: "Int4",
		decodeFunc:   "Int4",
		zeroIsNull:   true,
	},
	"Int8": {
		valueFields:  []string{"Int64"},
		encodeFunc:   "Int64",
		encodeMethod: "Int8",
		decodeFunc:   "Int8",
		zeroIsNull:   true,
	},
	"Float8": {
		valueFields:  []string{"Float64"},
		encodeFunc:   "Float64",
		encodeMethod: "Float8",
		decodeFunc:   "Float8",
		zeroIsNull:   true,
	},
	"Date": {
		valueFields:  []string{"Time"},
		encodeFunc:   "Time",
		encodeMethod: "Date",
		decodeFunc:   "Date",
	},
	"Timestamp": {
		valueFields:  []string{"Time"},
		encodeFunc:   "Time",
		encodeMethod: "Timestamp",
		decodeFunc:   "Timestamp",
	},
	"Timestamptz": {
		valueFields:  []string{"Time"},
		encodeFunc:   "Time",
		encodeMethod: "Timestamptz",
		decodeFunc:   "Timestamptz",
	},
	"UUID": {
		valueFields:  []string{"Bytes"},
		encodeFunc:   "UUID",
		encodeMethod: "UUID",
		decodeFunc:   "UUID",
	},
}

func run(pass *analysis.Pass) (any, error) {
	if skipPackage(pass) {
		return nil, nil
	}

	wrappers := collectTinyWrappers(pass)

	for _, file := range pass.Files {
		if skipFile(pass, file) {
			continue
		}

		parents := parentMap(file)
		reporter := reporter{
			pass:    pass,
			file:    file,
			ignores: lintignore.New(pass.Fset, file),
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.FuncDecl:
				checkPgconvWrapper(pass, reporter, n)
			case *ast.CompositeLit:
				checkCompositeLiteral(pass, reporter, n)
			case *ast.SelectorExpr:
				checkSelector(pass, reporter, parents, n)
			case *ast.CallExpr:
				checkWrapperCall(pass, reporter, wrappers, n)
			}
			return true
		})
	}
	return nil, nil
}

func skipPackage(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}
	switch pass.Pkg.Path() {
	case pgencodePath, pgdecodePath:
		return true
	default:
		return false
	}
}

func collectTinyWrappers(pass *analysis.Pass) map[*types.Func]tinyWrapper {
	wrappers := make(map[*types.Func]tinyWrapper)
	for _, file := range pass.Files {
		if skipFile(pass, file) {
			continue
		}

		ignores := lintignore.New(pass.Fset, file)
		ast.Inspect(file, func(node ast.Node) bool {
			decl, ok := node.(*ast.FuncDecl)
			if !ok || decl.Name == nil || ignores.Ignored(decl.Name.Pos(), analyzerName) {
				return true
			}
			wrapper, ok := tinyWrapperInfo(pass, decl)
			if !ok || wrapper.expr == nil {
				return true
			}
			fn, ok := pass.TypesInfo.Defs[decl.Name].(*types.Func)
			if ok {
				wrappers[fn] = wrapper
			}
			return true
		})
	}
	return wrappers
}

func checkPgconvWrapper(pass *analysis.Pass, r reporter, decl *ast.FuncDecl) {
	wrapper, ok := tinyWrapperInfo(pass, decl)
	if !ok {
		return
	}

	r.report(decl.Name.Pos(), decl.Name.End(), fmt.Sprintf("tiny %s wrapper %s; call %s directly at the use site", wrapper.kind, decl.Name.Name, wrapper.kind), nil)
}

func tinyWrapperInfo(pass *analysis.Pass, decl *ast.FuncDecl) (tinyWrapper, bool) {
	if decl.Body == nil || decl.Name == nil || len(decl.Body.List) > 4 {
		return tinyWrapper{}, false
	}

	params := paramNameList(decl.Type.Params)
	paramSet := make(map[string]bool, len(params))
	for _, param := range params {
		paramSet[param] = true
	}
	if len(paramSet) == 0 {
		return tinyWrapper{}, false
	}

	wrapper := tinyWrapper{name: decl.Name.Name, params: params}
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		if wrapper.kind != "" {
			return false
		}
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		ret, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range ret.Results {
			if use, ok := containsPgconvParamCall(pass, result, paramSet); ok {
				wrapper.kind = use.kind
				wrapper.qualifier = use.qualifier
				return false
			}
		}
		return true
	})
	if wrapper.kind == "" {
		return tinyWrapper{}, false
	}

	if len(decl.Body.List) == 1 {
		if ret, ok := decl.Body.List[0].(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
			if use, ok := containsPgconvParamCall(pass, ret.Results[0], paramSet); ok {
				wrapper.kind = use.kind
				wrapper.qualifier = use.qualifier
				wrapper.expr = ret.Results[0]
			}
		}
	}

	return wrapper, true
}

func skipFile(pass *analysis.Pass, file *ast.File) bool {
	if ast.IsGenerated(file) {
		return true
	}

	tf := pass.Fset.File(file.Pos())
	if tf == nil {
		return false
	}
	filename := filepath.ToSlash(tf.Name())
	return strings.HasSuffix(filename, "_test.go")
}

func parentMap(file *ast.File) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
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

func checkCompositeLiteral(pass *analysis.Pass, r reporter, lit *ast.CompositeLit) {
	typeName, ok := pgtypeName(pass.TypesInfo.TypeOf(lit.Type))
	if !ok {
		return
	}

	rule, ok := pgtypeRules[typeName]
	if !ok {
		return
	}

	fields := compositeFields(lit)
	valueField := firstPresentField(fields, rule.valueFields)
	if valueField == "" {
		return
	}

	display := encodeSuggestion(pass, rule, fields[valueField], fields["Valid"])
	var fixes []analysis.SuggestedFix
	if replacement, ok := r.encodeReplacement(rule, typeName, valueField, fields); ok {
		fixes = []analysis.SuggestedFix{r.replaceWithPackage(lit.Pos(), lit.End(), replacement.text, "Replace with "+replacement.display, pgencodePath, "pgencode")}
	}

	r.report(lit.Lbrace, lit.End(), fmt.Sprintf("manual pgtype.%s encode; use %s", typeName, display), fixes)
}

func checkSelector(pass *analysis.Pass, r reporter, parents map[ast.Node]ast.Node, sel *ast.SelectorExpr) {
	typeName, ok := pgtypeName(pass.TypesInfo.TypeOf(sel.X))
	if !ok {
		return
	}

	rule, ok := pgtypeRules[typeName]
	if !ok || !slices.Contains(rule.valueFields, sel.Sel.Name) {
		return
	}

	if isAssignmentTarget(parents, sel) {
		var fixes []analysis.SuggestedFix
		if fix, ok := r.assignmentTargetFix(parents, rule, typeName, sel); ok {
			fixes = []analysis.SuggestedFix{fix}
		}
		r.report(sel.Sel.Pos(), sel.Sel.End(), fmt.Sprintf("manual pgtype.%s field assignment; build the value with pgencode instead of setting .%s", typeName, sel.Sel.Name), fixes)
		return
	}

	guarded := isGuardedDecode(pass, parents, sel)
	if !(guarded && isAssignmentValue(parents, sel)) && !(guarded && isTinyDecodeHelper(pass, parents, sel)) {
		return
	}

	var fixes []analysis.SuggestedFix
	if fix, ok := r.fillDecodeFix(parents, rule, typeName, sel); ok {
		fixes = []analysis.SuggestedFix{fix}
	} else if fix, ok := r.valueDecodeFix(rule, sel); ok {
		fixes = []analysis.SuggestedFix{fix}
	}

	r.report(sel.Sel.Pos(), sel.Sel.End(), fmt.Sprintf("manual pgtype.%s decode; use pgdecode.%s(...).Value/Ptr/Fill instead of .%s", typeName, rule.decodeFunc, sel.Sel.Name), fixes)
}

func checkWrapperCall(pass *analysis.Pass, r reporter, wrappers map[*types.Func]tinyWrapper, call *ast.CallExpr) {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return
	}

	fn, ok := pass.TypesInfo.Uses[ident].(*types.Func)
	if !ok {
		return
	}

	wrapper, ok := wrappers[fn]
	if !ok || wrapper.expr == nil || len(wrapper.params) != len(call.Args) {
		return
	}

	fix, ok := r.inlineWrapperFix(wrapper, call)
	if !ok {
		return
	}

	r.report(call.Pos(), call.End(), fmt.Sprintf("call to tiny %s wrapper %s; use %s directly", wrapper.kind, wrapper.name, wrapper.kind), []analysis.SuggestedFix{fix})
}

func (r reporter) report(pos, end token.Pos, message string, fixes []analysis.SuggestedFix) {
	if r.ignores.Ignored(pos, analyzerName) {
		return
	}
	r.pass.Report(analysis.Diagnostic{
		Pos:            pos,
		End:            end,
		Message:        message,
		SuggestedFixes: fixes,
	})
}

func compositeFields(lit *ast.CompositeLit) map[string]ast.Expr {
	fields := make(map[string]ast.Expr)
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		fields[key.Name] = kv.Value
	}
	return fields
}

func paramNameList(fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	var names []string
	for _, field := range fields.List {
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	return names
}

func containsPgconvParamCall(pass *analysis.Pass, expr ast.Expr, params map[string]bool) (pgconvUse, bool) {
	call, ok := unparen(expr).(*ast.CallExpr)
	if !ok {
		return pgconvUse{}, false
	}
	if use, ok := pgconvCallKind(pass, call); ok && callUsesParam(call, params) {
		return use, true
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return pgconvUse{}, false
	}
	return containsPgconvParamCall(pass, sel.X, params)
}

func pgconvCallKind(pass *analysis.Pass, call *ast.CallExpr) (pgconvUse, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return pgconvUse{}, false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil {
		return pgconvUse{}, false
	}
	qualifier, _ := selectorQualifier(sel)
	switch fn.Pkg().Path() {
	case pgencodePath:
		return pgconvUse{kind: "pgencode", qualifier: qualifier}, true
	case pgdecodePath:
		return pgconvUse{kind: "pgdecode", qualifier: qualifier}, true
	default:
		return pgconvUse{}, false
	}
}

func selectorQualifier(sel *ast.SelectorExpr) (string, bool) {
	ident, ok := unparen(sel.X).(*ast.Ident)
	if !ok {
		return "", false
	}
	return ident.Name, true
}

func callUsesParam(call *ast.CallExpr, params map[string]bool) bool {
	for _, arg := range call.Args {
		if params[rootIdent(arg)] {
			return true
		}
	}
	return false
}

func rootIdent(expr ast.Expr) string {
	switch e := unparen(expr).(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return rootIdent(e.X)
	case *ast.StarExpr:
		return rootIdent(e.X)
	default:
		return ""
	}
}

func firstPresentField(fields map[string]ast.Expr, names []string) string {
	for _, name := range names {
		if _, ok := fields[name]; ok {
			return name
		}
	}
	return ""
}

func encodeSuggestion(pass *analysis.Pass, rule pgtypeRule, value, valid ast.Expr) string {
	if value == nil {
		return fmt.Sprintf("pgencode.%s(...).%s()", rule.encodeFunc, rule.encodeMethod)
	}

	if ptr := dereferencedExpr(value); ptr != nil && isNotNilCheck(valid, ptr, pass.Fset) {
		return fmt.Sprintf("pgencode.%sPtr(...).%s()", rule.encodeFunc, rule.encodeMethod)
	}

	if rule.encodeFunc == "String" && isNonEmptyStringCheck(valid, value, pass.Fset) {
		return fmt.Sprintf("pgencode.%s(...).EmptyIsNull().%s()", rule.encodeFunc, rule.encodeMethod)
	}

	if rule.zeroIsNull && isNonZeroCheck(valid, value, pass.Fset) {
		return fmt.Sprintf("pgencode.%s(...).ZeroIsNull().%s()", rule.encodeFunc, rule.encodeMethod)
	}

	return fmt.Sprintf("pgencode.%s(...).%s()", rule.encodeFunc, rule.encodeMethod)
}

func (r reporter) encodeReplacement(rule pgtypeRule, typeName, valueField string, fields map[string]ast.Expr) (encodeReplacement, bool) {
	value := fields[valueField]
	valid := fields["Valid"]
	if value == nil || !safeCompositeFields(typeName, valueField, fields, r.pass.Fset) {
		return encodeReplacement{}, false
	}

	qualifier, ok := r.packageQualifier(pgencodePath, "pgencode")
	if !ok {
		return encodeReplacement{}, false
	}

	fn := rule.encodeFunc
	arg := renderNode(r.pass.Fset, value)
	chain := ""
	ptr := dereferencedExpr(value)
	switch {
	case isTrue(valid):
	case ptr != nil && isNotNilCheck(valid, ptr, r.pass.Fset):
		fn += "Ptr"
		arg = renderNode(r.pass.Fset, ptr)
	case rule.encodeFunc == "String" && isNonEmptyStringCheck(valid, value, r.pass.Fset):
		chain = ".EmptyIsNull()"
	case rule.zeroIsNull && isNonZeroCheck(valid, value, r.pass.Fset):
		chain = ".ZeroIsNull()"
	default:
		return encodeReplacement{}, false
	}

	text := fmt.Sprintf("%s.%s(%s)%s.%s()", qualifier, fn, arg, chain, rule.encodeMethod)
	display := fmt.Sprintf("pgencode.%s(%s)%s.%s()", fn, arg, chain, rule.encodeMethod)
	return encodeReplacement{text: text, display: display}, true
}

func safeCompositeFields(typeName, valueField string, fields map[string]ast.Expr, fset *token.FileSet) bool {
	for field, expr := range fields {
		switch field {
		case valueField, "Valid":
			continue
		case "InfinityModifier":
			if typeName == "Date" || typeName == "Timestamp" || typeName == "Timestamptz" {
				rendered := renderNode(fset, expr)
				if rendered == "pgtype.Finite" || rendered == "0" {
					continue
				}
			}
			return false
		default:
			return false
		}
	}
	return true
}

func (r reporter) assignmentTargetFix(parents map[ast.Node]ast.Node, rule pgtypeRule, typeName string, sel *ast.SelectorExpr) (analysis.SuggestedFix, bool) {
	assign, ok := parents[sel].(*ast.AssignStmt)
	if !ok || assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return analysis.SuggestedFix{}, false
	}

	block, index, ok := enclosingBlock(parents, assign)
	if !ok {
		return analysis.SuggestedFix{}, false
	}

	value := assign.Rhs[0]
	target := sel.X
	var validAssign *ast.AssignStmt
	for _, candidateIndex := range []int{index - 1, index + 1} {
		if candidateIndex < 0 || candidateIndex >= len(block.List) {
			continue
		}
		candidate, ok := block.List[candidateIndex].(*ast.AssignStmt)
		if !ok || candidate.Tok != token.ASSIGN || len(candidate.Lhs) != 1 || len(candidate.Rhs) != 1 {
			continue
		}
		if isValidFieldAssignment(candidate.Lhs[0], target, r.pass.Fset) {
			validAssign = candidate
			break
		}
	}
	if validAssign == nil {
		return analysis.SuggestedFix{}, false
	}

	fields := map[string]ast.Expr{
		rule.valueFields[0]: value,
		"Valid":             validAssign.Rhs[0],
	}
	replacement, ok := r.encodeReplacement(rule, typeName, rule.valueFields[0], fields)
	if !ok {
		return analysis.SuggestedFix{}, false
	}

	start, end := assign.Pos(), assign.End()
	if validAssign.Pos() < start {
		start = validAssign.Pos()
	}
	if validAssign.End() > end {
		end = validAssign.End()
	}

	text := fmt.Sprintf("%s = %s", renderNode(r.pass.Fset, target), replacement.text)
	return r.replaceWithPackage(start, end, text, "Replace field assignments with "+replacement.display, pgencodePath, "pgencode"), true
}

func isValidFieldAssignment(expr ast.Expr, target ast.Expr, fset *token.FileSet) bool {
	sel, ok := unparen(expr).(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Valid" && sameExpr(sel.X, target, fset)
}

func enclosingBlock(parents map[ast.Node]ast.Node, stmt ast.Stmt) (*ast.BlockStmt, int, bool) {
	block, ok := parents[stmt].(*ast.BlockStmt)
	if !ok {
		return nil, -1, false
	}
	for i, candidate := range block.List {
		if candidate == stmt {
			return block, i, true
		}
	}
	return nil, -1, false
}

func (r reporter) fillDecodeFix(parents map[ast.Node]ast.Node, rule pgtypeRule, typeName string, sel *ast.SelectorExpr) (analysis.SuggestedFix, bool) {
	if typeName == "UUID" {
		return analysis.SuggestedFix{}, false
	}

	ifStmt := enclosingIfForDecode(parents, sel)
	if ifStmt == nil || ifStmt.Else != nil || len(ifStmt.Body.List) != 1 {
		return analysis.SuggestedFix{}, false
	}

	assign, ok := ifStmt.Body.List[0].(*ast.AssignStmt)
	if !ok || assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 || !containsNode(assign.Rhs[0], sel.Pos()) {
		return analysis.SuggestedFix{}, false
	}
	if !simpleAddressable(assign.Lhs[0]) {
		return analysis.SuggestedFix{}, false
	}

	qualifier, ok := r.packageQualifier(pgdecodePath, "pgdecode")
	if !ok {
		return analysis.SuggestedFix{}, false
	}
	text := fmt.Sprintf("%s.%s(%s).Fill(&%s)", qualifier, rule.decodeFunc, renderNode(r.pass.Fset, sel.X), renderNode(r.pass.Fset, assign.Lhs[0]))
	return r.replaceWithPackage(ifStmt.Pos(), ifStmt.End(), text, "Replace guarded assignment with pgdecode."+rule.decodeFunc+"(...).Fill", pgdecodePath, "pgdecode"), true
}

func enclosingIfForDecode(parents map[ast.Node]ast.Node, sel *ast.SelectorExpr) *ast.IfStmt {
	for node := ast.Node(sel); node != nil; node = parents[node] {
		if ifStmt, ok := parents[node].(*ast.IfStmt); ok {
			return ifStmt
		}
		switch parents[node].(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return nil
		}
	}
	return nil
}

func simpleAddressable(expr ast.Expr) bool {
	switch e := unparen(expr).(type) {
	case *ast.Ident:
		return true
	case *ast.SelectorExpr:
		return simpleAddressable(e.X)
	case *ast.StarExpr:
		return true
	default:
		return false
	}
}

func (r reporter) valueDecodeFix(rule pgtypeRule, sel *ast.SelectorExpr) (analysis.SuggestedFix, bool) {
	qualifier, ok := r.packageQualifier(pgdecodePath, "pgdecode")
	if !ok {
		return analysis.SuggestedFix{}, false
	}
	text := fmt.Sprintf("%s.%s(%s).Value()", qualifier, rule.decodeFunc, renderNode(r.pass.Fset, sel.X))
	return r.replaceWithPackage(sel.Pos(), sel.End(), text, "Replace with pgdecode."+rule.decodeFunc+"(...).Value", pgdecodePath, "pgdecode"), true
}

func (r reporter) inlineWrapperFix(wrapper tinyWrapper, call *ast.CallExpr) (analysis.SuggestedFix, bool) {
	pkgPath := pgencodePath
	defaultName := "pgencode"
	if wrapper.kind == "pgdecode" {
		pkgPath = pgdecodePath
		defaultName = "pgdecode"
	}

	qualifier, ok := r.packageQualifier(pkgPath, defaultName)
	if !ok {
		return analysis.SuggestedFix{}, false
	}

	replacement, ok := inlineExpression(r.pass.Fset, wrapper, call.Args, qualifier)
	if !ok {
		return analysis.SuggestedFix{}, false
	}

	return r.replaceWithPackage(call.Pos(), call.End(), replacement, "Inline "+wrapper.kind+" wrapper "+wrapper.name, pkgPath, defaultName), true
}

func inlineExpression(fset *token.FileSet, wrapper tinyWrapper, args []ast.Expr, qualifier string) (string, bool) {
	source := renderNode(fset, wrapper.expr)
	expr, err := parser.ParseExpr(source)
	if err != nil {
		return "", false
	}

	argText := make(map[string]string, len(args))
	for i, param := range wrapper.params {
		argText[param] = renderNode(fset, args[i])
	}

	ok := true
	astutil.Apply(expr, func(cursor *astutil.Cursor) bool {
		ident, isIdent := cursor.Node().(*ast.Ident)
		if !isIdent {
			return true
		}
		if text, exists := argText[ident.Name]; exists {
			replacement, err := parser.ParseExpr(text)
			if err != nil {
				ok = false
				return false
			}
			cursor.Replace(replacement)
			return true
		}
		if ident.Name == wrapper.qualifier && qualifier != "" {
			cursor.Replace(ast.NewIdent(qualifier))
		}
		return true
	}, nil)
	if !ok {
		return "", false
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, expr); err != nil {
		return "", false
	}
	return buf.String(), true
}

func (r reporter) replaceWithPackage(start, end token.Pos, text, message, pkgPath, defaultName string) analysis.SuggestedFix {
	_, importEdits, _ := r.importFor(pkgPath, defaultName)
	edits := append([]analysis.TextEdit{}, importEdits...)
	edits = append(edits, analysis.TextEdit{Pos: start, End: end, NewText: []byte(text)})
	return analysis.SuggestedFix{Message: message, TextEdits: edits}
}

func (r reporter) packageQualifier(pkgPath, defaultName string) (string, bool) {
	qualifier, _, ok := r.importFor(pkgPath, defaultName)
	return qualifier, ok
}

func (r reporter) importFor(pkgPath, defaultName string) (string, []analysis.TextEdit, bool) {
	if qualifier, ok := importedQualifier(r.file, pkgPath); ok {
		return qualifier, nil, qualifier != ""
	}

	qualifier := uniqueImportName(r.file, defaultName)
	edits, ok := addImportEdits(r.pass.Fset, r.file, qualifier, defaultName, pkgPath)
	return qualifier, edits, ok
}

func importedQualifier(file *ast.File, pkgPath string) (string, bool) {
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) != pkgPath {
			continue
		}
		if spec.Name == nil {
			return path.Base(pkgPath), true
		}
		if spec.Name.Name == "_" || spec.Name.Name == "." {
			return "", true
		}
		return spec.Name.Name, true
	}
	return "", false
}

func uniqueImportName(file *ast.File, defaultName string) string {
	used := fileLevelNames(file)
	if !used[defaultName] {
		return defaultName
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s%d", defaultName, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func fileLevelNames(file *ast.File) map[string]bool {
	used := make(map[string]bool)
	for _, spec := range file.Imports {
		if spec.Name != nil {
			used[spec.Name.Name] = true
			continue
		}
		used[path.Base(strings.Trim(spec.Path.Value, `"`))] = true
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name != nil {
				used[d.Name.Name] = true
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					used[s.Name.Name] = true
				case *ast.ValueSpec:
					for _, name := range s.Names {
						used[name.Name] = true
					}
				}
			}
		}
	}
	return used
}

func addImportEdits(fset *token.FileSet, file *ast.File, qualifier, defaultName, pkgPath string) ([]analysis.TextEdit, bool) {
	specText := importSpecText(qualifier, defaultName, pkgPath)
	importDecl := firstImportDecl(file)
	if importDecl == nil {
		return []analysis.TextEdit{{
			Pos:     file.Name.End(),
			End:     file.Name.End(),
			NewText: []byte("\n\nimport " + specText + "\n"),
		}}, true
	}

	if importDecl.Lparen.IsValid() {
		return []analysis.TextEdit{{
			Pos:     importDecl.Rparen,
			End:     importDecl.Rparen,
			NewText: []byte("\n\t" + specText),
		}}, true
	}

	if len(importDecl.Specs) != 1 {
		return nil, false
	}
	existing := renderNode(fset, importDecl.Specs[0])
	if existing == "" {
		return nil, false
	}
	text := fmt.Sprintf("import (\n\t%s\n\t%s\n)", existing, specText)
	return []analysis.TextEdit{{
		Pos:     importDecl.Pos(),
		End:     importDecl.End(),
		NewText: []byte(text),
	}}, true
}

func importSpecText(qualifier, defaultName, pkgPath string) string {
	if qualifier != defaultName {
		return fmt.Sprintf("%s %q", qualifier, pkgPath)
	}
	return fmt.Sprintf("%q", pkgPath)
}

func firstImportDecl(file *ast.File) *ast.GenDecl {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if ok && gen.Tok == token.IMPORT {
			return gen
		}
	}
	return nil
}

func dereferencedExpr(expr ast.Expr) ast.Expr {
	star, ok := unparen(expr).(*ast.StarExpr)
	if !ok {
		return nil
	}
	return star.X
}

func isNonEmptyStringCheck(expr, value ast.Expr, fset *token.FileSet) bool {
	bin, ok := unparen(expr).(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ {
		return false
	}
	return (sameExpr(bin.X, value, fset) && isEmptyString(bin.Y)) ||
		(sameExpr(bin.Y, value, fset) && isEmptyString(bin.X))
}

func isNonZeroCheck(expr, value ast.Expr, fset *token.FileSet) bool {
	bin, ok := unparen(expr).(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ {
		return false
	}
	return (sameExpr(bin.X, value, fset) && isZeroLiteral(bin.Y)) ||
		(sameExpr(bin.Y, value, fset) && isZeroLiteral(bin.X))
}

func isZeroLiteral(expr ast.Expr) bool {
	lit, ok := unparen(expr).(*ast.BasicLit)
	if !ok || (lit.Kind != token.INT && lit.Kind != token.FLOAT) {
		return false
	}
	switch lit.Value {
	case "0", "0.0", "0.", ".0":
		return true
	default:
		return false
	}
}

func isNotNilCheck(expr, value ast.Expr, fset *token.FileSet) bool {
	bin, ok := unparen(expr).(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ {
		return false
	}
	return (sameExpr(bin.X, value, fset) && isNilIdent(bin.Y)) ||
		(sameExpr(bin.Y, value, fset) && isNilIdent(bin.X))
}

func isTrue(expr ast.Expr) bool {
	ident, ok := unparen(expr).(*ast.Ident)
	return ok && ident.Name == "true"
}

func unparen(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}

func sameExpr(a, b ast.Expr, fset *token.FileSet) bool {
	return renderNode(fset, a) == renderNode(fset, b)
}

func renderNode(fset *token.FileSet, node ast.Node) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return ""
	}
	return buf.String()
}

func isEmptyString(expr ast.Expr) bool {
	lit, ok := unparen(expr).(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && lit.Value == `""`
}

func isNilIdent(expr ast.Expr) bool {
	ident, ok := unparen(expr).(*ast.Ident)
	return ok && ident.Name == "nil"
}

func isAssignmentTarget(parents map[ast.Node]ast.Node, sel *ast.SelectorExpr) bool {
	parent := parents[sel]
	switch p := parent.(type) {
	case *ast.AssignStmt:
		return slices.ContainsFunc(p.Lhs, func(expr ast.Expr) bool { return expr == sel })
	case *ast.IncDecStmt:
		return p.X == sel
	}
	return false
}

func isAssignmentValue(parents map[ast.Node]ast.Node, sel *ast.SelectorExpr) bool {
	for node := ast.Node(sel); node != nil; node = parents[node] {
		switch p := parents[node].(type) {
		case *ast.AssignStmt:
			return slices.ContainsFunc(p.Rhs, func(expr ast.Expr) bool { return containsNode(expr, sel.Pos()) })
		case *ast.FuncDecl, *ast.FuncLit:
			return false
		}
	}
	return false
}

func isGuardedDecode(pass *analysis.Pass, parents map[ast.Node]ast.Node, sel *ast.SelectorExpr) bool {
	for node := parents[sel]; node != nil; node = parents[node] {
		switch n := node.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return false
		case *ast.IfStmt:
			if containsNode(n.Cond, sel.Pos()) || containsNode(n.Body, sel.Pos()) || (n.Else != nil && containsNode(n.Else, sel.Pos())) {
				return containsValidCheck(pass, n.Cond, sel.X)
			}
		}
	}
	return false
}

func isTinyDecodeHelper(pass *analysis.Pass, parents map[ast.Node]ast.Node, sel *ast.SelectorExpr) bool {
	for node := parents[sel]; node != nil; node = parents[node] {
		switch n := node.(type) {
		case *ast.FuncDecl:
			if n.Body == nil || len(n.Body.List) > 5 || returnsPgtype(pass, n.Type.Results) {
				return false
			}
			return fieldListHasPgtype(pass, n.Type.Params) || fieldListHasPgtype(pass, n.Recv)
		case *ast.FuncLit:
			if n.Body == nil || len(n.Body.List) > 5 || returnsPgtype(pass, n.Type.Results) {
				return false
			}
			return fieldListHasPgtype(pass, n.Type.Params)
		}
	}
	return false
}

func containsNode(node ast.Node, pos token.Pos) bool {
	return node != nil && node.Pos() <= pos && pos < node.End()
}

func containsValidCheck(pass *analysis.Pass, expr ast.Expr, target ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if found {
			return false
		}
		sel, ok := node.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Valid" {
			return true
		}
		if _, ok := pgtypeName(pass.TypesInfo.TypeOf(sel.X)); ok && sameExpr(sel.X, target, pass.Fset) {
			found = true
			return false
		}
		return true
	})
	return found
}

func returnsPgtype(pass *analysis.Pass, fields *ast.FieldList) bool {
	return fieldListHasPgtype(pass, fields)
}

func fieldListHasPgtype(pass *analysis.Pass, fields *ast.FieldList) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		if _, ok := pgtypeName(pass.TypesInfo.TypeOf(field.Type)); ok {
			return true
		}
	}
	return false
}

func pgtypeName(t types.Type) (string, bool) {
	t = derefType(t)
	named, ok := t.(*types.Named)
	if !ok {
		return "", false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != pgtypePath {
		return "", false
	}
	return obj.Name(), true
}

func derefType(t types.Type) types.Type {
	for {
		ptr, ok := t.(*types.Pointer)
		if !ok {
			return t
		}
		t = ptr.Elem()
	}
}
