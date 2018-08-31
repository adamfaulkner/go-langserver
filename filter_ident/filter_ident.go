package filter_ident

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"log"
	"path/filepath"
	"sync"

	"github.com/adamfaulkner/go-langserver/import_resolver"
	"github.com/adamfaulkner/go-langserver/selector_walker"
)

// A FilterComputation is a traversal of an import graph to find the set of
// identifiers that we care about.
type FilterComputation struct {
	// identFilters maps package directory to IdentFilter.
	IdentFilters map[string]selector_walker.IdentFilter

	// importFilters maps package directory to set of import paths to
	// perserve.
	ImportFilters map[string]map[string]struct{}

	// nextPackages is the set of packages directories that remain to be processed.
	nextPackages map[string]struct{}

	parseCache *sync.Map

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
		IdentFilters:  importF,
		ImportFilters: make(map[string]map[string]struct{}),
		nextPackages:  nextPackages,
		bctx:          bctx,
		fset:          token.NewFileSet(),
		ir:            import_resolver.NewImportResolver(bctx),
		parseCache:    &sync.Map{},
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

func mergeScopes(fs []*ast.File) map[string]*ast.Object {
	megascope := make(map[string]*ast.Object)
	for _, f := range fs {
		for k, v := range f.Scope.Objects {
			megascope[k] = v
		}

	}
	return megascope
}

func mergeImports(ir *import_resolver.ImportResolver, fs []*ast.File, sourceDir string) (map[string]import_resolver.Import, error) {
	mergeimp := make(map[string]import_resolver.Import)
	for _, f := range fs {
		imports, err := ir.Resolve(f, sourceDir)
		if err != nil {
			return nil, err
		}
		for name, imp := range imports {
			mergeimp[name] = imp
		}
	}
	return mergeimp, nil
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

	idf, ok := f.IdentFilters[pD]
	if !ok {
		return errors.New("process package before import filter added?")
	}

	packageScope := mergeScopes(files)
	packageImports, err := mergeImports(f.ir, files, pD)
	if err != nil {
		return err
	}
	for _, file := range files {
		err = f.processFile(file, pD, idf, packageScope, packageImports)
		if err != nil {
			return err
		}
	}

	return nil
}

func (f *FilterComputation) parseFiles(dir string, filenames []string) ([]*ast.File, error) {

	files := make([]*ast.File, len(filenames))
	errors := make([]error, len(filenames))

	var wg sync.WaitGroup
	wg.Add(len(filenames))
	for i, filename := range filenames {
		path := filepath.Join(dir, filename)
		go func(i int) {
			files[i], errors[i] = f.parseFile(path)
			wg.Done()
		}(i)
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

func (f *FilterComputation) parseFile(path string) (*ast.File, error) {
	pcFile, ok := f.parseCache.Load(path)
	if ok {
		return pcFile.(*ast.File), nil
	}

	open := f.bctx.OpenFile // possibly nil
	var file *ast.File
	var err error
	if open != nil {
		src, err := open(path)
		if err != nil {
			err = fmt.Errorf("opening package file %s failed (%v)", path, err)
			return nil, err
		}
		file, err = parser.ParseFile(f.fset, path, src, 0)
		src.Close() // ignore Close error - parsing may have succeeded which is all we need
	} else {
		// Special-case when ctxt doesn't provide a custom OpenFile and use the
		// parser's file reading mechanism directly. This appears to be quite a
		// bit faster than opening the file and providing an io.ReaderCloser in
		// both cases.
		// TODO(gri) investigate performance difference (issue #19281)
		file, err = parser.ParseFile(f.fset, path, nil, 0)
	}
	f.parseCache.Store(path, file)

	return file, err
}

func (f *FilterComputation) processFile(
	file *ast.File,
	sourceDir string,
	idf selector_walker.IdentFilter,
	packageScope map[string]*ast.Object,
	packageImports map[string]import_resolver.Import,
) error {

	sw := selector_walker.NewSelectorWalker(file, idf, packageScope)

	sexpr, err := sw.NextSelector()
	for err == nil {
		err = f.processSexpr(sexpr, sourceDir, packageImports)
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

// TODO(adamf): Move where it makes sense.
// TODO(adamf): Incomplete
func sexprToPackageIdent(sexpr ast.SelectorExpr) (string, string, error) {
	switch xT := sexpr.X.(type) {
	case *ast.Ident:
		packageName := xT.String()
		return packageName, sexpr.Sel.String(), nil
	case *ast.SelectorExpr:
		return sexprToPackageIdent(*xT)
	default:
		return "", "", fmt.Errorf("Invalid sexpr. Not an identifier. %+v %T", sexpr, sexpr.X)
	}
}

func (f *FilterComputation) processSexpr(sexpr ast.SelectorExpr, srcDir string, importsMap map[string]import_resolver.Import) error {
	packageName, ident, err := sexprToPackageIdent(sexpr)
	if err != nil {
		// Skip it we're dumb.
		return nil
	}

	log.Println(importsMap)
	pkgDirI, ok := importsMap[packageName]
	if !ok {
		return fmt.Errorf("Unknown import: %s", packageName)
	}
	pkgDir := pkgDirI.SrcDir

	edgeIdentFilter, ok := f.IdentFilters[pkgDir]
	if !ok {
		edgeIdentFilter = selector_walker.IdentFilter{
			All:         false,
			Identifiers: map[string]struct{}{},
		}
		f.IdentFilters[pkgDir] = edgeIdentFilter
	}

	present := edgeIdentFilter.CheckIdent(ident)
	if !present {
		if ident == "Type" {
			log.Println("I found Type in filter ident", pkgDir)
		}

		edgeIdentFilter.Identifiers[ident] = struct{}{}
		f.nextPackages[pkgDir] = struct{}{}
	}

	// This import is relevant for this package. Update import filters.
	srcImportFilters, ok := f.ImportFilters[srcDir]
	if !ok {
		srcImportFilters = make(map[string]struct{})
		f.ImportFilters[srcDir] = srcImportFilters
	}
	srcImportFilters[pkgDirI.ImpPath] = struct{}{}

	return nil
}
