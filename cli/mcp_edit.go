package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/spf13/cobra"
)

// init registers the write leg of the edit loop on the shared MCP stdio server:
// apply_edits (the one write verb). It pairs with the read leg, extract_content
// (which emits each block's content_hash + placeholder-rendered text), and
// check_file, so a non-Claude MCP client runs the same author → check → fix loop
// the CLI skill drives — the client supplies the edits, kapi enforces the
// faithful round-trip and is the checker. No second model is involved.
func init() {
	RegisterMCPToolFactory(registerEditMCPTools)
}

// ApplyEditsInput is a typed change-set: the same shape `kapi apply` consumes.
// Each entry is a content edit or an asset edit (term/tm/brand/recipe).
type ApplyEditsInput struct {
	Changeset []changeEntry `json:"changeset" jsonschema:"the typed change-set entries to apply"`
}

// ApplyEditsMCPOutput reports the per-block content outcome and per-entry asset
// outcomes; OK is false when any edit drifted (stale) or was rejected by the
// inline-code guard, signalling the caller to re-inspect and retry.
type ApplyEditsMCPOutput struct {
	OK      bool          `json:"ok"`
	Applied []string      `json:"applied,omitempty"`
	Skipped []string      `json:"skipped,omitempty"`
	Stale   []string      `json:"stale,omitempty"`
	Guard   []string      `json:"guard_failed,omitempty"`
	Assets  []assetResult `json:"assets,omitempty"`
}

func registerEditMCPTools(server *mcp.Server, a *App) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "apply_edits",
		Description: "Apply a typed change-set — the one write verb. Content edits land through the byte-faithful round-trip (structure and inline codes preserved, drift-guarded by content_hash); asset edits (glossary term, TM pair, brand rule, recipe field) are written to their committed source and compiled into the cache. No AI provider is used.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ApplyEditsInput) (*mcp.CallToolResult, ApplyEditsMCPOutput, error) {
		return a.applyEditsMCP(ctx, in)
	})
}

func (a *App) applyEditsMCP(ctx context.Context, in ApplyEditsInput) (*mcp.CallToolResult, ApplyEditsMCPOutput, error) {
	var out applyOutput

	byFile := map[string][]changeEntry{}
	var fileOrder []string
	// A bare command carries the context for the asset appliers' project
	// resolution (they walk up from cwd); no flags are set, so they take their
	// project-default paths.
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)

	for _, e := range in.Changeset {
		switch e.Kind {
		case kindContent:
			if e.File == "" {
				return nil, ApplyEditsMCPOutput{}, fmt.Errorf("content entry for block %q has no \"file\"", e.ID)
			}
			if _, seen := byFile[e.File]; !seen {
				fileOrder = append(fileOrder, e.File)
			}
			byFile[e.File] = append(byFile[e.File], e)
		case kindTerm, kindTM, kindBrand, kindRecipe:
			out.Assets = append(out.Assets, a.applyAssetEntry(ctx, cmd, e))
		case "":
			return nil, ApplyEditsMCPOutput{}, errors.New("change-set entry has no \"kind\"")
		default:
			return nil, ApplyEditsMCPOutput{}, fmt.Errorf("unknown change kind %q", e.Kind)
		}
	}

	for _, file := range fileOrder {
		report := &coretools.ApplyReport{}
		byID, byHash := buildEditMaps(byFile[file])
		t := coretools.NewApplyEditsTool(byID, byHash, report)
		if derr := a.editDocument(ctx, file, t, "", true, "", nil); derr != nil {
			return nil, ApplyEditsMCPOutput{}, fmt.Errorf("%s: %w", displayName(file), derr)
		}
		out.Content.Applied = append(out.Content.Applied, report.Applied...)
		out.Content.Skipped = append(out.Content.Skipped, report.Skipped...)
		out.Content.Stale = append(out.Content.Stale, report.Stale...)
		out.Content.GuardFailed = append(out.Content.GuardFailed, report.GuardFailed...)
	}

	return nil, ApplyEditsMCPOutput{
		OK:      out.ok(),
		Applied: out.Content.Applied,
		Skipped: out.Content.Skipped,
		Stale:   out.Content.Stale,
		Guard:   out.Content.GuardFailed,
		Assets:  out.Assets,
	}, nil
}
