package gotype

import "go/build"

// Return a package name given the import path.
func getPackageName(
	importPath string,
	currentDir string,
	bctx *build.Context,
) (packageName string, err error) {

	pkg, err := bctx.Import(importPath, currentDir, 0)
	if err != nil {
		return "", err
	}

	return pkg.Name, nil

}
