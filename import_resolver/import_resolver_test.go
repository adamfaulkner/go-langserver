package import_resolver

import (
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImportResolver(t *testing.T) {
	bctx := build.Default
	p, err := bctx.Import("strings", "", 0)
	assert.NoError(t, err)
	dir := p.Dir

	// Check strings/reader. It should only contain one io.Writer.
	file := filepath.Join(dir, "reader.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	assert.NoError(t, err)

	ir := NewImportResolver(&bctx)
	imports, err := ir.Resolve(f, dir)
	assert.NoError(t, err)
	assert.Equal(t, len(imports), 3)
	assert.NotZero(t, imports["errors"])
	assert.NotZero(t, imports["io"])
	assert.NotZero(t, imports["utf8"])

	assert.Len(t, ir.pkgCache, 3)
	assert.Len(t, ir.findCache, 3)
}
