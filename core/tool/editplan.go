package tool

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/change"
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
// A text transform (a substitution, a case conversion) takes a third shape on
// any edition: TextEdits, positions in the edition's text and what to put
// there, applied as change.ReplaceText. Everything the edits leave alone is
// kept: the inline codes, the plural and select structure and the run flags.
//
// Secrets are the originals a recoverable transformer (redaction) vaults: the
// applier hands them to the tool's VaultSecrets sink before the rewrite is
// applied, so secret capture and source rewrite are atomic — a plan with
// secrets and no sink is an error, never a silent drop.
//
// Targets replaces target content per variant (e.g. unredact restoring
// originals into translated targets, a case conversion applied to a target).
// A replaced target whose wording changes becomes a draft, because nobody has
// read what the tool wrote, and keeps the origin that names who produced it
// (change.Consequences).
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
	// TextEdits are in-place text rewrites per edition; the zero key is the
	// source. Each entry is one pass, applied as a replace_text in order, so a
	// pass names positions in the text the passes before it left. On the source
	// it excludes NewRuns and ReplaceAll, and on a target it excludes Targets.
	TextEdits map[model.VariantKey][][]change.TextEdit
}

// Empty reports whether the plan changes nothing.
func (p *EditPlan) Empty() bool {
	return p.NewRuns == nil && p.ReplaceAll == nil &&
		len(p.Secrets) == 0 && len(p.Targets) == 0 && len(p.TextEdits) == 0
}

// AddTextEdits records one pass of in-place text edits on an edition, the
// source for the zero key, allocating the map on first use. A pass with no
// edit is not recorded.
func (p *EditPlan) AddTextEdits(key model.VariantKey, edits []change.TextEdit) {
	if len(edits) == 0 {
		return
	}
	if p.TextEdits == nil {
		p.TextEdits = make(map[model.VariantKey][][]change.TextEdit)
	}
	key = key.Canonical()
	p.TextEdits[key] = append(p.TextEdits[key], edits)
}

// SetTarget records a target-run replacement for a plain locale variant,
// allocating the map on first use.
func (p *EditPlan) SetTarget(loc model.LocaleID, runs []model.Run) {
	p.SetTargetVariant(model.Variant(loc), runs)
}

// SetTargetVariant records a target-run replacement for a variant key,
// allocating the map on first use. The key is kept canonical, so two
// spellings of one locale name one replacement.
func (p *EditPlan) SetTargetVariant(key model.VariantKey, runs []model.Run) {
	if p.Targets == nil {
		p.Targets = make(map[model.VariantKey][]model.Run)
	}
	p.Targets[key.Canonical()] = runs
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

// Ops compiles the plan into the operations that apply it to block: a
// set_content on the source for a rewrite, or a replace_text for each pass of
// its text edits, then the same for each target in the order of their keys. A
// structured rewrite carries its Edits, so the source's overlays follow it; an
// opaque one drops them. Every operation writes whatever the edition holds
// ("*"): the plan was made from the block in hand.
func (p *EditPlan) Ops(block *model.Block) ([]change.Op, error) {
	if p.ReplaceAll != nil && (p.NewRuns != nil || len(p.Edits) > 0) {
		return nil, errors.New("edit plan sets both ReplaceAll and NewRuns/Edits: a rewrite is either structured or opaque, never both")
	}
	if p.NewRuns == nil && len(p.Edits) > 0 {
		return nil, errors.New("edit plan has Edits but no NewRuns")
	}
	if len(p.TextEdits[model.VariantKey{}]) > 0 && (p.ReplaceAll != nil || p.NewRuns != nil) {
		return nil, errors.New("edit plan rewrites the source both in place (TextEdits) and whole (NewRuns or ReplaceAll)")
	}
	at := func(key model.EditionKey) change.Ref { return change.Ref{Block: block.ID, Edition: key} }
	replaceText := func(key model.EditionKey) []change.Op {
		var out []change.Op
		for _, pass := range p.TextEdits[key] {
			out = append(out, change.Op{Kind: change.KindReplaceText, At: at(key), IfMatch: change.AnyRevision,
				Body: &change.ReplaceText{Edits: pass}})
		}
		return out
	}
	var ops []change.Op
	switch {
	case p.ReplaceAll != nil:
		ops = append(ops, change.Op{Kind: change.KindSetContent, At: at(model.EditionKey{}), IfMatch: change.AnyRevision,
			Body: &change.SetContent{Runs: []model.Run{{Text: &model.TextRun{Text: *p.ReplaceAll}}}, Overlays: change.OverlayRebase{Drop: true}}})
	case p.NewRuns != nil:
		if len(p.Edits) == 0 && model.RunsText(authoritative(block).Runs) != model.RunsText(p.NewRuns) {
			return nil, fmt.Errorf("the plan changes the source text of block %q without a mapping. Return Edits for a structured rewrite or ReplaceAll for an opaque one", block.ID)
		}
		edits := p.Edits
		if edits == nil {
			edits = []model.RunEdit{}
		}
		ops = append(ops, change.Op{Kind: change.KindSetContent, At: at(model.EditionKey{}), IfMatch: change.AnyRevision,
			Body: &change.SetContent{Runs: p.NewRuns, Overlays: change.OverlayRebase{Edits: edits}}})
	}
	ops = append(ops, replaceText(model.EditionKey{})...)
	var keys []model.VariantKey
	for key := range p.Targets {
		keys = append(keys, key.Canonical())
	}
	for key := range p.TextEdits {
		if !key.IsZero() && !slices.Contains(keys, key.Canonical()) {
			keys = append(keys, key.Canonical())
		}
	}
	slices.SortFunc(keys, func(a, b model.VariantKey) int {
		at, _ := a.MarshalText()
		bt, _ := b.MarshalText()
		return strings.Compare(string(at), string(bt))
	})
	for _, key := range keys {
		if block.IsSourceEdition(key) {
			return nil, sourceLanguageTarget(block, key)
		}
		runs, replaced := p.Targets[key]
		if len(p.TextEdits[key]) > 0 {
			if replaced {
				text, _ := key.MarshalText()
				return nil, fmt.Errorf("edit plan rewrites the target in %s both in place (TextEdits) and whole (Targets)", text)
			}
			ops = append(ops, replaceText(key)...)
			continue
		}
		if runs == nil {
			runs = []model.Run{}
		}
		ops = append(ops, change.Op{Kind: change.KindSetContent, At: at(key), IfMatch: change.AnyRevision,
			Body: &change.SetContent{Runs: runs}})
	}
	return ops, nil
}

// applyEditPlan is the framework applier for a transform (AD-006): it vaults
// the plan's secrets, then applies the plan's operations (Ops) through
// change.ApplyBlock, the one function that changes a block's content. Order
// is fail-closed: a rewrite never lands without its recovery record. The
// source rewrite is an edit, so the block keeps the source it was read with
// and its writer can encode the new wording; the source's overlays follow a
// structured rewrite and every one left is in bounds; a target whose wording
// the plan changes becomes a draft and keeps its origin.
func applyEditPlan(toolName string, v *blockView, block *model.Block, plan EditPlan, vault func(BlockView, []Secret) error) error {
	ops, err := plan.Ops(block)
	if err != nil {
		return fmt.Errorf("transform tool %q: %w", toolName, err)
	}
	if len(plan.Secrets) > 0 {
		if vault == nil {
			return fmt.Errorf("transform tool %q produced %d secrets but set no VaultSecrets sink: a recoverable transform must vault its originals", toolName, len(plan.Secrets))
		}
		if err := vault(v, plan.Secrets); err != nil {
			return fmt.Errorf("transform tool %q: vault secrets: %w", toolName, err)
		}
	}
	if len(ops) == 0 {
		return nil
	}
	env := change.BlockEnv{Actor: change.Actor{Kind: change.ActorTool, Name: toolName}, Guards: change.Report}
	for _, r := range change.ApplyBlock(block, ops, env) {
		if r.Status == change.OpRefused {
			return fmt.Errorf("transform tool %q: block %q: %s %s refused: %w", toolName, block.ID, r.Op, editionText(r.At), r.Error)
		}
	}
	return nil
}
