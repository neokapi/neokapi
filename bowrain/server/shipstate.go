package server

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/neokapi/neokapi/bowrain/core/store"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
)

// applyShipStates enriches dashboard stats in place with failing-check counts,
// the derived per-locale ship state (store.DeriveShipState), and the on-brand
// rate, on both the project-wide locale stats and each collection rollup.
//
// The QA pass is bounded the same way the convergence derive path bounds it
// (deriveFunc): the expensive full-block read happens only when some locale has
// at least one translated block, and each translated block+locale pair is
// checked exactly once. Ship-state semantics are unchanged: FailingChecks is
// attributed only to locales at full coverage in at least one scope (checks
// cannot promote an under-covered locale). The on-brand rate, by contrast, is
// meaningful below full coverage — it rates the blocks that ARE translated — so
// it is derived for every locale with translated blocks.
//
// On-brand definition: a translated block is on-brand when the project's QA
// checks report no error-severity finding, AND its target is term-compliant for
// the locale (deterministic, offline — it uses no forbidden/competitor term and
// omits no mandated preferred/approved rendering; see termGate/blockTermCompliant),
// AND — where a persisted brand voice score exists for the block+locale (written
// by the worker's draft scoring, zero AI) — the score meets the scoring profile's
// minimum bar (VoiceProfile.OnBrandBar). A term-non-compliant block is treated
// exactly like a failing check: it counts against the on-brand rate AND, at full
// coverage, against FailingChecks, so it can never be governed or ai_shippable.
// Scopes with no voice scores fall back to checks(+terms), and OnBrandBasis says
// which evidence produced the number so consumers can present it honestly. Voice
// scores are read best-effort: a brand store hiccup degrades the rate rather than
// failing the dashboard. The gate is resolved once per (workspace, locale) by the
// caller and reused across every block; a nil gate (no termbase, no brand store)
// makes the term half a no-op, keeping the numbers byte-identical to before.
func applyShipStates(ctx context.Context, cs store.ContentStore, brandStore corebrand.BrandStore, projectID, stream string, gate *termGate, stats *store.TranslationDashboardStats) error {
	fullyCovered := func(ls store.LocaleTranslationStats) bool {
		return ls.TotalBlocks > 0 && ls.TranslatedBlocks >= ls.TotalBlocks
	}
	// shipCandidates: locales whose FailingChecks/ShipState need the check pass
	// (full coverage in some scope). rateCandidates: locales with anything
	// translated anywhere — the on-brand denominator. Ship candidates are a
	// subset of rate candidates (full coverage implies translated blocks).
	shipCandidates := map[string]bool{}
	rateCandidates := map[string]bool{}
	noteLocale := func(ls store.LocaleTranslationStats) {
		if fullyCovered(ls) {
			shipCandidates[ls.Locale] = true
		}
		if ls.TranslatedBlocks > 0 {
			rateCandidates[ls.Locale] = true
		}
	}
	for _, ls := range stats.LocaleStats {
		noteLocale(ls)
	}
	for _, coll := range stats.CollectionStats {
		for _, ls := range coll.Locales {
			noteLocale(ls)
		}
	}

	// Ship-gate failing block counts among the locale's translated blocks
	// (ship-candidate locales only) — a QA error-severity finding OR a term
	// violation — and on-brand block counts (all rate candidates), project-wide
	// and attributed per collection.
	failing := map[string]int{}
	failingByColl := map[string]map[string]int{}
	onBrand := map[string]int{}
	onBrandByColl := map[string]map[string]int{}
	voiceUsed := map[string]bool{}
	voiceUsedByColl := map[string]map[string]bool{}
	// termActive is per-locale (term governance is workspace/project-wide, not
	// per collection): true where the gate has a termbase or brand-vocab rule to
	// enforce for the locale, so the on_brand_basis can honestly note term checks
	// contributed. Applied to both the project-locale and every collection-locale.
	termActive := map[string]bool{}

	if len(rateCandidates) > 0 {
		blocks, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: stream})
		if err != nil {
			return fmt.Errorf("load blocks for ship-state checks: %w", err)
		}
		scores := latestVoiceScores(ctx, brandStore, projectID, stream)
		collByItem := make(map[string]string, len(stats.ItemStats))
		for _, it := range stats.ItemStats {
			collByItem[it.ItemName] = it.CollectionID
		}
		for localeStr := range rateCandidates {
			loc := model.LocaleID(localeStr)
			scored := scores[string(locale.Normalize(loc))]
			// Resolve the gate's per-locale governance once (never per block): it
			// drives both the term-compliance predicate below and the basis note.
			termActive[localeStr] = gate.active(ctx, loc)
			for _, sb := range blocks {
				if sb.Block == nil || !sb.Block.Translatable || sb.Block.Target(loc) == nil {
					continue
				}
				// A block fails the ship gate when its QA checks flag an
				// error-severity finding OR its target is not term-compliant for the
				// locale — the two are treated identically for both FailingChecks and
				// the on-brand rate. gate.compliant is offline (in-memory snapshot).
				blockFails := blockFailsChecks(ctx, sb.Block, loc) || !gate.compliant(ctx, sb.Block, loc)
				cid := collByItem[sb.ItemName]
				if blockFails && shipCandidates[localeStr] {
					failing[localeStr]++
					if failingByColl[cid] == nil {
						failingByColl[cid] = map[string]int{}
					}
					failingByColl[cid][localeStr]++
				}

				blockOnBrand := !blockFails
				if vs, ok := scored[sb.Block.ID]; ok {
					voiceUsed[localeStr] = true
					if voiceUsedByColl[cid] == nil {
						voiceUsedByColl[cid] = map[string]bool{}
					}
					voiceUsedByColl[cid][localeStr] = true
					if vs.score < vs.bar {
						blockOnBrand = false
					}
				}
				if blockOnBrand {
					onBrand[localeStr]++
					if onBrandByColl[cid] == nil {
						onBrandByColl[cid] = map[string]int{}
					}
					onBrandByColl[cid][localeStr]++
				}
			}
		}
	}

	for i := range stats.LocaleStats {
		ls := &stats.LocaleStats[i]
		ls.FailingChecks = failing[ls.Locale]
		ls.ShipState = store.DeriveShipState(ls.TranslatedBlocks, ls.TotalBlocks, ls.ApprovedBlocks, ls.FailingChecks)
		applyOnBrand(ls, onBrand[ls.Locale], voiceUsed[ls.Locale], termActive[ls.Locale])
	}
	for i := range stats.CollectionStats {
		coll := &stats.CollectionStats[i]
		for j := range coll.Locales {
			ls := &coll.Locales[j]
			ls.FailingChecks = failingByColl[coll.CollectionID][ls.Locale]
			ls.ShipState = store.DeriveShipState(ls.TranslatedBlocks, ls.TotalBlocks, ls.ApprovedBlocks, ls.FailingChecks)
			applyOnBrand(ls, onBrandByColl[coll.CollectionID][ls.Locale], voiceUsedByColl[coll.CollectionID][ls.Locale], termActive[ls.Locale])
		}
	}
	return nil
}

// applyOnBrand stamps the derived on-brand fields onto one locale scope. A
// scope with nothing translated gets no rate (nothing to rate — the additive
// fields stay omitted). The count is clamped to the translated denominator so
// a stats/block-read skew can never report a rate above 1. The basis names the
// evidence that informed the rate: QA checks always, plus terms when term
// governance was active for the locale, plus voice when a persisted score
// informed at least one block.
func applyOnBrand(ls *store.LocaleTranslationStats, onBrandCount int, voice, terms bool) {
	if ls.TranslatedBlocks <= 0 {
		return
	}
	ls.OnBrandBlocks = min(onBrandCount, ls.TranslatedBlocks)
	rate := float64(ls.OnBrandBlocks) / float64(ls.TranslatedBlocks)
	ls.OnBrandRate = &rate
	ls.OnBrandBasis = store.OnBrandBasisFor(voice, terms)
}

// blockFailsChecks reports whether a block's target for a locale carries an
// error-severity QA finding — the "fails the ship gate" predicate shared by the
// dashboard ship-state pass (applyShipStates), the convergence derive
// (countFailingBlocks), and the bulk approve-passing endpoint.
func blockFailsChecks(ctx context.Context, block *model.Block, loc model.LocaleID) bool {
	for _, issue := range runQAOnBlock(ctx, block, loc) {
		if issue.Severity == "error" {
			return true
		}
	}
	return false
}

// blockOnBrandAndPassing reports whether a translated block+locale is clean
// enough to ship without a person's review: it passes the QA checks with no
// error-severity finding, is term-compliant for the locale (via the shared
// gate), AND — where a persisted brand voice score exists for the block — the
// score meets the scoring profile's on-brand bar. This is exactly the per-block
// on-brand predicate applyShipStates aggregates into the on-brand rate (#1365);
// the bulk approve-passing endpoint reuses it to pick which pending drafts to
// auto-approve, so a target using a forbidden term or missing a mandated one is
// excluded and left pending for a person. `scored` is one locale's score map
// (latestVoiceScores(...)[normalize(locale)]); an empty map degrades to
// checks-only. A nil gate makes the term check a no-op, matching the dashboard.
func blockOnBrandAndPassing(ctx context.Context, block *model.Block, loc model.LocaleID, scored map[string]scoredBlock, gate *termGate) bool {
	if blockFailsChecks(ctx, block, loc) {
		return false
	}
	if !gate.compliant(ctx, block, loc) {
		return false
	}
	if vs, ok := scored[block.ID]; ok && vs.score < vs.bar {
		return false
	}
	return true
}

// scoredBlock is the latest persisted voice score for one block+locale, paired
// with the minimum bar of the profile that produced it.
type scoredBlock struct {
	score int
	bar   int
}

// latestVoiceScores reads the project's persisted brand voice scores and keeps
// the newest per (locale, block), each paired with its scoring profile's
// on-brand bar. Locale keys are normalized (scores are stored normalized).
// Best-effort by design: a nil brand store or a read failure yields an empty
// map, degrading the on-brand rate to checks-only rather than failing the
// dashboard.
func latestVoiceScores(ctx context.Context, brandStore corebrand.BrandStore, projectID, stream string) map[string]map[string]scoredBlock {
	out := map[string]map[string]scoredBlock{}
	if brandStore == nil {
		return out
	}
	scores, err := brandStore.GetScoresByStream(ctx, projectID, stream)
	if err != nil {
		slog.WarnContext(ctx, "voice scores unavailable; on-brand rate falls back to checks-only",
			"project_id", projectID, "error", err)
		return out
	}
	bars := map[string]int{}
	barFor := func(profileID string) int {
		if b, ok := bars[profileID]; ok {
			return b
		}
		profile, err := brandStore.GetProfile(ctx, profileID)
		if err != nil {
			profile = nil // deleted profile: apply the default bar to its scores
		}
		b := profile.OnBrandBar()
		bars[profileID] = b
		return b
	}
	// GetScoresByStream orders by checked_at DESC, so the first row seen per
	// (locale, block) is the latest measurement.
	for _, sc := range scores {
		if sc == nil || sc.BlockID == "" {
			continue
		}
		key := string(locale.Normalize(sc.Locale))
		if out[key] == nil {
			out[key] = map[string]scoredBlock{}
		}
		if _, seen := out[key][sc.BlockID]; seen {
			continue
		}
		out[key][sc.BlockID] = scoredBlock{score: sc.Score, bar: barFor(sc.ProfileID)}
	}
	return out
}
