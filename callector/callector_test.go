package callector_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tmr232/cmpgen/callector"
)

var tmpl = `
package main

import (
	"go/types"
)

func targetFunc[T any](t T) {}

type globalStruct struct{}

var globalVar = 2

func main() {
	type MyType struct {
		X int
	}

	localVar := 1
	
	targetFunc(%s)
}
`
var modFile = `module my-test-package

go 1.23.0`

func collectForArg(t *testing.T, argument string) callector.Argument {
	tmpdir, err := os.MkdirTemp(".", "temptestdata")
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(tmpdir)

	script := fmt.Sprintf(tmpl, argument)
	os.WriteFile(filepath.Join(tmpdir, "main.go"), []byte(script), 0644)
	os.WriteFile(filepath.Join(tmpdir, "go.mod"), []byte(modFile), 0644)

	pkg, err := callector.LoadPackage(tmpdir)
	if err != nil {
		t.Error(err)
	}

	calls := callector.CollectCalls("my-test-package", "targetFunc", pkg.Syntax[0], pkg.Fset, pkg.TypesInfo)

	require.Len(t, calls, 1, "there should be only one call")

	callInfo := calls[0]
	require.Len(t, callInfo.Arguments, 1, "there should be only one argument")

	arg := callInfo.Arguments[0]
	return arg
}

func TestReachableArguments(t *testing.T) {
	tests := []struct {
		Name      string
		Arg       string
		Reachable bool
	}{
		{"int literal", "1", true},
		{"string literal", "\"A\"", true},
		{"package-scope struct", "globalStruct{}", true},
		{"anonymous struct", "struct{}{}", true},
		{"function-scope variable", "localVar", false},
		{"package-scope variable", "globalVar", true},
		{"imported variable", "types.Universe", true},
		{"pointer to package-scope variable", "&globalVar", true},
		{"function-scope type", "MyType{x}", false},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			arg := collectForArg(t, tt.Arg)
			require.Equal(t, tt.Reachable, arg.Reachable)
		})

	}

}
