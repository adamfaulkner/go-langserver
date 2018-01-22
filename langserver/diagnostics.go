package langserver

import (
	"context"
	"fmt"
	"go/scanner"
	"go/token"
	"go/types"
	"strings"

	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/jsonrpc2"
)

type diagnostics map[string][]*lsp.Diagnostic // map of URI to diagnostics (for PublishDiagnosticParams)

func (h *LangHandler) publishAdamfDiagnostics(ctx context.Context, conn jsonrpc2.JSONRPC2, diags diagnostics) error {
	for filename, diags := range diags {
		params := lsp.PublishDiagnosticsParams{
			URI:         pathToURI(filename),
			Diagnostics: make([]lsp.Diagnostic, len(diags)),
		}
		for i, d := range diags {
			params.Diagnostics[i] = *d
		}
		if err := conn.Notify(ctx, "textDocument/publishDiagnostics", params); err != nil {
			return err
		}
	}
	return nil
}

// publishDiagnostics sends diagnostic information (such as compile
// errors) to the client.
func (h *LangHandler) publishDiagnostics(ctx context.Context, conn jsonrpc2.JSONRPC2, diags diagnostics) error {
	// Our diagnostics are currently disabled because they behave
	// incorrectly. We do not keep track of which files have failed /
	// succeeded, so we do not send empty diagnostics to clear compiler
	// errors/etc. https://github.com/sourcegraph/go-langserver/issues/23
	// Leaving the code here for when we do actually fix this.
	if cake := false; !cake {
		return nil
	}

	for filename, diags := range diags {
		params := lsp.PublishDiagnosticsParams{
			URI:         pathToURI(filename),
			Diagnostics: make([]lsp.Diagnostic, len(diags)),
		}
		for i, d := range diags {
			params.Diagnostics[i] = *d
		}
		if err := conn.Notify(ctx, "textDocument/publishDiagnostics", params); err != nil {
			return err
		}
	}
	return nil
}

func errsToDiagnostics(typeErrs []error) (diagnostics, error) {
	diags := diagnostics{}
	for _, typeErr := range typeErrs {
		var (
			p   token.Position
			msg string
		)
		switch e := typeErr.(type) {
		case types.Error:
			p = e.Fset.Position(e.Pos)
			msg = e.Msg
		case scanner.Error:
			p = e.Pos
			msg = e.Msg
		case scanner.ErrorList:
			if len(e) == 0 {
				continue
			}
			p = e[0].Pos
			msg = e[0].Msg
			if len(e) > 1 {
				msg = fmt.Sprintf("%s (and %d more errors)", msg, len(e)-1)
			}
		default:
			return nil, fmt.Errorf("unexpected type error: %#+v", typeErr)
		}
		// LSP is 0-indexed, so subtract one from the numbers Go reports.
		start := lsp.Position{Line: p.Line - 1, Character: p.Column - 1}
		end := lsp.Position{Line: p.Line, Character: p.Column}
		diag := &lsp.Diagnostic{
			Range: lsp.Range{
				Start: start,
				End:   end,
			},
			Severity: lsp.Error,
			Source:   "go",
			Message:  strings.TrimSpace(msg),
		}
		if diags == nil {
			diags = diagnostics{}
		}
		diags[p.Filename] = append(diags[p.Filename], diag)
	}
	return diags, nil
}
