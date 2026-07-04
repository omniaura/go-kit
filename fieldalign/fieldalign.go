// Package fieldalign finds structs that would use less memory if their
// fields were sorted, and rewrites them into the optimal order while
// preserving comments, struct tags, blank lines, and multi-name field
// groups.
//
// It is a drop-in replacement for
// golang.org/x/tools/go/analysis/passes/fieldalignment, whose suggested
// fix discards field comments (golang/go#20744) and splits multi-name
// declarations. Detection (the size model and diagnostic messages) is
// identical to the upstream analyzer; only the fix differs.
//
// Structs are never rewritten when doing so could change behavior or
// lose information:
//
//   - structs constructed anywhere in the package with unkeyed composite
//     literals (reordering would silently reassign values)
//   - structs containing a structs.HostLayout field (layout-sensitive)
//   - structs annotated with a "fieldalign:ignore" directive comment
//   - layouts the rewriter cannot reorder safely (e.g. several fields
//     declared on one line of a multi-line struct)
//
// In those cases the diagnostic is still reported, without a fix.
package fieldalign

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// IgnoreDirective marks a struct type declaration that must not be
// reordered. Put it in a comment on the type declaration:
//
//	// fieldalign:ignore layout mirrors the C struct
//	type raw struct { ... }
const IgnoreDirective = "fieldalign:ignore"

var Analyzer = &analysis.Analyzer{
	Name: "fieldalignment",
	Doc: "find structs that would use less memory if their fields were sorted\n\n" +
		"Comment-preserving replacement for the x/tools fieldalignment pass: the\n" +
		"suggested fixes keep field comments, tags, and multi-name declarations.",
	URL: "https://pkg.go.dev/github.com/omniaura/go-kit/fieldalign",
	Run: run,
}

// A Fix describes one struct whose field order is suboptimal.
type Fix struct {
	// Pos is the position of the "struct" keyword.
	Pos token.Pos
	// Name is the declared type name, or "" for anonymous structs.
	Name string
	// Message is the upstream-compatible diagnostic message.
	Message string

	OldSize, NewSize         int64
	OldPtrBytes, NewPtrBytes int64

	// EditPos/EditEnd/NewText describe the rewrite. NewText is nil when
	// no safe automatic fix exists; SkipReason then says why.
	EditPos, EditEnd token.Pos
	NewText          []byte
	SkipReason       string

	// perm is the optimal permutation over flattened field indexes:
	// perm[i] is the original index of the field placed at position i.
	perm []int
}

// Fixable reports whether the fix carries an applicable edit.
func (f *Fix) Fixable() bool { return f.NewText != nil }

// SavedBytes is the per-instance size reduction.
func (f *Fix) SavedBytes() int64 { return f.OldSize - f.NewSize }

func run(pass *analysis.Pass) (any, error) {
	unkeyed := unkeyedStructs(pass.TypesInfo, pass.Files)
	for _, file := range pass.Files {
		tokFile := pass.Fset.File(file.FileStart)
		if tokFile == nil {
			continue
		}
		src, err := pass.ReadFile(tokFile.Name())
		if err != nil {
			continue // e.g. cgo-generated intermediate file
		}
		fixes := FileFixes(pass.Fset, file, src, pass.TypesInfo, pass.TypesSizes, unkeyed)
		for i := range fixes {
			fix := &fixes[i]
			diag := analysis.Diagnostic{
				Pos:     fix.Pos,
				End:     fix.Pos + token.Pos(len("struct")),
				Message: fix.Message,
			}
			if fix.Fixable() {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "Rearrange fields",
					TextEdits: []analysis.TextEdit{{
						Pos:     fix.EditPos,
						End:     fix.EditEnd,
						NewText: fix.NewText,
					}},
				}}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// FileFixes computes fixes for every suboptimal struct in file. src must
// be the file's source bytes. unkeyed is the set of struct types that
// appear in unkeyed composite literals (see UnkeyedStructs); fixes for
// those structs are reported without an edit. Overlapping edits (nested
// structs) keep only the outermost edit; re-run after applying to
// converge.
func FileFixes(fset *token.FileSet, file *ast.File, src []byte, info *types.Info, std types.Sizes, unkeyed map[*types.Struct]bool) []Fix {
	sizes := sizesFor(std)
	names, ignored := declaredStructs(file)

	var fixes []Fix
	ast.Inspect(file, func(n ast.Node) bool {
		node, ok := n.(*ast.StructType)
		if !ok {
			return true
		}
		if ignored[node] || len(node.Fields.List) == 0 {
			return true
		}
		tv, ok := info.Types[node]
		if !ok {
			return true
		}
		typ, ok := tv.Type.(*types.Struct)
		if !ok {
			return true
		}
		if hostLayoutSensitive(typ) {
			return true
		}

		fix, ok := analyze(node, typ, sizes)
		if !ok {
			return true
		}
		fix.Name = names[node]

		if unkeyed[typ] {
			fix.SkipReason = "constructed with unkeyed composite literals; convert them to keyed literals first"
		} else {
			computeEdit(fset, node, src, fix)
		}
		fixes = append(fixes, *fix)
		return true
	})

	// Nested structs produce overlapping edits; keep only the outermost.
	// The inner struct is fixed on a subsequent run.
	for i := range fixes {
		for j := range fixes {
			if i == j || !fixes[i].Fixable() || !fixes[j].Fixable() {
				continue
			}
			if fixes[j].EditPos <= fixes[i].EditPos && fixes[i].EditEnd <= fixes[j].EditEnd {
				fixes[i].NewText = nil
				fixes[i].SkipReason = "nested in another struct being rewritten; re-run to fix"
			}
		}
	}
	return fixes
}

// UnkeyedStructs returns the set of struct types constructed with unkeyed
// composite literals anywhere in files. Reordering those structs would
// silently reassign field values, so they are never rewritten.
func UnkeyedStructs(info *types.Info, files []*ast.File) map[*types.Struct]bool {
	return unkeyedStructs(info, files)
}

func unkeyedStructs(info *types.Info, files []*ast.File) map[*types.Struct]bool {
	set := make(map[*types.Struct]bool)
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || len(lit.Elts) == 0 {
				return true
			}
			for _, elt := range lit.Elts {
				if _, ok := elt.(*ast.KeyValueExpr); ok {
					return true
				}
			}
			tv, ok := info.Types[lit]
			if !ok {
				return true
			}
			if st, ok := tv.Type.Underlying().(*types.Struct); ok {
				set[st] = true
			}
			return true
		})
	}
	return set
}

// declaredStructs maps struct type expressions to their declared names,
// and collects structs opted out via the ignore directive.
func declaredStructs(file *ast.File) (names map[*ast.StructType]string, ignored map[*ast.StructType]bool) {
	names = make(map[*ast.StructType]string)
	ignored = make(map[*ast.StructType]bool)
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			names[st] = ts.Name.Name
			if hasDirective(gd.Doc) || hasDirective(ts.Doc) || hasDirective(ts.Comment) {
				ignored[st] = true
			}
		}
	}
	return names, ignored
}

func hasDirective(cg *ast.CommentGroup) bool {
	if cg == nil {
		return false
	}
	for _, c := range cg.List {
		if strings.Contains(c.Text, IgnoreDirective) {
			return true
		}
	}
	return false
}

func hostLayoutSensitive(typ *types.Struct) bool {
	for field := range typ.Fields() {
		ft := field.Type()
		if named, ok := ft.(*types.Named); ok {
			obj := named.Obj()
			if obj.Name() == "HostLayout" && obj.Pkg() != nil && obj.Pkg().Path() == "structs" {
				return true
			}
		}
	}
	return false
}

// analyze reports whether the struct's field order is suboptimal, and if
// so returns a Fix carrying sizes, message, and the optimal permutation
// encoded for computeEdit via fix.perm.
func analyze(node *ast.StructType, typ *types.Struct, s *gcSizes) (*Fix, bool) {
	optimal, indexes := optimalOrder(typ, s)
	optSize, optPtrs := s.Sizeof(optimal), s.ptrdata(optimal)
	curSize, curPtrs := s.Sizeof(typ), s.ptrdata(typ)

	var message string
	switch {
	case curSize != optSize:
		message = fmt.Sprintf("struct of size %d could be %d", curSize, optSize)
	case curPtrs != optPtrs:
		message = fmt.Sprintf("struct with %d pointer bytes could be %d", curPtrs, optPtrs)
	default:
		return nil, false // already optimal
	}

	return &Fix{
		Pos:         node.Pos(),
		Message:     message,
		OldSize:     curSize,
		NewSize:     optSize,
		OldPtrBytes: curPtrs,
		NewPtrBytes: optPtrs,
		perm:        indexes,
	}, true
}

// computeEdit fills in fix.EditPos/EditEnd/NewText, or fix.SkipReason.
//
// Multi-line structs are rewritten by reordering source-line chunks: each
// declaration group's chunk spans from the line after the previous group
// (so doc comments, directives, and blank separator lines travel with the
// field below them) through the group's own line including any trailing
// comment. The struct's braces and anything after the last field are left
// untouched. Single-line structs (necessarily comment-free) are re-printed
// from the AST.
func computeEdit(fset *token.FileSet, node *ast.StructType, src []byte, fix *Fix) {
	groups := node.Fields.List
	perm := fix.perm

	// Map flattened field indexes (one per name, as in types.Struct) to
	// declaration groups.
	groupOf := make([]int, 0, len(perm))
	for gi, g := range groups {
		n := max(1, len(g.Names))
		for range n {
			groupOf = append(groupOf, gi)
		}
	}
	if len(groupOf) != len(perm) {
		fix.SkipReason = "field count mismatch"
		return
	}

	// Derive the group order from the flattened permutation. Because
	// optimalOrder breaks ties by original index, a declaration group's
	// fields always come out contiguous and in order; verify anyway.
	var order []int
	for pos := 0; pos < len(perm); {
		gi := groupOf[perm[pos]]
		n := max(1, len(groups[gi].Names))
		first := firstFlatIndex(groups, gi)
		for k := range n {
			if pos+k >= len(perm) || perm[pos+k] != first+k {
				fix.SkipReason = "declaration group would need splitting"
				return
			}
		}
		order = append(order, gi)
		pos += n
	}

	tokFile := fset.File(node.Pos())
	if tokFile == nil {
		fix.SkipReason = "no file mapping"
		return
	}
	openLine := tokFile.Line(node.Fields.Opening)
	closeLine := tokFile.Line(node.Fields.Closing)

	if openLine == closeLine {
		astEdit(node, order, fix)
		return
	}

	// Line spans per group, including doc and trailing comments.
	type span struct{ start, end int }
	spans := make([]span, len(groups))
	for gi, g := range groups {
		startPos := g.Pos()
		if g.Doc != nil {
			startPos = g.Doc.Pos()
		}
		endPos := g.End()
		if g.Comment != nil {
			endPos = g.Comment.End()
		}
		spans[gi] = span{tokFile.Line(startPos), tokFile.Line(endPos)}
	}

	// The rewriter needs one group per line range, strictly between the
	// braces, in source order.
	prevEnd := openLine
	for gi := range groups {
		if spans[gi].start <= prevEnd {
			fix.SkipReason = "multiple fields share a source line"
			return
		}
		prevEnd = spans[gi].end
	}
	if prevEnd >= closeLine {
		fix.SkipReason = "closing brace shares a line with the last field"
		return
	}

	lineOffset := func(line int) int {
		return tokFile.Offset(tokFile.LineStart(line))
	}

	// Chunk boundaries: group gi owns the lines from the end of the
	// previous group (exclusive) through its own last line.
	chunkStart := make([]int, len(groups))
	for gi := range groups {
		if gi == 0 {
			chunkStart[gi] = openLine + 1
		} else {
			chunkStart[gi] = spans[gi-1].end + 1
		}
	}

	var out bytes.Buffer
	for oi, gi := range order {
		chunk := src[lineOffset(chunkStart[gi]):lineOffset(spans[gi].end+1)]
		if oi == 0 {
			// Don't open the struct body with the blank separator
			// lines a later chunk may have carried along.
			chunk = bytes.TrimLeft(chunk, "\n")
		}
		out.Write(chunk)
	}

	fix.EditPos = tokFile.Pos(lineOffset(openLine + 1))
	fix.EditEnd = tokFile.Pos(lineOffset(spans[len(spans)-1].end + 1))
	fix.NewText = out.Bytes()
}

// astEdit rewrites a single-line struct (comment-free by construction) by
// re-printing its field list in the new group order.
func astEdit(node *ast.StructType, order []int, fix *Fix) {
	reordered := make([]*ast.Field, len(order))
	for oi, gi := range order {
		g := node.Fields.List[gi]
		reordered[oi] = &ast.Field{Names: g.Names, Type: g.Type, Tag: g.Tag}
	}
	newStr := &ast.StructType{
		Struct: token.NoPos,
		Fields: &ast.FieldList{List: reordered},
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), newStr); err != nil {
		fix.SkipReason = "printing rearranged struct: " + err.Error()
		return
	}
	fix.EditPos = node.Pos()
	fix.EditEnd = node.End()
	fix.NewText = buf.Bytes()
}

func firstFlatIndex(groups []*ast.Field, gi int) int {
	idx := 0
	for i := range gi {
		idx += max(1, len(groups[i].Names))
	}
	return idx
}
