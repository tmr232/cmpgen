package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	approvals "github.com/approvals/go-approval-tests"
	"github.com/tmr232/cmpgen/callector"
)

func TestGeneration(t *testing.T) {
	pkg, err := callector.LoadPackage("./testdata/package/")
	require.Nil(t, err)

	for _, file := range pkg.Syntax {
		filename := filepath.Base(pkg.Fset.File(file.Pos()).Name())
		t.Run(strings.TrimSuffix(filename, ".go"), func(t *testing.T) {
			code, err := GenerateComparatorsForFile(file, pkg.Fset, pkg.TypesInfo)
			require.Nil(t, err)

			approvals.UseFolder(filepath.Join("testdata", "approvals"))
			approvals.VerifyString(t, code, approvals.Options().WithExtension(".go"))
		})
	}
}
