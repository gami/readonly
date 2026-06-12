// Package readonly defines an Analyzer that reports writes to struct fields
// marked with a `readonly` tag.
//
// Fields tagged `readonly:"external"` may stay exported (e.g. for ORM or
// JSON serialization), but writes from another package are reported: direct
// reassignment, writes into the field's contents (sub-fields and slice/map
// elements), and whole-struct stores through a pointer. Writes within the
// declaring package (including its external test package) and initialization
// via composite literals are allowed.
//
// Fields tagged `readonly:"immutable"` cannot be reassigned anywhere, even
// in the declaring package; only composite literal initialization sets them.
package readonly

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
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
	*userPtr = model.User{}

are rejected. readonly:"external" allows writes from the declaring package;
readonly:"immutable" rejects reassignment everywhere. Composite literal
initialization (model.User{Status: s}) is always allowed. Unrecognized
readonly tag values are reported at the declaration site.`

const (
	// tagKey is the struct tag key this analyzer inspects.
	tagKey = "readonly"
	// tagExternal restricts writes to the declaring package.
	tagExternal = "external"
	// tagImmutable forbids reassignment everywhere.
	tagImmutable = "immutable"
)

// Analyzer is the readonly analyzer.
var Analyzer = &analysis.Analyzer{
	Name:     "readonly",
	Doc:      doc,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.AssignStmt)(nil),
		(*ast.IncDecStmt)(nil),
		(*ast.RangeStmt)(nil),
		(*ast.StructType)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range stmt.Lhs {
				checkWrite(pass, lhs)
			}
		case *ast.IncDecStmt:
			checkWrite(pass, stmt.X)
		case *ast.RangeStmt:
			if stmt.Tok == token.ASSIGN {
				if stmt.Key != nil {
					checkWrite(pass, stmt.Key)
				}
				if stmt.Value != nil {
					checkWrite(pass, stmt.Value)
				}
			}
		case *ast.StructType:
			checkTagValues(pass, stmt)
		}
	})
	return nil, nil
}

// checkTagValues reports readonly tags with unrecognized values so that a
// typo cannot silently leave a field unprotected.
func checkTagValues(pass *analysis.Pass, st *ast.StructType) {
	for _, f := range st.Fields.List {
		if f.Tag == nil {
			continue
		}
		raw, err := strconv.Unquote(f.Tag.Value)
		if err != nil {
			continue
		}
		if v, ok := reflect.StructTag(raw).Lookup(tagKey); ok && v != tagExternal && v != tagImmutable {
			pass.Reportf(f.Tag.Pos(), "invalid readonly tag value %q (valid values: %q, %q)", v, tagExternal, tagImmutable)
		}
	}
}

// checkWrite reports expr if it writes to or through a readonly field
// declared in another package. expr is the target of an assignment, ++/--,
// or range clause.
func checkWrite(pass *analysis.Pass, expr ast.Expr) {
	expr = ast.Unparen(expr)
	if star, ok := expr.(*ast.StarExpr); ok {
		checkStarStore(pass, star)
		return
	}
	// Walk the target expression inward so that writes into the contents of
	// a readonly field (order.User.Name, user.Items[0]) are caught, not just
	// reassignment of the field itself.
	for {
		switch e := expr.(type) {
		case *ast.SelectorExpr:
			if checkSelection(pass, e) {
				return
			}
			expr = ast.Unparen(e.X)
		case *ast.IndexExpr:
			expr = ast.Unparen(e.X)
		default:
			return
		}
	}
}

// checkSelection reports sel if any field on its selection path — including
// fields traversed implicitly through embedding — is readonly outside the
// current package. It reports whether a diagnostic was emitted.
func checkSelection(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	selection, ok := pass.TypesInfo.Selections[sel]
	if !ok || selection.Kind() != types.FieldVal {
		return false
	}
	t := selection.Recv()
	for _, idx := range selection.Index() {
		if ptr, ok := t.Underlying().(*types.Pointer); ok {
			t = ptr.Elem()
		}
		st, ok := t.Underlying().(*types.Struct)
		if !ok {
			return false
		}
		field := st.Field(idx)
		if m := tagMode(st.Tag(idx)); violates(pass, m, field) {
			pass.Reportf(sel.Sel.Pos(), "%s", describe(m, typeName(t), field))
			return true
		}
		t = field.Type()
	}
	return false
}

// checkStarStore reports *ptr = v when the pointed-to struct declares
// readonly fields in another package: such a store overwrites the protected
// fields wholesale.
func checkStarStore(pass *analysis.Pass, star *ast.StarExpr) {
	tv, ok := pass.TypesInfo.Types[star.X]
	if !ok {
		return
	}
	ptr, ok := tv.Type.Underlying().(*types.Pointer)
	if !ok {
		return
	}
	elem := ptr.Elem()
	st, ok := elem.Underlying().(*types.Struct)
	if !ok {
		return
	}
	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		if m := tagMode(st.Tag(i)); violates(pass, m, field) {
			pass.Reportf(star.Pos(), "cannot assign to *%s: %s", typeName(elem), describe(m, typeName(elem), field))
			return
		}
	}
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

// tagMode returns the protection mode declared by a raw struct tag, or ""
// if the field is unprotected (no tag, or an unrecognized value — those are
// reported separately at the declaration site).
func tagMode(tag string) string {
	switch v, _ := reflect.StructTag(tag).Lookup(tagKey); v {
	case tagExternal, tagImmutable:
		return v
	}
	return ""
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
