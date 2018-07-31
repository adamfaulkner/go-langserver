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
	assert.Len(t, fc.importFilters, 0)

	// Strings package from standard lib.
	fc = NewFilterComputation(bctx, []string{
		"/usr/lib/go/src/strings",
	})

	err = fc.Run()
	assert.NoError(t, err)

	// As of this writing, this should have the following contents:
	// strings (all), io.Writer, unicode.SpecialCase, unsafe.Pointer
	assert.Len(t, fc.importFilters, 4)
	assert.Contains(t, fc.importFilters,
		"/usr/lib/go/src/strings",
		"/usr/lib/go/src/io",
		"/usr/lib/go/src/unicode",
		"/usr/lib/go/src/unsafe")
}

func TestComplicatedFilterComputation(t *testing.T) {
	bctx := &build.Default

	// Empty set of packages should complete immediately with no effects.
	fc := NewFilterComputation(bctx, []string{})
	err := fc.Run()
	assert.NoError(t, err)
	assert.Len(t, fc.importFilters, 0)

	// Strings package from standard lib.
	fc = NewFilterComputation(bctx, []string{
		"/home/adamf/vm_data/repos/server/go/src/dropbox/waterfall/internal/",
	})

	err = fc.Run()
	assert.NoError(t, err)

	// As of this writing, this should have the following contents:
	// strings (all), io.Writer, unicode.SpecialCase, unsafe.Pointer
	assert.Len(t, fc.importFilters, 4)
	assert.Contains(t, fc.importFilters,
		"/usr/lib/go/src/strings",
		"/usr/lib/go/src/io",
		"/usr/lib/go/src/unicode",
		"/usr/lib/go/src/unsafe")
}
