package server

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/venue"
)

// applyShipStates enriches dashboard stats in place with failing-check counts,
// stale/basis-unknown decision counts, the derived per-locale ship state
// (store.DeriveShipState), and the on-brand rate, on both the project-wide
// locale stats and each collection rollup. It is the ONE place the server
// decides what a locale can ship as: the dashboard, the public ship manifest
// and the workspace loop rollup all read the ShipState it stamps.
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
// minimum bar (VoiceProfile.ComplianceBar). A term-non-compliant block is treated
// exactly like a failing check: it counts against the on-brand rate AND, at full
// coverage, against FailingChecks, so it can never be governed or ai_shippable.
// Scopes with no voice scores fall back to checks(+terms), and OnBrandBasis says
// which evidence produced the number so consumers can present it honestly. Voice
// scores are read best-effort: a brand store hiccup degrades the rate rather than
// failing the dashboard. The gate is resolved once per (workspace, locale) by the
// caller and reused across every block; a nil gate (no terms, no brand store)
// makes the term half a no-op, keeping the numbers byte-identical to before.
//
// Staleness is graded by the ledger, not by this pass: TallyDecisionBasis joins
// each decision's recorded basis to the block's current source hash. It runs
// unbounded by the coverage gate because a stale unit withholds a scope at any
// coverage, and it is one grouped query rather than a per-block read.
func applyShipStates(ctx context.Context, cs store.ContentStore, brandStore coreprofile.Store, projectID, stream string, gate *termGate, stats *store.TranslationDashboardStats) error {
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
	// per collection): true where the gate has a terms store or brand-vocab rule to
	// enforce for the locale, so the on_brand_basis can honestly note term checks
	// contributed. Applied to both the project-locale and every collection-locale.
	termActive := map[string]bool{}

	// The item → collection map both passes attribute through. Built from the
	// full item list (the dashboard pages its response only after this pass).
	collByItem := make(map[string]string, len(stats.ItemStats))
	for _, it := range stats.ItemStats {
		collByItem[it.ItemName] = it.CollectionID
	}

	basis, err := tallyDecisionBasis(ctx, cs, projectID, stream, collByItem)
	if err != nil {
		return err
	}

	if len(rateCandidates) > 0 {
		scores := latestVoiceScores(ctx, brandStore, projectID, stream)

		// Per-locale governance and voice scores resolve once, outside the walk:
		// they are indexed by locale, not by block, and re-resolving them per
		// batch would repeat the same work for every page of the corpus.
		type localeCtx struct {
			loc    model.LocaleID
			scored map[string]scoredBlock
		}
		locales := make(map[string]localeCtx, len(rateCandidates))
		for localeStr := range rateCandidates {
			loc := model.LocaleID(localeStr)
			locales[localeStr] = localeCtx{loc: loc, scored: scores[string(locale.Normalize(loc))]}
			// Drives both the term-compliance predicate below and the basis note.
			termActive[localeStr] = gate.active(ctx, loc)
		}

		// The corpus is walked a batch at a time and the locale loop moved
		// inside it. Everything accumulated here is a counter, so a batch can be
		// folded and dropped — which is the point: reading a whole project's
		// blocks to compute these tallies is what OOM-killed the server, and the
		// dashboard asks for them on every load.
		walkErr := store.EachBlockBatch(ctx, cs,
			store.BlockQuery{ProjectID: projectID, Stream: stream},
			store.DefaultBlockBatch,
			func(batch []*venue.StoredBlock) error {
				for _, sb := range batch {
					for localeStr, lc := range locales {
						loc := lc.loc
						scored := lc.scored
						if sb.Block == nil || !sb.Block.Translatable || sb.Block.Target(loc) == nil {
							continue
						}
						// A block fails the ship gate when its QA checks flag an
						// error-severity finding OR its target is not term-compliant for
						// the locale — the two are treated identically for both
						// FailingChecks and the on-brand rate. gate.compliant is offline
						// (in-memory snapshot).
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
				return nil
			})
		if walkErr != nil {
			return fmt.Errorf("load blocks for ship-state checks: %w", walkErr)
		}
	}

	for i := range stats.LocaleStats {
		ls := &stats.LocaleStats[i]
		ls.FailingChecks = failing[ls.Locale]
		ls.StaleBlocks = basis.forLocale(ls.Locale).Stale
		ls.BasisUnknownBlocks = basis.forLocale(ls.Locale).BasisUnknown
		ls.ShipState = store.DeriveShipState(ls.TranslatedBlocks, ls.TotalBlocks, ls.ApprovedBlocks, ls.FailingChecks, ls.StaleBlocks)
		applyOnBrand(ls, onBrand[ls.Locale], voiceUsed[ls.Locale], termActive[ls.Locale])
	}
	for i := range stats.CollectionStats {
		coll := &stats.CollectionStats[i]
		for j := range coll.Locales {
			ls := &coll.Locales[j]
			ls.FailingChecks = failingByColl[coll.CollectionID][ls.Locale]
			ls.StaleBlocks = basis.forCollection(coll.CollectionID, ls.Locale).Stale
			ls.BasisUnknownBlocks = basis.forCollection(coll.CollectionID, ls.Locale).BasisUnknown
			ls.ShipState = store.DeriveShipState(ls.TranslatedBlocks, ls.TotalBlocks, ls.ApprovedBlocks, ls.FailingChecks, ls.StaleBlocks)
			applyOnBrand(ls, onBrandByColl[coll.CollectionID][ls.Locale], voiceUsedByColl[coll.CollectionID][ls.Locale], termActive[ls.Locale])
		}
	}
	return nil
}

// basisCounts is one scope's stale / unknown-basis decision counts.
type basisCounts struct {
	Stale        int
	BasisUnknown int
}

// basisRollup carries the graded decision basis at both aggregation levels the
// dashboard reports: project-wide per locale, and per (collection, locale).
// Locale keys are normalized on both sides (locale.Normalize), so a ledger
// variant and a project's declared target language cannot miss each other over
// a difference in spelling.
type basisRollup struct {
	project map[string]basisCounts
	byColl  map[string]map[string]basisCounts
}

// forLocale returns the project-wide counts for one locale as the dashboard
// spells it.
func (r basisRollup) forLocale(loc string) basisCounts {
	return r.project[basisLocaleKey(loc)]
}

// forCollection returns one collection's counts for one locale.
func (r basisRollup) forCollection(collectionID, loc string) basisCounts {
	return r.byColl[collectionID][basisLocaleKey(loc)]
}

func basisLocaleKey(loc string) string {
	return string(locale.Normalize(model.LocaleID(loc)))
}

// tallyDecisionBasis grades the stream's decisions against the source the
// project holds now and folds the per-(item, variant) counts the ledger returns
// into the two scopes the dashboard reports. The ledger keys on variants (locale
// plus optional tone/channel); the dashboard reports locales, so every variant
// of a locale folds into that locale's counts.
//
// A content store with no decision ledger grades nothing, which reads as no
// stale units — the same answer a project that has never recorded a decision
// gives, and the honest one: a store that keeps no decisions holds no basis to
// contradict.
func tallyDecisionBasis(ctx context.Context, cs store.ContentStore, projectID, stream string, collByItem map[string]string) (basisRollup, error) {
	out := basisRollup{project: map[string]basisCounts{}, byColl: map[string]map[string]basisCounts{}}
	ds, ok := cs.(store.DecisionStore)
	if !ok {
		return out, nil
	}
	tallies, err := ds.TallyDecisionBasis(ctx, projectID, stream)
	if err != nil {
		return out, fmt.Errorf("grade decision basis: %w", err)
	}
	for _, t := range tallies {
		if t.Stale == 0 && t.BasisUnknown == 0 {
			continue
		}
		var variant model.VariantKey
		if err := variant.UnmarshalText([]byte(t.Variant)); err != nil || variant.Locale == "" {
			continue // a variant that names no locale belongs to no locale scope
		}
		loc := basisLocaleKey(string(variant.Locale))

		p := out.project[loc]
		p.Stale += t.Stale
		p.BasisUnknown += t.BasisUnknown
		out.project[loc] = p

		cid := collByItem[t.ItemName]
		if out.byColl[cid] == nil {
			out.byColl[cid] = map[string]basisCounts{}
		}
		c := out.byColl[cid][loc]
		c.Stale += t.Stale
		c.BasisUnknown += t.BasisUnknown
		out.byColl[cid][loc] = c
	}
	return out, nil
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
	return blockApproveBlocker(ctx, block, loc, scored, gate) == approveBlockerNone
}

// approveBlocker names the first bar a pending target misses. It exists so a
// caller can report WHY a block was left for a person in the same vocabulary
// the review queue's entries carry, rather than in one undifferentiated
// "skipped" count.
type approveBlocker string

const (
	approveBlockerNone   approveBlocker = ""
	approveBlockerChecks approveBlocker = "checks"
	approveBlockerTerms  approveBlocker = "terms"
	approveBlockerVoice  approveBlocker = "voice"
)

// blockApproveBlocker is the one predicate behind blockOnBrandAndPassing: it
// applies the three bars in gate order and names the first one the target
// misses, or approveBlockerNone when it clears them all. Order is significant
// only for attribution — a target missing two bars is reported against the
// first — so the reported reasons sum to the skipped count.
func blockApproveBlocker(ctx context.Context, block *model.Block, loc model.LocaleID, scored map[string]scoredBlock, gate *termGate) approveBlocker {
	if blockFailsChecks(ctx, block, loc) {
		return approveBlockerChecks
	}
	if !gate.compliant(ctx, block, loc) {
		return approveBlockerTerms
	}
	if vs, ok := scored[block.ID]; ok && vs.score < vs.bar {
		return approveBlockerVoice
	}
	return approveBlockerNone
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
func latestVoiceScores(ctx context.Context, brandStore coreprofile.Store, projectID, stream string) map[string]map[string]scoredBlock {
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
		b := profile.ComplianceBar()
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
