package check

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// SettleSourceStatus runs the provider-free source checks over one block,
// stamps its SourceStatus at the written baseline unless a person has
// established it, and records whether the source fails its checks
// (model.Block.SetSourceFailing). It is the one derivation both venues share,
// the Bowrain server's settle pass and the local converge's translate-after
// stage, so they hold a block on identical findings.
//
// It is provider-free: gating on deterministic checks never spends AI credits
// on an unready corpus.
func SettleSourceStatus(ctx context.Context, b *model.Block) {
	if b == nil || !b.Translatable {
		return
	}
	part := &model.Part{Type: model.PartBlock, Resource: b}

	// Source hygiene (empty/whitespace, doubled words, stray control chars…).
	lint := NewContentLintTool()
	_, _ = lint.ApplyContext(ctx, part)

	// The emptiness guard is the shared run-aware presence predicate, so a
	// block that is only a placeholder is settled too.
	if !model.RunsHaveContent(b.SourceRuns()) {
		return
	}
	if b.SourceStatus != model.SourceStatusEstablished {
		b.SourceStatus = model.SourceStatusWritten
	}
	b.SetSourceFailing(hasFailingSourceFinding(tool.NewBlockViewWithContext(ctx, b)))
}

// FindingLister lets annotations outside the unified quality.findings shape
// (e.g. the voice annotation) expose their findings to the source-readiness
// gate without this package importing them.
type FindingLister interface {
	CheckFindings() []Finding
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
