package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/leverage"
)

// MemoryResolver returns the project's server content memory for a workspace.
// It mirrors the server's workspaceStores.getMemory: a per-workspace, PostgreSQL-
// backed Store keyed by the workspace slug. It is optional on WorkerDeps —
// when nil (self-hosted without a content memory, or a bare worker) the convergence job
// degrades to the previous AI-only behavior. Errors are treated as "no content memory
// available" by the caller, never fatal to a translation job.
type MemoryResolver interface {
	GetMemory(workspaceSlug string) (memory.Store, error)
}

// MemoryResolverFunc adapts a plain function to the MemoryResolver interface.
type MemoryResolverFunc func(workspaceSlug string) (memory.Store, error)

// GetMemory implements MemoryResolver.
func (f MemoryResolverFunc) GetMemory(workspaceSlug string) (memory.Store, error) {
	return f(workspaceSlug)
}

// recycleResult reports how a recycle pass split the input blocks: which blocks
// were filled from the content memory (and must skip the AI translate step) and which
// remain for AI. Counts are block counts, matching the convergence loop's
// block-count denominators.
type recycleResult struct {
	// filled holds the blocks whose target was set from a content-memory match; they are
	// persisted as-is and never sent to the AI translator.
	filled []*model.Block
	// remainder holds the blocks with no usable content-memory match; only these go to AI.
	remainder []*model.Block
	// memoryCount is len(filled) — blocks attributed to content memory in ViaMemory reporting.
	memoryCount int
}

// defaultMemoryMinScore is the recycle lookup threshold used when a project does
// not set one — the framework's canonical fuzzy floor (0.7 → the recycle tool's
// default FuzzyThreshold of 70), so the platform and the CLI recycle at the same
// threshold. The lookup floor is not the fill bar: the tool fills only at or
// above its fill threshold (near-exact), so a fuzzy match below that surfaces as
// an alt-translation candidate, not a silent fill.
const defaultMemoryMinScore = 0.7

// recycleBlocks runs the one framework recycle tool over the stored blocks and
// partitions them into content memory-filled vs. remainder. It mirrors the
// built-in `translate` flow's recycle→translate ordering: exact (and, at the
// configured fuzzy threshold, near-exact) matches are pre-filled from the
// project content memory so only genuinely-new segments are sent to the paid AI step.
//
// The tool fills a target only for near-exact and better matches, guarded so a
// fill never drops an inline code the source carries; a block is counted as
// content memory-filled when it carries a fresh target for the locale after the
// pass. minScore comes from the project's recipe content memory threshold when
// present (default fuzzy at defaultMemoryMinScore, matching the CLI recycle flow).
func recycleBlocks(ctx context.Context, tm memory.Store, storedBlocks []*store.StoredBlock, sourceLocale, targetLocale model.LocaleID, minScore float64) (recycleResult, error) {
	if minScore <= 0 {
		minScore = defaultMemoryMinScore
	}

	// Blocks that already carry a target for this locale are neither recycled
	// nor re-translated — they are already done. Keep them out of both buckets
	// so we never re-pay or double-count them.
	var candidates []*store.StoredBlock
	for _, sb := range storedBlocks {
		if sb == nil || sb.Block == nil {
			continue
		}
		if hasLocaleTarget(sb.Block, targetLocale) {
			continue
		}
		candidates = append(candidates, sb)
	}

	memoryTool := leverage.NewTool(tm, sourceLocale, targetLocale, int(minScore*100))

	parts := storedBlocksToParts(candidates)
	outParts, err := runToolOnParts(ctx, memoryTool, parts)
	if err != nil {
		return recycleResult{}, err
	}

	var res recycleResult
	for _, b := range partsToBlocks(outParts) {
		if hasLocaleTarget(b, targetLocale) {
			res.filled = append(res.filled, b)
		} else {
			res.remainder = append(res.remainder, b)
		}
	}
	res.memoryCount = len(res.filled)
	return res, nil
}

// hasLocaleTarget reports whether the block carries a non-empty target for the
// locale (any variant tone/channel under that locale counts).
func hasLocaleTarget(b *model.Block, locale model.LocaleID) bool {
	if b == nil {
		return false
	}
	for key, t := range b.Targets {
		if key.Locale != locale || t == nil {
			continue
		}
		if len(t.Runs) > 0 && model.RunsText(t.Runs) != "" {
			return true
		}
	}
	return false
}

// localeTargetRuns returns the target runs for a locale (first matching
// variant), or nil when the block has no target for it.
func localeTargetRuns(b *model.Block, locale model.LocaleID) []model.Run {
	if b == nil {
		return nil
	}
	for key, t := range b.Targets {
		if key.Locale != locale || t == nil {
			continue
		}
		if len(t.Runs) > 0 {
			return t.Runs
		}
	}
	return nil
}

// memoryEntryID derives a deterministic, content-addressed entry ID from the
// language pair and the flattened source/target text. Two ingests of the same
// translation therefore hit the same row (Add upserts by ID) and never
// duplicate — the "add idempotently, content-hash keyed" contract.
func memoryEntryID(sourceLocale, targetLocale model.LocaleID, sourceRuns, targetRuns []model.Run) string {
	h := sha256.New()
	h.Write([]byte(sourceLocale))
	h.Write([]byte{0})
	h.Write([]byte(targetLocale))
	h.Write([]byte{0})
	h.Write([]byte(model.RunsText(sourceRuns)))
	h.Write([]byte{0})
	h.Write([]byte(model.RunsText(targetRuns)))
	return "tmseed-" + hex.EncodeToString(h.Sum(nil))[:32]
}

// cloneRunsForMemory makes an independent copy of a run slice so a stored content-memory entry
// never aliases a live block's runs.
func cloneRunsForMemory(runs []model.Run) []model.Run {
	if len(runs) == 0 {
		return nil
	}
	out := make([]model.Run, len(runs))
	copy(out, runs)
	return out
}

// resolveJobMemory returns the project content memory for a job's workspace via the deps'
// resolver, or nil when no resolver is configured or resolution fails (a content memory is
// optional; its absence degrades to AI-only, never an error).
func resolveJobMemory(deps *WorkerDeps, job *TranslationJob) memory.Store {
	if deps == nil || deps.MemoryResolver == nil {
		return nil
	}
	slug := job.WorkspaceSlug
	if slug == "" {
		slug = "_anon"
	}
	tm, err := deps.MemoryResolver.GetMemory(slug)
	if err != nil || tm == nil {
		return nil
	}
	return tm
}

// projectMemoryMinScore reads the recycle lookup threshold from the project's
// recipe config. Default is fuzzy at defaultMemoryMinScore (0.7) — the same
// threshold the CLI recycle flow uses — so exact and non-ambiguous near-exact
// matches (0.99 tag-mismatch demotions) pre-fill and lower fuzzy matches surface
// as alt-translation candidates. An ambiguous exact is never filled, and no fill
// is committed that would drop an inline code the source carries, so the default
// cannot silently publish a bad target. An explicit `tm_fuzzy_threshold`
// property (0-100) overrides in either direction: 85 maps to a 0.85 minimum
// score, and 100 restores strict exact-only leverage. An unparsable or
// non-positive value falls back to the default.
func projectMemoryMinScore(proj *store.Project) float64 {
	if proj == nil || proj.Properties == nil {
		return defaultMemoryMinScore
	}
	raw := proj.Properties["tm_fuzzy_threshold"]
	if raw == "" {
		return defaultMemoryMinScore
	}
	pct, err := strconv.Atoi(raw)
	if err != nil || pct <= 0 {
		return defaultMemoryMinScore
	}
	if pct > 100 {
		pct = 100
	}
	return float64(pct) / 100
}

// filterStoredByRemainder returns the subset of stored blocks whose IDs appear
// in the remainder set, preserving the original StoredBlock wrapper (with its
// store metadata) the AI translate loop expects.
func filterStoredByRemainder(stored []*store.StoredBlock, remainder []*model.Block) []*store.StoredBlock {
	keep := make(map[string]struct{}, len(remainder))
	for _, b := range remainder {
		if b != nil {
			keep[b.ID] = struct{}{}
		}
	}
	out := make([]*store.StoredBlock, 0, len(remainder))
	for _, sb := range stored {
		if sb == nil || sb.Block == nil {
			continue
		}
		if _, ok := keep[sb.Block.ID]; ok {
			out = append(out, sb)
		}
	}
	return out
}
