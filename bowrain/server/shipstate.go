package server

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/venue"
)

// applyShipStates enriches dashboard stats in place with failing-check counts,
// stale/basis-unknown decision counts, the derived per-locale ship state
// (store.DeriveShipState), and the compliance rate, on both the project-wide
// locale stats and each collection rollup. It is the ONE place the server
// decides what a locale can ship as: the dashboard, the public ship manifest
// and the workspace loop rollup all read the ShipState it stamps.
//
// The per-block judgement is stored, not repeated. A dashboard load asks the
// content store to COUNT the ship-gate verdicts it holds (store.ShipVerdictStore)
// and hands back only the pairs whose verdict no longer matches the content or
// the governance — so a load over an unchanged project reads no blocks at all,
// and a load after an edit reads what was edited. A store that keeps no verdicts
// is asked to judge every translated pair, which is the derivation this pass has
// always run. Ship-state semantics are unchanged: FailingChecks is
// attributed only to locales at full coverage in at least one scope (checks
// cannot promote an under-covered locale). The compliance rate, by contrast, is
// meaningful below full coverage — it rates the blocks that ARE translated — so
// it is derived for every locale with translated blocks.
//
// On-brand definition: a translated block is compliant when the project's
// rule-based checks report no error-severity finding, AND its target is
// term-compliant for the locale (deterministic, offline — it uses no
// forbidden/competitor term and omits no mandated preferred/approved rendering;
// see termGate/blockTermCompliant), AND — where a persisted voice score exists
// for the block+locale (written by the worker's draft scoring, zero AI) — the
// score meets the scoring profile's minimum bar (VoiceProfile.ComplianceBar).
// A term-non-compliant block is treated exactly like a failing check: it counts
// against the compliance rate AND, at full coverage, against FailingChecks, so
// it can never be governed or ai_shippable.
// Scopes with no voice scores fall back to checks(+terms), and ComplianceBasis says
// which evidence produced the number so consumers can present it honestly. Voice
// scores are read best-effort: a voice store hiccup degrades the rate rather than
// failing the dashboard. The gate is resolved once per (workspace, locale) by the
// caller and reused across every block; a nil gate (no terms, no voice store)
// makes the term half a no-op, keeping the numbers byte-identical to before.
//
// Staleness is graded by the ledger, not by this pass: TallyDecisionBasis joins
// each decision's recorded basis to the block's current source hash. It runs
// unbounded by the coverage gate because a stale unit withholds a scope at any
// coverage, and it is one grouped query rather than a per-block read.
func applyShipStates(ctx context.Context, cs store.ContentStore, voiceStore coreprofile.Store, projectID, stream string, gate *termGate, stats *store.TranslationDashboardStats) error {
	fullyCovered := func(ls store.LocaleTranslationStats) bool {
		return ls.TotalBlocks > 0 && ls.TranslatedBlocks >= ls.TotalBlocks
	}
	// shipCandidates: locales whose FailingChecks/ShipState need the check pass
	// (full coverage in some scope). rateCandidates: locales with anything
	// translated anywhere — the compliant denominator. Ship candidates are a
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

	// termActive is per-locale (term governance is workspace/project-wide, not
	// per collection): true where the gate has a terms store or brand-vocab rule to
	// enforce for the locale, so the compliance_basis can honestly note term checks
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

	// The per-(collection, locale) tallies, counted by the store where it holds
	// verdicts and folded in here for the pairs it could not.
	var rollup store.ShipGateRollup
	if len(rateCandidates) > 0 {
		locales := slices.Sorted(maps.Keys(rateCandidates))
		for _, localeStr := range locales {
			// Drives both the term-compliance predicate and the basis note.
			termActive[localeStr] = gate.active(ctx, model.LocaleID(localeStr))
		}
		rollup, err = deriveShipGate(ctx, cs, voiceStore, projectID, stream, gate, locales, collByItem)
		if err != nil {
			return err
		}
	}

	// A scope's compliant count is its clean blocks less the ones a voice score
	// withholds; its failing count is reported only for a locale some scope
	// covers fully. Both read the same tallies, project-wide (summed over the
	// collections every block belongs to exactly one of) and per collection.
	project := map[string]store.ShipGateCounts{}
	for _, byLocale := range rollup.Scopes {
		for loc, c := range byLocale {
			p := project[loc]
			p.Failing += c.Failing
			p.Clean += c.Clean
			p.Scored += c.Scored
			p.CleanBelowBar += c.CleanBelowBar
			project[loc] = p
		}
	}

	stamp := func(ls *store.LocaleTranslationStats, c store.ShipGateCounts, b basisCounts) {
		if shipCandidates[ls.Locale] {
			ls.FailingChecks = c.Failing
		}
		ls.StaleBlocks = b.Stale
		ls.BasisUnknownBlocks = b.BasisUnknown
		ls.ShipState = store.DeriveShipState(ls.TranslatedBlocks, ls.TotalBlocks, ls.ApprovedBlocks, ls.FailingChecks, ls.StaleBlocks)
		applyCompliance(ls, c.Clean-c.CleanBelowBar, c.Scored > 0, termActive[ls.Locale])
	}

	for i := range stats.LocaleStats {
		ls := &stats.LocaleStats[i]
		stamp(ls, project[ls.Locale], basis.forLocale(ls.Locale))
	}
	for i := range stats.CollectionStats {
		coll := &stats.CollectionStats[i]
		for j := range coll.Locales {
			ls := &coll.Locales[j]
			stamp(ls, rollup.CountsFor(coll.CollectionID, ls.Locale), basis.forCollection(coll.CollectionID, ls.Locale))
		}
	}
	return nil
}

// deriveShipGate produces the per-(collection, locale) ship-gate tallies the
// dashboard reports, judging as few blocks as it can.
//
// Where the content store keeps verdicts, the counting happens in the database
// and only the pairs whose stored verdict no longer matches the content or the
// governance come back to be judged — none, over a project nothing has touched.
// Where it does not, every translated pair is judged, walking the corpus a batch
// at a time so peak memory is the batch rather than the project.
//
// Voice scores are applied over the scored set rather than folded into the
// stored verdict, and deliberately: the worker rewrites scores on every
// convergence pass, and a verdict that named its score in its basis would be
// retired by the loop as fast as it was recorded — which is to say the
// whole-project read would be back.
func deriveShipGate(
	ctx context.Context,
	cs store.ContentStore,
	voiceStore coreprofile.Store,
	projectID, stream string,
	gate *termGate,
	locales []string,
	collByItem map[string]string,
) (store.ShipGateRollup, error) {
	scores := latestVoiceScores(ctx, voiceStore, projectID, stream)

	// Per-locale governance and voice scores resolve once, outside any walk:
	// they are indexed by locale, not by block, and re-resolving them per batch
	// would repeat the same work for every page of the corpus.
	scored := make(map[string]map[string]scoredBlock, len(locales))
	for _, l := range locales {
		scored[l] = scores[string(locale.Normalize(model.LocaleID(l)))]
	}

	fingerprint := gate.fingerprint(ctx, locales)
	vs, keepsVerdicts := store.ShipVerdicts(cs)

	var rollup store.ShipGateRollup
	var stale []store.ShipGateStale
	if keepsVerdicts {
		q := store.ShipGateQuery{
			ProjectID: projectID, Stream: stream,
			Gate: fingerprint, Locales: locales,
			Scores: shipGateScores(scored),
		}
		var err error
		rollup, err = vs.ShipGateRollup(ctx, q)
		if err != nil {
			return rollup, fmt.Errorf("roll up ship-gate verdicts: %w", err)
		}
		stale = rollup.Stale
		if len(stale) == 0 {
			return rollup, nil
		}
	}

	// One pair's judgement, folded into the tallies and queued for storage.
	verdicts := make([]store.ShipGateVerdict, 0, min(len(stale), store.DefaultBlockBatch*4))
	judge := func(block *model.Block, itemName, localeStr, basis string) {
		loc := model.LocaleID(localeStr)
		// A block fails the ship gate when its rule-based checks flag an
		// error-severity finding OR its target is not term-compliant for the
		// locale — the two are treated identically for both FailingChecks and
		// the compliance rate. gate.compliant is offline (in-memory snapshot).
		fails := blockFailsChecks(ctx, block, loc) || !gate.compliant(ctx, block, loc)
		sb, isScored := scored[localeStr][block.ID]
		rollup.Add(collByItem[itemName], localeStr, fails, isScored, isScored && sb.score < sb.bar)
		if keepsVerdicts {
			verdicts = append(verdicts, store.ShipGateVerdict{
				BlockID: block.ID, Locale: localeStr,
				Basis: basis,
				Fails: fails,
			})
		}
	}

	if !keepsVerdicts {
		// Everything the project holds, a batch at a time. Everything
		// accumulated is a counter, so a batch can be folded and dropped —
		// reading a whole project's blocks into memory is what OOM-killed the
		// server, and this pass is the one that used to do it on every load.
		walkErr := store.EachBlockBatch(ctx, cs,
			store.BlockQuery{ProjectID: projectID, Stream: stream},
			store.DefaultBlockBatch,
			func(batch []*venue.StoredBlock) error {
				for _, b := range batch {
					if b.Block == nil || !b.Block.Translatable {
						continue
					}
					for _, localeStr := range locales {
						if b.Block.Target(model.LocaleID(localeStr)) == nil {
							continue
						}
						judge(b.Block, b.ItemName, localeStr, "")
					}
				}
				return nil
			})
		if walkErr != nil {
			return rollup, fmt.Errorf("load blocks for ship-state checks: %w", walkErr)
		}
		return rollup, nil
	}

	// Only the pairs the store could not answer for. They arrive grouped by
	// block, so a block whose every locale went stale is hydrated once, and each
	// batch is recorded before the next is read: a verdict stands on its own, so
	// a pass that is cancelled halfway leaves the half it finished behind rather
	// than nothing, and the batch is what peak memory costs either way.
	for _, group := range shipGateBatches(stale, store.DefaultBlockBatch) {
		blocks, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: stream, IDs: group.ids})
		if err != nil {
			return rollup, fmt.Errorf("load blocks for ship-state checks: %w", err)
		}
		byID := make(map[string]*venue.StoredBlock, len(blocks))
		for _, b := range blocks {
			if b != nil && b.Block != nil {
				byID[b.Block.ID] = b
			}
		}
		verdicts = verdicts[:0]
		for _, pair := range group.pairs {
			b := byID[pair.BlockID]
			// A pair the store named but the block read did not return was
			// deleted between the two queries. It has no verdict to record and
			// no content to count.
			if b == nil || !b.Block.Translatable || b.Block.Target(model.LocaleID(pair.Locale)) == nil {
				continue
			}
			judge(b.Block, b.ItemName, pair.Locale, pair.Basis)
		}
		if err := vs.PutShipGateVerdicts(ctx, projectID, stream, fingerprint, verdicts); err != nil {
			return rollup, fmt.Errorf("store ship-gate verdicts: %w", err)
		}
	}
	return rollup, nil
}

// shipGateScores flattens the resolved per-locale voice scores into the pairs
// the store's rollup needs, carrying only whether each clears its profile's bar.
func shipGateScores(scored map[string]map[string]scoredBlock) []store.ShipGateScore {
	var out []store.ShipGateScore
	for localeStr, byBlock := range scored {
		for blockID, sb := range byBlock {
			out = append(out, store.ShipGateScore{
				BlockID: blockID, Locale: localeStr,
				BelowBar: sb.score < sb.bar,
			})
		}
	}
	return out
}

// shipGateGroup is one hydration's worth of stale pairs: the distinct block ids
// to read, and the pairs those blocks answer for.
type shipGateGroup struct {
	ids   []string
	pairs []store.ShipGateStale
}

// shipGateBatches groups stale pairs into hydrations of at most size distinct
// blocks. The input is ordered by block, so a block's locales stay together and
// no block is read twice.
func shipGateBatches(stale []store.ShipGateStale, size int) []shipGateGroup {
	if size <= 0 {
		size = store.DefaultBlockBatch
	}
	var out []shipGateGroup
	cur := shipGateGroup{}
	seen := map[string]bool{}
	for _, st := range stale {
		if !seen[st.BlockID] {
			if len(cur.ids) >= size {
				out = append(out, cur)
				cur, seen = shipGateGroup{}, map[string]bool{}
			}
			seen[st.BlockID] = true
			cur.ids = append(cur.ids, st.BlockID)
		}
		cur.pairs = append(cur.pairs, st)
	}
	if len(cur.pairs) > 0 {
		out = append(out, cur)
	}
	return out
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

// applyCompliance stamps the derived compliance fields onto one locale scope. A
// scope with nothing translated gets no rate (nothing to rate — the additive
// fields stay omitted). The count is clamped to the translated denominator so
// a stats/block-read skew can never report a rate above 1. The basis names the
// evidence that informed the rate: rule-based checks always, plus terms when
// term governance was active for the locale, plus voice when a persisted score
// informed at least one block.
func applyCompliance(ls *store.LocaleTranslationStats, compliantCount int, voice, terms bool) {
	if ls.TranslatedBlocks <= 0 {
		return
	}
	ls.CompliantBlocks = min(compliantCount, ls.TranslatedBlocks)
	rate := float64(ls.CompliantBlocks) / float64(ls.TranslatedBlocks)
	ls.ComplianceRate = &rate
	ls.ComplianceBasis = store.ComplianceBasisFor(voice, terms)
}

// blockFailsChecks reports whether a block's target for a locale carries an
// error-severity finding — the "fails the ship gate" predicate shared by the
// dashboard ship-state pass (applyShipStates), the convergence derive
// (countFailingBlocks), and the bulk approve-passing endpoint.
//
// The standard per-locale set only. These callers sweep a whole project a block
// at a time and hold no item, so they resolve no point; the editor's check
// endpoints, which do hold one, add what the governance there declares.
func blockFailsChecks(ctx context.Context, block *model.Block, loc model.LocaleID) bool {
	for _, issue := range runChecksOnBlock(ctx, block, pointChecks{TargetLocale: loc}) {
		if issue.Severity == "error" {
			return true
		}
	}
	return false
}

// blockCompliantAndPassing reports whether a translated block+locale is clean
// enough to ship without a person's review: it passes the rule-based checks
// with no error-severity finding, is term-compliant for the locale (via the
// shared gate), AND — where a persisted voice score exists for the block — the
// score meets the scoring profile's compliance bar. This is exactly the per-block
// compliant predicate applyShipStates aggregates into the compliance rate (#1365);
// the bulk approve-passing endpoint reuses it to pick which pending drafts to
// auto-approve, so a target using a forbidden term or missing a mandated one is
// excluded and left pending for a person. `scored` is one locale's score map
// (latestVoiceScores(...)[normalize(locale)]); an empty map degrades to
// checks-only. A nil gate makes the term check a no-op, matching the dashboard.
func blockCompliantAndPassing(ctx context.Context, block *model.Block, loc model.LocaleID, scored map[string]scoredBlock, gate *termGate) bool {
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

// blockApproveBlocker is the one predicate behind blockCompliantAndPassing: it
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

// latestVoiceScores reads the project's persisted voice scores and keeps
// the newest per (locale, block), each paired with its scoring profile's
// compliance bar. Locale keys are normalized (scores are stored normalized).
// Best-effort by design: a nil voice store or a read failure yields an empty
// map, degrading the compliance rate to checks-only rather than failing the
// dashboard.
func latestVoiceScores(ctx context.Context, voiceStore coreprofile.Store, projectID, stream string) map[string]map[string]scoredBlock {
	out := map[string]map[string]scoredBlock{}
	if voiceStore == nil {
		return out
	}
	scores, err := voiceStore.GetScoresByStream(ctx, projectID, stream)
	if err != nil {
		slog.WarnContext(ctx, "voice scores unavailable; compliance rate falls back to checks-only",
			"project_id", projectID, "error", err)
		return out
	}
	bars := map[string]int{}
	barFor := func(profileID string) int {
		if b, ok := bars[profileID]; ok {
			return b
		}
		profile, err := voiceStore.GetProfile(ctx, profileID)
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
