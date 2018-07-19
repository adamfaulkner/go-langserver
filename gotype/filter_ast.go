package gotype

import (
	"go/ast"
	"log"
	"strings"
)

// If we only need to typecheck top level declarations and not function
// bodies, which imports do we need to recursively typecheck?
func detectTopLevelRelevantImports(file *ast.File) []string {
	log.Println("file", file.Name)

	// Get the package names that are referenced by the top level declarations.
	packageNames := map[string]struct{}{}

	for _, decl := range file.Decls {
		switch dclT := decl.(type) {
		case *ast.FuncDecl:
			log.Println("declT funcDecl:", dclT.Name)
			processFuncDecl(dclT, packageNames)

		case *ast.GenDecl:
			processGenDecl(dclT, packageNames)
		default:
			continue
		}
	}

	log.Println("pkgnames", packageNames)

	// Now that we have package names, process imports to figure out which
	// import paths are relevant.

	var relevantImports []string

	for _, imp := range file.Imports {
		var impName string
		path := imp.Path.Value
		if imp.Name != nil {
			impName = imp.Name.Name
		} else {
			idx := strings.LastIndex(path, "/")
			impName = strings.Trim(path[idx+1:], "\"")
		}
		log.Println("impname", impName)

		// We can't really help when using the dot import.
		if impName == "." {
			relevantImports = append(relevantImports, path)
		} else {
			_, ok := packageNames[impName]
			if ok {
				relevantImports = append(relevantImports, path)
			}
		}
	}

	return relevantImports
}

// Get the package names that are referenced in a GenDecl.
func processGenDecl(decl *ast.GenDecl, packageNames map[string]struct{}) {
}

// Get the package names that are referenced in a FuncDecl.
func processFuncDecl(decl *ast.FuncDecl, packageNames map[string]struct{}) {
	packageNames["errors"] = struct{}{}
}
