package gotype

import (
	"context"
	"go/build"
	"go/token"
	"go/types"
	"testing"

	"github.com/adamfaulkner/go-langserver/filter_ident"
	"github.com/stretchr/testify/assert"
)

/*
func TestStripAstFile(t *testing.T) {

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "/usr/lib/go/src/strings/builder.go", nil, 0)
	assert.NoError(t, err)

	idf := selector_walker.IdentFilter{
		Identifiers: map[string]struct{}{
			"Builder": struct{}{},
		},
	}

	importFilter := map[string]struct{}{
		"unsafe": struct{}{},
	}

	stripAstFile(file, importFilter, idf)

	for _, decl := range file.Decls {
		t.Log(decl)
	}

	assert.Len(t, file.Imports, 1)
	assert.Len(t, file.Decls, 12)
}
*/

func TestIntegration(t *testing.T) {
	packages := make(map[string]*types.Package)
	fset := token.NewFileSet()

	srcImporter := NewSourceImporter(context.Background(), &build.Default, fset, packages)

	fc := filter_ident.NewFilterComputation(&build.Default, []string{"/usr/lib/go/src/strings"})
	err := fc.Run()
	assert.NoError(t, err)

	srcImporter.importFilters = fc.ImportFilters
	t.Log("importFilters:", fc.ImportFilters)
	srcImporter.identFilters = fc.IdentFilters
	t.Log("identFilters", fc.IdentFilters)

	_, err = srcImporter.Import("strings")
	assert.NoError(t, err)
}
