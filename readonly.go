// Package readonly defines an Analyzer that reports writes to struct fields
// marked with a `readonly` tag.
//
// Fields tagged `readonly:"external"` may stay exported (e.g. for ORM or
// JSON serialization), but writes from another package are reported: direct
// reassignment, writes into the field's contents (sub-fields, slice/map
// elements, the pointee, and the delete/clear/copy builtins), and
// whole-struct stores into existing storage (through a pointer, into a slice
// or map element, or into a field). Writes within the declaring package
// (including its external test package), initialization via composite
// literals, and reassignment of a plain variable are allowed.
//
// Fields tagged `readonly:"immutable"` cannot be reassigned anywhere, even
// in the declaring package; only composite literal initialization sets them.
//
// The `shallow` option (e.g. `readonly:"external,shallow"`) limits
// protection to reassignment of the field itself, leaving its contents
// writable.
//
// With -report-address-of, taking the address of a readonly field also
// counts as a write when the address is passed to a call, when a pointer
// receiver method is called on the field, or when a local pointer bound to
// the address is later written through, passed to a call, or used as a
// pointer receiver.
package readonly

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const doc = `readonly reports writes to struct fields tagged with a readonly tag

Fields can stay exported for ORM/JSON purposes while writes like

	user.Status = StatusDeleted
	order.User.Name = "x"
	user.Items[0] = "x"
	delete(user.Meta, "k")
	*userPtr = model.User{}
	users[i] = model.User{}

are rejected. readonly:"external" allows writes from the declaring package;
readonly:"immutable" rejects reassignment everywhere. The shallow option
(readonly:"external,shallow") protects only the field itself, leaving its
contents writable. Composite literal initialization (model.User{Status: s})
is always allowed. Unrecognized readonly tag values are reported at the
declaration site.

With -report-address-of, passing &user.Status to a call, calling a pointer
receiver method on a readonly field, and writing through or passing a local
pointer bound to &user.Status are reported as well.`

const (
	// tagKey is the struct tag key this analyzer inspects.
	tagKey = "readonly"
	// tagExternal restricts writes to the declaring package.
	tagExternal = "external"
	// tagImmutable forbids reassignment everywhere.
	tagImmutable = "immutable"
	// optShallow limits protection to reassignment of the field itself.
	optShallow = "shallow"
)

// protection is a parsed readonly tag value: a mode and its options.
type protection struct {
	mode    string // tagExternal, tagImmutable, or "" if unprotected
	shallow bool   // contents of the field stay writable
}

// parseTag parses a raw struct tag into a protection. Unrecognized modes
// yield no protection — they are reported separately at the declaration
// site by checkTagValues.
func parseTag(tag string) protection {
	v, ok := reflect.StructTag(tag).Lookup(tagKey)
	if !ok {
		return protection{}
	}
	parts := strings.Split(v, ",")
	if parts[0] != tagExternal && parts[0] != tagImmutable {
		return protection{}
	}
	p := protection{mode: parts[0]}
	for _, opt := range parts[1:] {
		if opt == optShallow {
			p.shallow = true
		}
	}
	return p
}

// Analyzer is the readonly analyzer with default options. Its flags
// (-allow-all-test-files, -report-address-of) configure it from the command
// line; use NewAnalyzer to configure it programmatically.
var Analyzer = NewAnalyzer(Options{})

// Options configure an Analyzer. Each field has a flag of the same name on
// the analyzer built from it, and the flag defaults to the field's value.
type Options struct {
	// AllowAllTestFiles exempts writes in any *_test.go file from the write
	// checks. By default only the declaring package's own test files are
	// exempt (see foreign); this extends that to every test package so
	// that, e.g., a repository test in another package can mutate fixtures.
	AllowAllTestFiles bool

	// ReportAddressOf treats taking the address of a readonly field as a
	// write in the situations listed in the package doc. It is off by
	// default because &u.Field is also how read-only pointers are built for
	// optional API fields (resp.ID = &u.ID); those stores are never
	// reported, but a read-only helper call such as fmt.Println(&u.ID) is.
	ReportAddressOf bool
}

// NewAnalyzer builds a readonly analyzer configured by opts. Each analyzer
// owns its options, so several can coexist with different settings. Flag
// registration lives here rather than in init() so the package has no
// init() — a requirement for inclusion in golangci-lint.
func NewAnalyzer(opts Options) *analysis.Analyzer {
	// Flags write into this copy, and Run reads it after flag parsing.
	o := opts
	a := &analysis.Analyzer{
		Name:     "readonly",
		Doc:      doc,
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			return run(pass, o)
		},
	}
	a.Flags.BoolVar(&o.AllowAllTestFiles, "allow-all-test-files", o.AllowAllTestFiles,
		"allow writes to readonly fields in any *_test.go file, not just the declaring package's tests")
	a.Flags.BoolVar(&o.ReportAddressOf, "report-address-of", o.ReportAddressOf,
		"report the address of a readonly field being passed to a call, used as a pointer receiver, or written through a local pointer")
	return a
}

// checker holds the per-pass state of the analysis.
type checker struct {
	pass *analysis.Pass
	opts Options

	// tracked maps a variable bound to the address of a readonly field
	// (p := &u.ID) to where that address was taken. Later uses of the
	// variable as a write target, call argument, or pointer receiver are
	// reported against the field. Populated only with -report-address-of.
	tracked map[*types.Var]*addrSite
}

// addrSite records an address-of expression whose target is a readonly
// field, together with the violation it carries.
type addrSite struct {
	name string         // the variable the address was bound to
	expr *ast.UnaryExpr // the &u.Field expression
	msg  string         // describe(...) output for the field
}

// hit is a forbidden write found on an expression path.
type hit struct {
	pos  token.Pos // where to report: the field selector or the pointer variable
	msg  string    // describe(...) output for the field
	site *addrSite // non-nil when reached through a tracked pointer variable
}

func run(pass *analysis.Pass, opts Options) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	c := &checker{pass: pass, opts: opts}

	if opts.ReportAddressOf {
		c.tracked = map[*types.Var]*addrSite{}
		c.collectAddressBindings(insp)
	}

	nodeFilter := []ast.Node{
		(*ast.AssignStmt)(nil),
		(*ast.IncDecStmt)(nil),
		(*ast.RangeStmt)(nil),
		(*ast.CallExpr)(nil),
		(*ast.StructType)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range stmt.Lhs {
				c.checkWrite(lhs)
			}
		case *ast.IncDecStmt:
			c.checkWrite(stmt.X)
		case *ast.RangeStmt:
			if stmt.Tok == token.ASSIGN {
				if stmt.Key != nil {
					c.checkWrite(stmt.Key)
				}
				if stmt.Value != nil {
					c.checkWrite(stmt.Value)
				}
			}
		case *ast.CallExpr:
			if arg, ok := mutatingBuiltinArg(pass, stmt); ok {
				c.checkContentsWrite(arg)
			}
			if opts.ReportAddressOf {
				c.checkCall(stmt)
			}
		case *ast.StructType:
			checkTagValues(pass, stmt)
		}
	})
	return nil, nil
}

// mutatingBuiltinArg returns the argument that call writes into when call is
// one of the builtins that mutate their first argument in place: delete(m, k),
// clear(x), and copy(dst, src). Shadowed identifiers are resolved through
// type information, so a user-defined delete function is not matched.
func mutatingBuiltinArg(pass *analysis.Pass, call *ast.CallExpr) (ast.Expr, bool) {
	id, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return nil, false
	}
	b, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	if !ok || len(call.Args) == 0 {
		return nil, false
	}
	switch b.Name() {
	case "delete", "clear", "copy":
		return call.Args[0], true
	}
	return nil, false
}

// checkTagValues reports readonly tags with unrecognized values, and tag
// keys that differ from "readonly" only in case, so that a typo cannot
// silently leave a field unprotected.
func checkTagValues(pass *analysis.Pass, st *ast.StructType) {
	for _, f := range st.Fields.List {
		if f.Tag == nil {
			continue
		}
		raw, err := strconv.Unquote(f.Tag.Value)
		if err != nil {
			continue
		}
		for _, key := range tagKeys(raw) {
			if key != tagKey && strings.EqualFold(key, tagKey) {
				pass.Reportf(f.Tag.Pos(), "unrecognized struct tag key %q (did you mean %q?)", key, tagKey)
			}
		}
		v, ok := reflect.StructTag(raw).Lookup(tagKey)
		if !ok {
			continue
		}
		parts := strings.Split(v, ",")
		if parts[0] != tagExternal && parts[0] != tagImmutable {
			pass.Reportf(f.Tag.Pos(), "invalid readonly tag value %q (valid values: %q, %q)", parts[0], tagExternal, tagImmutable)
			continue
		}
		for _, opt := range parts[1:] {
			if opt != optShallow {
				pass.Reportf(f.Tag.Pos(), "invalid readonly tag option %q (valid options: %q)", opt, optShallow)
			}
		}
	}
}

// tagKeys returns the keys of a struct tag, parsed the way
// reflect.StructTag.Lookup does. Parsing stops at the first malformed pair,
// which go vet's structtag check reports separately.
func tagKeys(tag string) []string {
	var keys []string
	for tag != "" {
		i := 0
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		tag = tag[i:]
		if tag == "" {
			break
		}
		i = 0
		for i < len(tag) && tag[i] > ' ' && tag[i] != ':' && tag[i] != '"' && tag[i] != 0x7f {
			i++
		}
		if i == 0 || i+1 >= len(tag) || tag[i] != ':' || tag[i+1] != '"' {
			break
		}
		name := tag[:i]
		tag = tag[i+1:]
		i = 1
		for i < len(tag) && tag[i] != '"' {
			if tag[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(tag) {
			break
		}
		keys = append(keys, name)
		tag = tag[i+1:]
	}
	return keys
}

// collectAddressBindings records every variable bound to the address of a
// readonly field: p := &u.ID, p = &u.ID, and var p = &u.ID. Only the first
// binding of a variable is kept, so a later rebinding to something else
// does not stop its uses from being reported.
func (c *checker) collectAddressBindings(insp *inspector.Inspector) {
	insp.Preorder([]ast.Node{(*ast.AssignStmt)(nil), (*ast.ValueSpec)(nil)}, func(n ast.Node) {
		var lhs, rhs []ast.Expr
		switch n := n.(type) {
		case *ast.AssignStmt:
			if n.Tok != token.DEFINE && n.Tok != token.ASSIGN {
				return
			}
			lhs, rhs = n.Lhs, n.Rhs
		case *ast.ValueSpec:
			for _, name := range n.Names {
				lhs = append(lhs, name)
			}
			rhs = n.Values
		}
		if len(lhs) != len(rhs) {
			return
		}
		for i, l := range lhs {
			id, ok := l.(*ast.Ident)
			if !ok {
				continue
			}
			v, ok := c.pass.TypesInfo.ObjectOf(id).(*types.Var)
			if !ok {
				continue
			}
			if _, done := c.tracked[v]; done {
				continue
			}
			addr, ok := ast.Unparen(rhs[i]).(*ast.UnaryExpr)
			if !ok || addr.Op != token.AND {
				continue
			}
			if h, ok := c.walkPath(ast.Unparen(addr.X), true); ok {
				c.tracked[v] = &addrSite{name: v.Name(), expr: addr, msg: h.msg}
			}
		}
	})
}

// checkWrite reports expr if it writes to or through a readonly field
// declared in another package. expr is the target of an assignment, ++/--,
// or range clause.
func (c *checker) checkWrite(expr ast.Expr) {
	expr = ast.Unparen(expr)
	if c.exemptTestFile(expr.Pos()) {
		return
	}
	if h, ok := c.walkPath(expr, true); ok {
		c.report(h, "")
		return
	}
	// Nothing on the selection path is protected, so consider the store as
	// a whole: assigning a struct value overwrites every field inside it.
	// A plain variable (u = model.User{}) is treated like initialization,
	// and a map element (m[k] = v) replaces which value the key holds: map
	// elements are not addressable, so nothing can observe the old value
	// being overwritten in place. Any other target (*p, xs[i], o.User) is
	// existing storage that something else may still refer to.
	switch e := expr.(type) {
	case *ast.Ident:
		return
	case *ast.IndexExpr:
		if tv, ok := c.pass.TypesInfo.Types[ast.Unparen(e.X)]; ok {
			if _, isMap := tv.Type.Underlying().(*types.Map); isMap {
				return
			}
		}
	}
	c.checkWholeStore(expr)
}

// checkContentsWrite reports expr if writing into its contents (as the
// delete, clear, and copy builtins do) touches a readonly field declared in
// another package. Unlike checkWrite, expr itself is never reassigned, so
// shallow protection permits the write.
func (c *checker) checkContentsWrite(expr ast.Expr) {
	expr = ast.Unparen(expr)
	if c.exemptTestFile(expr.Pos()) {
		return
	}
	if h, ok := c.walkPath(expr, false); ok {
		c.report(h, "")
	}
}

// checkCall implements -report-address-of for one call: a pointer receiver
// method called on a readonly field, and arguments that are the address of
// a readonly field or a pointer variable bound to one.
func (c *checker) checkCall(call *ast.CallExpr) {
	if c.exemptTestFile(call.Pos()) {
		return
	}
	if sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
		c.checkReceiver(sel)
	}
	// Conversions and builtins do not write through their arguments.
	if tv, ok := c.pass.TypesInfo.Types[call.Fun]; ok && (tv.IsType() || tv.IsBuiltin()) {
		return
	}
	for _, arg := range call.Args {
		switch a := ast.Unparen(arg).(type) {
		case *ast.UnaryExpr:
			if a.Op != token.AND {
				continue
			}
			if h, ok := c.walkPath(ast.Unparen(a.X), true); ok {
				c.report(h, "address passed to a call")
			}
		case *ast.Ident:
			if h, ok := c.trackedHit(a); ok {
				c.report(h, "passed to a call")
			}
		}
	}
}

// checkReceiver reports sel when it names a pointer receiver method reached
// through a readonly field. If the receiver is a value, Go implicitly takes
// its address, so the field itself may be reassigned by the method (direct;
// shallow does not exempt it). If the receiver is already a pointer, the
// method writes through it, which is a contents write.
func (c *checker) checkReceiver(sel *ast.SelectorExpr) {
	selection, ok := c.pass.TypesInfo.Selections[sel]
	if !ok || selection.Kind() != types.MethodVal {
		return
	}
	fn, ok := selection.Obj().(*types.Func)
	if !ok {
		return
	}
	recv := fn.Type().(*types.Signature).Recv()
	if recv == nil || !isPointer(recv.Type()) {
		return
	}

	x := ast.Unparen(sel.X)
	xIsPointer := false
	if u, ok := x.(*ast.UnaryExpr); ok && u.Op == token.AND {
		x = ast.Unparen(u.X) // (&a.Profile).SetName(): explicit address
	} else if tv, ok := c.pass.TypesInfo.Types[x]; ok {
		xIsPointer = isPointer(tv.Type)
	}

	// The method may be promoted through embedded fields, index[:len-1];
	// those are traversed implicitly from x and the last of them is the
	// actual receiver.
	index := selection.Index()
	embedded := index[:len(index)-1]
	direct := !xIsPointer
	if len(embedded) > 0 {
		lastDirect := !isPointer(fieldTypeAt(selection.Recv(), embedded))
		if h, ok := c.checkFieldPath(selection.Recv(), embedded, lastDirect, sel.Sel.Pos()); ok {
			c.report(h, "pointer receiver call")
			return
		}
		direct = false // x is traversed into, not addressed itself
	}

	if id, ok := x.(*ast.Ident); ok {
		if h, ok := c.trackedHit(id); ok {
			h.pos = sel.Sel.Pos()
			c.report(h, "pointer receiver call")
		}
		return
	}
	if h, ok := c.walkPath(x, direct); ok {
		c.report(h, "pointer receiver call")
	}
}

// walkPath walks the target expression inward so that writes into the
// contents of a readonly field (order.User.Name, user.Items[0]) are caught,
// not just reassignment of the field itself. direct indicates that expr is
// itself reassigned; only the first selector's final field is then reassigned
// directly, and everything deeper is a contents write, which shallow
// protection permits. A path that ends in a tracked pointer variable after
// at least one step (*p, p.X) is a write through that pointer.
func (c *checker) walkPath(expr ast.Expr, direct bool) (hit, bool) {
	stepped := false
	for {
		switch e := expr.(type) {
		case *ast.SelectorExpr:
			if h, ok := c.checkSelection(e, direct); ok {
				return h, true
			}
			direct = false
			stepped = true
			expr = ast.Unparen(e.X)
		case *ast.IndexExpr:
			direct = false
			stepped = true
			expr = ast.Unparen(e.X)
		case *ast.SliceExpr:
			direct = false
			stepped = true
			expr = ast.Unparen(e.X)
		case *ast.StarExpr:
			direct = false
			stepped = true
			expr = ast.Unparen(e.X)
		case *ast.Ident:
			if stepped {
				return c.trackedHit(e)
			}
			return hit{}, false
		default:
			return hit{}, false
		}
	}
}

// trackedHit returns a hit if id refers to a variable bound to the address
// of a readonly field.
func (c *checker) trackedHit(id *ast.Ident) (hit, bool) {
	v, ok := c.pass.TypesInfo.Uses[id].(*types.Var)
	if !ok {
		return hit{}, false
	}
	site, ok := c.tracked[v]
	if !ok {
		return hit{}, false
	}
	return hit{pos: id.Pos(), msg: site.msg, site: site}, true
}

// checkSelection returns a hit if a protected field on sel's selection path
// — including fields traversed implicitly through embedding — is written in
// violation of its mode. direct indicates that the final field of the path
// is itself the assignment target (as opposed to a write into its contents).
func (c *checker) checkSelection(sel *ast.SelectorExpr, direct bool) (hit, bool) {
	selection, ok := c.pass.TypesInfo.Selections[sel]
	if !ok || selection.Kind() != types.FieldVal {
		return hit{}, false
	}
	return c.checkFieldPath(selection.Recv(), selection.Index(), direct, sel.Sel.Pos())
}

// checkFieldPath checks the fields selected by index, starting from type t,
// and returns a hit at pos for the first one written in violation of its
// mode. direct applies to the last field only.
func (c *checker) checkFieldPath(t types.Type, index []int, direct bool, pos token.Pos) (hit, bool) {
	for i, idx := range index {
		if ptr, ok := t.Underlying().(*types.Pointer); ok {
			t = ptr.Elem()
		}
		st, ok := t.Underlying().(*types.Struct)
		if !ok {
			return hit{}, false
		}
		field := st.Field(idx)
		p := parseTag(st.Tag(idx))
		reassigned := direct && i == len(index)-1
		if violates(c.pass, p.mode, field) && (!p.shallow || reassigned) {
			return hit{pos: pos, msg: describe(p.mode, typeName(t), field)}, true
		}
		t = field.Type()
	}
	return hit{}, false
}

// fieldTypeAt returns the type of the field selected by index from t, or
// nil if the path does not go through structs.
func fieldTypeAt(t types.Type, index []int) types.Type {
	for _, idx := range index {
		if ptr, ok := t.Underlying().(*types.Pointer); ok {
			t = ptr.Elem()
		}
		st, ok := t.Underlying().(*types.Struct)
		if !ok {
			return nil
		}
		t = st.Field(idx).Type()
	}
	return t
}

// checkWholeStore reports target = v when target's type contains, by value,
// a readonly field that the current package may not write: such a store
// overwrites the protected field wholesale.
func (c *checker) checkWholeStore(target ast.Expr) {
	tv, ok := c.pass.TypesInfo.Types[target]
	if !ok {
		return
	}
	owner, p, field := findProtectedField(c.pass, tv.Type, map[types.Type]bool{})
	if field == nil {
		return
	}
	c.pass.Reportf(target.Pos(), "cannot assign to %s: %s", types.ExprString(target), describe(p.mode, typeName(owner), field))
}

// findProtectedField searches t for a field the current package may not
// write, descending through fields held by value: nested and embedded
// structs and arrays. Pointers, slices, and maps are not followed, since
// storing over them replaces a reference rather than the referenced
// contents. A whole store reassigns every such field directly, so the
// shallow option does not exempt it. It returns the struct type owning the
// field, its protection, and the field, or a nil field if none is found.
func findProtectedField(pass *analysis.Pass, t types.Type, seen map[types.Type]bool) (types.Type, protection, *types.Var) {
	t = types.Unalias(t)
	if seen[t] {
		return nil, protection{}, nil
	}
	seen[t] = true
	switch u := t.Underlying().(type) {
	case *types.Array:
		return findProtectedField(pass, u.Elem(), seen)
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			field := u.Field(i)
			if p := parseTag(u.Tag(i)); violates(pass, p.mode, field) {
				return t, p, field
			}
			if owner, p, f := findProtectedField(pass, field.Type(), seen); f != nil {
				return owner, p, f
			}
		}
	}
	return nil, protection{}, nil
}

// report emits h. reason says how the write happens when that is not a
// plain assignment ("address passed to a call"); for a hit reached through
// a tracked pointer the message also names the variable and where its
// address was taken.
func (c *checker) report(h hit, reason string) {
	msg := h.msg
	switch {
	case h.site != nil:
		if reason == "" {
			reason = "written"
		}
		pos := c.pass.Fset.Position(h.site.expr.Pos())
		msg = fmt.Sprintf("%s (%s through %s, address taken at %s:%d:%d)", msg, reason, h.site.name, filepath.Base(pos.Filename), pos.Line, pos.Column)
	case reason != "":
		msg = fmt.Sprintf("%s (%s)", msg, reason)
	}
	c.pass.Reportf(h.pos, "%s", msg)
}

// exemptTestFile reports whether pos lies in a *_test.go file and
// AllowAllTestFiles exempts writes there.
func (c *checker) exemptTestFile(pos token.Pos) bool {
	if !c.opts.AllowAllTestFiles {
		return false
	}
	if f := c.pass.Fset.File(pos); f != nil {
		return strings.HasSuffix(f.Name(), "_test.go")
	}
	return false
}

// foreign reports whether pkg is a package other than the one being
// analyzed. The declaring package's own external test package (path suffixed
// with "_test") counts as the declaring package.
func foreign(pass *analysis.Pass, pkg *types.Package) bool {
	if pkg == nil || pkg == pass.Pkg {
		return false
	}
	return pkg.Path() != strings.TrimSuffix(pass.Pkg.Path(), "_test")
}

// violates reports whether the current package is forbidden from writing
// field under protection mode m.
func violates(pass *analysis.Pass, m string, field *types.Var) bool {
	switch m {
	case tagImmutable:
		return true
	case tagExternal:
		return foreign(pass, field.Pkg())
	}
	return false
}

// describe renders the diagnostic for a forbidden write to owner.field.
func describe(m string, owner string, field *types.Var) string {
	if m == tagImmutable {
		return fmt.Sprintf("field %s.%s is immutable", owner, field.Name())
	}
	return fmt.Sprintf("field %s.%s is readonly outside package %s", owner, field.Name(), field.Pkg().Path())
}

// typeName returns the declared name of t, or "struct" for anonymous types.
func typeName(t types.Type) string {
	if named, ok := types.Unalias(t).(*types.Named); ok {
		return named.Obj().Name()
	}
	return "struct"
}

// isPointer reports whether t is a pointer type.
func isPointer(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Pointer)
	return ok
}
