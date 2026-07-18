package tools

import (
	"context"
	"sync/atomic"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// SourceGateConfig configures the source-gate leading stage.
type SourceGateConfig struct {
	// Gate is the resolved source-first gate level (authored|checked|approved|
	// none). Empty resolves to the default (checked) via ResolveSourceGate — a
	// typo must not silently disable the gate, so an unrecognized value also
	// gates at checked.
	Gate string `json:"gate,omitempty" schema:"title=Source Gate,description=Minimum source-authoring status a block must reach before its translations are produced,enum=none|authored|checked|approved,default=checked"`
}

// SourceGateTool is the leading source-transform stage of source-first
// convergence (strategy 2026-07-dogfood doc 07 / roadmap epic 019), the local
// CLI counterpart of the Bowrain server's settleSource + gateItemsBySource. It
// runs FIRST in the converge flow and, per translatable source block:
//
//  1. settles the source — runs the provider-free source checks and stamps
//     SourceStatus (authored→checked) via the shared check.SettleSourceStatus,
//     the same derivation the server settle and `kapi check` use; and
//  2. gates the block — a block whose settled SourceStatus ranks below the
//     configured gate is HELD (model.Block.SetSourceHeld): it stays in the
//     stream (its source passes through to output untouched — the target simply
//     falls back to source, normal drift) but carries the hold marker the
//     downstream producers (recycle, translate) read to skip it. A block that
//     clears the gate has any stale hold cleared and translates normally.
//
// When the gate is `none` the tool is a no-op passthrough: no settle cost, no
// hold — the deliberate "raw MT, no gate" opt-out, parity with the server.
//
// It counts the source blocks it held this pass (Held); the converge loop reads
// that to surface "N blocks held on source" and to park the run with
// source_not_ready when a locale has nothing producible. Counters are atomic
// because a wrapping ParallelBlockTool may drive Process's per-block work from
// several goroutines.
type SourceGateTool struct {
	tool.BaseTool
	gate model.SourceGateLevel

	held  atomic.Int64
	total atomic.Int64
}

// NewSourceGateTool builds the source-gate leading stage for the given resolved
// gate level.
func NewSourceGateTool(gate model.SourceGateLevel) *SourceGateTool {
	t := &SourceGateTool{gate: gate}
	t.ToolName = "source-gate"
	t.ToolDescription = "Settles source authoring status and holds blocks below the source gate before translation"
	return t
}

// Process overrides BaseTool.Process so the stage holds the *model.Block
// directly: settling calls the SHARED check.SettleSourceStatus (so this in-flow
// gate and the server settle can never derive readiness differently), and the
// hold uses model.Block.SetSourceHeld (which cleanly deletes the marker when a
// block clears the gate, rather than leaving an empty property). The block is
// forwarded unchanged either way — the hold is metadata, not a content rewrite.
func (t *SourceGateTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				return nil
			}
			t.gateOne(ctx, part)
			select {
			case out <- part:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// gateOne settles and gates one part's block (a no-op for non-block or
// non-translatable parts, and for the `none` opt-out).
func (t *SourceGateTool) gateOne(ctx context.Context, part *model.Part) {
	if t.gate == model.SourceGateNone {
		return // opt-out: no settle, no hold
	}
	b, ok := part.Resource.(*model.Block)
	if !ok || b == nil || !b.Translatable {
		return
	}
	t.total.Add(1)

	// Settle via the shared helper — the same authored→checked derivation the
	// server settle and `kapi check` use.
	check.SettleSourceStatus(ctx, b)

	if t.gate.Admits(b.SourceStatus) {
		b.SetSourceHeld(false) // ready: clear any stale hold from a prior pass
		return
	}
	b.SetSourceHeld(true)
	t.held.Add(1)
}

// NewSourceGateFromConfig builds the source-gate tool from a config map (the
// schema-driven flow path). It resolves the gate string through the canonical
// ResolveSourceGate so an unset/unknown value lands on the default (checked).
func NewSourceGateFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	var cfg SourceGateConfig
	if raw, ok := config["gate"].(string); ok {
		cfg.Gate = raw
	}
	gate, _ := model.ResolveSourceGate(cfg.Gate)
	return NewSourceGateTool(gate), nil
}

// Snapshot returns how many translatable source blocks the pass held below the
// gate and how many it considered in total.
func (t *SourceGateTool) Snapshot() (held, total int) {
	return int(t.held.Load()), int(t.total.Load())
}
