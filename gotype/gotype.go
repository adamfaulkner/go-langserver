package gotype

import (
	"context"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"log"
	"path"
	"path/filepath"
	"strings"
)

func expandErrors(err error) []error {
	list, ok := err.(scanner.ErrorList)
	if !ok {
		return []error{err}
	}
	result := make([]error, len(list))
	for i, e := range list {
		result[i] = error(e)
	}
	return result
}

func filenameToImportPath(filename string, bctx *build.Context) (string, error) {
	gopaths := filepath.SplitList(bctx.GOPATH) // list will be empty with no GOPATH
	for _, gopath := range gopaths {
		if !filepath.IsAbs(gopath) {
			return "", fmt.Errorf("build context GOPATH must be an absolute path (GOPATH=%q)", gopath)
		}
	}

	pkgDir := filename
	if !bctx.IsDir(filename) {
		pkgDir = path.Dir(filename)
	}
	var srcDir string
	if strings.HasPrefix(filename, bctx.GOROOT) {
		srcDir = bctx.GOROOT // if workspace is Go stdlib
	} else {
		srcDir = "" // with no GOPATH, only stdlib will work
		for _, gopath := range gopaths {
			if strings.HasPrefix(pkgDir, gopath) {
				srcDir = gopath
				break
			}
		}
	}
	srcDir = path.Join(srcDir, "src") + "/"
	importPath := strings.TrimPrefix(pkgDir, srcDir)
	return importPath, nil
}

// Check a file. Context is used for cancellation, build context is used for
// all the filesystem related operations.
func CheckFile(ctx context.Context, origFilename string, bctx *build.Context) []error {
	fset := token.NewFileSet()
	importPath, err := filenameToImportPath(origFilename, bctx)
	if err != nil {
		return []error{err}
	}

	var retErrs []error

	// Cgo must be enabled for FakeImportC to work.
	if bctx.CgoEnabled == false {
		log.Println("bctx.CgoEnabled = false, failing to typecheck.")
		return nil
	}

	// if checkPkgFiles is called multiple times, set up conf only once
	typeConf := types.Config{
		FakeImportC: true,
		Error: func(err error) {
			retErrs = append(retErrs, expandErrors(err)...)
		},

		// Changed because I want to use the srcimporter with go 1.8
		Importer: New(ctx, bctx, fset, make(map[string]*types.Package)),
		// In Go 1.9, we can just do something like this.
		//Importer: importer.Lookup("source", "")
		// Changed to work with go 1.8
		Sizes: &types.StdSizes{8, 8},
	}

	// Get the file we want.
	bp, err := bctx.Import(importPath, "", 0)
	if err != nil {
		log.Println("Error reading package", err)
		return []error{err}
	}

	testPackage := strings.HasSuffix(origFilename, "_test.go")
	var relativePaths []string
	if testPackage {
		relativePaths = bp.TestGoFiles
	} else {
		relativePaths = bp.GoFiles
	}

	parsedFiles := make([]*ast.File, len(relativePaths))
	for i, relativePath := range relativePaths {
		// Parsing is an expensive operation, check if the context has expired.
		if ctx.Err() != nil {
			return []error{ctx.Err()}
		}

		absPath := filepath.Join(bp.Dir, relativePath)
		src, err := bctx.OpenFile(absPath)
		if err != nil {
			log.Println("Error opening file", err)
			return []error{err}
		}
		parsedFiles[i], err = parser.ParseFile(fset, absPath, src, 0)
		src.Close()
		if err != nil {
			log.Println("Error parsing file", err)
			return []error{err}
		}
	}

	log.Println("Checking", importPath)
	_, err = typeConf.Check(importPath, fset, parsedFiles, nil)
	if err != nil {
		retErrs = append(retErrs, err)
	}
	return retErrs
}
