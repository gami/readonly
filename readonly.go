// Package readonly defines an Analyzer that reports reassignment of struct
// fields marked with the `readonly:"external"` tag from outside the package
// that declares them.
//
// Fields tagged `readonly:"external"` may stay exported (e.g. for ORM or
// JSON serialization), but any assignment to them from another package is
// reported. Assignment within the declaring package and initialization via
// composite literals are allowed.
package readonly

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const doc = `readonly reports reassignment of fields tagged readonly:"external" from outside their declaring package

Fields can stay exported for ORM/JSON purposes while assignments like

	user.Status = StatusDeleted

are rejected unless they occur in the package that declares the field.
Composite literal initialization (model.User{Status: s}) is always allowed.`

// tagKey is the struct tag key this analyzer inspects.
const tagKey = "readonly"

// tagExternal restricts assignment to the declaring package.
const tagExternal = "external"

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
				checkWrite(pass, stmt.Key)
				checkWrite(pass, stmt.Value)
			}
		}
	})
	return nil, nil
}

// checkWrite reports expr if it writes to a tagged field declared in another
// package. expr is the target of an assignment, ++/--, or range clause.
func checkWrite(pass *analysis.Pass, expr ast.Expr) {
	if expr == nil {
		return
	}
	sel, ok := ast.Unparen(expr).(*ast.SelectorExpr)
	if !ok {
		return
	}
	selection, ok := pass.TypesInfo.Selections[sel]
	if !ok || selection.Kind() != types.FieldVal {
		return
	}
	field, owner := resolveField(selection)
	if field == nil {
		return
	}
	tag, ok := reflect.StructTag(owner.tag).Lookup(tagKey)
	if !ok || tag != tagExternal {
		return
	}
	if field.Pkg() == nil || field.Pkg().Path() == pass.Pkg.Path() {
		return
	}
	pass.Reportf(sel.Pos(), "field %s.%s is readonly outside package %s",
		owner.name, field.Name(), field.Pkg().Path())
}

// fieldOwner describes the struct that directly declares a field, along with
// the field's raw struct tag.
type fieldOwner struct {
	name string // name of the declaring named type, or "struct" if anonymous
	tag  string // raw struct tag of the field
}

// resolveField walks the selection's index path (which may traverse embedded
// fields) and returns the final field together with its declaring struct.
func resolveField(sel *types.Selection) (*types.Var, fieldOwner) {
	t := sel.Recv()
	var field *types.Var
	var owner fieldOwner
	for _, idx := range sel.Index() {
		if ptr, ok := t.Underlying().(*types.Pointer); ok {
			t = ptr.Elem()
		}
		st, ok := t.Underlying().(*types.Struct)
		if !ok {
			return nil, fieldOwner{}
		}
		owner.name = "struct"
		if named, ok := types.Unalias(t).(*types.Named); ok {
			owner.name = named.Obj().Name()
		}
		field = st.Field(idx)
		owner.tag = st.Tag(idx)
		t = field.Type()
	}
	return field, owner
}
