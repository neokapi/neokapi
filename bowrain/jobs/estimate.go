package jobs

import (
	"context"
	"unicode/utf8"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/memory"
)

// This file is the side-effect-free pre-flight estimator behind the convergence
// ESTIMATE endpoint (roadmap epic 019, theme B). It answers "what would a run
// cost" WITHOUT starting a run or calling any AI provider: it partitions the
// project's source blocks by the source-first gate (settle-then-translate), then
// for the READY source computes, per locale, how many units are pending, how
// many an aggressive content memory recycle would cover, and how many remain for paid AI —
// with a token estimate. It reuses the very same recycleBlocks the translation
// job uses, so the "content memory N · AI M" split it reports is the split the run produces,
// not a separate guess.

// EstimateLocaleWork is the estimated work for one target locale over the
// READY source: pending units (owed a draft), the content memory-recyclable subset, the
// remaining paid-AI subset, and a rough input-token estimate for that AI work.
type EstimateLocaleWork struct {
	Locale        string `json:"locale"`
	Pending       int    `json:"pending"`        // translatable ready-source blocks the locale still owes a draft
	ViaMemory     int    `json:"via_tm"`         // pending blocks a content memory recycle would fill (free)
	ViaAI         int    `json:"via_ai"`         // pending blocks left for paid AI
	TokenEstimate int    `json:"token_estimate"` // ~input tokens for the AI remainder (chars/4)
}

// SourceReadiness reports how the project's source blocks split against the
// active source gate: how many translatable source blocks are ready to
// translate vs. held below the gate (need settling / source review).
type SourceReadiness struct {
	Gate  model.SourceGateLevel `json:"gate"`  // resolved gate level
	Total int                   `json:"total"` // translatable source blocks considered
	Ready int                   `json:"ready"` // blocks at or above the gate
	Held  int                   `json:"held"`  // blocks below the gate (settle first)
}

// ConvergenceEstimate is the full pre-flight: source readiness first, then the
// per-locale translation work for the ready source, with totals. It is
// provider-free and starts no run.
type ConvergenceEstimate struct {
	Source SourceReadiness `json:"source"`
	// Locales is the per-locale work over the READY source. When the source is
	// fully held it is empty (nothing to estimate translating yet).
	Locales []EstimateLocaleWork `json:"locales"`
	// Totals rolls up Locales: pending, content memory, AI, and tokens across all locales.
	Totals EstimateTotals `json:"totals"`
	// Note documents the estimation basis for agents reading the JSON.
	Note string `json:"note"`
}

// EstimateTotals is the cross-locale rollup of the per-locale work.
type EstimateTotals struct {
	Pending       int `json:"pending"`
	ViaMemory     int `json:"via_tm"`
	ViaAI         int `json:"via_ai"`
	TokenEstimate int `json:"token_estimate"`
}

// estimateNote is the estimation-method disclosure carried in the estimate. It
// mirrors the CLI plan note so both surfaces describe the same basis.
const estimateNote = "Source readiness is evaluated against defaults.source_gate; only the ready source is estimated for translation. Content-memory leverage uses the project's recycle threshold (fuzzy at 0.7 by default; tm_fuzzy_threshold overrides); the token estimate is source chars / 4 for the AI remainder. No AI provider is called and no run is started."

// EstimateConvergence computes the side-effect-free pre-flight estimate for a
// project's convergence run. It loads the project's source blocks once, gates
// them by SourceStatus vs. the project's source gate, and for the ready source
// estimates per-locale pending/content memory/AI work. tm may be nil (no content-memory leverage: every
// pending unit reads as AI work). It never writes to the store and never calls
// an AI provider.
func EstimateConvergence(ctx context.Context, cs store.ContentStore, tm memory.Store, proj *store.Project) (ConvergenceEstimate, error) {
	est := ConvergenceEstimate{Note: estimateNote}
	if proj == nil {
		return est, nil
	}

	gate := store.SourceGateFor(proj)
	est.Source.Gate = gate

	blocks, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: proj.ID, Stream: "main"})
	if err != nil {
		return est, err
	}

	// The stream's recorded bases, read once for every locale. The pending
	// predicate is the run's own (decisionLedger.needsDraft), so a stale unit is
	// priced here exactly because the run will re-draft it.
	ledger := loadDecisionLedger(ctx, cs, proj.ID, "main")

	// Partition source blocks by the gate. A disabled gate (none) admits every
	// block, so Ready == Total and nothing is held — the estimate then covers the
	// whole corpus, matching the run's raw-MT behavior.
	var ready []*venue.StoredBlock
	for _, sb := range blocks {
		if sb == nil || sb.Block == nil || !sb.Block.Translatable {
			continue
		}
		est.Source.Total++
		if gate.Admits(sb.Block.SourceStatus) {
			est.Source.Ready++
			ready = append(ready, sb)
		} else {
			est.Source.Held++
		}
	}

	// Per-locale translation work over the READY source only: a held block never
	// translates regardless of scope, so it must not inflate the credit estimate
	// the user consents to. This is the source-readiness-first ordering (epic 019
	// acceptance #5): the ready portion is what carries a real credit cost.
	minScore := projectMemoryMinScore(proj)
	source := proj.DefaultSourceLanguage
	for _, loc := range proj.TargetLanguages {
		target := loc
		work := EstimateLocaleWork{Locale: string(loc)}

		// Pending = ready blocks the locale still owes a draft: no target, or one
		// whose recorded basis names source wording the block no longer holds.
		var pending []*venue.StoredBlock
		for _, sb := range ready {
			if !ledger.needsDraft(sb, target) {
				continue
			}
			pending = append(pending, sb)
		}
		work.Pending = len(pending)
		if work.Pending == 0 {
			continue // locale already fully covered over the ready source
		}

		// content-memory leverage: reuse the run's recycle partition so the split is truthful.
		// A nil content memory (or an error) means no leverage — everything reads as AI work.
		remainder := pending
		if tm != nil {
			if res, rerr := recycleBlocks(ctx, tm, pending, source, target, minScore, ledger); rerr == nil {
				work.ViaMemory = res.memoryCount
				remainder = filterStoredByRemainder(pending, res.remainder)
			}
		}
		work.ViaAI = len(remainder)
		for _, sb := range remainder {
			work.TokenEstimate += estimateSourceTokens(sb.Block.SourceText())
		}

		est.Locales = append(est.Locales, work)
		est.Totals.Pending += work.Pending
		est.Totals.ViaMemory += work.ViaMemory
		est.Totals.ViaAI += work.ViaAI
		est.Totals.TokenEstimate += work.TokenEstimate
	}
	return est, nil
}

// estimateSourceTokens approximates the input token count of a source text as
// chars/4 (rounded up) — the same source-only chars-per-token heuristic the CLI
// `up --plan` planner uses (host.EstimateTokens), so the server estimate and the
// CLI plan quote the same token basis. Kept local so the jobs package needs no
// host dependency.
func estimateSourceTokens(text string) int {
	chars := utf8.RuneCountInString(text)
	if chars == 0 {
		return 0
	}
	return (chars + 3) / 4
}
