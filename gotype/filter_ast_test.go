package gotype

/*
func TestSelectorWalker(t *testing.T) {
	// Import a package so we have something to work with.
	bctx := build.Default
	p, err := bctx.Import("strings", "", 0)
	assert.NoError(t, err)
	dir := p.Dir
	file := filepath.Join(dir, "reader.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)

	allIdents := identFilter{
		all: true,
	}

	walker := NewSelectorWalker(f, allIdents)

	selector, err := walker.NextSelector()
	assert.NoError(t, err)
	assert.Equal(t, selector.Sel.Name, "io")

}
*/
