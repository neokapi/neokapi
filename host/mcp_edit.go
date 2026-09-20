package host

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	coretools "github.com/neokapi/neokapi/core/tools"
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

// applyEditsInput is a typed change-set: the same shape `kapi apply` consumes.
// Each entry is a content edit or an asset edit (term/tm/brand/recipe).
type applyEditsInput struct {
	Changeset []changeEntry `json:"changeset" jsonschema:"the typed change-set entries to apply"`
	Project   string        `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
}

// applyEditsMCPOutput reports the per-block content outcome and per-entry asset
// outcomes; OK is false when any edit drifted (stale) or was rejected by the
// inline-code guard, signalling the caller to re-inspect and retry. Comments
// holds each file's comment edits and the check of what they wrote, and OK is
// false when one was refused, did not run, or left that check not passing.
type applyEditsMCPOutput struct {
	OK      bool          `json:"ok"`
	Applied []string      `json:"applied,omitempty"`
	Skipped []string      `json:"skipped,omitempty"`
	Stale   []string      `json:"stale,omitempty"`
	Guard   []string      `json:"guard_failed,omitempty"`
	Assets  []assetResult `json:"assets,omitempty"`

	Comments []commentFileResult `json:"comments,omitempty"`
}

func registerEditMCPTools(server *mcp.Server, a *App) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "apply_edits",
		Description: "Apply a typed change-set: the one write verb. For document wording, each entry " +
			"uses kind=content, file, id, content_hash and text (the new wording). Read block IDs and " +
			"hashes with extract_content. The replacement field is for voice rules. Content edits land through the " +
			"byte-faithful round-trip (structure and inline codes preserved, drift-guarded by content_hash); " +
			"asset edits (terms entry, content memory pair, voice rule, recipe field) are written to their " +
			"committed source and compiled into the cache. No AI provider is used. Read the " +
			"context://<project-relative-path> resource before editing content, then run check_file on " +
			"each changed file to review findings and analyzer coverage. For a code comment, an entry uses kind=comment, file, " +
			"id and lines (as check_file reports them, such as func/Parse), comment_sha256 (the fingerprint check_file reports " +
			"for the comment; a comment whose bytes differ is refused as changed, and current_text may carry the prose as " +
			"read instead) and " +
			"text (the comment's prose without comment markers, or a /* */ comment's delimiters and the * opening each line). " +
			"Every byte outside the comment is kept, a /* */ comment keeps its layout, the result must " +
			"parse and the language's formatter must agree; a directive, a generated file's comment, a changed comment, " +
			"text holding */ in a /* */ comment and text that drops a code block or reference are refused with a reason and write nothing. " +
			"A comment in a language whose plugin or formatter is not installed, or whose formatter does not format the file, did not run and is not written. " +
			"A project's formatter runs code that project controls, and an agent that can write files can write the configuration it loads, so apply_edits never runs it: " +
			"such a comment did not run, with the reason formatter, and a person applies it with kapi apply in a terminal. " +
			"Each written file's result carries a check scoped to the change.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in applyEditsInput) (*mcp.CallToolResult, applyEditsMCPOutput, error) {
		return a.applyEditsMCP(ctx, in)
	})
}

func (a *App) applyEditsMCP(ctx context.Context, in applyEditsInput) (*mcp.CallToolResult, applyEditsMCPOutput, error) {
	if err := validateContentWording(in.Changeset); err != nil {
		return nil, applyEditsMCPOutput{}, err
	}
	var out applyOutput

	byFile := map[string][]changeEntry{}
	var fileOrder []string
	var comments []changeEntry
	// The asset appliers resolve their store from this command's project: the
	// one the call named, else the one the server started in. A term or a
	// content-memory pair is written into that project's store rather than into
	// whichever project the server's working directory happens to sit in.
	cmd, _, err := a.mcpCallCommand(ctx, "apply-edits", in.Project)
	if err != nil {
		return nil, applyEditsMCPOutput{}, err
	}

	for _, e := range in.Changeset {
		switch e.Kind {
		case kindContent:
			if e.File == "" {
				return nil, applyEditsMCPOutput{}, fmt.Errorf("content entry for block %q has no \"file\"", e.ID)
			}
			if _, seen := byFile[e.File]; !seen {
				fileOrder = append(fileOrder, e.File)
			}
			byFile[e.File] = append(byFile[e.File], e)
		case kindComment:
			comments = append(comments, e)
		case kindTerm, kindMemory, kindVoice, kindRecipe:
			out.Assets = append(out.Assets, a.applyAssetEntry(ctx, cmd, e))
		case "":
			return nil, applyEditsMCPOutput{}, errors.New("change-set entry has no \"kind\"")
		default:
			return nil, applyEditsMCPOutput{}, fmt.Errorf("unknown change kind %q", e.Kind)
		}
	}

	for _, file := range fileOrder {
		report := &coretools.ApplyReport{}
		byID, byHash := buildEditMaps(byFile[file])
		t := coretools.NewApplyEditsTool(byID, byHash, report)
		if derr := a.EditDocument(ctx, file, t, "", true, "", nil); derr != nil {
			return nil, applyEditsMCPOutput{}, fmt.Errorf("%s: %w", DisplayName(file), derr)
		}
		out.Content.Applied = append(out.Content.Applied, report.Applied...)
		out.Content.Skipped = append(out.Content.Skipped, report.Skipped...)
		out.Content.Stale = append(out.Content.Stale, report.Stale...)
		out.Content.GuardFailed = append(out.Content.GuardFailed, report.GuardFailed...)
	}
	if len(comments) > 0 {
		// The check of a written comment resolves governance from the call's
		// project, as check_file does.
		out.Comments = a.applyComments(ctx, cmd, comments, false, "", mcpFormatterTrust())
	}

	return nil, applyEditsMCPOutput{
		OK:       out.ok(),
		Applied:  out.Content.Applied,
		Skipped:  out.Content.Skipped,
		Stale:    out.Content.Stale,
		Guard:    out.Content.GuardFailed,
		Assets:   out.Assets,
		Comments: out.Comments,
	}, nil
}
