package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"

	"go/types"

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

func isCallTo(target callTarget, typesInfo *types.Info) func(*ast.CallExpr) bool {
	return func(queriedNode *ast.CallExpr) bool {

		var node ast.Node = queriedNode

		for {
			switch current := node.(type) {
			case *ast.CallExpr:
				node = current.Fun
			case *ast.SelectorExpr:
				node = current.Sel
			case *ast.Ident:
				definition, exists := typesInfo.Uses[current]
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
					typeArgs := typesInfo.Instances[current].TypeArgs
					for i := range typeArgs.Len() {
						fmt.Println(typeArgs.At(i))
					}
					return true
				}
				return false
			default:
				return false
			}
		}

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

func targetFunc[T any](t T) {}

func app(dir string) {
	goat.Flag(dir).Default("")

	pkg := loadPackages(dir)
	targetFunc(1)
	targetFunc("A")
	fmt.Println(findCallsTo(pkg, callTarget{"go-sort-by-key/cmd/gencmp", "targetFunc"}))

	for _, call := range findCallsTo(pkg, callTarget{"go-sort-by-key/cmd/gencmp", "targetFunc"}) {
		for _, arg := range call.Args {
			if isStringLiteral(arg) {
				fmt.Println(arg)
			}
		}
	}
}

//go:generate go run github.com/tmr232/goat/cmd/goater
func main() {
	goat.Run(app)
}
