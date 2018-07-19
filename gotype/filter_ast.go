package gotype

import (
	"go/ast"
	"log"
	"strings"
)

// Add all importPaths from file to packageNames.
func allRelevantImports(file *ast.File, packageNames map[string]struct{}) {
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, "\"")
		packageNames[path] = struct{}{}
	}
}

// If we only need to typecheck top level declarations and not function
// bodies, which imports do we need to recursively typecheck?
func detectTopLevelRelevantImports(file *ast.File) []string {
	log.Println("file", file.Name)

	// Get the package names that are referenced by the top level declarations.
	packageNames := map[string]struct{}{}

	for _, decl := range file.Decls {
		switch dclT := decl.(type) {
		case *ast.FuncDecl:
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
		path := strings.Trim(imp.Path.Value, "\"")
		if imp.Name != nil {
			impName = imp.Name.Name
		} else {
			idx := strings.LastIndex(path, "/")
			impName = path[idx+1:]
		}

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
	// I don't think we actually need to do this because you can't really mess
	// with any other packages inside of a receiver.
	if decl.Recv != nil {
		processFieldList(decl.Recv, packageNames)
	}
	processFuncType(decl.Type, packageNames)
}

func processFuncType(t *ast.FuncType, packageNames map[string]struct{}) {
	processFieldList(t.Params, packageNames)
	if t.Results != nil {
		processFieldList(t.Results, packageNames)
	}
}

func processFieldList(fl *ast.FieldList, packageNames map[string]struct{}) {
	defer func() {
		if err := recover(); err != nil {
			log.Println(err)

		}
	}()
	for _, field := range fl.List {
		if field == nil {
			log.Println("nil field?")
			continue
		}
		// We only care about types of fields.
		processExpr(field.Type, packageNames)
	}
}

func processExpr(e ast.Expr, packageNames map[string]struct{}) {
	// TODO: Rest of this.
	switch eT := e.(type) {

	case *ast.SelectorExpr:
		log.Println("SelectorExpr:", eT.X, eT.Sel)
		// Here's where something is actually being selected. We could be much
		// more precise and only care about these.
		processExpr(eT.X, packageNames)
	case *ast.Ident:
		packageNames[eT.Name] = struct{}{}

	default:
		//log.Printf("%+v", e)
	}

}
