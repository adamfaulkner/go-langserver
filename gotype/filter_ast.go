package gotype

import (
	"go/ast"
	"go/build"
	"log"
	"strings"
	"sync"
)

type identFilter struct {
	all         bool
	identifiers map[string]struct{}
}

type importTraversal struct {
	importFilter map[string]identFilter
	declFilter   map[string]identFilter
}

// Add all importPaths from file to packageNames.
func allRelevantImports(file *ast.File, packageNames map[string]struct{}) {
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, "\"")
		packageNames[path] = struct{}{}
	}
}

// If we only need to typecheck top level declarations and not function
// bodies, which imports do we need to recursively typecheck?
func detectTopLevelRelevantImports(
	file *ast.File,
	bctx *build.Context,
	currentDir string,
	buildPkgCache *sync.Map,
) ([]string, error) {

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

	// Now that we have package names, process imports to figure out which
	// import paths are relevant.

	var relevantImports []string

	for _, imp := range file.Imports {
		var impName string
		path := strings.Trim(imp.Path.Value, "\"")

		if path == "C" {
			// We don't care about import C.
			continue
		}

		if imp.Name != nil {
			// Easy case -- if the import gave the package a name, then we can
			// use it.
			impName = imp.Name.Name
		} else {
			// Difficult case -- if the import did not give the package a name,
			// we have to actually import the package (package statement only)
			// to see what its name is.
			var err error
			impName, err = getPackageName(path, currentDir, bctx, buildPkgCache)
			if err != nil {
				return nil, err
			}
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

	return relevantImports, nil
}

// Get the package names that are referenced in a GenDecl.
func processGenDecl(decl *ast.GenDecl, packageNames map[string]struct{}) {
	for _, spec := range decl.Specs {
		switch specT := spec.(type) {
		case *ast.ValueSpec:
			processValueSpec(specT, packageNames)

		case *ast.TypeSpec:
			processExpr(specT.Type, packageNames)

		default:
			// Don't need to worry about import specs.
		}

	}
}

func processValueSpec(vs *ast.ValueSpec, packageNames map[string]struct{}) {
	processExpr(vs.Type, packageNames)
	for _, v := range vs.Values {
		processExpr(v, packageNames)
	}
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
	case *ast.BadExpr:
		// Unclear what I can do with this.
		return

	// Simple stuff:
	case *ast.Ident:
		packageNames[eT.Name] = struct{}{}

	case *ast.Ellipsis:
		if eT.Elt != nil {
			processExpr(eT.Elt, packageNames)
		}

	case *ast.BasicLit:
		// These are never relevant.
		return

	case *ast.FuncLit:
		processFuncType(eT.Type, packageNames)

	case *ast.CompositeLit:
		processCompositeLit(eT, packageNames)

	// Exprs:
	case *ast.ParenExpr:
		processExpr(eT.X, packageNames)

	case *ast.SelectorExpr:
		// Here's where something is actually being selected. We could be much
		// more precise and only care about these.
		processExpr(eT.X, packageNames)

	case *ast.IndexExpr:
		// Should not happen.
		processExpr(eT.X, packageNames)
		processExpr(eT.Index, packageNames)

	case *ast.SliceExpr:
		// Should not happen.
		// Actually it will happen.
		log.Println("SliceExpr?")

	case *ast.TypeAssertExpr:
		// Should not happen.
		// Actually it will happen.
		log.Println("TypeAssertExpr?")

	case *ast.CallExpr:
		processCallExpr(eT, packageNames)

	case *ast.StarExpr:
		processExpr(eT.X, packageNames)

	case *ast.UnaryExpr:
		processExpr(eT.X, packageNames)

	case *ast.BinaryExpr:
		processExpr(eT.X, packageNames)
		processExpr(eT.Y, packageNames)

	case *ast.KeyValueExpr:
		processExpr(eT.Key, packageNames)
		processExpr(eT.Value, packageNames)

		// Types:
	case *ast.ArrayType:
		processExpr(eT.Len, packageNames)
		processExpr(eT.Elt, packageNames)

	case *ast.StructType:
		processFieldList(eT.Fields, packageNames)

	case *ast.FuncType:
		processFuncType(eT, packageNames)

	case *ast.InterfaceType:
		processFieldList(eT.Methods, packageNames)

	case *ast.MapType:
		processExpr(eT.Key, packageNames)
		processExpr(eT.Value, packageNames)

	case *ast.ChanType:
		processExpr(eT.Value, packageNames)

	default:
		//log.Printf("%+v", e)
	}

}

func processCompositeLit(cl *ast.CompositeLit, packageNames map[string]struct{}) {
	if cl.Type != nil {
		processExpr(cl.Type, packageNames)
	}
	for _, elt := range cl.Elts {
		processExpr(elt, packageNames)
	}
}

func processCallExpr(ce *ast.CallExpr, packageNames map[string]struct{}) {
	processExpr(ce.Fun, packageNames)
	for _, arg := range ce.Args {
		processExpr(arg, packageNames)
	}
}
