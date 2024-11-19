package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/constant"
	"go/printer"
	"go/token"
	"os"
	"strings"
	"text/template"

	"go/types"

	"github.com/pkg/errors"
	"golang.org/x/tools/imports"

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

func removeUnusedImports(filename, src string) (string, error) {

	processed, err := imports.Process(filename, []byte(src), nil)
	if err != nil {
		return "", fmt.Errorf("import processing error: %v", err)
	}

	return string(processed), nil
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

	return source, nil

}

func app(dir string) {
	goat.Flag(dir).Default("")

	pkg := loadPackages(dir)

	for _, file := range pkg.Syntax {
		code, err := generateComparatorsForFile(file, pkg.Fset, pkg.TypesInfo)
		if err != nil {
			panic(err)
		}
		if code == "" {
			continue
		}

		filepath := pkg.Fset.File(file.Pos()).Name()
		filepath = strings.TrimSuffix(filepath, ".go")
		filepath = filepath + "_cmpgen.go"

		code, err = removeUnusedImports(filepath, code)
		if err != nil {
			panic(err)
		}

		err = os.WriteFile(filepath, []byte(code), 0644)
		if err != nil {
			panic(err)
		}
	}
}

//go:generate go run github.com/tmr232/goat/cmd/goater
func main() {
	goat.Run(app)
}
