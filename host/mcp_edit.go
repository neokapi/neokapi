package host

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/sectionedit"
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
	Preview   bool          `json:"preview,omitempty" jsonschema:"preview a section entry as an offset plan without writing; section entries only"`
}

// applyEditsMCPOutput reports the per-block content outcome and per-entry asset
// outcomes; OK is false when any edit drifted (stale) or was rejected by the
// inline-code guard, signalling the caller to re-inspect and retry.
type applyEditsMCPOutput struct {
	OK      bool               `json:"ok"`
	Section *SectionEditResult `json:"section,omitempty"`
	Applied []string           `json:"applied,omitempty"`
	Skipped []string           `json:"skipped,omitempty"`
	Stale   []string           `json:"stale,omitempty"`
	Guard   []string           `json:"guard_failed,omitempty"`
	Assets  []assetResult      `json:"assets,omitempty"`
}

type inspectSectionsInput struct {
	File string `json:"file" jsonschema:"local Markdown, HTML or DOCX file"`
}

func registerEditMCPTools(server *mcp.Server, a *App) {
	mcp.AddTool(server, &mcp.Tool{Name: "inspect_sections", Description: "Read heading sections with native neokapi block ranges and source snapshot. Content is a Markdown reading projection. Read the file context resource before authoring; use apply_edits kind=section and preview=true, then apply and check_file. The POC supports Markdown, HTML and DOCX with explicit source/fragment limits."}, func(ctx context.Context, req *mcp.CallToolRequest, in inspectSectionsInput) (*mcp.CallToolResult, sectionedit.Document, error) {
		doc, err := a.InspectSections(ctx, in.File)
		return nil, doc, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "apply_edits",
		Description: "Apply a typed change-set: the one write verb. For document wording, each entry " +
			"uses kind=content, file, id, content_hash and text (the new wording). Read block IDs and " +
			"hashes with extract_content. The replacement field is for voice rules. Content edits land through the " +
			"byte-faithful round-trip (structure and inline codes preserved, drift-guarded by content_hash); " +
			"asset edits (terms entry, content memory pair, voice rule, recipe field) are written to their " +
			"committed source and compiled into the cache. No AI provider is used. Read the " +
			"context://<project-relative-path> resource before editing content, then run check_file on " +
			"each changed file to review findings and analyzer coverage. For section edits use inspect_sections, " +
			"then one kind=section entry with file, id, snapshot and text (Markdown body). preview=true " +
			"returns an immutable writer offset plan without writing. The heading is preserved.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in applyEditsInput) (*mcp.CallToolResult, applyEditsMCPOutput, error) {
		return a.applyEditsMCP(ctx, in)
	})
}

func (a *App) applyEditsMCP(ctx context.Context, in applyEditsInput) (*mcp.CallToolResult, applyEditsMCPOutput, error) {
	if err := validateContentWording(in.Changeset); err != nil {
		return nil, applyEditsMCPOutput{}, err
	}
	if err := validateSectionChangeSet(in.Changeset); err != nil {
		return nil, applyEditsMCPOutput{}, err
	}
	if len(in.Changeset) == 1 && in.Changeset[0].Kind == kindSection {
		entry := in.Changeset[0]
		result, err := a.ApplySectionEdit(ctx, entry.File, sectionedit.Edit{ID: entry.ID, Snapshot: entry.Snapshot, Text: entry.Text}, in.Preview, "")
		if err != nil {
			return nil, applyEditsMCPOutput{}, err
		}
		return nil, applyEditsMCPOutput{OK: true, Section: &result}, nil
	}
	if in.Preview {
		return nil, applyEditsMCPOutput{}, errors.New("preview is supported for one section entry only")
	}
	var out applyOutput

	byFile := map[string][]changeEntry{}
	var fileOrder []string
	// A bare command carries the context for the asset appliers' project
	// resolution (they walk up from cwd); no flags are set, so they take their
	// project-default paths.
	cmd := NewEnvCommand(ctx, "apply-edits")

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

	return nil, applyEditsMCPOutput{
		OK:      out.ok(),
		Applied: out.Content.Applied,
		Skipped: out.Content.Skipped,
		Stale:   out.Content.Stale,
		Guard:   out.Content.GuardFailed,
		Assets:  out.Assets,
	}, nil
}
