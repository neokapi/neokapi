package tool

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
)

// EditPlan is what a Transform producer returns (AD-006 Transformer model).
// The producer is read-only — it inspects the block through a BlockView and
// describes the rewrite; the framework applier (BaseTool dispatch) is the only
// code that mutates the block's representation. A zero EditPlan is a no-op.
//
// Source rewrites take one of two shapes:
//
//   - Structured: NewRuns holds the rewritten source and Edits the
//     span→replacement mapping (in the flattened-text rune coordinate space of
//     the OLD runs). The applier rebases the surviving run-anchored source
//     overlays across the rewrite (model.RemapOverlays): spans overlapping an
//     edit are dropped, the rest follow the text. NewRuns with no Edits is a
//     structure-only rewrite — runs added, removed, or reclassified without
//     changing the text flattening — and the applier verifies the flattening is
//     unchanged before re-anchoring.
//
//   - Opaque: ReplaceAll replaces the whole source with plain text. There is no
//     derivable mapping, so the applier drops every source-side overlay (and any
//     inline codes) rather than leave them dangling. Reserve this for rewrites
//     with no structured form (an LLM rewrite, a whole-text conversion).
//
// Secrets are the originals a recoverable transformer (redaction) vaults: the
// applier hands them to the tool's VaultSecrets sink before the rewrite is
// applied, so secret capture and source rewrite are atomic — a plan with
// secrets and no sink is an error, never a silent drop.
//
// Targets replaces target content per variant (e.g. unredact restoring
// originals into translated targets, a case conversion applied to a target).
// Variant metadata (status, provenance) is preserved; only the runs change.
type EditPlan struct {
	// NewRuns is the rewritten source for a structured transform; nil means no
	// source rewrite.
	NewRuns []model.Run
	// Edits is the structured old→new mapping for overlay rebasing, expressed
	// against the pre-rewrite flattened source text in rune offsets, sorted
	// ascending and non-overlapping (see model.RunEdit).
	Edits []model.RunEdit
	// ReplaceAll is the opaque whole-source rewrite; mutually exclusive with
	// NewRuns/Edits.
	ReplaceAll *string
	// Secrets are originals to vault atomically with the rewrite.
	Secrets []Secret
	// Targets are per-variant target-run replacements.
	Targets map[model.VariantKey][]model.Run
}

// Empty reports whether the plan changes nothing.
func (p *EditPlan) Empty() bool {
	return p.NewRuns == nil && p.ReplaceAll == nil &&
		len(p.Secrets) == 0 && len(p.Targets) == 0
}

// SetTarget records a target-run replacement for a plain locale variant,
// allocating the map on first use.
func (p *EditPlan) SetTarget(loc model.LocaleID, runs []model.Run) {
	p.SetTargetVariant(model.Variant(loc), runs)
}

// SetTargetVariant records a target-run replacement for a variant key,
// allocating the map on first use.
func (p *EditPlan) SetTargetVariant(key model.VariantKey, runs []model.Run) {
	if p.Targets == nil {
		p.Targets = make(map[model.VariantKey][]model.Run)
	}
	p.Targets[key] = runs
}

// Secret is one vaulted original produced by a recoverable transformer: the
// stable placeholder token, the category, the visible stand-in (Disp), and the
// original sensitive text. The applier passes secrets to the tool's
// VaultSecrets sink; the original never enters the rewritten content.
type Secret struct {
	Token    string
	Category string
	Disp     string
	Original string
}

// FullSpanEdit returns the single whole-text RunEdit mapping oldRuns'
// flattening to newRuns' — the "everything changed" structured mapping for a
// run rewrite whose per-span edits are not derivable. Every source overlay
// span overlaps it, so the applier drops them all while the run structure is
// preserved (unlike ReplaceAll, which also flattens inline codes). Returns nil
// when the flattenings are equal — a structure-only rewrite needs no edit.
func FullSpanEdit(oldRuns, newRuns []model.Run) []model.RunEdit {
	oldText, newText := model.RunsText(oldRuns), model.RunsText(newRuns)
	if oldText == newText {
		return nil
	}
	return []model.RunEdit{{
		Start:  0,
		End:    len([]rune(oldText)),
		NewLen: len([]rune(newText)),
	}}
}

// applyEditPlan is the framework applier — the single place a transform
// mutates a Block (AD-006). Order is fail-closed: secrets are vaulted first
// (a rewrite never lands without its recovery record), then the source rewrite
// is applied and surviving overlays rebased, then targets are replaced, and
// finally every surviving source overlay span is asserted in-bounds.
func applyEditPlan(toolName string, v *blockView, block *model.Block, plan EditPlan, vault func(BlockView, []Secret) error) error {
	if plan.ReplaceAll != nil && (plan.NewRuns != nil || len(plan.Edits) > 0) {
		return fmt.Errorf("transform tool %q: edit plan sets both ReplaceAll and NewRuns/Edits — a rewrite is structured or opaque, not both", toolName)
	}
	if plan.NewRuns == nil && len(plan.Edits) > 0 {
		return fmt.Errorf("transform tool %q: edit plan has Edits but no NewRuns", toolName)
	}

	if len(plan.Secrets) > 0 {
		if vault == nil {
			return fmt.Errorf("transform tool %q produced %d secrets but set no VaultSecrets sink — a recoverable transform must vault its originals", toolName, len(plan.Secrets))
		}
		if err := vault(v, plan.Secrets); err != nil {
			return fmt.Errorf("transform tool %q: vault secrets: %w", toolName, err)
		}
	}

	rewrote := false
	switch {
	case plan.ReplaceAll != nil:
		block.SetSourceText(*plan.ReplaceAll)
		model.DropSourceOverlays(block)
		rewrote = true
	case plan.NewRuns != nil:
		old := block.Source
		if len(plan.Edits) == 0 && model.RunsText(old) != model.RunsText(plan.NewRuns) {
			return fmt.Errorf("transform tool %q changed the source text of block %q without a mapping — return Edits for a structured rewrite or ReplaceAll for an opaque one", toolName, block.ID)
		}
		block.SetSourceRuns(plan.NewRuns)
		model.RemapOverlays(block, old, plan.Edits)
		rewrote = true
	}

	for key, runs := range plan.Targets {
		setTargetVariantRuns(block, key, runs)
	}

	if rewrote {
		if bad, ok := block.SourceOverlaysInBounds(); !ok {
			return fmt.Errorf("transform tool %q rewrote the source of block %q but its edit plan left source overlay %q anchored out of bounds — the Edits do not describe the rewrite", toolName, block.ID, bad)
		}
	}
	return nil
}

// setTargetVariantRuns replaces a target variant's runs, preserving existing
// variant metadata (status, provenance) when the variant already exists.
func setTargetVariantRuns(b *model.Block, key model.VariantKey, runs []model.Run) {
	if b.Targets == nil {
		b.Targets = make(map[model.VariantKey]*model.Target)
	}
	if t, ok := b.Targets[key]; ok && t != nil {
		t.Runs = runs
		return
	}
	b.Targets[key] = &model.Target{Runs: runs}
}
