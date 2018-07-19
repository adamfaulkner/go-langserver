package gotype

import "go/build"

type pkgCacheKey struct {
	importPath string
	currentDir string
}

// Return a package name given the import path.
func getPackageName(
	importPath string,
	currentDir string,
	bctx *build.Context,
	pkgCache map[pkgCacheKey]*build.Package,
) (packageName string, err error) {

	cacheKey := pkgCacheKey{
		importPath: importPath,
		currentDir: currentDir,
	}
	if pkg, ok := pkgCache[cacheKey]; ok {
		return pkg.Name, nil
	}

	pkg, err := bctx.Import(importPath, currentDir, 0)
	if err != nil {
		return "", err
	}

	pkgCache[cacheKey] = pkg

	return pkg.Name, nil

}
