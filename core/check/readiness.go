package check

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// SettleSourceStatus runs the provider-free source checks over one block and
// stamps its SourceStatus with the terminal readiness stamp — the single
// authored→checked derivation both venues share (the Bowrain server's
// settleBlockStatus and the local converge's source-gate leading stage), so the
// server and CLI promote/demote a block from identical findings. Content-lint
// supplies the source-side hygiene findings; the readiness tool promotes a clean
// block to `checked` and demotes a block with a major+ finding to `authored`,
// leaving an already-`approved` clean source untouched.
//
// It is deliberately provider-free: the automated `checked` gate is satisfied by
// deterministic checks, so settling an un-ready corpus never itself burns AI
// credits. Deeper LLM-backed source voice-checking layers on top later, not in
// front of the gate.
func SettleSourceStatus(ctx context.Context, b *model.Block) {
	if b == nil || !b.Translatable {
		return
	}
	part := &model.Part{Type: model.PartBlock, Resource: b}

	// Source hygiene (empty/whitespace, doubled words, stray control chars…).
	lint := NewContentLintTool()
	_, _ = lint.ApplyContext(ctx, part)

	// Terminal readiness stamp: reads the findings the checks left and
	// promotes/demotes SourceStatus. A clean, already-approved source keeps its
	// approval.
	_, _ = NewSourceReadinessTool().ApplyContext(ctx, part)
}

// FindingLister lets annotations outside the unified quality.findings shape
// (e.g. the voice annotation) expose their findings to the source-readiness
// gate without this package importing them.
type FindingLister interface {
	CheckFindings() []Finding
}

// NewSourceReadinessTool creates the source-readiness stamp: a terminal check
// step that promotes a block's SourceStatus from the `authored` baseline to
// `checked` once the source is clean, and demotes it back to `authored` when it
// is not. It is the source-side counterpart of a translation producer stamping
// a target.
//
// It is derived, not a checker itself: it reads the findings the upstream
// source checks already left on the block (the unified Findings annotation
// plus any FindingLister annotation such as voice) and decides readiness from
// them, so it belongs LAST in a source-check sequence. A failing finding leaves
// the source at the `authored` baseline; a finding that only reports is
// tolerated. An already-`approved` source that is still clean keeps its
// approval (a clean re-check never downgrades a human sign-off).
func NewSourceReadinessTool() *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "source-check",
		ToolDescription: "Marks source content checked once it clears its voice/terminology checks",
	}
	t.Annotate = func(v tool.BlockView) error {
		// The emptiness guard is the shared run-aware presence predicate, so a
		// block that is only a placeholder still gets a readiness stamp. Under
		// SourceText() it flattened to "" and was skipped, so it never reached
		// `checked` and the source gate held it out of translation forever.
		if !v.Translatable() || !model.RunsHaveContent(v.SourceRuns()) {
			return nil
		}

		switch {
		case hasFailingSourceFinding(v):
			// A failing finding regresses readiness to the authored baseline,
			// even if the source was previously checked or approved — the source
			// changed (or a rule did) and no longer clears its checks.
			v.SetSourceStatus(model.SourceStatusAuthored)
		case v.SourceStatus().Rank() >= model.SourceStatusApproved.Rank():
			// Clean and already approved: a re-check never undoes a human
			// sign-off. Leave it as-is.
		default:
			v.SetSourceStatus(model.SourceStatusChecked)
		}
		return nil
	}
	return t
}

// hasFailingSourceFinding reports whether an upstream source check left a
// failing finding on the block, in the unified Findings annotation or in any
// FindingLister annotation (e.g. voice).
func hasFailingSourceFinding(v tool.BlockView) bool {
	for _, f := range Findings(v) {
		if f.Fails && !f.Suggested {
			return true
		}
	}
	for _, a := range v.Annotations() {
		if lister, ok := a.(FindingLister); ok {
			for _, f := range lister.CheckFindings() {
				if f.Fails && !f.Suggested {
					return true
				}
			}
		}
	}
	return false
}
