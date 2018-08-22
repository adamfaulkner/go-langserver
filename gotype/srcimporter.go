// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package srcimporter implements importing directly
// from source files rather than installed packages.
package gotype

import (
	"context"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/adamfaulkner/go-langserver/selector_walker"
)

// An Importer provides the context for importing packages from source code.
type Importer struct {
	ctxt     *build.Context
	fset     *token.FileSet
	sizes    types.Sizes
	packages map[string]*types.Package

	ctx context.Context

	// These should come from the result of a FilterComputation

	// Map from package directory to import paths that should be considered.
	importFilters map[string]map[string]struct{}
	// identFilters maps package directory to IdentFilter.
	identFilters map[string]selector_walker.IdentFilter
}

// NewImporter returns a new Importer for the given context, file set, and map
// of packages. The context is used to resolve import paths to package paths,
// and identifying the files belonging to the package. If the context provides
// non-nil file system functions, they are used instead of the regular package
// os functions. The file set is used to track position information of package
// files; and imported packages are added to the packages map.
func NewSourceImporter(ctx context.Context, ctxt *build.Context, fset *token.FileSet, packages map[string]*types.Package) *Importer {
	return &Importer{
		ctxt: ctxt,
		fset: fset,
		// Changed to work in go 1.8.
		sizes:    &types.StdSizes{8, 8},
		packages: packages,
		ctx:      ctx,
	}
}

// Importing is a sentinel taking the place in Importer.packages
// for a package that is in the process of being imported.
var importing types.Package

// ErrPackage is a sentinal takeng the place in Importer.packages for a package
// that cannot be imported due to typechecking errors.
var errpackage types.Package

// Import(path) is a shortcut for ImportFrom(path, "", 0).
func (p *Importer) Import(path string) (*types.Package, error) {
	return p.ImportFrom(path, "", 0)
}

func newEmptyPackage(path string) *types.Package {
	nameIdx := strings.LastIndex(path, "/")
	name := path[nameIdx+1:]
	pkg := types.NewPackage(path, name)
	pkg.MarkComplete()
	return pkg
}

// ImportFrom imports the package with the given import path resolved from the given srcDir,
// adds the new package to the set of packages maintained by the importer, and returns the
// package. Package path resolution and file system operations are controlled by the context
// maintained with the importer. The import mode must be zero but is otherwise ignored.
// Packages that are not comprised entirely of pure Go files may fail to import because the
// type checker may not be able to determine all exported entities (e.g. due to cgo dependencies).
func (p *Importer) ImportFrom(path, srcDir string, mode types.ImportMode) (*types.Package, error) {
	// Do not do anything if context has expired.
	if p.ctx.Err() != nil {
		return nil, p.ctx.Err()
	}

	if mode != 0 {
		panic("non-zero import mode")
	}

	// determine package path (do vendor resolution)
	var bp *build.Package
	var err error

	switch {
	default:
		if abs, err := p.absPath(srcDir); err == nil { // see issue #14282
			srcDir = abs
		}
		bp, err = p.ctxt.Import(path, srcDir, build.FindOnly)

	case build.IsLocalImport(path):
		// "./x" -> "srcDir/x"
		bp, err = p.ctxt.ImportDir(filepath.Join(srcDir, path), build.FindOnly)

	case p.isAbsPath(path):
		return nil, fmt.Errorf("invalid absolute import path %q", path)
	}
	if err != nil {
		return nil, err // err may be *build.NoGoError - return as is
	}

	// package unsafe is known to the type checker
	if bp.ImportPath == "unsafe" {
		return types.Unsafe, nil
	}

	// no need to re-import if the package was imported completely before
	origImportPath := bp.ImportPath
	pkg := p.packages[origImportPath]
	if pkg != nil {
		if pkg == &importing {
			return nil, fmt.Errorf("import cycle through package %q", bp.ImportPath)
		}
		if pkg == &errpackage {
			return nil, fmt.Errorf("package previously failed to import, not retrying %q", bp.ImportPath)
		}
		if !pkg.Complete() {
			// Package exists but is not complete - we cannot handle this
			// at the moment since the source importer replaces the package
			// wholesale rather than augmenting it (see #19337 for details).
			// Return incomplete package with error (see #16088).
			return pkg, fmt.Errorf("reimported partially imported package %q", bp.ImportPath)
		}
		return pkg, nil
	}

	p.packages[origImportPath] = &importing
	defer func() {
		// clean up in case of error
		// TODO(gri) Eventually we may want to leave a (possibly empty)
		// package in the map in all cases (and use that package to
		// identify cycles). See also issue 16088.
		if p.packages[origImportPath] == &importing {
			p.packages[origImportPath] = nil
		}
	}()

	// collect package files
	bp, err = p.ctxt.ImportDir(bp.Dir, 0)
	if err != nil {
		return nil, err // err may be *build.NoGoError - return as is
	}

	var filenames []string
	filenames = append(filenames, bp.GoFiles...)
	filenames = append(filenames, bp.CgoFiles...)

	files, err := p.parseFiles(bp.Dir, filenames)
	if err != nil {
		return nil, err
	}

	// type-check package files
	var firstHardErr error
	conf := types.Config{
		IgnoreFuncBodies: true,
		FakeImportC:      true,
		// continue type-checking after the first error
		Error: func(err error) {
			if firstHardErr == nil && !err.(types.Error).Soft {
				firstHardErr = err
			}
		},
		Importer: p,
		Sizes:    p.sizes,
	}
	pkg, err = conf.Check(bp.ImportPath, p.fset, files, nil)
	if err != nil {
		// If there was a hard error it is possibly unsafe
		// to use the package as it may not be fully populated.
		// Do not return it (see also #20837, #20855).
		if firstHardErr != nil {
			pkg = nil
			err = firstHardErr // give preference to first hard error over any soft error
		}
		p.packages[origImportPath] = &errpackage
		return pkg, fmt.Errorf("type-checking package %q failed (%v)", bp.ImportPath, err)
	}
	if firstHardErr != nil {
		// this can only happen if we have a bug in go/types
		panic("package is not safe yet no error was returned")
	}

	p.packages[origImportPath] = pkg
	return pkg, nil
}

func (p *Importer) parseFiles(dir string, filenames []string) ([]*ast.File, error) {
	open := p.ctxt.OpenFile // possibly nil

	files := make([]*ast.File, len(filenames))
	errors := make([]error, len(filenames))

	importFilter := p.importFilters[dir]
	identFilter := p.identFilters[dir]

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
				files[i], errors[i] = parser.ParseFile(p.fset, filepath, src, 0)
				stripAstFile(files[i], importFilter, identFilter)
				src.Close() // ignore Close error - parsing may have succeeded which is all we need
			} else {
				// Special-case when ctxt doesn't provide a custom OpenFile and use the
				// parser's file reading mechanism directly. This appears to be quite a
				// bit faster than opening the file and providing an io.ReaderCloser in
				// both cases.
				// TODO(gri) investigate performance difference (issue #19281)
				files[i], errors[i] = parser.ParseFile(p.fset, filepath, nil, 0)
				stripAstFile(files[i], importFilter, identFilter)
			}
		}(i, p.joinPath(dir, filename))
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

// context-controlled file system operations

func (p *Importer) absPath(path string) (string, error) {
	// TODO(gri) This should be using p.ctxt.AbsPath which doesn't
	// exist but probably should. See also issue #14282.
	return filepath.Abs(path)
}

func (p *Importer) isAbsPath(path string) bool {
	if f := p.ctxt.IsAbsPath; f != nil {
		return f(path)
	}
	return filepath.IsAbs(path)
}

func (p *Importer) joinPath(elem ...string) string {
	if f := p.ctxt.JoinPath; f != nil {
		return f(elem...)
	}
	return filepath.Join(elem...)
}

// Destructively modify an *ast.File to only include the imports in
// importFilters and the identifiers in identFilter.
func stripAstFile(
	f *ast.File,
	importFilters map[string]struct{},
	identFilter selector_walker.IdentFilter) {

	log.Println("Stripping", f.Name.String())

	// Remove all decls that aren't relevant.
	d := 0
	for _, decl := range f.Decls {
		switch dT := decl.(type) {
		case *ast.FuncDecl:
			if identFilter.CheckFuncDecl(dT) {
				f.Decls[d] = decl
				d++
			} else {
				log.Println("removing func decl", dT.Name.String())

			}

		case *ast.GenDecl:
			stripGenDecl(dT, identFilter)
			// Empty gendlcs cannot go into the file.
			if len(dT.Specs) > 0 {
				f.Decls[d] = dT
				d++
			}
		}
	}
	f.Decls = f.Decls[:d]

	// Remove all irrelevant imports.
	d = 0
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, "\"")
		if _, ok := importFilters[p]; ok {
			f.Imports[d] = imp
			d++
		}
	}
	f.Imports = f.Imports[:d]
}

func stripGenDecl(gd *ast.GenDecl, identFilter selector_walker.IdentFilter) {
	d := 0
	for _, spec := range gd.Specs {
		switch sT := spec.(type) {
		case *ast.ImportSpec:
			// Ignore for now.
			gd.Specs[d] = spec
			d++
		case *ast.ValueSpec:
			// If any name matches, include.
			use := false
			for _, name := range sT.Names {
				if identFilter.CheckIdent(name.String()) {
					use = true
					break
				}
			}

			if use {
				gd.Specs[d] = spec
				d++
			}
		case *ast.TypeSpec:
			if identFilter.CheckIdent(sT.Name.String()) {
				gd.Specs[d] = spec
				d++
			} else {

				log.Println("REmoving", sT.Name.String())
			}
		}
	}
	gd.Specs = gd.Specs[:d]
}
