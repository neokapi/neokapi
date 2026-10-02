package store

import (
	"maps"

	"github.com/neokapi/neokapi/core/model"
)

// PropSourceStatus is the reserved block-property key that carries a Block's
// source-authoring status (written→established) through the store's
// properties JSON. The ContentStore serializes only Block.Properties for a
// block's source-side metadata — it has no SourceStatus column — so the
// source-first convergence hold would lose the status it stamps unless the
// stores fold it into properties on write and lift it back out on read. The key
// is identical to the one core/venue uses on the wire, so the value
// round-trips losslessly across push → store → read.
const PropSourceStatus = "__source_status"

// TranslateAfterProperty is the project Properties key that holds the recipe's
// `defaults.translate_after` level (written | established | none). A push
// carries the recipe's level and ApplyRecipeSettings writes it here; once
// written, the server reads this property and nothing else.
const TranslateAfterProperty = "translate_after"

// TranslateAfterFor resolves a project's translate_after level from its
// settings, applying the default (`written`) when unset. A value the recipe
// schema does not recognize falls back to the default rather than silently
// disabling the hold. It is the single reader both the server orchestrator and
// the translation worker consult, so the hold is enforced identically at the
// fan-out decision and at the per-block translation.
func TranslateAfterFor(proj *Project) model.TranslateAfterLevel {
	raw := ""
	if proj != nil && proj.Properties != nil {
		raw = proj.Properties[TranslateAfterProperty]
	}
	level, _ := model.ResolveTranslateAfter(raw)
	return level
}

// PropsForStore returns the block's Properties augmented with the status of its
// authoritative edition, under the reserved PropSourceStatus key, ready to
// serialize into the store's properties JSON. It never mutates the block's own
// map (copy-on-write). A block whose source carries no committed status returns
// its Properties unchanged, so the common case carries no extra key.
func PropsForStore(b *model.Block) map[string]string {
	if b == nil {
		return nil
	}
	src, _ := b.Edition(b.Authoritative(model.AuthorityPolicy{}))
	if src.Status == "" {
		return b.Properties
	}
	props := make(map[string]string, len(b.Properties)+1)
	maps.Copy(props, b.Properties)
	props[PropSourceStatus] = string(src.Status)
	return props
}

// ApplySourceStatusFromProps lifts the reserved PropSourceStatus key out of a
// block's freshly-scanned Properties onto the status of its authoritative
// edition, and strips it, so the status never leaks back out as an ordinary
// property. It is the read-side counterpart of PropsForStore. The status is
// part of the block as stored, so it is set alone: the source content and its
// origin stay as the scan produced them.
func ApplySourceStatusFromProps(b *model.Block) {
	if b == nil || b.Properties == nil {
		return
	}
	status, ok := b.Properties[PropSourceStatus]
	if !ok {
		return
	}
	b.SetEditionStatus(b.Authoritative(model.AuthorityPolicy{}), model.Status(status))
	delete(b.Properties, PropSourceStatus)
	if len(b.Properties) == 0 {
		b.Properties = nil
	}
}
