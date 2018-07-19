package langserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"sync"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	"github.com/adamfaulkner/go-langserver/pkg/lsp"
	"github.com/adamfaulkner/go-langserver/suggestions"
	"github.com/sourcegraph/jsonrpc2"
)

// NewHandler creates a Go language server handler.
func NewHandler() jsonrpc2.Handler {
	return lspHandler{jsonrpc2.HandlerWithError((&LangHandler{
		HandlerShared: &HandlerShared{},
	}).handle)}
}

// lspHandler wraps LangHandler to correctly handle requests in the correct
// order.
//
// The LSP spec dictates a strict ordering that requests should only be
// processed serially in the order they are received. However, implementations
// are allowed to do concurrent computation if it doesn't affect the
// result. We actually can return responses out of order, since vscode does
// not seem to have issues with that. We also do everything concurrently,
// except methods which could mutate the state used by our typecheckers (ie
// textDocument/didOpen, etc). Those are done serially since applying them out
// of order could result in a different textDocument.
type lspHandler struct {
	jsonrpc2.Handler
}

// Handle implements jsonrpc2.Handler
func (h lspHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if isFileSystemRequest(req.Method) {
		h.Handler.Handle(ctx, conn, req)
		return
	}
	go h.Handler.Handle(ctx, conn, req)
}

// LangHandler is a Go language server LSP/JSON-RPC handler.
type LangHandler struct {
	mu sync.Mutex
	HandlerCommon
	*HandlerShared
	init *InitializeParams // set by "initialize" request

	currentCtx              context.Context
	cancel                  *cancel
	cancelOngoingOperations func()
}

// reset clears all internal state in h.
func (h *LangHandler) reset(init *InitializeParams) error {
	if isFileURI(lsp.DocumentURI(init.InitializeParams.RootPath)) {
		log.Printf("Passing an initialize rootPath URI (%q) is deprecated. Use rootUri instead.", init.InitializeParams.RootPath)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if err := h.HandlerCommon.Reset(init.Root()); err != nil {
		return err
	}
	if !h.HandlerShared.Shared {
		// Only reset the shared data if this lang server is running
		// by itself.
		if err := h.HandlerShared.Reset(!init.NoOSFileSystemAccess); err != nil {
			return err
		}
	}
	h.init = init
	h.cancel = &cancel{}
	return nil
}

// handle implements jsonrpc2.Handler.
func (h *LangHandler) handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (result interface{}, err error) {
	return h.Handle(ctx, conn, req)
}

// Handle creates a response for a JSONRPC2 LSP request. Note: LSP has strict
// ordering requirements, so this should not just be wrapped in an
// jsonrpc2.AsyncHandler. Ensure you have the same ordering as used in the
// NewHandler implementation.
func (h *LangHandler) Handle(ctx context.Context, conn jsonrpc2.JSONRPC2, req *jsonrpc2.Request) (result interface{}, err error) {
	// Prevent any uncaught panics from taking the entire server down.
	defer func() {
		if perr := panicf(recover(), "%v", req.Method); perr != nil {
			err = perr
		}
	}()

	var cancelManager *cancel
	h.mu.Lock()
	cancelManager = h.cancel
	if req.Method != "initialize" && h.init == nil {
		h.mu.Unlock()
		return nil, errors.New("server must be initialized")
	}
	h.mu.Unlock()
	if err := h.CheckReady(); err != nil {
		if req.Method == "exit" {
			err = nil
		}
		return nil, err
	}

	if conn, ok := conn.(*jsonrpc2.Conn); ok && conn != nil {
		h.InitTracer(conn)
	}
	span, ctx, err := h.SpanForRequest(ctx, "lang", req, opentracing.Tags{"mode": "go"})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			ext.Error.Set(span, true)
			span.LogEvent(fmt.Sprintf("error: %v", err))
		}
		span.Finish()
	}()

	// Notifications don't have an ID, so they can't be cancelled
	if cancelManager != nil && !req.Notif {
		var cancel func()
		ctx, cancel = cancelManager.WithCancel(ctx, req.ID)
		defer cancel()
	}

	switch req.Method {
	case "initialize":
		if h.init != nil {
			return nil, errors.New("language server is already initialized")
		}
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params InitializeParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}

		if err := h.reset(&params); err != nil {
			return nil, err
		}

		// TODO(adamf): vscode is sending us garbage when using incremental.
		// kind := lsp.TDSKIncremental
		kind := lsp.TDSKFull
		return lsp.InitializeResult{
			Capabilities: lsp.ServerCapabilities{
				TextDocumentSync: lsp.TextDocumentSyncOptionsOrKind{
					Kind: &kind,
				},
				CompletionProvider: &lsp.CompletionOptions{
					ResolveProvider:   false,
					TriggerCharacters: []string{"."},
				},
			},
		}, nil

	case "textDocument/completion":
		return h.handleCompletionRequest(req)

	case "initialized":
		// A notification that the client is ready to receive requests. Ignore
		return nil, nil

	case "shutdown":
		h.ShutDown()
		return nil, nil

	case "exit":
		if c, ok := conn.(*jsonrpc2.Conn); ok {
			c.Close()
		}
		return nil, nil

	case "$/cancelRequest":
		// notification, don't send back results/errors
		if req.Params == nil {
			return nil, nil
		}
		var params lsp.CancelParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, nil
		}
		if cancelManager == nil {
			return nil, nil
		}
		cancelManager.Cancel(jsonrpc2.ID{
			Num:      params.ID.Num,
			Str:      params.ID.Str,
			IsString: params.ID.IsString,
		})
		return nil, nil

	default:
		if isFileSystemRequest(req.Method) {
			uri, _, err := h.handleFileSystemRequest(ctx, req)
			if uri != "" {
				go h.checkAndReportDiagnostics(conn, uri)
			}
			return nil, err
		}

		return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeMethodNotFound, Message: fmt.Sprintf("method not supported: %s", req.Method)}
	}
}

func (h *LangHandler) handleCompletionRequest(req *jsonrpc2.Request) (result interface{}, err error) {

	if req.Method != "textDocument/completion" {
		return nil, errors.New("Wrong method for completion request")
	}

	position := lsp.TextDocumentPositionParams{}
	err = json.Unmarshal(*req.Params, &position)
	if err != nil {
		return nil, err
	}

	ctx := h.currentCtx

	filename := h.FilePath(position.TextDocument.URI)

	file, err := h.FS.Open(ctx, filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	contents, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	cursor, ok, errMsg := OffsetForPosition(contents, position.Position)
	if !ok {
		return nil, errors.New(errMsg)
	}

	return suggestions.CompletionRequest(ctx, filename, contents, cursor)
}
