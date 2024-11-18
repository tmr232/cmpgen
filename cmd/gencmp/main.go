package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"

	"go/types"

	"github.com/pkg/errors"
	"github.com/tmr232/goat"
	"golang.org/x/tools/go/packages"
)

func loadPackages(dir string) *packages.Package {
	cfg := &packages.Config{
		Mode:       packages.NeedTypes | packages.NeedTypesInfo | packages.NeedFiles | packages.NeedSyntax | packages.NeedName | packages.NeedImports | packages.NeedDeps,
		Context:    nil,
		Logf:       nil,
		Dir:        dir,
		Env:        nil,
		BuildFlags: nil,
		Fset:       nil,
		ParseFile:  nil,
		Tests:      false,
		Overlay:    nil,
	}

	pkgs, err := packages.Load(cfg)
	if err != nil {
		log.Fatal(err)
	}

	if len(pkgs) != 1 {
		log.Fatalf("Expected 1 package, found %d", len(pkgs))
	}

	return pkgs[0]
}

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

		definition, exists := typesInfo.Uses[ident]
		if !exists {
			return false
		}

		funcDef, isFunc := definition.(*types.Func)
		if !isFunc {
			return false
		}

		if funcDef.Pkg() == nil {
			return false
		}
		if funcDef.Pkg().Path() == target.PkgPath && funcDef.Name() == target.Name {
			return true
		}
		return false

	}
}

func findCallsTo(pkg *packages.Package, target callTarget) []*ast.CallExpr {
	var calls []*ast.CallExpr
	for _, syntax := range pkg.Syntax {
		calls = append(calls, findNodesIf(syntax, isCallTo(target, pkg.TypesInfo))...)
	}
	return calls
}

func isStringLiteral(node ast.Node) bool {
	if basicLit, ok := node.(*ast.BasicLit); ok {
		return basicLit.Kind == token.STRING
	}
	return false
}

type CallInfo struct {
	call     *ast.CallExpr
	ident    *ast.Ident
	typeArgs *types.TypeList
	args     []types.TypeAndValue
}

func isPackageScope(scope *types.Scope) bool {
	return scope.Parent() == types.Universe
}

func isReachableScope(scope *types.Scope) bool {
	return isPackageScope(scope) || scope == types.Universe
}

func collectCallInfo(pkg *packages.Package, call *ast.CallExpr) (CallInfo, error) {
	ident := getCallIdent(call)

	typeArgs := pkg.TypesInfo.Instances[ident].TypeArgs
	// Ensure we can use the type args
	for i := range typeArgs.Len() {
		typeArg := typeArgs.At(i)
		if !isUsableType(typeArg) {
			return CallInfo{}, errors.New("Type argument must be basic or named")
		}
	}

	args := make([]types.TypeAndValue, len(call.Args))
	// Arguments must either be constants or be defined in the package scope
	for i, callArg := range call.Args {
		argTypeAndValue := pkg.TypesInfo.Types[callArg]
		args[i] = argTypeAndValue

		// If we have a constant, all is well
		if argTypeAndValue.Value != nil {
			continue
		}

		// If the argument is named, make sure it's in a suitable scope
		argIdent := getArgIdent(callArg)
		if argIdent != nil {
			def := pkg.TypesInfo.Uses[argIdent]
			if !isReachableScope(def.Parent()) {
				return CallInfo{}, errors.New("Named arguments must be reachable")
			}
		}
	}

	return CallInfo{
		call:     call,
		ident:    ident,
		typeArgs: typeArgs,
		args:     args,
	}, nil
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

func targetFunc[T any](t T) {}

type global struct{}

var x = 2

func app(dir string) {
	goat.Flag(dir).Default("")
	type MyType struct {
		X int
	}
	pkg := loadPackages(dir)
	targetFunc(1)
	targetFunc("A")
	targetFunc(global{})
	targetFunc(struct{}{})
	a := 1
	targetFunc(a)
	targetFunc(x)
	targetFunc(types.Universe)
	targetFunc(&x)
	targetFunc(MyType{x})
	targetFunc[MyType](MyType{x})
	fmt.Println(findCallsTo(pkg, callTarget{"go-sort-by-key/cmd/gencmp", "targetFunc"}))

	for _, call := range findCallsTo(pkg, callTarget{"go-sort-by-key/cmd/gencmp", "targetFunc"}) {
		fmt.Println("Call:", pkg.Fset.Position(call.Pos()))
		_, err := collectCallInfo(pkg, call)
		if err != nil {
			fmt.Println("Error at", pkg.Fset.Position(call.Pos()))
			fmt.Println(err)
		}
	}

}

//go:generate go run github.com/tmr232/goat/cmd/goater
func main() {
	goat.Run(app)
}
