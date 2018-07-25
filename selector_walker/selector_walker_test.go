package gotype

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectorWalker(t *testing.T) {
	// Import a package so we have something to work with.
	bctx := build.Default
	p, err := bctx.Import("strings", "", 0)
	assert.NoError(t, err)
	dir := p.Dir

	// Check strings/reader. It should only contain one io.Writer.
	file := filepath.Join(dir, "reader.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	assert.NoError(t, err)

	allIdents := identFilter{
		all: true,
	}

	walker := NewSelectorWalker(f, allIdents)

	selector, err := walker.NextSelector()
	assert.NoError(t, err)
	assert.Equal(t, selector.Sel.Name, "Writer")
	assert.IsType(t, &ast.Ident{}, selector.X)
	pkg := selector.X.(*ast.Ident).Name
	assert.Equal(t, pkg, "io")

	selector, err = walker.NextSelector()
	assert.Error(t, err)

	// Check strings/strings. It should contain three references to
	// unicode.SpecialCase.

	file = filepath.Join(dir, "strings.go")
	f, err = parser.ParseFile(fset, file, nil, 0)
	assert.NoError(t, err)
	walker = NewSelectorWalker(f, allIdents)

	for i := 0; i < 3; i++ {
		selector, err := walker.NextSelector()
		assert.NoError(t, err)
		assert.Equal(t, selector.Sel.Name, "SpecialCase")
		assert.IsType(t, &ast.Ident{}, selector.X)
		pkg := selector.X.(*ast.Ident).Name
		assert.Equal(t, pkg, "unicode")
	}
	selector, err = walker.NextSelector()
	assert.Error(t, err)
}
