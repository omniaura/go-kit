package stringbuilderlint

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/constant"
	"go/format"
	"go/token"
	"go/types"
	"path"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const (
	analyzerName = "stringbuilderlint"
	stringsPath  = "strings"
	strconvPath  = "strconv"
)

// Analyzer reports string concatenation and primitive fmt.Sprintf calls that
// should use strings.Builder.
var Analyzer = &analysis.Analyzer{
	Name: analyzerName,
	Doc:  "check for string concatenation and primitive fmt.Sprintf calls that should use strings.Builder",
	URL:  "https://pkg.go.dev/github.com/omniaura/go-kit/stringbuilderlint",
	Run:  run,
}

type reporter struct {
	pass    *analysis.Pass
	file    *ast.File
	parents map[ast.Node]ast.Node
	ignores suppressions
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if generated(file) {
			continue
		}

		r := reporter{
			pass:    pass,
			file:    file,
			parents: parentMap(file),
			ignores: newSuppressions(pass, file),
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.BinaryExpr:
				r.checkConcat(node)
			case *ast.CallExpr:
				r.checkSprintf(node)
			}
			return true
		})
	}

	return nil, nil
}

func (r reporter) checkConcat(expr *ast.BinaryExpr) {
	if expr.Op != token.ADD {
		return
	}
	if !isString(r.pass.TypesInfo.Types[expr].Type) {
		return
	}
	if isStringConcatParent(r.pass, r.parents, expr) {
		return
	}
	if r.pass.TypesInfo.Types[expr].Value != nil && r.pass.TypesInfo.Types[expr].Value.Kind() == constant.String {
		return
	}
	parts := stringConcatParts(r.pass, expr)
	if len(parts) <= 2 {
		return
	}
	if r.ignores.ignored(expr.Pos()) {
		return
	}

	pieces := make([]builderPiece, 0, len(parts))
	for _, part := range parts {
		if literal, ok := stringLiteralPiece(r.pass, part); ok {
			pieces = append(pieces, literal)
			continue
		}
		text, ok := renderNode(r.pass.Fset, part)
		if !ok {
			return
		}
		pieces = append(pieces, builderPiece{
			expr:       part,
			exprText:   text,
			conversion: conversion{kind: convertString},
		})
	}

	fix, ok := r.builderFix(expr.Pos(), expr.End(), pieces, "Replace with strings.Builder")
	diag := analysis.Diagnostic{
		Pos:     expr.Pos(),
		End:     expr.End(),
		Message: fmt.Sprintf("string concatenation with %d parts should use strings.Builder", len(parts)),
	}
	if ok {
		diag.SuggestedFixes = []analysis.SuggestedFix{fix}
	}
	r.pass.Report(diag)
}

func (r reporter) checkSprintf(call *ast.CallExpr) {
	if !isFmtSprintfCall(r.pass, call) || r.ignores.ignored(call.Pos()) {
		return
	}
	pieces, ok := r.sprintfPieces(call)
	if !ok {
		return
	}
	fix, ok := r.builderFix(call.Pos(), call.End(), pieces, "Replace with strings.Builder")
	diag := analysis.Diagnostic{
		Pos:     call.Pos(),
		End:     call.End(),
		Message: "fmt.Sprintf with primitive arguments should use strings.Builder and strconv",
	}
	if ok {
		diag.SuggestedFixes = []analysis.SuggestedFix{fix}
	}
	r.pass.Report(diag)
}

type builderPiece struct {
	literal    string
	literalLen int
	expr       ast.Expr
	exprText   string
	conversion conversion
	tempName   string
}

func stringLiteralPiece(pass *analysis.Pass, expr ast.Expr) (builderPiece, bool) {
	tv := pass.TypesInfo.Types[expr]
	if tv.Value == nil || tv.Value.Kind() != constant.String {
		return builderPiece{}, false
	}
	value := constant.StringVal(tv.Value)
	return builderPiece{
		literal:    strconv.Quote(value),
		literalLen: len(value),
	}, true
}

type conversionKind int

const (
	convertString conversionKind = iota + 1
	convertBool
	convertSigned
	convertUnsigned
	convertFloat
)

type conversion struct {
	kind      conversionKind
	base      int
	verb      byte
	precision int
	bitSize   int
	upper     bool
}

func (r reporter) sprintfPieces(call *ast.CallExpr) ([]builderPiece, bool) {
	if len(call.Args) == 0 {
		return nil, false
	}
	formatValue := r.pass.TypesInfo.Types[call.Args[0]].Value
	if formatValue == nil || formatValue.Kind() != constant.String {
		return nil, false
	}
	segments, ok := parseFormat(constant.StringVal(formatValue))
	if !ok {
		return nil, false
	}

	argIndex := 1
	pieces := make([]builderPiece, 0, len(segments))
	for _, segment := range segments {
		if segment.literal != "" {
			pieces = append(pieces, builderPiece{
				literal:    strconv.Quote(segment.literal),
				literalLen: len(segment.literal),
			})
			continue
		}
		if argIndex >= len(call.Args) {
			return nil, false
		}
		expr := call.Args[argIndex]
		argIndex++
		conv, ok := primitiveConversion(r.pass.TypesInfo.Types[expr].Type, segment)
		if !ok {
			return nil, false
		}
		text, ok := renderNode(r.pass.Fset, expr)
		if !ok {
			return nil, false
		}
		pieces = append(pieces, builderPiece{
			expr:       expr,
			exprText:   text,
			conversion: conv,
		})
	}
	if argIndex != len(call.Args) {
		return nil, false
	}
	return pieces, true
}

type formatSegment struct {
	literal   string
	verb      byte
	precision int
}

func parseFormat(format string) ([]formatSegment, bool) {
	var segments []formatSegment
	var literal strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			literal.WriteByte(format[i])
			continue
		}
		if i+1 >= len(format) {
			return nil, false
		}
		if format[i+1] == '%' {
			literal.WriteByte('%')
			i++
			continue
		}
		if literal.Len() > 0 {
			segments = append(segments, formatSegment{literal: literal.String()})
			literal.Reset()
		}

		i++
		if strings.ContainsRune("#0+- ", rune(format[i])) {
			return nil, false
		}
		if format[i] >= '0' && format[i] <= '9' {
			return nil, false
		}
		precision := -2
		if format[i] == '.' {
			i++
			if i >= len(format) || format[i] < '0' || format[i] > '9' {
				return nil, false
			}
			precision = 0
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				precision = precision*10 + int(format[i]-'0')
				i++
			}
			if i >= len(format) {
				return nil, false
			}
		}
		if format[i] == '[' || format[i] == '*' {
			return nil, false
		}
		segments = append(segments, formatSegment{
			verb:      format[i],
			precision: precision,
		})
	}
	if literal.Len() > 0 {
		segments = append(segments, formatSegment{literal: literal.String()})
	}
	return segments, true
}

func primitiveConversion(t types.Type, segment formatSegment) (conversion, bool) {
	basic, ok := t.Underlying().(*types.Basic)
	if !ok {
		return conversion{}, false
	}
	kind := basic.Kind()
	switch segment.verb {
	case 's':
		return conversion{kind: convertString}, kind == types.String
	case 't':
		return conversion{kind: convertBool}, kind == types.Bool
	case 'v':
		switch {
		case kind == types.String:
			return conversion{kind: convertString}, true
		case kind == types.Bool:
			return conversion{kind: convertBool}, true
		case signedInteger(kind):
			return conversion{kind: convertSigned, base: 10}, true
		case unsignedInteger(kind):
			return conversion{kind: convertUnsigned, base: 10}, true
		case floatKind(kind):
			return conversion{kind: convertFloat, verb: 'g', precision: -1, bitSize: floatBitSize(kind)}, true
		default:
			return conversion{}, false
		}
	case 'd':
		return integerConversion(kind, 10, false)
	case 'b':
		return integerConversion(kind, 2, false)
	case 'o':
		return integerConversion(kind, 8, false)
	case 'x':
		return integerConversion(kind, 16, false)
	case 'X':
		return integerConversion(kind, 16, true)
	case 'e', 'E', 'f', 'F':
		if !floatKind(kind) {
			return conversion{}, false
		}
		verb := segment.verb
		if verb == 'F' {
			verb = 'f'
		}
		precision := segment.precision
		if precision == -2 {
			precision = 6
		}
		return conversion{kind: convertFloat, verb: verb, precision: precision, bitSize: floatBitSize(kind)}, true
	case 'g', 'G':
		if !floatKind(kind) {
			return conversion{}, false
		}
		precision := segment.precision
		if precision == -2 {
			precision = -1
		}
		return conversion{kind: convertFloat, verb: segment.verb, precision: precision, bitSize: floatBitSize(kind)}, true
	default:
		return conversion{}, false
	}
}

func integerConversion(kind types.BasicKind, base int, upper bool) (conversion, bool) {
	switch {
	case signedInteger(kind):
		return conversion{kind: convertSigned, base: base, upper: upper}, true
	case unsignedInteger(kind):
		return conversion{kind: convertUnsigned, base: base, upper: upper}, true
	default:
		return conversion{}, false
	}
}

func signedInteger(kind types.BasicKind) bool {
	switch kind {
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
		return true
	default:
		return false
	}
}

func unsignedInteger(kind types.BasicKind) bool {
	switch kind {
	case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
		return true
	default:
		return false
	}
}

func floatKind(kind types.BasicKind) bool {
	return kind == types.Float32 || kind == types.Float64
}

func floatBitSize(kind types.BasicKind) int {
	if kind == types.Float32 {
		return 32
	}
	return 64
}

func (r reporter) builderFix(start, end token.Pos, pieces []builderPiece, message string) (analysis.SuggestedFix, bool) {
	needsStrconv := false
	for _, piece := range pieces {
		switch piece.conversion.kind {
		case convertBool, convertSigned, convertUnsigned, convertFloat:
			needsStrconv = true
		}
	}

	imports := []importNeed{{pkgPath: stringsPath, defaultName: "strings"}}
	if needsStrconv {
		imports = append(imports, importNeed{pkgPath: strconvPath, defaultName: "strconv"})
	}
	qualifiers, importEdits, ok := r.importsFor(imports)
	if !ok {
		return analysis.SuggestedFix{}, false
	}

	used := namesInPieces(pieces)
	for _, qualifier := range qualifiers {
		used[qualifier] = true
	}
	builderName := uniqueName(used, "sb")
	used[builderName] = true
	for i := range pieces {
		if pieces[i].expr == nil {
			continue
		}
		name := uniqueName(used, fmt.Sprintf("builderPart%d", i))
		used[name] = true
		pieces[i].tempName = name
	}

	replacement, ok := renderBuilderReplacement(pieces, builderName, qualifiers[stringsPath], qualifiers[strconvPath])
	if !ok {
		return analysis.SuggestedFix{}, false
	}
	edits := append([]analysis.TextEdit{}, importEdits...)
	edits = append(edits, analysis.TextEdit{Pos: start, End: end, NewText: []byte(replacement)})
	return analysis.SuggestedFix{Message: message, TextEdits: edits}, true
}

func renderBuilderReplacement(pieces []builderPiece, builderName, stringsQualifier, strconvQualifier string) (string, bool) {
	var buf strings.Builder
	buf.WriteString("func() string {\n")
	for _, piece := range pieces {
		if piece.expr == nil {
			continue
		}
		converted, ok := convertedExpr(piece, stringsQualifier, strconvQualifier)
		if !ok {
			return "", false
		}
		fmt.Fprintf(&buf, "\t%s := %s\n", piece.tempName, converted)
	}
	fmt.Fprintf(&buf, "\tvar %s %s.Builder\n", builderName, stringsQualifier)
	growParts := growTerms(pieces)
	if len(growParts) > 0 {
		fmt.Fprintf(&buf, "\t%s.Grow(%s)\n", builderName, strings.Join(growParts, " + "))
	}
	for _, piece := range pieces {
		if piece.literal != "" {
			fmt.Fprintf(&buf, "\t%s.WriteString(%s)\n", builderName, piece.literal)
			continue
		}
		fmt.Fprintf(&buf, "\t%s.WriteString(%s)\n", builderName, piece.tempName)
	}
	fmt.Fprintf(&buf, "\treturn %s.String()\n", builderName)
	buf.WriteString("}()")

	formatted, err := format.Source([]byte("package p\n\nvar _ = " + buf.String()))
	if err != nil {
		return buf.String(), true
	}
	const prefix = "package p\n\nvar _ = "
	return strings.TrimSpace(strings.TrimPrefix(string(formatted), prefix)), true
}

func convertedExpr(piece builderPiece, stringsQualifier, strconvQualifier string) (string, bool) {
	switch piece.conversion.kind {
	case convertString:
		return piece.exprText, true
	case convertBool:
		return fmt.Sprintf("%s.FormatBool(%s)", strconvQualifier, piece.exprText), true
	case convertSigned:
		text := fmt.Sprintf("%s.FormatInt(int64(%s), %d)", strconvQualifier, piece.exprText, piece.conversion.base)
		if piece.conversion.upper {
			text = fmt.Sprintf("%s.ToUpper(%s)", stringsQualifier, text)
		}
		return text, true
	case convertUnsigned:
		text := fmt.Sprintf("%s.FormatUint(uint64(%s), %d)", strconvQualifier, piece.exprText, piece.conversion.base)
		if piece.conversion.upper {
			text = fmt.Sprintf("%s.ToUpper(%s)", stringsQualifier, text)
		}
		return text, true
	case convertFloat:
		return fmt.Sprintf("%s.FormatFloat(float64(%s), %q, %d, %d)", strconvQualifier, piece.exprText, piece.conversion.verb, piece.conversion.precision, piece.conversion.bitSize), true
	default:
		return "", false
	}
}

func growTerms(pieces []builderPiece) []string {
	var terms []string
	literalBytes := 0
	flushLiteral := func() {
		if literalBytes == 0 {
			return
		}
		terms = append(terms, strconv.Itoa(literalBytes))
		literalBytes = 0
	}
	for _, piece := range pieces {
		if piece.literal != "" {
			literalBytes += piece.literalLen
			continue
		}
		flushLiteral()
		terms = append(terms, "len("+piece.tempName+")")
	}
	flushLiteral()
	return terms
}

func namesInPieces(pieces []builderPiece) map[string]bool {
	used := make(map[string]bool)
	for _, piece := range pieces {
		if piece.expr == nil {
			continue
		}
		ast.Inspect(piece.expr, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if ok {
				used[ident.Name] = true
			}
			return true
		})
	}
	return used
}

func uniqueName(used map[string]bool, base string) string {
	if !used[base] {
		return base
	}
	for i := 1; ; i++ {
		name := fmt.Sprintf("%s%d", base, i)
		if !used[name] {
			return name
		}
	}
}

func stringConcatParts(pass *analysis.Pass, expr ast.Expr) []ast.Expr {
	if paren, ok := expr.(*ast.ParenExpr); ok {
		return stringConcatParts(pass, paren.X)
	}
	binary, ok := expr.(*ast.BinaryExpr)
	if !ok || binary.Op != token.ADD || !isString(pass.TypesInfo.Types[binary].Type) {
		return []ast.Expr{expr}
	}
	parts := stringConcatParts(pass, binary.X)
	parts = append(parts, stringConcatParts(pass, binary.Y)...)
	return parts
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

func isString(t types.Type) bool {
	if t == nil {
		return false
	}
	basic, ok := t.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.String
}

func isFmtSprintfCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "Sprintf" && objectPackagePath(pass.TypesInfo.Uses[selector.Sel]) == "fmt"
}

func objectPackagePath(obj types.Object) string {
	if obj == nil || obj.Pkg() == nil {
		return ""
	}
	return obj.Pkg().Path()
}

func renderNode(fset *token.FileSet, node any) (string, bool) {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return "", false
	}
	return buf.String(), true
}

type importNeed struct {
	pkgPath     string
	defaultName string
}

func (r reporter) importsFor(needs []importNeed) (map[string]string, []analysis.TextEdit, bool) {
	qualifiers := make(map[string]string, len(needs))
	var missing []importNeed
	used := fileNames(r.file)
	for _, need := range needs {
		if qualifier, ok := importedQualifier(r.file, need.pkgPath); ok {
			if qualifier == "" {
				return nil, nil, false
			}
			qualifiers[need.pkgPath] = qualifier
			used[qualifier] = true
			continue
		}
		qualifier := uniqueName(used, need.defaultName)
		used[qualifier] = true
		qualifiers[need.pkgPath] = qualifier
		missing = append(missing, importNeed{pkgPath: need.pkgPath, defaultName: qualifier})
	}
	if len(missing) == 0 {
		return qualifiers, nil, true
	}
	edits, ok := addImportEdits(r.pass.Fset, r.file, missing)
	return qualifiers, edits, ok
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

func fileNames(file *ast.File) map[string]bool {
	used := make(map[string]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if ok {
			used[ident.Name] = true
		}
		return true
	})
	for _, spec := range file.Imports {
		if spec.Name != nil {
			used[spec.Name.Name] = true
			continue
		}
		used[path.Base(strings.Trim(spec.Path.Value, `"`))] = true
	}
	return used
}

func addImportEdits(fset *token.FileSet, file *ast.File, missing []importNeed) ([]analysis.TextEdit, bool) {
	specs := make([]string, 0, len(missing))
	for _, need := range missing {
		specs = append(specs, importSpecText(need.defaultName, path.Base(need.pkgPath), need.pkgPath))
	}
	importDecl := firstImportDecl(file)
	if importDecl == nil {
		return []analysis.TextEdit{{
			Pos:     file.Name.End(),
			End:     file.Name.End(),
			NewText: []byte("\n\nimport (\n\t" + strings.Join(specs, "\n\t") + "\n)\n"),
		}}, true
	}

	if importDecl.Lparen.IsValid() {
		return []analysis.TextEdit{{
			Pos:     importDecl.Rparen,
			End:     importDecl.Rparen,
			NewText: []byte("\n\t" + strings.Join(specs, "\n\t")),
		}}, true
	}

	if len(importDecl.Specs) != 1 {
		return nil, false
	}
	existing, ok := renderNode(fset, importDecl.Specs[0])
	if !ok {
		return nil, false
	}
	allSpecs := append([]string{existing}, specs...)
	return []analysis.TextEdit{{
		Pos:     importDecl.Pos(),
		End:     importDecl.End(),
		NewText: []byte("import (\n\t" + strings.Join(allSpecs, "\n\t") + "\n)"),
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
