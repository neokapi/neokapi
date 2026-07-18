package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strconv"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
)

// TMResolver returns the project's server translation memory for a workspace.
// It mirrors the server's workspaceStores.getTM: a per-workspace, PostgreSQL-
// backed TMStore keyed by the workspace slug. It is optional on WorkerDeps —
// when nil (self-hosted without a TM, or a bare worker) the convergence job
// degrades to the previous AI-only behavior. Errors are treated as "no TM
// available" by the caller, never fatal to a translation job.
type TMResolver interface {
	GetTM(workspaceSlug string) (sievepen.TMStore, error)
}

// TMResolverFunc adapts a plain function to the TMResolver interface.
type TMResolverFunc func(workspaceSlug string) (sievepen.TMStore, error)

// GetTM implements TMResolver.
func (f TMResolverFunc) GetTM(workspaceSlug string) (sievepen.TMStore, error) {
	return f(workspaceSlug)
}

// recycleResult reports how a recycle pass split the input blocks: which blocks
// were filled from the TM (and must skip the AI translate step) and which
// remain for AI. Counts are block counts, matching the convergence loop's
// block-count denominators.
type recycleResult struct {
	// filled holds the blocks whose target was set from a TM match; they are
	// persisted as-is and never sent to the AI translator.
	filled []*model.Block
	// remainder holds the blocks with no usable TM match; only these go to AI.
	remainder []*model.Block
	// tmCount is len(filled) — blocks attributed to TM in ViaTM reporting.
	tmCount int
}

// defaultTMMinScore is the recycle lookup threshold used when a project does
// not set one — the framework's canonical fuzzy threshold, matching the CLI
// recycle flow's defaults (sievepen.TMLeverageConfig defaults MinScore to 0.7;
// core/tools.TMLeverageConfig defaults FuzzyThreshold to 70). Fill safety is
// unchanged: sievepen's tool sets a target only for exact-tier match types, so
// a fuzzy match below near-exact surfaces as an alt-translation candidate, not
// a silent fill.
const defaultTMMinScore = 0.7

// recycleBlocks runs the framework's content-aware TM leverage tool over the
// stored blocks and partitions them into TM-filled vs. remainder. It mirrors
// the built-in `translate` flow's recycle→translate ordering: exact (and, at
// the configured fuzzy threshold, near-exact) matches are pre-filled from the
// project TM so only genuinely-new segments are sent to the paid AI step.
//
// The tool sets a target only for exact-tier matches (see sievepen.TMLeverageTool);
// a block is counted as TM-filled when it carries a fresh target for the locale
// after the pass. minScore comes from the project's recipe TM threshold when
// present (default fuzzy at defaultTMMinScore, matching the CLI recycle flow).
func recycleBlocks(ctx context.Context, tm sievepen.TMStore, storedBlocks []*store.StoredBlock, sourceLocale, targetLocale model.LocaleID, minScore float64) (recycleResult, error) {
	if minScore <= 0 {
		minScore = defaultTMMinScore
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

	tmTool := sievepen.NewTMLeverageTool(tm, sievepen.TMLeverageConfig{
		MinScore:     minScore,
		MaxResults:   5,
		SourceLocale: sourceLocale,
		TargetLocale: targetLocale,
	})

	parts := storedBlocksToParts(candidates)
	outParts, err := runToolOnParts(ctx, tmTool, parts)
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
	res.tmCount = len(res.filled)
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

// seedTMFromBlocks adds each block's existing target translation for the given
// locale to the project TM, so a future convergence recycles it instead of
// paying AI. It is idempotent: the entry ID is derived from a content hash of
// the source/target/locale pair, so re-ingesting the same translation upserts
// the same row rather than duplicating it (Add is an ID-keyed upsert on every
// backend). Blocks without a usable target for the locale are skipped.
//
// origin labels where the seed came from ("push" for pushed targets on ingest,
// "ai-draft" for a freshly-drafted AI target); ref is an optional provenance
// reference (job/push id). Returns the number of entries added.
func seedTMFromBlocks(ctx context.Context, tm sievepen.TMStore, blocks []*model.Block, projectID string, sourceLocale, targetLocale model.LocaleID, origin, ref string) int {
	if tm == nil {
		return 0
	}
	added := 0
	now := time.Now()
	for _, b := range blocks {
		if b == nil || !b.Translatable {
			continue
		}
		sourceRuns := b.Source
		if len(sourceRuns) == 0 || model.RunsText(sourceRuns) == "" {
			continue
		}
		targetRuns := localeTargetRuns(b, targetLocale)
		if len(targetRuns) == 0 || model.RunsText(targetRuns) == "" {
			continue
		}
		entry := sievepen.TMEntry{
			ID: tmEntryID(sourceLocale, targetLocale, sourceRuns, targetRuns),
			Variants: map[model.LocaleID][]model.Run{
				sourceLocale: cloneRunsForTM(sourceRuns),
				targetLocale: cloneRunsForTM(targetRuns),
			},
			HintSrcLang: sourceLocale,
			ProjectID:   projectID,
			Origins: []sievepen.Origin{{
				Source:    origin,
				Key:       b.Name,
				Reference: ref,
				AddedAt:   now,
				AddedBy:   "convergence",
			}},
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := tm.Add(ctx, entry); err != nil {
			slog.WarnContext(ctx, "seed TM entry failed", "block", b.ID, "error", err)
			continue
		}
		added++
	}
	return added
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

// tmEntryID derives a deterministic, content-addressed entry ID from the
// language pair and the flattened source/target text. Two ingests of the same
// translation therefore hit the same row (Add upserts by ID) and never
// duplicate — the "add idempotently, content-hash keyed" contract.
func tmEntryID(sourceLocale, targetLocale model.LocaleID, sourceRuns, targetRuns []model.Run) string {
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

// cloneRunsForTM makes an independent copy of a run slice so a stored TM entry
// never aliases a live block's runs.
func cloneRunsForTM(runs []model.Run) []model.Run {
	if len(runs) == 0 {
		return nil
	}
	out := make([]model.Run, len(runs))
	copy(out, runs)
	return out
}

// resolveJobTM returns the project TM for a job's workspace via the deps'
// resolver, or nil when no resolver is configured or resolution fails (a TM is
// optional; its absence degrades to AI-only, never an error).
func resolveJobTM(deps *WorkerDeps, job *TranslationJob) sievepen.TMStore {
	if deps == nil || deps.TMResolver == nil {
		return nil
	}
	slug := job.WorkspaceSlug
	if slug == "" {
		slug = "_anon"
	}
	tm, err := deps.TMResolver.GetTM(slug)
	if err != nil || tm == nil {
		return nil
	}
	return tm
}

// projectTMMinScore reads the recycle lookup threshold from the project's
// recipe config. Default is fuzzy at defaultTMMinScore (0.7) — the same
// threshold the CLI recycle flow uses — so near-exact matches (tag-mismatch and
// ambiguous-exact demotions at 0.99) pre-fill and fuzzy matches surface as
// alt-translation candidates. Only exact-tier match types ever fill a target
// (sievepen.TMLeverageTool), so the default cannot silently fill from a low
// fuzzy match. An explicit `tm_fuzzy_threshold` property (0-100) overrides in
// either direction: 85 maps to a 0.85 minimum score, and 100 restores strict
// exact-only leverage. An unparsable or non-positive value falls back to the
// default.
func projectTMMinScore(proj *store.Project) float64 {
	if proj == nil || proj.Properties == nil {
		return defaultTMMinScore
	}
	raw := proj.Properties["tm_fuzzy_threshold"]
	if raw == "" {
		return defaultTMMinScore
	}
	pct, err := strconv.Atoi(raw)
	if err != nil || pct <= 0 {
		return defaultTMMinScore
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
