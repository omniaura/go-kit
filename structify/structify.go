// Package structify rewrites functions that take too many parameters into
// functions that take a single generated params struct — and rewrites every
// caller to match, across all loaded packages.
//
//	func CreateUser(ctx context.Context, name, email string, age int, admin bool) error
//
// becomes
//
//	// CreateUserParams bundles the parameters of CreateUser.
//	type CreateUserParams struct {
//		Name  string
//		Email string
//		Age   int
//		Admin bool
//	}
//
//	func CreateUser(ctx context.Context, arg CreateUserParams) error
//
// with every call site rewritten to CreateUser(ctx, CreateUserParams{Name: …}).
// A context.Context parameter stays positional, per Go convention — and if
// it isn't the first parameter, it is REORDERED to the front as part of the
// rewrite (callers' ctx arguments move with it).
//
// Because a signature change is only safe when every reference is visible
// and rewritable, a function is SKIPPED (diagnosed, not rewritten) when it:
//
//   - is variadic, generic, or has blank (_) parameters
//   - is referenced as a function value (assigned, passed, stored)
//   - is a method that satisfies an interface in the loaded packages
//   - has callers in generated files (regeneration would break the build)
//   - has a caller passing a multi-value call f(g()) or missing an import
//     needed to name the params struct
//   - has multiple context.Context parameters, or (when hoisting a
//     non-leading ctx) a call site whose ctx argument references a
//     parameter being rewritten
//   - carries a //go:* or //export directive
//   - is suppressed with "//lint:ignore structify" (or the funcparamlint
//     spelling) or "structify:ignore"
//
// The rewrite is text-based and formatting-preserving: caller edits are
// zero-width insertions around the existing argument expressions, so they
// compose with any other rewrites happening inside those arguments.
package structify

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Config controls detection.
type Config struct {
	// MaxParams: functions with more input parameters than this are
	// candidates. The count includes a leading context.Context, matching
	// funcparamlint. Default 4.
	MaxParams int
	// Generated: also structify functions declared in generated files.
	// Callers in generated files always block a rewrite regardless.
	Generated bool
}

func (c Config) maxParams() int {
	if c.MaxParams > 0 {
		return c.MaxParams
	}
	return 4
}

// Edit is a text edit in a file: replace [Start, End) with Text.
// Start == End is a pure insertion.
type Edit struct {
	Start, End int
	Text       string
}

// Target is one candidate function.
type Target struct {
	// Name is the function or Recv.Method name for reporting.
	Name string
	// Pos is the position of the function name.
	Pos token.Position
	// StructName is the generated params struct name (empty when skipped).
	StructName string
	// NumParams is the input parameter count that triggered detection.
	NumParams int
	// NumCallers is the number of rewritten call sites.
	NumCallers int
	// SkipReason is non-empty when the function was diagnosed but not
	// rewritten.
	SkipReason string
}

// Result of planning over a package set.
type Result struct {
	Rewritten []*Target
	Skipped   []*Target
	// Edits maps filename to its edits, unsorted and deduplicated.
	Edits map[string][]Edit
}

type field struct {
	name     string // struct field name
	typeText string // source text of the type
	objs     []types.Object
}

type candidate struct {
	pkg    *packages.Package
	file   *ast.File
	decl   *ast.FuncDecl
	obj    *types.Func
	target *Target

	ctxIndex    int    // index of the context.Context param, -1 if none
	ctxName     string // its name ("_" when blank/unnamed)
	ctxTypeText string // its type as written in source
	fields      []field
	argName     string
	structName  string
	qualByFile  map[string]string // filename -> qualifier ("", "pkg.")
}

// refInfo is one identifier reference to a function object.
type refInfo struct {
	pkg       *packages.Package
	file      *ast.File
	filename  string
	ident     *ast.Ident
	call      *ast.CallExpr // non-nil when the ref is the callee of this call
	generated bool
}

// Plan computes the rewrite plan for all packages. pkgs should come from
// packages.Load with syntax, types, deps and (ideally) Tests: true so
// callers in test files are seen and rewritten.
func Plan(pkgs []*packages.Package, cfg Config) (*Result, error) {
	if len(pkgs) == 0 {
		return &Result{Edits: map[string][]Edit{}}, nil
	}
	fset := pkgs[0].Fset

	// ---- Pass 1: index every file once -------------------------------
	type fileInfo struct {
		pkg       *packages.Package
		file      *ast.File
		filename  string
		src       []byte
		generated bool
		suppress  suppressions
	}
	var files []*fileInfo
	seenFile := map[string]bool{}
	// callee[ident] = enclosing call for idents in callee position
	callee := map[*ast.Ident]*ast.CallExpr{}
	// refs by target key (position of the object's declaration)
	refsByPos := map[token.Pos][]refInfo{}
	// interface method names -> interfaces (for the implements gate)
	var ifaces []*types.Interface

	var walkErr error
	forEachPkg(pkgs, func(p *packages.Package) {
		if p.TypesInfo == nil {
			return
		}
		for i, f := range p.Syntax {
			filename := p.CompiledGoFiles[i]
			fi := &fileInfo{pkg: p, file: f, filename: filename, generated: ast.IsGenerated(f)}
			if !seenFile[filename] {
				seenFile[filename] = true
				fi.suppress = parseSuppressions(fset, f)
				files = append(files, fi)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					switch fun := call.Fun.(type) {
					case *ast.Ident:
						callee[fun] = call
					case *ast.SelectorExpr:
						callee[fun.Sel] = call
					}
				}
				return true
			})
			for ident, obj := range p.TypesInfo.Uses {
				fn, ok := obj.(*types.Func)
				if !ok || fn.Pos() == token.NoPos {
					continue
				}
				if fset.File(ident.Pos()) == nil || fset.Position(ident.Pos()).Filename != filename {
					continue
				}
				refsByPos[fn.Pos()] = append(refsByPos[fn.Pos()], refInfo{
					pkg: p, file: f, filename: filename, ident: ident,
					call: callee[ident], generated: fi.generated,
				})
			}
		}
		// Collect interfaces for the implements gate.
		scope := p.Types.Scope()
		for _, name := range scope.Names() {
			tn, ok := scope.Lookup(name).(*types.TypeName)
			if !ok {
				continue
			}
			if intf, ok := tn.Type().Underlying().(*types.Interface); ok && intf.NumMethods() > 0 {
				ifaces = append(ifaces, intf)
			}
		}
	})
	if walkErr != nil {
		return nil, walkErr
	}

	// ---- Pass 2: find candidates in primary packages -----------------
	res := &Result{Edits: map[string][]Edit{}}
	reserved := map[*types.Package]map[string]bool{} // struct names claimed per package
	seenDecl := map[token.Pos]bool{}

	var cands []*candidate
	forEachPkg(pkgs, func(p *packages.Package) {
		if p.TypesInfo == nil {
			return
		}
		for i, f := range p.Syntax {
			filename := p.CompiledGoFiles[i]
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Body == nil || seenDecl[fd.Name.Pos()] {
					continue
				}
				seenDecl[fd.Name.Pos()] = true
				n := paramCount(fd.Type.Params)
				if n <= cfg.maxParams() {
					continue
				}
				if strings.HasSuffix(filename, "_test.go") {
					continue
				}
				if ast.IsGenerated(f) && !cfg.Generated {
					continue
				}
				obj, ok := p.TypesInfo.Defs[fd.Name].(*types.Func)
				if !ok {
					continue
				}
				c := &candidate{
					pkg: p, file: f, decl: fd, obj: obj, ctxIndex: -1,
					target: &Target{
						Name:      declName(fd),
						Pos:       fset.Position(fd.Name.Pos()),
						NumParams: n,
					},
					qualByFile: map[string]string{},
				}
				cands = append(cands, c)
			}
		}
	})
	sort.Slice(cands, func(i, j int) bool {
		return cands[i].target.Pos.String() < cands[j].target.Pos.String()
	})

	// ---- Pass 3: gate and plan each candidate ------------------------
	fileByName := map[string]*fileInfo{}
	for _, fi := range files {
		fileByName[fi.filename] = fi
	}
	src := func(filename string) ([]byte, error) {
		fi := fileByName[filename]
		if fi.src == nil {
			b, err := readFile(filename)
			if err != nil {
				return nil, err
			}
			fi.src = b
		}
		return fi.src, nil
	}

	// Params that will be renamed to arg.Field by ANY candidate: a ctx
	// argument expression that references one cannot be moved textually.
	renamed := map[types.Object]bool{}
	for _, c := range cands {
		for _, p := range flattenParams(c.decl.Type.Params) {
			if p.name == nil || p.name.Name == "_" || isContextParam(c.pkg, p) {
				continue
			}
			if o := c.pkg.TypesInfo.Defs[p.name]; o != nil {
				renamed[o] = true
			}
		}
	}

	for _, c := range cands {
		fi := fileByName[fset.Position(c.decl.Name.Pos()).Filename]
		if fi != nil && fi.suppress.ignored(fset, c.decl.Name.Pos()) {
			continue // suppressed: not even diagnosed
		}
		reason := c.plan(fset, ifaces, refsByPos, reserved, renamed, src)
		if reason != "" {
			c.target.SkipReason = reason
			res.Skipped = append(res.Skipped, c.target)
			continue
		}
		if err := c.emit(fset, refsByPos, res, src); err != nil {
			return nil, err
		}
		res.Rewritten = append(res.Rewritten, c.target)
	}

	dedupEdits(res.Edits)
	return res, nil
}

// plan runs the safety gates and fills in the candidate's rewrite plan,
// returning a skip reason or "".
func (c *candidate) plan(fset *token.FileSet, ifaces []*types.Interface, refsByPos map[token.Pos][]refInfo, reserved map[*types.Package]map[string]bool, renamed map[types.Object]bool, src func(string) ([]byte, error)) string {
	fd := c.decl
	sig := c.obj.Signature()

	if fd.Type.TypeParams != nil || (fd.Recv != nil && recvHasTypeParams(fd.Recv)) {
		return "generic function"
	}
	if sig.Variadic() {
		return "variadic parameters"
	}
	if hasCompilerDirective(fd.Doc) {
		return "carries a //go: or //export directive"
	}

	// Parameters. A context.Context stays positional: leading stays put,
	// non-leading is reordered to the front as part of the rewrite. It
	// never becomes a struct field (per the context package docs).
	params := flattenParams(fd.Type.Params)
	if len(params) == 0 {
		return "unnamed parameters"
	}
	for i, p := range params {
		if isContextParam(c.pkg, p) {
			if c.ctxIndex >= 0 {
				return "multiple context.Context parameters"
			}
			c.ctxIndex = i
			c.ctxName = "_"
			if p.name != nil && p.name.Name != "" {
				c.ctxName = p.name.Name
			}
			text, err := nodeText(fset, src, p.typ)
			if err != nil {
				return "unreadable source"
			}
			c.ctxTypeText = text
		}
	}
	fieldNames := map[string]bool{}
	for i, p := range params {
		if i == c.ctxIndex {
			continue
		}
		if p.name == nil || p.name.Name == "_" || p.name.Name == "" {
			return "blank or unnamed parameter"
		}
		fn := fieldName(p.name.Name)
		if fieldNames[fn] {
			return fmt.Sprintf("parameters %q collide as field %s", p.name.Name, fn)
		}
		fieldNames[fn] = true
		text, err := nodeText(fset, src, p.typ)
		if err != nil {
			return "unreadable source"
		}
		obj := c.pkg.TypesInfo.Defs[p.name]
		c.fields = append(c.fields, field{name: fn, typeText: text, objs: []types.Object{obj}})
	}
	if len(c.fields) == 0 {
		return "nothing to group"
	}

	// Interface gate for methods.
	if fd.Recv != nil {
		recvT := sig.Recv().Type()
		for _, intf := range ifaces {
			if intf.NumMethods() == 0 {
				continue
			}
			if m := findIfaceMethod(intf, c.obj.Name()); m != nil {
				if types.Implements(recvT, intf) || types.Implements(types.NewPointer(recvT), intf) {
					return "satisfies an interface"
				}
			}
		}
	}

	// Reference gates.
	refs := refsByPos[c.obj.Pos()]
	for _, ref := range refs {
		if ref.call == nil {
			return "used as a function value"
		}
		if ref.generated {
			return "called from generated code"
		}
		nargs := len(ref.call.Args)
		if c.ctxIndex >= 0 {
			nargs--
		}
		if nargs != len(c.fields) {
			return "caller passes a multi-value call"
		}
		// Reordering ctx to the front moves its argument text; that text
		// must not contain an identifier some rewrite renames to arg.X.
		if c.ctxIndex > 0 {
			bad := false
			ast.Inspect(ref.call.Args[c.ctxIndex], func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && renamed[ref.pkg.TypesInfo.Uses[id]] {
					bad = true
				}
				return !bad
			})
			if bad {
				return "context argument at a call site references a parameter being rewritten"
			}
		}
	}

	// Struct name (per-package uniqueness).
	pkgReserved := reserved[c.pkg.Types]
	if pkgReserved == nil {
		pkgReserved = map[string]bool{}
		reserved[c.pkg.Types] = pkgReserved
	}
	base := c.obj.Name()
	name := base + "Params"
	if taken(c.pkg, pkgReserved, name) && fd.Recv != nil {
		name = recvTypeName(fd.Recv) + upperFirst(base) + "Params"
	}
	if taken(c.pkg, pkgReserved, name) {
		return fmt.Sprintf("struct name %s already taken", name)
	}
	pkgReserved[name] = true
	c.structName = name
	c.target.StructName = name

	// Qualifier per caller file (methods may be called from files that
	// don't import the defining package).
	for _, ref := range refs {
		qual, ok := qualifierFor(ref, c.pkg)
		if !ok {
			return "caller file does not import the defining package"
		}
		c.qualByFile[ref.filename] = qual
	}

	// A param reused on the left of := (legal: it becomes an assignment)
	// cannot be rewritten to a selector — arg.X, err := ... is invalid.
	paramObjs := map[types.Object]bool{}
	for _, f := range c.fields {
		for _, o := range f.objs {
			paramObjs[o] = true
		}
	}
	reassigned := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Tok != token.DEFINE {
			return true
		}
		for _, lhs := range as.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && paramObjs[c.pkg.TypesInfo.Uses[id]] {
				reassigned = true
			}
		}
		return true
	})
	if reassigned {
		return "parameter reassigned with :="
	}

	// Argument name that collides with nothing in the function.
	used := map[string]bool{}
	ast.Inspect(fd, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			used[id.Name] = true
		}
		return true
	})
	c.argName = "arg"
	for i := 0; used[c.argName]; i++ {
		c.argName = fmt.Sprintf("arg%d", i)
	}
	return ""
}

// emit produces the edits for a planned candidate.
func (c *candidate) emit(fset *token.FileSet, refsByPos map[token.Pos][]refInfo, res *Result, src func(string) ([]byte, error)) error {
	fd := c.decl
	declFile := fset.Position(fd.Name.Pos()).Filename
	add := func(filename string, e Edit) {
		res.Edits[filename] = append(res.Edits[filename], e)
	}
	off := func(p token.Pos) int { return fset.Position(p).Offset }

	// 1. Struct declaration above the function (above its doc comment).
	var b strings.Builder
	fmt.Fprintf(&b, "// %s bundles the parameters of %s.\ntype %s struct {\n", c.structName, c.target.Name, c.structName)
	for _, f := range c.fields {
		fmt.Fprintf(&b, "\t%s %s\n", f.name, f.typeText)
	}
	b.WriteString("}\n\n")
	insertAt := fd.Pos()
	if fd.Doc != nil {
		insertAt = fd.Doc.Pos()
	}
	add(declFile, Edit{Start: off(insertAt), End: off(insertAt), Text: b.String()})

	// 2. Parameter list. Leading (or absent) ctx: replace everything
	// after it with "arg Struct". Non-leading ctx: rewrite the whole
	// list, hoisting ctx to the front.
	params := flattenParams(fd.Type.Params)
	pend := fd.Type.Params.Closing
	if c.ctxIndex <= 0 {
		first := 0
		if c.ctxIndex == 0 {
			first = 1
		}
		pstart := params[first].fieldStart
		add(declFile, Edit{Start: off(pstart), End: off(pend), Text: c.argName + " " + c.structName})
	} else {
		pstart := params[0].fieldStart
		text := c.ctxName + " " + c.ctxTypeText + ", " + c.argName + " " + c.structName
		add(declFile, Edit{Start: off(pstart), End: off(pend), Text: text})
	}

	// 3. Body: param uses become arg.Field.
	paramObj := map[types.Object]string{}
	for _, f := range c.fields {
		for _, o := range f.objs {
			paramObj[o] = f.name
		}
	}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if fname, ok := paramObj[c.pkg.TypesInfo.Uses[id]]; ok {
			add(declFile, Edit{Start: off(id.Pos()), End: off(id.End()), Text: c.argName + "." + fname})
		}
		return true
	})

	// 4. Call sites. With ctx leading or absent these are pure insertions
	// around the existing arguments. With a hoisted ctx, its argument text
	// additionally moves to the front (safe: the gate above proved it
	// contains nothing being renamed).
	for _, ref := range refsByPos[c.obj.Pos()] {
		call := ref.call
		args := call.Args
		qual := c.qualByFile[ref.filename]

		open := qual + c.structName + "{" + c.fields[0].name + ": "
		if c.ctxIndex > 0 {
			ctxArg := args[c.ctxIndex]
			ctxText, err := nodeText(fset, src, ctxArg)
			if err != nil {
				return err
			}
			// Move: prepend ctx before the first argument, delete it
			// (and its preceding separator) from its old spot.
			open = ctxText + ", " + open
			add(ref.filename, Edit{
				Start: off(args[c.ctxIndex-1].End()), End: off(ctxArg.End()),
			})
			rest := make([]ast.Expr, 0, len(args)-1)
			rest = append(rest, args[:c.ctxIndex]...)
			rest = append(rest, args[c.ctxIndex+1:]...)
			args = rest
		} else if c.ctxIndex == 0 {
			args = args[1:]
		}

		add(ref.filename, Edit{Start: off(args[0].Pos()), End: off(args[0].Pos()), Text: open})
		for k := 1; k < len(args); k++ {
			add(ref.filename, Edit{
				Start: off(args[k].Pos()), End: off(args[k].Pos()),
				Text: c.fields[k].name + ": ",
			})
		}
		last := args[len(args)-1]
		add(ref.filename, Edit{Start: off(last.End()), End: off(last.End()), Text: "}"})
		c.target.NumCallers++
	}
	return nil
}

// ---- helpers ----------------------------------------------------------

type param struct {
	name       *ast.Ident
	typ        ast.Expr
	fieldStart token.Pos // start of the *ast.Field this param belongs to
}

func flattenParams(fl *ast.FieldList) []param {
	var out []param
	if fl == nil {
		return out
	}
	for _, f := range fl.List {
		if len(f.Names) == 0 {
			out = append(out, param{name: nil, typ: f.Type, fieldStart: f.Pos()})
			continue
		}
		for _, n := range f.Names {
			out = append(out, param{name: n, typ: f.Type, fieldStart: f.Pos()})
		}
	}
	return out
}

func paramCount(fl *ast.FieldList) int {
	n := 0
	for _, p := range flattenParams(fl) {
		_ = p
		n++
	}
	return n
}

func declName(fd *ast.FuncDecl) string {
	if fd.Recv != nil {
		return recvTypeName(fd.Recv) + "." + fd.Name.Name
	}
	return fd.Name.Name
}

func recvTypeName(recv *ast.FieldList) string {
	if len(recv.List) == 0 {
		return ""
	}
	t := recv.List[0].Type
	for {
		switch tt := t.(type) {
		case *ast.StarExpr:
			t = tt.X
		case *ast.IndexExpr:
			t = tt.X
		case *ast.IndexListExpr:
			t = tt.X
		case *ast.Ident:
			return tt.Name
		default:
			return ""
		}
	}
}

func recvHasTypeParams(recv *ast.FieldList) bool {
	if len(recv.List) == 0 {
		return false
	}
	switch recv.List[0].Type.(type) {
	case *ast.IndexExpr, *ast.IndexListExpr:
		return true
	}
	if st, ok := recv.List[0].Type.(*ast.StarExpr); ok {
		switch st.X.(type) {
		case *ast.IndexExpr, *ast.IndexListExpr:
			return true
		}
	}
	return false
}

func isContextParam(p *packages.Package, pr param) bool {
	if pr.typ == nil {
		return false
	}
	t := p.TypesInfo.TypeOf(pr.typ)
	return t != nil && t.String() == "context.Context"
}

func hasCompilerDirective(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, cm := range doc.List {
		if strings.HasPrefix(cm.Text, "//go:") || strings.HasPrefix(cm.Text, "//export ") || cm.Text == "//export" {
			return true
		}
	}
	return false
}

func findIfaceMethod(intf *types.Interface, name string) *types.Func {
	for i := range intf.NumMethods() {
		if m := intf.Method(i); m.Name() == name {
			return m
		}
	}
	return nil
}

func taken(p *packages.Package, reserved map[string]bool, name string) bool {
	return reserved[name] || p.Types.Scope().Lookup(name) != nil
}

func qualifierFor(ref refInfo, target *packages.Package) (string, bool) {
	if ref.pkg.Types == target.Types || samePath(ref.pkg, target) {
		return "", true
	}
	// Prefer the qualifier already used by the call expression.
	if sel, ok := ref.call.Fun.(*ast.SelectorExpr); ok {
		if x, ok := sel.X.(*ast.Ident); ok {
			if _, isPkg := ref.pkg.TypesInfo.Uses[x].(*types.PkgName); isPkg {
				return x.Name + ".", true
			}
		}
	}
	// Method call: find an existing import of the defining package.
	for _, imp := range ref.file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path != target.PkgPath {
			continue
		}
		if imp.Name != nil {
			switch imp.Name.Name {
			case "_":
				continue
			case ".":
				return "", true
			default:
				return imp.Name.Name + ".", true
			}
		}
		return target.Types.Name() + ".", true
	}
	return "", false
}

func samePath(a, b *packages.Package) bool {
	// Test variants share the path but not the *types.Package.
	return strings.TrimSuffix(a.PkgPath, "_test") == b.PkgPath
}

var initialisms = map[string]string{
	"id": "ID", "ids": "IDs", "url": "URL", "uri": "URI", "api": "API",
	"http": "HTTP", "https": "HTTPS", "json": "JSON", "xml": "XML",
	"sql": "SQL", "db": "DB", "uid": "UID", "ip": "IP", "ttl": "TTL",
	"acl": "ACL", "cpu": "CPU", "ram": "RAM", "os": "OS", "ok": "OK",
}

func fieldName(param string) string {
	if up, ok := initialisms[strings.ToLower(param)]; ok {
		return up
	}
	return upperFirst(param)
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func nodeText(fset *token.FileSet, src func(string) ([]byte, error), n ast.Node) (string, error) {
	pos := fset.Position(n.Pos())
	end := fset.Position(n.End())
	b, err := src(pos.Filename)
	if err != nil {
		return "", err
	}
	return string(b[pos.Offset:end.Offset]), nil
}

func forEachPkg(pkgs []*packages.Package, fn func(*packages.Package)) {
	seen := map[string]bool{}
	var visit func(p *packages.Package)
	visit = func(p *packages.Package) {
		if seen[p.ID] {
			return
		}
		seen[p.ID] = true
		fn(p)
	}
	for _, p := range pkgs {
		visit(p)
	}
}

func dedupEdits(edits map[string][]Edit) {
	for f, es := range edits {
		seen := map[Edit]bool{}
		out := es[:0]
		for _, e := range es {
			if !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
		edits[f] = out
	}
}

// Apply applies edits to src (sorted descending by Start; insertions at the
// same offset keep their emit order).
func Apply(src []byte, edits []Edit) []byte {
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].Start != edits[j].Start {
			return edits[i].Start > edits[j].Start
		}
		return edits[i].End > edits[j].End
	})
	out := src
	for _, e := range edits {
		out = append(out[:e.Start:e.Start], append([]byte(e.Text), out[e.End:]...)...)
	}
	return out
}
