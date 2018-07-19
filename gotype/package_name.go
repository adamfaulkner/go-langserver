package gotype

import (
	"go/build"
	"sync"
)

type pkgCacheKey struct {
	importPath string
	currentDir string
}

// Return a package name given the import path.
func getPackageName(
	importPath string,
	currentDir string,
	bctx *build.Context,
	pkgCache *sync.Map,
) (packageName string, err error) {

	cacheKey := pkgCacheKey{
		importPath: importPath,
		currentDir: currentDir,
	}
	pkgI, ok := pkgCache.Load(cacheKey)
	if ok {
		return pkgI.(*build.Package).Name, nil
	}

	pkg, err := bctx.Import(importPath, currentDir, 0)
	if err != nil {
		return "", err
	}

	pkgCache.Store(cacheKey, pkg)

	return pkg.Name, nil

}
