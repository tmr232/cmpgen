package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/constant"
	"go/format"
	"go/printer"
	"go/token"
	"log"
	"reflect"
	"slices"
	"strings"
	"text/template"

	"github.com/pkg/errors"

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

func isStringLiteral(node ast.Node) bool {
	if basicLit, ok := node.(*ast.BasicLit); ok {
		return basicLit.Kind == token.STRING
	}
	return false
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

type RegistryKey struct {
	Type   reflect.Type
	Fields string
}

var registry = make(map[RegistryKey]any)

func newRegistryKey[T any](fields ...string) RegistryKey {
	return RegistryKey{Type: reflect.TypeOf(*new(T)), Fields: strings.Join(fields, ", ")}
}

func Register[T any](fn any, fields ...string) {
	registry[newRegistryKey[T](fields...)] = fn
}

func CmpByFields[T any](field string, fields ...string) func(T, T) int {
	regKey := newRegistryKey[T](slices.Insert(fields, 0, field)...)
	if regFunc, ok := registry[regKey]; ok {
		return regFunc.(func(T, T) int)
	}
	panic(fmt.Sprintf("No comparator registered for CmpByFields[%s](%s)", regKey.Type.Name(), regKey.Fields))
}

func GenerateCompareFunc(typeName string, fields ...string) string {
	const tmpl = `Register[{{.TypeName}}](
		func (a, b {{.TypeName}}) int {
			return cmp.Or(
				{{- range .Fields}}
				cmp.Compare(a.{{.}}, b.{{.}}),
				{{- end}}
			)
		},
		{{join .QuotedFields ","}},
		)`

	funcTemplate := template.Must(template.New("compare").Funcs(template.FuncMap{
		"join": strings.Join,
	}).Parse(tmpl))

	quotedFields := make([]string, len(fields))
	for i, field := range fields {
		quotedFields[i] = fmt.Sprintf("\"%s\"", field)
	}

	data := struct {
		TypeName     string
		Fields       []string
		QuotedFields []string
	}{
		TypeName:     typeName,
		Fields:       fields,
		QuotedFields: quotedFields,
	}

	var buf bytes.Buffer
	if err := funcTemplate.Execute(&buf, data); err != nil {
		panic(err)
	}

	return buf.String()
}

func formatImports(imports []*ast.ImportSpec) string {
	importLines := make([]string, 0)
	for _, importSpec := range imports {
		if importSpec.Name != nil {
			importLines = append(importLines, fmt.Sprint(importSpec.Name.Name, importSpec.Path.Value))
		} else {
			importLines = append(importLines, importSpec.Path.Value)
		}
	}
	return fmt.Sprintf("import (\n\t%s\n)", strings.Join(importLines, "\n\t"))
}

func formatSource(src string) string {
	formattedSrc, err := format.Source([]byte(src))
	if err != nil {
		fmt.Println(src)
		panic(err)
	}
	return string(formattedSrc)
}

func formatNode(fset *token.FileSet, node any) (string, error) {
	var buf bytes.Buffer
	err := printer.Fprint(&buf, fset, node)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func generateComparatorsForFile(file *ast.File, fset *token.FileSet, typesInfo *types.Info) (string, error) {
	callInfos := make([]CallInfo, 0)
	for _, call := range findCallsIn(file, typesInfo, callTarget{"go-sort-by-key/cmd/gencmp", "CmpByFields"}) {
		callInfo := collectCallInfo(typesInfo, call)
		// Ensure all call arguments are string literals
		for _, arg := range callInfo.Arguments {
			if !isStringLiteral(arg.Expr) {
				return "", errors.New("All arguments must be string literals")
			}
		}
		// Ensure the type argument is reachable
		typeArg := callInfo.TypeArguments[0]
		if !typeArg.Reachable {
			return "", errors.New("Type argument must be reachable")
		}

		callInfos = append(callInfos, callInfo)
	}

	if len(callInfos) == 0 {
		// No calls - no need to generate anything!
		return "", nil
	}

	cmpFuncs := make([]string, 0)
	for _, callInfo := range callInfos {
		fields := make([]string, 0)
		for _, arg := range callInfo.Arguments {
			fields = append(fields, constant.StringVal(arg.TypeAndValue.Value))
		}
		typeArg, err := formatNode(fset, callInfo.TypeArguments[0].Expr)
		if err != nil {
			return "", errors.Wrap(err, "Failed to get type argument name")
		}
		cmpFuncs = append(cmpFuncs, GenerateCompareFunc(typeArg, fields...))
	}

	imports := formatImports(file.Imports)

	init := fmt.Sprintf("func init() {\n%s\n}", strings.Join(cmpFuncs, "\n\n"))

	source := fmt.Sprintf("package %s\n\n%s\n\n%s", file.Name.Name, imports, init)

	return formatSource(source), nil

}

func generateComparators(pkg *packages.Package) error {
	for _, syntax := range pkg.Syntax {
		for _, call := range findCallsIn(syntax, pkg.TypesInfo, callTarget{"go-sort-by-key/cmd/gencmp", "CmpByFields"}) {
			callInfo := collectCallInfo(pkg.TypesInfo, call)
			// Ensure we have fields to compare by
			if len(callInfo.Arguments) < 1 {
				return errors.New("Must provide fields for comparison")
			}
			// Ensure all call arguments are string literals
			for _, arg := range callInfo.Arguments {
				if !isStringLiteral(arg.Expr) {
					return errors.New("All arguments must be string literals")
				}
			}
			// Ensure the type argument is reachable
			typeArg := callInfo.TypeArguments[0]
			if !typeArg.Reachable {
				return errors.New("Type argument must be reachable")
			}

			fields := make([]string, 0)
			for _, arg := range callInfo.Arguments {
				fields = append(fields, constant.StringVal(arg.TypeAndValue.Value))
			}
			fmt.Println(GenerateCompareFunc(typeArg.Code(), fields...))
		}
		for _, importSpec := range syntax.Imports {
			name := "<>"
			if importSpec.Name != nil {
				name = importSpec.Name.Name
			}
			fmt.Println(importSpec.Path.Value, name)
		}
	}
	return nil
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

	// TODO: Separate the search for calls into separate files (pkg.Syntax) so that we can generate
	// 	     a file per file, and just copy the imports over instead of fiddling with them.
	for _, call := range findCallsTo(pkg, callTarget{"go-sort-by-key/cmd/gencmp", "targetFunc"}) {
		fmt.Println("Call:", pkg.Fset.Position(call.Pos()))
		callInfo := collectCallInfo(pkg.TypesInfo, call)
		for _, arg := range callInfo.Arguments {
			if !arg.Reachable {
				fmt.Println("Unreachable argument")
			}
			fmt.Println("Arg", arg.PackagePath(), arg.Code())
		}
		for _, typeArg := range callInfo.TypeArguments {
			if !typeArg.Reachable {
				fmt.Println("Unreachable type argument")
			}
			fmt.Println("Type Arg", typeArg.PackagePath(), typeArg.Code())
		}
	}
	fmt.Println(pkg.Imports)

	generateComparators(pkg)

	for _, s := range pkg.Syntax {
		fmt.Println(s.Name)
		gen, _ := generateComparatorsForFile(s, pkg.Fset, pkg.TypesInfo)
		fmt.Println(gen)
	}
}

func trySort() {
	CmpByFields[int]("A", "B")
	CmpByFields[global]("A", "B")
	CmpByFields[ast.BadDecl]("A", "B")
}

//go:generate go run github.com/tmr232/goat/cmd/goater
func main() {
	goat.Run(app)
}
