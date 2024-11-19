package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/constant"
	"go/format"
	"go/printer"
	"go/token"
	"strings"
	"text/template"

	"github.com/pkg/errors"

	"go/types"

	"github.com/tmr232/cmpgen"
	"github.com/tmr232/cmpgen/callector"
	"github.com/tmr232/goat"
	"golang.org/x/tools/go/packages"
)

func loadPackages(dir string) *packages.Package {
	pkg, err := callector.LoadPackage(dir)
	if err != nil {
		panic(err)
	}

	return pkg
}

func GenerateCompareFunc(typeName string, fields ...string) string {
	// TODO: Ensure the package names are imported under these names.
	const tmpl = `cmpgen.Register[{{.TypeName}}](
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

func formatImports(imports []*ast.ImportSpec, extraImports ...string) string {
	importLines := make([]string, 0)
	for _, importSpec := range imports {
		if importSpec.Name != nil {
			importLines = append(importLines, fmt.Sprint(importSpec.Name.Name, importSpec.Path.Value))
		} else {
			importLines = append(importLines, importSpec.Path.Value)
		}
	}
	importLines = append(importLines, extraImports...)
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
func isStringLiteral(node ast.Node) bool {
	if basicLit, ok := node.(*ast.BasicLit); ok {
		return basicLit.Kind == token.STRING
	}
	return false
}

func generateComparatorsForFile(file *ast.File, fset *token.FileSet, typesInfo *types.Info) (string, error) {
	callInfos := callector.CollectCalls("github.com/tmr232/cmpgen", "CmpByFields", file, fset, typesInfo)
	for _, callInfo := range callInfos {
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

	imports := formatImports(file.Imports, "\"cmp\"")

	init := fmt.Sprintf("func init() {\n%s\n}", strings.Join(cmpFuncs, "\n\n"))

	source := fmt.Sprintf("package %s\n\n%s\n\n%s", file.Name.Name, imports, init)

	return formatSource(source), nil

}

func targetFunc[T any](t T) {}

type global struct{}

var x = 2

func app(dir string) {
	goat.Flag(dir).Default("")
	fmt.Println(dir)
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
	// fmt.Println(findCallsTo(pkg, callTarget{"go-sort-by-key/cmd/gencmp", "targetFunc"}))

	// for _, call := range findCallsTo(pkg, callTarget{"go-sort-by-key/cmd/gencmp", "targetFunc"}) {
	// 	fmt.Println("Call:", pkg.Fset.Position(call.Pos()))
	// 	callInfo := collectCallInfo(pkg.TypesInfo, call)
	// 	for _, arg := range callInfo.Arguments {
	// 		if !arg.Reachable {
	// 			fmt.Println("Unreachable argument")
	// 		}
	// 		fmt.Println("Arg", arg.PackagePath(), arg.Code())
	// 	}
	// 	for _, typeArg := range callInfo.TypeArguments {
	// 		if !typeArg.Reachable {
	// 			fmt.Println("Unreachable type argument")
	// 		}
	// 		fmt.Println("Type Arg", typeArg.PackagePath(), typeArg.Code())
	// 	}
	// }

	// generateComparators(pkg)

	for _, s := range pkg.Syntax {
		gen, _ := generateComparatorsForFile(s, pkg.Fset, pkg.TypesInfo)
		fmt.Println(gen)
	}
}

func trySort() {
	cmpgen.CmpByFields[int]("A", "B")
	cmpgen.CmpByFields[global]("A", "B")
	cmpgen.CmpByFields[ast.BadDecl]("A", "B")
}

//go:generate go run github.com/tmr232/goat/cmd/goater
func main() {
	goat.Run(app)
}
