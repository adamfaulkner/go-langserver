package gotype

import (
	"errors"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// Return a package name given the import path.
func getPackageName(
	importPath string,
	currentDir string,
	bctx *build.Context,
) (packageName string, err error) {

	// Parsing files is really really slow. To avoid this, use FindOnly to find
	// the package, then use PackageClauseOnly to only parse the package line of a
	// single file from this package.

	pkg, err := bctx.Import(importPath, currentDir, build.FindOnly)
	if err != nil {
		return "", err
	}
	files, err := bctx.ReadDir(pkg.Dir)
	if err != nil {
		return "", err
	}

	var picked string
	for _, f := range files {
		if f.IsDir() {
			continue
		}

		if strings.HasSuffix(f.Name(), ".go") {
			picked = f.Name()
			break
		}
	}

	if picked == "" {
		return "", errors.New("Cannot find any go files in that package?")
	}

	var fullPath string
	if bctx.JoinPath != nil {
		fullPath = bctx.JoinPath(pkg.Dir, picked)
	} else {
		fullPath = filepath.Join(pkg.Dir, picked)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, fullPath, nil, parser.PackageClauseOnly)
	if err != nil {
		return "", err
	}

	return f.Name.Name, nil

}
