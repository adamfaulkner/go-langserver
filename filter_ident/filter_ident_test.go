package filter_ident

import (
	"go/build"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterComputation(t *testing.T) {
	bctx := &build.Default

	// Empty set of packages should complete immediately with no effects.
	fc := NewFilterComputation(bctx, []string{})
	err := fc.Run()
	assert.NoError(t, err)
	assert.Len(t, fc.IdentFilters, 0)

	// Strings package from standard lib.
	fc = NewFilterComputation(bctx, []string{
		"/usr/lib/go/src/strings",
	})

	err = fc.Run()
	assert.NoError(t, err)

	// As of this writing, this should have the following contents:
	// strings (all), io.Writer, unicode.SpecialCase, unsafe.Pointer
	assert.Len(t, fc.IdentFilters, 4)
	assert.Contains(t, fc.IdentFilters,
		"/usr/lib/go/src/strings",
		"/usr/lib/go/src/io",
		"/usr/lib/go/src/unicode",
		"/usr/lib/go/src/unsafe")

	// Check importFilters.
	assert.Len(t, fc.ImportFilters, 1)
	assert.Contains(t, fc.ImportFilters,
		"/usr/lib/go/src/strings",
	)
	stringsImports := fc.ImportFilters["/usr/lib/go/src/strings"]
	assert.Len(t, stringsImports, 3)
	assert.Contains(
		t,
		stringsImports,
		"io",
		"unicode",
		"unsafe")
}

/*
func TestComplicatedFilterComputation(t *testing.T) {
	bctx := &build.Default

	// Empty set of packages should complete immediately with no effects.
	fc := NewFilterComputation(bctx, []string{})
	err := fc.Run()
	assert.NoError(t, err)
	assert.Len(t, fc.identFilters, 0)

	// Strings package from standard lib.
	fc = NewFilterComputation(bctx, []string{
	})

	err = fc.Run()
	assert.NoError(t, err)

	// As of this writing, this should have the following contents:
	// strings (all), io.Writer, unicode.SpecialCase, unsafe.Pointer
	assert.Len(t, fc.identFilters, 4)
	assert.Contains(t, fc.identFilters,
		"/usr/lib/go/src/strings",
		"/usr/lib/go/src/io",
		"/usr/lib/go/src/unicode",
		"/usr/lib/go/src/unsafe")

	assert.Len(t, fc.importFilters, 4)
}
*/
