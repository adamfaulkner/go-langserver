package selector_walker

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

	allIdents := IdentFilter{
		All: true,
	}

	walker := NewSelectorWalker(f, allIdents, f.Scope.Objects)

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
	walker = NewSelectorWalker(f, allIdents, f.Scope.Objects)

	for i := 0; i < 3; i++ {
		selector, err := walker.NextSelector()
		assert.NoError(t, err)
		assert.Equal(t, selector.Sel.Name, "SpecialCase")
		assert.IsType(t, &ast.Ident{}, selector.X)
		pkg := selector.X.(*ast.Ident).Name
		assert.Equal(t, pkg, "unicode")
	}
	_, err = walker.NextSelector()
	assert.Error(t, err)
	assert.Equal(t, err, SelectorWalkerFinished)

	// Now set an ident filter and make sure it filters identifiers
	// appropriately.
	idFilter := IdentFilter{
		Identifiers: map[string]struct{}{
			"ToLowerSpecial": struct{}{},
		},
	}

	walker = NewSelectorWalker(f, idFilter, f.Scope.Objects)
	selector, err = walker.NextSelector()
	assert.NoError(t, err)

	assert.Equal(t, selector.Sel.Name, "SpecialCase")
	assert.IsType(t, &ast.Ident{}, selector.X)
	pkg = selector.X.(*ast.Ident).Name
	assert.Equal(t, pkg, "unicode")

	_, err = walker.NextSelector()
	assert.Error(t, err)
	assert.Equal(t, err, SelectorWalkerFinished)

	// Make sure ident filter works with receivery functions.
	// If this works correctly, we should find io.Writer.
	file = filepath.Join(dir, "reader.go")
	f, err = parser.ParseFile(fset, file, nil, 0)
	assert.NoError(t, err)
	idFilter = IdentFilter{
		Identifiers: map[string]struct{}{
			"Reader": struct{}{},
		},
	}

	walker = NewSelectorWalker(f, idFilter, f.Scope.Objects)
	selector, err = walker.NextSelector()
	assert.NoError(t, err)

	assert.Equal(t, selector.Sel.Name, "Writer")
	assert.IsType(t, &ast.Ident{}, selector.X)
	pkg = selector.X.(*ast.Ident).Name
	assert.Equal(t, pkg, "io")

	_, err = walker.NextSelector()
	assert.Error(t, err)
	assert.Equal(t, err, SelectorWalkerFinished)
}
