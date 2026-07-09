package lookuptablelint

import (
	"flag"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"
)

const (
	analyzerName      = "lookuptablelint"
	minLookupEntries  = 2
	defaultMaxEntries = 64
)

var maxEntries = defaultMaxEntries

// Analyzer reports static lookup maps that should be predicate helpers.
var Analyzer = &analysis.Analyzer{
	Name: analyzerName,
	Doc:  "check for small static lookup maps that should be single-case switch predicates",
	URL:  "https://pkg.go.dev/github.com/omniaura/go-kit/lookuptablelint",
	Run:  run,
}

func init() {
	Analyzer.Flags.Init(analyzerName, flag.ExitOnError)
	Analyzer.Flags.IntVar(&maxEntries, "max_entries", defaultMaxEntries, "maximum lookup-table entries to report")
}

type tableKind int

const (
	boolTable tableKind = iota + 1
	structTable
)

type candidate struct {
	name  *ast.Ident
	obj   *types.Var
	kind  tableKind
	count int

	safe       bool
	lookupUses int
}

func run(pass *analysis.Pass) (any, error) {
	if maxEntries < minLookupEntries {
		return nil, nil
	}

	candidates := collectCandidates(pass)
	if len(candidates) == 0 {
		return nil, nil
	}

	checkUses(pass, candidates)
	for _, cand := range candidates {
		if !cand.safe || cand.lookupUses == 0 {
			continue
		}
		pass.Report(analysis.Diagnostic{
			Pos:     cand.name.Pos(),
			End:     cand.name.End(),
			Message: fmt.Sprintf("static lookup table %s has %d entries; prefer predicate %s with a single-case switch", cand.name.Name, cand.count, predicateName(cand.name.Name)),
		})
	}
	return nil, nil
}

func collectCandidates(pass *analysis.Pass) map[*types.Var]*candidate {
	candidates := make(map[*types.Var]*candidate)
	for _, file := range pass.Files {
		if generated(file) {
			continue
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				values, ok := spec.(*ast.ValueSpec)
				if !ok || len(values.Values) != len(values.Names) {
					continue
				}
				for i, name := range values.Names {
					if name == nil || name.Name == "_" || ast.IsExported(name.Name) {
						continue
					}
					lit, ok := values.Values[i].(*ast.CompositeLit)
					if !ok {
						continue
					}
					kind, count, ok := lookupTable(pass, lit)
					if !ok {
						continue
					}
					obj, ok := pass.TypesInfo.Defs[name].(*types.Var)
					if !ok {
						continue
					}
					candidates[obj] = &candidate{
						name:  name,
						obj:   obj,
						kind:  kind,
						count: count,
						safe:  true,
					}
				}
			}
		}
	}
	return candidates
}

func lookupTable(pass *analysis.Pass, lit *ast.CompositeLit) (tableKind, int, bool) {
	tv, ok := pass.TypesInfo.Types[lit]
	if !ok {
		return 0, 0, false
	}
	mapType, ok := tv.Type.(*types.Map)
	if !ok || !switchableKeyType(mapType.Key()) {
		return 0, 0, false
	}

	var kind tableKind
	switch {
	case isBoolType(mapType.Elem()):
		kind = boolTable
	case isUnnamedEmptyStruct(mapType.Elem()):
		kind = structTable
	default:
		return 0, 0, false
	}

	if len(lit.Elts) < minLookupEntries || len(lit.Elts) > maxEntries {
		return 0, 0, false
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok || !constantKey(pass, kv.Key) {
			return 0, 0, false
		}
		switch kind {
		case boolTable:
			if !constantTrue(pass, kv.Value) {
				return 0, 0, false
			}
		case structTable:
			if !emptyStructValue(pass, kv.Value) {
				return 0, 0, false
			}
		}
	}
	return kind, len(lit.Elts), true
}

func checkUses(pass *analysis.Pass, candidates map[*types.Var]*candidate) {
	for _, file := range pass.Files {
		parents := parentMap(file)
		ast.Inspect(file, func(node ast.Node) bool {
			id, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			obj, ok := pass.TypesInfo.Uses[id].(*types.Var)
			if !ok {
				return true
			}
			cand := candidates[obj]
			if cand == nil {
				return true
			}
			if lookupUse(cand, id, parents) {
				cand.lookupUses++
			} else {
				cand.safe = false
			}
			return true
		})
	}
}

func lookupUse(cand *candidate, id *ast.Ident, parents map[ast.Node]ast.Node) bool {
	index, ok := parents[id].(*ast.IndexExpr)
	if !ok || index.X != id || indexWrite(index, parents) {
		return false
	}
	if cand.kind == structTable && !twoValueLookup(index, parents) {
		return false
	}
	return true
}

func indexWrite(index *ast.IndexExpr, parents map[ast.Node]ast.Node) bool {
	switch parent := parents[index].(type) {
	case *ast.AssignStmt:
		for _, lhs := range parent.Lhs {
			if lhs == index {
				return true
			}
		}
	case *ast.IncDecStmt:
		return parent.X == index
	}
	return false
}

func twoValueLookup(index *ast.IndexExpr, parents map[ast.Node]ast.Node) bool {
	switch parent := parents[index].(type) {
	case *ast.AssignStmt:
		return len(parent.Rhs) == 1 && parent.Rhs[0] == index && len(parent.Lhs) == 2
	case *ast.ValueSpec:
		return len(parent.Values) == 1 && parent.Values[0] == index && len(parent.Names) == 2
	default:
		return false
	}
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

func switchableKeyType(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	switch basic.Kind() {
	case types.String,
		types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
		return true
	default:
		return false
	}
}

func isBoolType(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

func isUnnamedEmptyStruct(t types.Type) bool {
	st, ok := t.(*types.Struct)
	return ok && st.NumFields() == 0
}

func constantKey(pass *analysis.Pass, expr ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[expr]
	return ok && tv.Value != nil
}

func constantTrue(pass *analysis.Pass, expr ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[expr]
	return ok && tv.Value != nil && tv.Value.Kind() == constant.Bool && constant.BoolVal(tv.Value)
}

func emptyStructValue(pass *analysis.Pass, expr ast.Expr) bool {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok || len(lit.Elts) != 0 {
		return false
	}
	tv, ok := pass.TypesInfo.Types[lit]
	return ok && isUnnamedEmptyStruct(tv.Type)
}

func predicateName(name string) string {
	prefix := "is"
	if ast.IsExported(name) {
		prefix = "Is"
	}
	r, size := utf8.DecodeRuneInString(name)
	if r == utf8.RuneError && size == 0 {
		return prefix
	}
	return prefix + string(unicode.ToUpper(r)) + name[size:]
}

func generated(file *ast.File) bool {
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			break
		}
		text := group.Text()
		if strings.Contains(text, "Code generated") && strings.Contains(text, "DO NOT EDIT") {
			return true
		}
	}
	return false
}
