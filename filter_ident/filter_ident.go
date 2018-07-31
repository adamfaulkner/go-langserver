package filter_ident

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"sync"

	"github.com/adamfaulkner/go-langserver/import_resolver"
	"github.com/adamfaulkner/go-langserver/selector_walker"
)

// A FilterComputation is a traversal of an import graph to find the set of
// identifiers that we care about.
type FilterComputation struct {
	// importFilters maps package directory to IdentFilter.
	importFilters map[string]selector_walker.IdentFilter
	// nextPackages is the set of packages directories that remain to be processed.
	nextPackages map[string]struct{}

	bctx *build.Context
	fset *token.FileSet
	ir   *import_resolver.ImportResolver
}

func NewFilterComputation(bctx *build.Context, packageDirs []string) *FilterComputation {
	importF := map[string]selector_walker.IdentFilter{}
	nextPackages := map[string]struct{}{}
	for _, packageDir := range packageDirs {
		importF[packageDir] = selector_walker.IdentFilter{
			All: true,
		}
		nextPackages[packageDir] = struct{}{}
	}

	return &FilterComputation{
		importFilters: importF,
		nextPackages:  nextPackages,
		bctx:          bctx,
		fset:          token.NewFileSet(),
		ir:            import_resolver.NewImportResolver(bctx),
	}
}

func (f *FilterComputation) Run() error {
	for len(f.nextPackages) > 0 {
		var nextPackageDir string
		for nextPackageDir = range f.nextPackages {
			break
		}
		delete(f.nextPackages, nextPackageDir)

		err := f.processPackageDir(nextPackageDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *FilterComputation) processPackageDir(pD string) error {
	pkg, err := f.bctx.ImportDir(pD, 0)
	if err != nil {
		return err
	}

	// TODO(adamf): Test Mode
	files, err := f.parseFiles(pD, pkg.GoFiles)
	if err != nil {
		return err
	}

	idf, ok := f.importFilters[pD]
	if !ok {
		return errors.New("process package before import filter added?")
	}

	for _, file := range files {
		err = f.processFile(file, pD, idf)
		if err != nil {
			return err
		}
	}

	return nil
}

func (f *FilterComputation) parseFiles(dir string, filenames []string) ([]*ast.File, error) {
	open := f.bctx.OpenFile // possibly nil

	files := make([]*ast.File, len(filenames))
	errors := make([]error, len(filenames))

	var wg sync.WaitGroup
	wg.Add(len(filenames))
	for i, filename := range filenames {
		go func(i int, filepath string) {
			defer wg.Done()
			if open != nil {
				src, err := open(filepath)
				if err != nil {
					errors[i] = fmt.Errorf("opening package file %s failed (%v)", filepath, err)
					return
				}
				files[i], errors[i] = parser.ParseFile(f.fset, filepath, src, 0)
				src.Close() // ignore Close error - parsing may have succeeded which is all we need
			} else {
				// Special-case when ctxt doesn't provide a custom OpenFile and use the
				// parser's file reading mechanism directly. This appears to be quite a
				// bit faster than opening the file and providing an io.ReaderCloser in
				// both cases.
				// TODO(gri) investigate performance difference (issue #19281)
				files[i], errors[i] = parser.ParseFile(f.fset, filepath, nil, 0)
			}
		}(i, filepath.Join(dir, filename))
	}
	wg.Wait()

	// if there are errors, return the first one for deterministic results
	for _, err := range errors {
		if err != nil {
			return nil, err
		}
	}

	return files, nil
}

func (f *FilterComputation) processFile(file *ast.File, sourceDir string, idf selector_walker.IdentFilter) error {
	sw := selector_walker.NewSelectorWalker(file, idf)
	importsMap, err := f.ir.Resolve(file, sourceDir)
	if err != nil {
		return err
	}

	sexpr, err := sw.NextSelector()
	for err == nil {
		err = f.processSexpr(sexpr, importsMap)
		if err != nil {
			return err
		}
		sexpr, err = sw.NextSelector()
	}

	if err == selector_walker.SelectorWalkerFinished {
		return nil
	}

	return err
}

func (f *FilterComputation) processSexpr(sexpr ast.SelectorExpr, importsMap map[string]string) error {
	var packageName string
	switch xT := sexpr.X.(type) {
	case *ast.Ident:
		packageName = xT.String()
	default:
		return fmt.Errorf("Invalid sexpr. Not an identifier. %v %t", sexpr, sexpr.X)
	}

	pkgDir, ok := importsMap[packageName]
	if !ok {
		return fmt.Errorf("Unknown import: %s", packageName)
	}

	edgeImportFilter, ok := f.importFilters[pkgDir]
	if !ok {
		edgeImportFilter = selector_walker.IdentFilter{
			All:         false,
			Identifiers: map[string]struct{}{},
		}
		f.importFilters[pkgDir] = edgeImportFilter
	}

	ident := sexpr.Sel.Name
	present := edgeImportFilter.CheckIdent(ident)
	if !present {
		edgeImportFilter.Identifiers[ident] = struct{}{}
		f.nextPackages[pkgDir] = struct{}{}
	}
	return nil
}
