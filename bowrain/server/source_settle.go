package server

import (
	"context"
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// This file is the server venue of source-first convergence (strategy
// 2026-07-dogfood doc 07 / roadmap epic 019): settle the SOURCE before
// translating it into N languages. It runs as a small explicit server-side
// settle step (approach (a) in the epic), mirroring how #1312 folded the
// content memory-first recycle step into the job pipeline rather than resurrecting the flow
// engine — the server still bypasses flows, so a leading source-transform stage
// has no home; a focused settle pass over the source-locale blocks is the
// smaller, consistent change.
//
// Settlement is deliberately provider-free: the automated gate (`checked`) is
// satisfied by deterministic source checks (source content hygiene today;
// terminology/brand extensible), so holding an unsettled corpus never itself
// burns AI credits — exactly the "no surprise AI spend" the first-run decision
// demands. Deeper, LLM-backed source brand-checking is layered on top later
// (deferred), not in front of the gate.

// propSettledHash records the source content hash a block's SourceStatus was
// stamped against. When the source changes (content-hash change), the recorded
// status no longer describes the current source, so settlement resets the block
// to the authored baseline and re-checks it — re-gating ONLY the changed block,
// never the whole corpus (epic 019 acceptance #6). It rides in block properties
// alongside the status itself.
const propSettledHash = "__source_settled_hash"

// settleResult reports one source-settlement pass over a project.
type settleResult struct {
	// Settled is how many source blocks the pass re-stamped this run.
	Settled int
	// BlockedOnSource is how many translatable source blocks remain below the
	// source gate after settlement — the count surfaced as "settle your source
	// first" and the trigger for a source-review task + a source_not_ready hold.
	BlockedOnSource int
	// Total is how many translatable source blocks were considered.
	Total int
	// Gate is the resolved gate level applied (for logging/observability).
	Gate model.SourceGateLevel
}

// sourceGateFor resolves the project's source-first gate level from its settings
// (defaults.source_gate, carried to the server in project Properties). It is a
// thin alias for the shared store reader so the server and the worker resolve
// the gate identically.
func sourceGateFor(proj *platstore.Project) model.SourceGateLevel {
	return platstore.SourceGateFor(proj)
}

// settleSource runs the source-settlement phase over a project's source-locale
// blocks: for each translatable block it (re)derives the source-authoring
// status — resetting a block whose source changed since it was last settled,
// running the provider-free source checks, and stamping SourceStatus via the
// framework's SourceReadinessTool — then persists the blocks whose status moved.
// It returns how many blocks settled and how many remain below the gate.
//
// It is a no-op (no store writes, BlockedOnSource=0) when the gate is disabled
// (`source_gate: none`): the opt-out must never pay the settlement cost or hold
// the fan-out.
func (o *convergenceOrchestrator) settleSource(ctx context.Context, projectID string) (settleResult, error) {
	s := o.server
	// No content store (the in-memory driveWith tests): report the gate as
	// disabled so the caller emits no settle event and never holds — settlement
	// has nothing to read.
	res := settleResult{Gate: model.SourceGateNone}
	if s.ContentStore == nil {
		return res, nil
	}
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil {
		return res, fmt.Errorf("load project: %w", err)
	}
	res.Gate = sourceGateFor(proj)
	if res.Gate == model.SourceGateNone {
		return res, nil // opt-out: no gate, no settle
	}

	// Walked a batch at a time, and persisted a batch at a time. Reading the
	// project in one query is what OOM-killed the production server: it idles
	// at ~16 MiB and this read drove it past 934 MiB of a 1 GiB task on a
	// thousand-item project. Peak is now the batch, whatever the corpus.
	err = platstore.EachBlockBatch(ctx, s.ContentStore,
		platstore.BlockQuery{ProjectID: projectID, Stream: "main"}, 0,
		func(blocks []*venue.StoredBlock) error {
			return o.settleBatch(ctx, projectID, blocks, &res)
		})
	if err != nil {
		return res, err
	}
	return res, nil
}

// settleBatch settles one batch of blocks and persists the ones that moved,
// so the batch becomes garbage before the next one is read.
func (o *convergenceOrchestrator) settleBatch(
	ctx context.Context,
	projectID string,
	blocks []*venue.StoredBlock,
	res *settleResult,
) error {
	var changed []*model.Block
	for _, sb := range blocks {
		if sb == nil || sb.Block == nil || !sb.Block.Translatable {
			continue
		}
		b := sb.Block
		res.Total++

		before := b.SourceStatus
		beforeHash := settledHash(b)
		// Re-gate on source change: if the block's source no longer matches the
		// content it was settled against, its committed status (and any human
		// approval) is stale — reset to the authored baseline so this pass
		// re-checks it from scratch. Only the changed block resets; untouched
		// blocks keep their status and are skipped by the store write below.
		if sourceChangedSinceSettle(b, sb.ContentHash) {
			b.SourceStatus = model.SourceStatusNew
		}

		settleBlockStatus(ctx, b)
		stampSettledHash(b, sb.ContentHash)

		if b.SourceStatus != before {
			res.Settled++
		}
		if !res.Gate.Admits(b.SourceStatus) {
			res.BlockedOnSource++
		}
		// Persist only blocks that actually moved: a status change OR a newly
		// recorded/updated settled-hash. An already-settled, unchanged block is
		// skipped, so a steady-state run rewrites nothing (re-gate ONLY the
		// changed block — epic 019 acceptance #6).
		if b.SourceStatus != before || settledHash(b) != beforeHash {
			changed = append(changed, b)
		}
	}

	if len(changed) > 0 {
		if err := o.server.ContentStore.StoreBlocks(ctx, projectID, "main", changed); err != nil {
			return fmt.Errorf("persist settled source: %w", err)
		}
	}
	return nil
}

// gateItemsBySource partitions a project's items by the source-first gate: an
// item is producible when at least one of its translatable source blocks clears
// the gate; an item whose blocks are ALL below the gate is held on source. It
// returns the producible item names (order preserved) and how many named items
// were fully held. When the gate is disabled (`source_gate: none`) every item is
// producible and none is held, so the caller behaves exactly as before
// source-first.
func (o *convergenceOrchestrator) gateItemsBySource(ctx context.Context, projectID string, itemNames []string) (producible []string, blockedItems int, err error) {
	s := o.server
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil {
		return nil, 0, fmt.Errorf("load project: %w", err)
	}
	gate := sourceGateFor(proj)
	if gate == model.SourceGateNone {
		return itemNames, 0, nil // opt-out: translate everything
	}

	// Per-item readiness: does the item have any translatable source block whose
	// source clears the gate? An item with no translatable blocks is treated as
	// producible (nothing to hold — it falls through to the normal no-op path).
	//
	// Walked in batches: what this needs from the corpus is two booleans per
	// ITEM, so holding the blocks themselves is the whole cost and none of the
	// answer.
	readyItem := map[string]bool{}
	seenItem := map[string]bool{}
	err = platstore.EachBlockBatch(ctx, s.ContentStore,
		platstore.BlockQuery{ProjectID: projectID, Stream: "main"}, 0,
		func(blocks []*venue.StoredBlock) error {
			for _, sb := range blocks {
				if sb == nil || sb.Block == nil || !sb.Block.Translatable {
					continue
				}
				seenItem[sb.ItemName] = true
				if gate.Admits(sb.Block.SourceStatus) {
					readyItem[sb.ItemName] = true
				}
			}
			return nil
		})
	if err != nil {
		return nil, 0, fmt.Errorf("load blocks for gate: %w", err)
	}
	for _, name := range itemNames {
		switch {
		case !seenItem[name]:
			producible = append(producible, name) // no translatable source — not held
		case readyItem[name]:
			producible = append(producible, name)
		default:
			blockedItems++
		}
	}
	return producible, blockedItems, nil
}

// settleBlockStatus runs the provider-free source checks over one block and
// stamps its SourceStatus with the framework's SourceReadinessTool — the same
// terminal readiness stamp `kapi check` and the local converge's source-gate
// stage use, so the server and CLI derive the same authored→checked promotion
// from the same findings. It is a thin alias for the shared core helper so the
// two venues cannot drift.
func settleBlockStatus(ctx context.Context, b *model.Block) {
	check.SettleSourceStatus(ctx, b)
}

// sourceChangedSinceSettle reports whether a block's current source content hash
// differs from the hash its SourceStatus was last stamped against. A block with
// no committed status (New) has nothing stale to reset. A block with a committed
// status but no recorded hash is a status that has not yet been through a local
// settle pass — e.g. an approval pushed from the wire (core/venue carries
// __source_status but not the settled-hash). The first settle records the
// current hash for it (stampSettledHash, always called), so any SUBSEQUENT
// content edit is caught and re-gated. Because every push starts a convergence
// run that settles, a push-approved block is always stamped before it can be
// edited — so the "committed status, no hash" case reads as unchanged here and
// is verified on this pass rather than demoted (which would defeat a legitimate
// pushed approval).
func sourceChangedSinceSettle(b *model.Block, currentHash string) bool {
	if b.SourceStatus == model.SourceStatusNew {
		return false
	}
	prev := ""
	if b.Properties != nil {
		prev = b.Properties[propSettledHash]
	}
	if prev == "" || currentHash == "" {
		return false
	}
	return prev != currentHash
}

// settledHash reads the content hash a block's SourceStatus was last stamped
// against (empty when never settled).
func settledHash(b *model.Block) string {
	if b.Properties == nil {
		return ""
	}
	return b.Properties[propSettledHash]
}

// stampSettledHash records the content hash this settle pass judged, so the next
// pass can detect a source change and re-gate the block.
func stampSettledHash(b *model.Block, currentHash string) {
	if currentHash == "" {
		return
	}
	if b.Properties == nil {
		b.Properties = map[string]string{}
	}
	b.Properties[propSettledHash] = currentHash
}
