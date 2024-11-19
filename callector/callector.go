package callector

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

func findNodesIf[T ast.Node](file *ast.File, pred func(node T) bool) []T {
	var matchingNodes []T
	for _, decl := range file.Decls {
		ast.Inspect(decl, func(node ast.Node) bool {
			if typedNode, isRightType := node.(T); isRightType {
				if pred(typedNode) {
					matchingNodes = append(matchingNodes, typedNode)
					// We recurse the entire AST without stopping as there may be
					// nested calls when we create subcommands.
				}
			}
			return true
		})
	}
	return matchingNodes
}

type callTarget struct {
	PkgPath string
	Name    string
}

func getCallIdent(callExpr *ast.CallExpr) *ast.Ident {
	var node ast.Node = callExpr

	for {
		switch current := node.(type) {
		case *ast.CallExpr:
			node = current.Fun
		case *ast.IndexExpr:
			node = current.X
		case *ast.SelectorExpr:
			node = current.Sel
		case *ast.Ident:
			return current
		default:
			return nil
		}
	}
}

func getArgIdent(arg ast.Expr) *ast.Ident {
	var node ast.Node = arg
	for {
		switch current := node.(type) {
		case *ast.SelectorExpr:
			node = current.Sel
		case *ast.Ident:
			return current
		case *ast.UnaryExpr:
			node = current.X
		default:
			return nil
		}
	}
}

func isCallTo(target callTarget, typesInfo *types.Info) func(*ast.CallExpr) bool {
	return func(node *ast.CallExpr) bool {

		ident := getCallIdent(node)
		if ident == nil {
			//TODO: Does this ever happen?
			return false
		}
		fmt.Println("Ident", ident, target)

		definition, exists := typesInfo.Uses[ident]
		if !exists {
			fmt.Println("Doesn't exist")
			return false
		}

		funcDef, isFunc := definition.(*types.Func)
		if !isFunc {
			return false
		}

		if funcDef.Pkg() == nil {
			return false
		}
		fmt.Println(funcDef.Pkg().Path(), funcDef.Name())
		if funcDef.Pkg().Path() == target.PkgPath && funcDef.Name() == target.Name {
			return true
		}
		return false

	}
}

func findCallsIn(syntax *ast.File, typesInfo *types.Info, target callTarget) []*ast.CallExpr {
	return findNodesIf(syntax, isCallTo(target, typesInfo))
}
func findCallsTo(pkg *packages.Package, target callTarget) []*ast.CallExpr {
	var calls []*ast.CallExpr
	for _, syntax := range pkg.Syntax {
		calls = append(calls, findCallsIn(syntax, pkg.TypesInfo, target)...)
	}
	return calls
}

func getTypePackagePath(t types.Type) string {
	switch t := t.(type) {
	case *types.Basic:
		return ""
	case *types.Named:
		return t.Obj().Pkg().Path()
	case *types.Pointer:
		return getTypePackagePath(t.Elem())
	default:
		panic("This should never happen as all types passed here should've been reachable")
	}
}

func getValuePackagePath(def types.Object) string {
	return def.Pkg().Path()
}

type TypeArg struct {
	Type      types.Type
	Expr      ast.Expr
	Reachable bool
}

func (t TypeArg) PackagePath() string {
	if !t.Reachable {
		return ""
	}
	return getTypePackagePath(t.Type)
}

func (t TypeArg) Code() string {
	return t.Type.String()
}

type Argument struct {
	TypeAndValue types.TypeAndValue
	Expr         ast.Expr
	Def          types.Object
	Reachable    bool
}

func (a Argument) PackagePath() string {
	if a.Def == nil {
		return ""
	}
	return a.Def.Pkg().Path()
}

func (a Argument) Code() string {
	if a.TypeAndValue.Value != nil {
		return a.TypeAndValue.Value.ExactString()
	}
	if a.Def != nil {
		return a.Def.Name()
	}
	return ""
}

type CallInfo struct {
	Call          *ast.CallExpr
	TypeArguments []TypeArg
	Arguments     []Argument
}

func isPackageScope(scope *types.Scope) bool {
	return scope.Parent() == types.Universe
}

func isReachableScope(scope *types.Scope) bool {
	return isPackageScope(scope) || scope == types.Universe
}

func getCallTypeArgs(call *ast.CallExpr) []ast.Expr {
	switch expr := call.Fun.(type) {
	case *ast.IndexExpr:
		return []ast.Expr{expr.Index}
	case *ast.IndexListExpr:
		return expr.Indices
	default:
		return make([]ast.Expr, 0)
	}
}

func collectCallInfo(typesInfo *types.Info, call *ast.CallExpr) CallInfo {
	ident := getCallIdent(call)

	typeArgs := typesInfo.Instances[ident].TypeArgs
	typeArguments := make([]TypeArg, typeArgs.Len())
	astTypeArgs := getCallTypeArgs(call)
	for i := range typeArgs.Len() {
		typeArg := typeArgs.At(i)
		reachable := isUsableType(typeArg)
		var expr ast.Expr
		if i < len(astTypeArgs) {
			expr = astTypeArgs[i]
		}
		typeArguments[i] = TypeArg{Type: typeArg, Reachable: reachable, Expr: expr}
	}

	arguments := make([]Argument, len(call.Args))
	// Arguments must either be constants or be defined in the package scope
	for i, callArg := range call.Args {
		argTypeAndValue := typesInfo.Types[callArg]

		// If we have a constant, all is well
		if argTypeAndValue.Value != nil {
			arguments[i] = Argument{TypeAndValue: argTypeAndValue, Def: nil, Reachable: true, Expr: callArg}
		} else {

			// If the argument is named, make sure it's in a suitable scope
			argIdent := getArgIdent(callArg)
			if argIdent == nil {
				arguments[i] = Argument{TypeAndValue: argTypeAndValue, Def: nil, Reachable: true}
			} else { // argIdent != nil
				def := typesInfo.Uses[argIdent]
				arguments[i] = Argument{TypeAndValue: argTypeAndValue, Def: def, Reachable: isReachableScope(def.Parent())}
			}
		}

	}

	return CallInfo{
		Call:          call,
		TypeArguments: typeArguments,
		Arguments:     arguments,
	}
}

func isUsableType(t types.Type) bool {
	switch t := t.(type) {
	case *types.Basic:
		return true
	case *types.Named:
		return isReachableScope(t.Obj().Parent())
	case *types.Pointer:
		return isUsableType(t.Elem())
	default:
		return false
	}
}

func CollectCalls(pkg, function string, file *ast.File, fset *token.FileSet, typesInfo *types.Info) []CallInfo {
	calls := findCallsIn(file, typesInfo, callTarget{pkg, function})
	callInfos := make([]CallInfo, len(calls))
	for i, call := range calls {
		callInfos[i] = collectCallInfo(typesInfo, call)
	}
	return callInfos
}

func LoadPackage(dir string) (*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo | packages.NeedFiles | packages.NeedSyntax | packages.NeedName | packages.NeedImports | packages.NeedDeps,
		Dir:  dir,
	}

	pkgs, err := packages.Load(cfg)
	if err != nil {
		return nil, err
	}
	return pkgs[0], nil
}
