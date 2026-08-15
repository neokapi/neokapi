package store

import (
	"maps"

	"github.com/neokapi/neokapi/core/model"
)

// PropSourceStatus is the reserved block-property key that carries a Block's
// source-authoring status (authored → checked → approved) through the store's
// properties JSON. The ContentStore serializes only Block.Properties for a
// block's source-side metadata — it has no SourceStatus column — so the
// source-first convergence gate would lose the status it stamps unless the
// stores fold it into properties on write and lift it back out on read. The key
// is identical to the one core/venue uses on the wire, so the value
// round-trips losslessly across push → store → read.
const PropSourceStatus = "__source_status"

// SourceGateProperty is the project-settings key that carries the recipe's
// `defaults.source_gate` level (checked | approved | authored | none) to the
// server, alongside the other recipe-derived settings in project Properties.
const SourceGateProperty = "source_gate"

// SourceGateFor resolves a project's source-first convergence gate level from
// its settings, applying the default (`checked`) when unset. A value the recipe
// schema does not recognize falls back to the default rather than silently
// disabling the gate. It is the single reader both the server orchestrator and
// the translation worker consult, so the gate is enforced identically at the
// fan-out decision and at the per-block translation.
func SourceGateFor(proj *Project) model.SourceGateLevel {
	raw := ""
	if proj != nil && proj.Properties != nil {
		raw = proj.Properties[SourceGateProperty]
	}
	gate, _ := model.ResolveSourceGate(raw)
	return gate
}

// PropsForStore returns the block's Properties augmented with its SourceStatus
// under the reserved PropSourceStatus key, ready to serialize into the store's
// properties JSON. It never mutates the block's own map (copy-on-write). A block
// with no committed SourceStatus returns its Properties unchanged, so the common
// case carries no extra key.
func PropsForStore(b *model.Block) map[string]string {
	if b == nil || b.SourceStatus == "" {
		if b == nil {
			return nil
		}
		return b.Properties
	}
	props := make(map[string]string, len(b.Properties)+1)
	maps.Copy(props, b.Properties)
	props[PropSourceStatus] = string(b.SourceStatus)
	return props
}

// ApplySourceStatusFromProps lifts the reserved PropSourceStatus key out of a
// block's freshly-scanned Properties onto Block.SourceStatus and strips it, so
// the status never leaks back out as an ordinary property. It is the read-side
// counterpart of PropsForStore.
func ApplySourceStatusFromProps(b *model.Block) {
	if b == nil || b.Properties == nil {
		return
	}
	status, ok := b.Properties[PropSourceStatus]
	if !ok {
		return
	}
	b.SourceStatus = model.SourceStatus(status)
	delete(b.Properties, PropSourceStatus)
	if len(b.Properties) == 0 {
		b.Properties = nil
	}
}
