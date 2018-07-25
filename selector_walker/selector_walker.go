package gotype

import (
	"errors"
	"go/ast"
	"go/token"
)

type identFilter struct {
	all         bool
	identifiers map[string]struct{}
}

func (i *identFilter) checkIdent(ident string) bool {
	if i.all {
		return true
	}
	_, ok := i.identifiers[ident]
	return ok
}

// TODO(adamf): Switch all of this to dirs in order to handle vendor.
/*
type importTraversal struct {
	bctx *build.Context

	// Map from import path to identFilter used for imports.
	importFilter map[string]identFilter
	// Map from import path to identFilter used for decls.
	declFilter map[string]identFilter

	// TODO(adamf): Maybe a better data structure here would be better.
	// Import paths that need reprocessing.
	queue []string

	// map from path to cached AST.
	astCache map[string]*ast.File
	// Fileset for the above ASTs.
	fset *token.FileSet
}

func (i *importTraversal) parseFile(path string) (*ast.File, error) {
	file, ok := i.astCache[path]
	if ok {
		return file, nil
	}

	var err error

	if i.bctx.OpenFile != nil {
		src, err := i.bctx.OpenFile(path)
		if err != nil {
			return nil, err
		}
		// TODO(adamf): Need a parse cache.
		file, err = parser.ParseFile(i.fset, path, src, 0)
		src.Close() // ignore Close error - parsing may have succeeded which is all we need
	} else {
		// Special-case when ctxt doesn't provide a custom OpenFile and use the
		// parser's file reading mechanism directly. This appears to be quite a
		// bit faster than opening the file and providing an io.ReaderCloser in
		// both cases.
		// TODO(gri) investigate performance difference (issue #19281)
		file, err = parser.ParseFile(i.fset, path, nil, 0)
	}

	if err != nil {
		return nil, err
	}

	i.astCache[path] = file
	return file, nil

}

func (i *importTraversal) computeClosure() error {
	// Loop invariant: All packages are either
	// 1. In queue.
	// 2. Fully specified in importFilter and declFilter. Fully specified means
	// that there's no entries in declFilter that cannot be typechecked because
	// of missing entries for this package.

	for len(i.queue) > 0 {
		next := i.queue[0]
		i.queue = i.queue[1:]

	}
}

// TODO: Remove the allocations inherent in these return types.

func (i *importTraversal) extractSelectorsFromDecl(decl ast.Decl) []ast.SelectorExpr {
	// TODO
	return nil
}

func (i *importTraversal) getEdgesFromPackage(pkg string) ([]ast.SelectorExpr, error) {
	pkgDeclFilter, ok := i.declFilter[pkg]
	if !ok {
		return nil, errors.New("getEdgesFromPackage without declFilter for package.")
	}

	p, err := i.bctx.Import(pkg, "", 0)
	if err != nil {
		return nil, err
	}

	dir := p.Dir
	var edges []ast.SelectorExpr

	var allFilenames []string
	allFilenames = append(allFilenames, p.GoFiles...)
	// TODO: test mode
	allFilenames = append(allFilenames, p.TestGoFiles...)

	for _, fileName := range allFilenames {
		path := filepath.Join(dir, fileName)
		file, err := i.parseFile(path)
		if err != nil {
			return nil, err
		}

		for _, decl := range file.Decls {
		}

	}
}
*/

type selectorWalker struct {
	// Contains the list of remaining decls to look at. These still need to be filtered with identFilter.
	declList []ast.Decl
	// When current decl is a GenDecl, this refers to the next spec to look at. These still need to be filtered with identFilter
	specList []ast.Spec
	// Contains the list of reminaing exprs to look at. These do not need to be filtered with identFilter by their nature.
	exprList []ast.Expr
	// Contains a filter to use for identifiers.
	idf identFilter
}

func NewSelectorWalker(f *ast.File, idf identFilter) *selectorWalker {
	return &selectorWalker{
		declList: f.Decls,
		idf:      idf,
	}

}

var selectorWalkerFinished = errors.New("Finished walking")

func (s *selectorWalker) NextSelector() (ast.SelectorExpr, error) {
	if len(s.exprList) > 0 {
		return s.processExprList()
	}

	if len(s.specList) > 0 {
		return s.processSpecList()
	}

	if len(s.declList) > 0 {
		return s.processDeclList()
	}

	return ast.SelectorExpr{}, selectorWalkerFinished
}

// Append types to exprList from a field list.
func (s *selectorWalker) appendFieldList(fl *ast.FieldList) {
	for _, f := range fl.List {
		s.exprList = append(s.exprList, f.Type)
	}
}

func (s *selectorWalker) processExprList() (ast.SelectorExpr, error) {
	nextExpr := s.exprList[0]
	s.exprList = s.exprList[1:]

	switch neT := nextExpr.(type) {
	case *ast.SelectorExpr:
		// Finally! Base case.
		return *neT, nil

	case *ast.BadExpr:
		return ast.SelectorExpr{}, errors.New("BadExpr!")
	case *ast.Ident:
		// skip
	case *ast.Ellipsis:
		s.exprList = append(s.exprList, neT.Elt)
	case *ast.BasicLit:
		// skip
	case *ast.FuncLit:
		s.exprList = append(s.exprList, neT.Type)
	case *ast.CompositeLit:
		s.exprList = append(s.exprList, neT.Type)
		s.exprList = append(s.exprList, neT.Elts...)
	case *ast.ParenExpr:
		s.exprList = append(s.exprList, neT.X)
	case *ast.IndexExpr:
		s.exprList = append(s.exprList, neT.X)
		s.exprList = append(s.exprList, neT.Index)
	case *ast.SliceExpr:
		s.exprList = append(s.exprList, neT.X)
		s.exprList = append(s.exprList, neT.Low)
		s.exprList = append(s.exprList, neT.High)
		s.exprList = append(s.exprList, neT.Max)
	case *ast.TypeAssertExpr:
		s.exprList = append(s.exprList, neT.X)
		s.exprList = append(s.exprList, neT.Type)
	case *ast.CallExpr:
		s.exprList = append(s.exprList, neT.Fun)
		s.exprList = append(s.exprList, neT.Args...)
	case *ast.StarExpr:
		s.exprList = append(s.exprList, neT.X)
	case *ast.UnaryExpr:
		s.exprList = append(s.exprList, neT.X)
	case *ast.BinaryExpr:
		s.exprList = append(s.exprList, neT.X)
		s.exprList = append(s.exprList, neT.Y)
	case *ast.KeyValueExpr:
		s.exprList = append(s.exprList, neT.Key)
		s.exprList = append(s.exprList, neT.Value)
	case *ast.ArrayType:
		s.exprList = append(s.exprList, neT.Len) // Not necessary
		s.exprList = append(s.exprList, neT.Elt)
	case *ast.StructType:
		s.appendFieldList(neT.Fields)
	case *ast.FuncType:
		s.appendFieldList(neT.Params)
		s.appendFieldList(neT.Results)
	case *ast.InterfaceType:
		s.appendFieldList(neT.Methods)
	case *ast.MapType:
		s.exprList = append(s.exprList, neT.Key)
		s.exprList = append(s.exprList, neT.Value)
	case *ast.ChanType:
		s.exprList = append(s.exprList, neT.Value)
	}
	return s.NextSelector()
}

func (s *selectorWalker) processSpecList() (ast.SelectorExpr, error) {
	nextSpec := s.specList[0]
	s.specList = s.specList[1:]

	switch nsT := nextSpec.(type) {
	case *ast.ValueSpec:
		for i, name := range nsT.Names {
			if s.idf.checkIdent(name.Name) {
				s.exprList = append(s.exprList, nsT.Type)
				if len(nsT.Values) > i {
					s.exprList = append(s.exprList, nsT.Values[i])
				}
			}
		}

	case *ast.TypeSpec:
		if s.idf.checkIdent(nsT.Name.Name) {
			s.exprList = append(s.exprList, nsT.Type)
		}

	default:
		return ast.SelectorExpr{}, errors.New("Unexpected spec.")

	}

	return s.NextSelector()
}

func (s *selectorWalker) processDeclList() (ast.SelectorExpr, error) {
	nextDecl := s.declList[0]
	s.declList = s.declList[1:]

	switch ndT := nextDecl.(type) {
	case *ast.BadDecl:
		return ast.SelectorExpr{}, errors.New("Bad Decl Found")
	case *ast.GenDecl:
		// We don't bother with imports.
		if ndT.Tok == token.IMPORT {
			return s.NextSelector()
		}
		s.specList = ndT.Specs
		return s.NextSelector()

	case *ast.FuncDecl:
		s.exprList = []ast.Expr{ndT.Type}
		return s.NextSelector()

	default:
		return ast.SelectorExpr{}, errors.New("Unexpected type of decl")
	}

}

// Add all importPaths from file to packageNames.

/*
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
*/
