package suggestions

import (
	"context"
	"go/build"
	"go/token"
	"go/types"

	"github.com/adamfaulkner/go-langserver/gotype"
	"github.com/adamfaulkner/go-langserver/pkg/lsp"
	"github.com/mdempsky/gocode/suggest"
)

// Based off of github.com/mdempsky/gocode fork
//
// To create suggestions, we need the following:
// 1. an Importer.
// 2. a filename.
// 3. current contents of the file.
// 4. Cursor position within the file.
//
//
// For initial version, going to just shim this program in. Later I want to
// actually reuse the typechecked package we already computed.

func transformSuggestions(cands []suggest.Candidate) []lsp.CompletionItem {
	ret := make([]lsp.CompletionItem, len(cands))
	for i, cand := range cands {
		// TODO(adamf): fill in the rest of ret.
		ret[i].Label = cand.Suggestion()
	}
	return ret
}

func CompletionRequest(
	ctx context.Context,
	bctx *build.Context,
	filename string,
	contents []byte,
	cursor int) (result interface{}, err error) {

	imp := gotype.NewSourceImporter(
		ctx,
		bctx,
		token.NewFileSet(),
		make(map[string]*types.Package))

	cfg := suggest.Config{
		Importer: imp,
		Builtin:  false,
	}

	cands, _ := cfg.Suggest(filename, contents, cursor)
	return transformSuggestions(cands), nil
}
