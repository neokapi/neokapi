package tools

import (
	"slices"
	"sync"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// Edit is one caller-supplied content edit in a change-set: the new block text
// in edit text form (model.RunsEditText: inline codes as <x id="…"/>
// placeholders and character references as their characters, exactly as
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
// exit code: a Stale or GuardFailed entry, or an edit NotFound lists, means the
// change-set could not be fully applied and the caller should re-inspect and
// retry, while Applied and Skipped are success outcomes. Block IDs are recorded
// in each bucket.
type ApplyReport struct {
	mu          sync.Mutex
	Applied     []string // block source rewritten to the supplied text
	Skipped     []string // already in the desired state (idempotent no-op)
	Stale       []string // content_hash no longer matches — source drifted
	GuardFailed []string // edit would drop or unbalance an inline code, or flatten plural/select branches; rejected

	// The edits the pass was given and the ones a block of the file matched,
	// by key. NotFound is the difference once the pass is over.
	wantIDs, wantHashes       map[string]bool
	matchedIDs, matchedHashes map[string]bool
}

func (r *ApplyReport) record(bucket *[]string, id string) {
	r.mu.Lock()
	*bucket = append(*bucket, id)
	r.mu.Unlock()
}

// expect notes the edits a pass is given, so NotFound can name the ones no
// block matched.
func (r *ApplyReport) expect(byID, byHash map[string]Edit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wantIDs, r.wantHashes = map[string]bool{}, map[string]bool{}
	r.matchedIDs, r.matchedHashes = map[string]bool{}, map[string]bool{}
	for id := range byID {
		r.wantIDs[id] = true
	}
	for h := range byHash {
		r.wantHashes[h] = true
	}
}

// matchID and matchHash note that a block matched the edit given under key.
func (r *ApplyReport) matchID(id string) {
	r.mu.Lock()
	r.matchedIDs[id] = true
	r.mu.Unlock()
}

func (r *ApplyReport) matchHash(hash string) {
	r.mu.Lock()
	r.matchedHashes[hash] = true
	r.mu.Unlock()
}

// NotFoundHashPrefix marks a NotFound entry that names an edit by the content
// hash it was given, for an edit given without a block id.
const NotFoundHashPrefix = "content_hash:"

// NotFound lists the edits no editable block of the file matched, once the
// pass is over: the block id of an edit given one, else NotFoundHashPrefix and
// the content hash it named. Such an edit changed nothing. Its block may have
// been removed or renamed since the caller read the file, or the id or hash may
// be mistyped, so the caller reads the file again. The list is sorted.
func (r *ApplyReport) NotFound() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for id := range r.wantIDs {
		if !r.matchedIDs[id] {
			out = append(out, id)
		}
	}
	for h := range r.wantHashes {
		if !r.matchedHashes[h] {
			out = append(out, NotFoundHashPrefix+h)
		}
	}
	slices.Sort(out)
	return out
}

// OK reports whether every edit landed cleanly: no drift, no rejected edit and
// no edit that matched no block. The command maps !OK to a non-zero exit so a
// fix loop re-inspects.
func (r *ApplyReport) OK() bool {
	if len(r.NotFound()) > 0 {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.Stale) == 0 && len(r.GuardFailed) == 0
}

// NewApplyEditsTool builds the apply-edits tool: a source Transform that
// rewrites each translatable Block to caller-supplied text, faithfully — the
// provider-free sibling of the AI rewrite tool. It looks each block up in the
// change-set by ID (falling back to content hash), drift-guards against the
// canonical block identity, reconstructs the runs from the edit text,
// and rejects any edit that would corrupt the block's inline codes. Blocks with
// no edit pass through unchanged, and report.NotFound lists each edit no
// editable block matched once the pass is over.
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
	report.expect(byID, byHash)

	t.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
		var plan tool.EditPlan
		if !v.Translatable() {
			return plan, nil
		}

		e, ok := byID[v.ID()]
		oldRuns := v.SourceRuns()
		canonHash := model.ComputeContentHash(v.SourceText())
		if ok {
			report.matchID(v.ID())
		} else {
			// Fall back to matching by canonical content hash (the block's ID may
			// not be stable across re-parses for some formats; its identity is).
			if e, ok = byHash[canonHash]; !ok {
				return plan, nil // no edit targets this block — pass through
			}
			report.matchHash(canonHash)
		}

		// Idempotent no-op: the block is already in the desired state. Checked
		// before the drift guard so re-running a fully-applied change-set stays a
		// no-op instead of tripping on the now-changed content hash. A text
		// written with every code as a token, character references included,
		// reads the block the same way.
		if e.Text == model.RunsEditText(oldRuns) || e.Text == model.RunsPlaceholderText(oldRuns) {
			report.record(&report.Skipped, v.ID())
			return plan, nil
		}

		// Drift guard: the block must still be the one the caller inspected. A
		// stale hash means the source changed underneath the edit.
		if e.ContentHash != "" && e.ContentHash != canonHash {
			report.record(&report.Stale, v.ID())
			return plan, nil
		}

		newRuns := model.ParseRunsEditText(e.Text, oldRuns)

		// Faithfulness guard: apply only when every inline code survives exactly
		// and the paired codes stay nested. Flat placeholder text cannot express
		// plural/select branches, so a changed structured block is refused here.
		// A character reference is a character in the edit text, so an edit may
		// drop or move one.
		if !model.EditKeepsInlineCodes(oldRuns, newRuns) {
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
