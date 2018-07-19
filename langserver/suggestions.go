package langserver

import (
	"context"
	"encoding/json"
	"errors"
	"go/importer"
	"go/types"
	"io/ioutil"

	"github.com/adamfaulkner/go-langserver/pkg/lsp"
	"github.com/mdempsky/gocode/suggest"
	"github.com/sourcegraph/jsonrpc2"
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

func transformLSPReqest(
	ctx context.Context,
	req *lsp.TextDocumentPositionParams,
	handler *LangHandler,
) (filename string, contents []byte, cursor int, err error) {

	filename = handler.FilePath(req.TextDocument.URI)

	file, err := handler.FS.Open(ctx, filename)
	if err != nil {
		return filename, contents, cursor, err
	}
	defer file.Close()

	contents, err = ioutil.ReadAll(file)
	if err != nil {
		return filename, contents, cursor, err
	}

	cursor, ok, errMsg := OffsetForPosition(contents, req.Position)
	if !ok {
		return "", nil, -1, errors.New(errMsg)
	}
	return filename, contents, cursor, nil

}

func transformSuggestions(cands []suggest.Candidate) []lsp.CompletionItem {
	ret := make([]lsp.CompletionItem, len(cands))
	for i, cand := range cands {
		ret[i].Label = cand.Suggestion()
	}
	return ret
}

func CompletionRequest(
	ctx context.Context,
	req *jsonrpc2.Request,
	handler *LangHandler) (result interface{}, err error) {

	if req.Method != "textDocument/completion" {
		return nil, errors.New("Wrong method for completion request")
	}

	position := lsp.TextDocumentPositionParams{}
	err = json.Unmarshal(*req.Params, &position)
	if err != nil {
		return nil, err
	}

	filename, contents, cursor, err := transformLSPReqest(ctx, &position, handler)
	if err != nil {
		return nil, err
	}

	cfg := suggest.Config{
		Importer: importer.For("source", nil).(types.ImporterFrom),
		// TODO(adamf): This should be an option.
		Builtin: true,
	}

	cands, _ := cfg.Suggest(filename, contents, cursor)
	return transformSuggestions(cands), nil
}
