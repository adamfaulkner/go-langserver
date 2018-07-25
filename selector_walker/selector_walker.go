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
	if len(fl.List) == 0 {
		return
	}

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
		if neT.Results != nil {
			s.appendFieldList(neT.Results)
		}
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
