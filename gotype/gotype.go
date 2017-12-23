package gotype

import (
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

func CheckFile(origFilename string, bctx *build.Context) []error {
	fset := token.NewFileSet()
	importPath, err := filenameToImportPath(origFilename, bctx)
	if err != nil {
		return []error{err}
	}

	var retErrs []error

	if bctx.CgoEnabled == true {
		log.Println("bctx.CgoEnabled = true, failing to typecheck.")
		return nil
	}

	// if checkPkgFiles is called multiple times, set up conf only once
	typeConf := types.Config{
		FakeImportC: true,
		Error: func(err error) {
			retErrs = append(retErrs, expandErrors(err)...)
		},

		// Changed because I want to use the srcimporter with go 1.8
		Importer: New(bctx, fset, make(map[string]*types.Package)),
		// Changed to work with go 1.8
		Sizes: &types.StdSizes{8, 8},
	}

	// Get the file we want.
	src, err := bctx.OpenFile(origFilename)
	if err != nil {
		log.Println("Error reading file", err)
		return []error{err}
	}

	parsedFile, err := parser.ParseFile(fset, origFilename, src, 0)
	if err != nil {
		log.Println("Error parsing file", err)
		return []error{err}
	}

	log.Println("Checking", importPath)
	_, err = typeConf.Check(importPath, fset, []*ast.File{parsedFile}, nil)
	if err != nil {
		retErrs = append(retErrs, err)
	}
	return retErrs
}
