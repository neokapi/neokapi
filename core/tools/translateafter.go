package tools

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// TranslateAfterConfig configures the translate-after leading stage.
type TranslateAfterConfig struct {
	// Level is the source level a block must reach before it is translated
	// (none|written|established). Empty resolves to the default (written); any
	// other value outside the three levels is refused when the tool is built.
	Level string `json:"level,omitempty" schema:"title=Translate After,description=What a block's source must reach before its translations are produced: written (its checks pass) or established (a person reviewed it); none holds nothing,enum=none|written|established,default=written"`
}

// TranslateAfterTool is the leading source-transform stage of source-first
// convergence, the local CLI counterpart of the Bowrain server's settleSource +
// gateItemsBySource. It runs first in the converge flow and, per translatable
// source block:
//
//  1. settles the source: runs the provider-free source checks and stamps
//     SourceStatus (written) via the shared check.SettleSourceStatus, the same
//     derivation the server settle and `kapi check` use; and
//  2. holds the block when its source has not reached the configured level
//     (its checks fail, or an `established` level waits for a person), with
//     model.Block.SetSourceHeld. The block stays in the stream, its source
//     passes through to output untouched and the target falls back to source
//     (normal drift), but it carries the hold marker the downstream producers
//     (recycle, translate) read to skip it. A block that reaches the level has
//     any stale hold cleared and translates normally.
//
// At level `none` the tool is a passthrough with no settle cost and no hold,
// the deliberate "raw MT" opt-out, matching the server.
//
// It counts the source blocks it held this pass; the converge loop reads that
// to surface "N blocks held on source" and to park the run with
// source_not_ready when a locale has nothing producible. Counters are atomic
// because a wrapping ParallelBlockTool may drive Process's per-block work from
// several goroutines.
type TranslateAfterTool struct {
	tool.BaseTool
	level model.TranslateAfterLevel

	held  atomic.Int64
	total atomic.Int64
}

// NewTranslateAfterTool builds the translate-after leading stage for the given
// resolved level.
func NewTranslateAfterTool(level model.TranslateAfterLevel) *TranslateAfterTool {
	t := &TranslateAfterTool{level: level}
	t.ToolName = "translate-after"
	t.ToolDescription = "Settles source authoring status and holds each block's translation until its source reaches a level"
	return t
}

// Process overrides BaseTool.Process so the stage holds the *model.Block
// directly: settling calls the shared check.SettleSourceStatus (so this in-flow
// hold and the server settle derive readiness the same way), and the hold uses
// model.Block.SetSourceHeld (which deletes the marker when a block reaches the
// level, rather than leaving an empty property). The block is forwarded
// unchanged either way: the hold is metadata, and the content stays as read.
func (t *TranslateAfterTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				return nil
			}
			t.holdOne(ctx, part)
			select {
			case out <- part:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// holdOne settles one part's block and holds it when its source is below the
// level (a no-op for non-block or non-translatable parts, and for `none`).
func (t *TranslateAfterTool) holdOne(ctx context.Context, part *model.Part) {
	if t.level == model.TranslateAfterNone {
		return // opt-out: no settle, no hold
	}
	b, ok := part.Resource.(*model.Block)
	if !ok || b == nil || !b.Translatable {
		return
	}
	t.total.Add(1)

	// Settle via the shared helper, the same derivation the server settle uses.
	check.SettleSourceStatus(ctx, b)

	if t.level.AdmitsBlock(b) {
		b.SetSourceHeld(false) // ready: clear any stale hold from a prior pass
		return
	}
	b.SetSourceHeld(true)
	t.held.Add(1)
}

// NewTranslateAfterFromConfig builds the translate-after tool from a config map
// (the schema-driven flow path). An unset level is the default (written); a
// value that names no level is an error.
func NewTranslateAfterFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	var cfg TranslateAfterConfig
	if raw, ok := config["level"].(string); ok {
		cfg.Level = raw
	}
	level, known := model.ResolveTranslateAfter(cfg.Level)
	if !known {
		return nil, fmt.Errorf("translate-after: level %q is not a source level. Use written (the default), established or none", cfg.Level)
	}
	return NewTranslateAfterTool(level), nil
}

// Snapshot returns how many translatable source blocks the pass held below the
// level and how many it considered in total.
func (t *TranslateAfterTool) Snapshot() (held, total int) {
	return int(t.held.Load()), int(t.total.Load())
}
