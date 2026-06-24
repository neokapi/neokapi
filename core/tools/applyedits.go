package tools

import (
	"sync"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// Edit is one caller-supplied content edit in a change-set: the new block text
// (with inline codes rendered as <x id="…"/> placeholders, exactly as
// `kapi inspect` emits them) and, optionally, the content hash the caller saw
// when it read the block. The hash is the drift anchor — if it no longer
// matches the block's current canonical identity, the source changed since the
// caller inspected and the edit is skipped rather than applied to stale text.
type Edit struct {
	Text        string
	ContentHash string
}

// ApplyReport records the per-block outcome of an apply-edits pass so the
// caller (the `kapi apply` command, the MCP tool) can report it and decide the
// exit code: a Stale or GuardFailed entry means the change-set could not be
// fully applied and the caller should re-inspect and retry, while Applied and
// Skipped are success outcomes. Block IDs are recorded in each bucket.
type ApplyReport struct {
	mu          sync.Mutex
	Applied     []string // block source rewritten to the supplied text
	Skipped     []string // already in the desired state (idempotent no-op)
	Stale       []string // content_hash no longer matches — source drifted
	GuardFailed []string // edit would drop/unbalance an inline code — rejected
}

func (r *ApplyReport) record(bucket *[]string, id string) {
	r.mu.Lock()
	*bucket = append(*bucket, id)
	r.mu.Unlock()
}

// OK reports whether every edit landed cleanly — no drift and no rejected
// edits. The command maps !OK to a non-zero exit so a fix loop re-inspects.
func (r *ApplyReport) OK() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.Stale) == 0 && len(r.GuardFailed) == 0
}

// NewApplyEditsTool builds the apply-edits tool: a source Transform that
// rewrites each translatable Block to caller-supplied text, faithfully — the
// provider-free sibling of the AI rewrite tool. It looks each block up in the
// change-set by ID (falling back to content hash), drift-guards against the
// canonical block identity, reconstructs the runs from the placeholder text,
// and rejects any edit that would corrupt the block's inline codes. Blocks with
// no edit pass through unchanged.
//
// It depends only on core/model + core/tool — no providers/ai — so the
// caller-supplied edit loop carries no LLM dependency. It returns a
// *tool.BaseTool so the CLI drives it through the same byte-faithful round-trip
// (editDocument) the `rewrite` command uses.
func NewApplyEditsTool(byID, byHash map[string]Edit, report *ApplyReport) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "apply-edits",
		ToolDescription: "Applies caller-supplied content edits to a file's blocks, preserving structure and inline codes",
	}

	t.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
		var plan tool.EditPlan
		if !v.Translatable() {
			return plan, nil
		}

		e, ok := byID[v.ID()]
		oldRuns := v.SourceRuns()
		canonHash := model.ComputeContentHash(v.SourceText())
		if !ok {
			// Fall back to matching by canonical content hash (the block's ID may
			// not be stable across re-parses for some formats; its identity is).
			if e, ok = byHash[canonHash]; !ok {
				return plan, nil // no edit targets this block — pass through
			}
		}

		oldText := model.RunsPlaceholderText(oldRuns)
		// Idempotent no-op: the block is already in the desired state. Checked
		// before the drift guard so re-running a fully-applied change-set stays a
		// no-op instead of tripping on the now-changed content hash.
		if e.Text == oldText {
			report.record(&report.Skipped, v.ID())
			return plan, nil
		}

		// Drift guard: the block must still be the one the caller inspected. A
		// stale hash means the source changed underneath the edit.
		if e.ContentHash != "" && e.ContentHash != canonHash {
			report.record(&report.Stale, v.ID())
			return plan, nil
		}

		newRuns := model.ParseRunsPlaceholderText(e.Text, oldRuns)

		if model.HasStructuredRuns(oldRuns) {
			// Plural/select runs have no linear text mapping: replace the whole
			// source opaquely with the rewritten plain text (the applier drops the
			// stale source overlays).
			plain := model.RunsText(newRuns)
			plan.ReplaceAll = &plain
			report.record(&report.Applied, v.ID())
			return plan, nil
		}

		// Faithfulness guard: apply only when every inline code survives exactly
		// and the paired codes stay balanced; otherwise leave the source
		// unchanged rather than write malformed markup.
		if !model.InlineCodesPreserved(oldRuns, newRuns) {
			report.record(&report.GuardFailed, v.ID())
			return plan, nil
		}

		plan.NewRuns = newRuns
		plan.Edits = tool.FullSpanEdit(oldRuns, newRuns)
		report.record(&report.Applied, v.ID())
		return plan, nil
	}
	return t
}
