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
// On-brand definition: a translated block is compliant when it holds a passing
// result for every dimension governing its locale. The rule-based checks always
// govern and must report no error-severity finding. Terminology governs where
// terms or voice profile rules apply to the locale (termGate.termsGoverned),
// and the target must be term-compliant: it uses no forbidden/competitor term
// and omits no mandated preferred/approved rendering (see blockTermCompliance).
// The voice bar governs where a voice profile applies, and the block's latest
// persisted score (written by the worker's draft scoring, zero AI) must meet
// the scoring profile's bar (VoiceProfile.ComplianceBar).
// A term-non-compliant block is treated exactly like a failing check: it counts
// against the compliance rate AND, at full coverage, against FailingChecks, so
// it can never be governed or ai_shippable. A block below its voice bar counts
// against the rate only.
// A block with no terminology result in a locale terms govern withholds the
// scope's ship state as a failing check does, and a locale terminology does not
// govern is approved rather than governed once every block is approved
// (store.DeriveShipState).
// A block no bar fails that lacks a result for a governing dimension (an empty
// target where terms govern, an unscored block where a voice profile governs)
// is counted in NotCheckedBlocks, and one in a locale neither terms nor a voice
// profile govern in NotGovernedBlocks. Both are left out of the rate.
// ComplianceBasis names the dimensions governing each locale so consumers can
// present the number honestly. Voice scores are read best-effort: a voice store
// hiccup degrades the rate rather than failing the dashboard. The gate is
// resolved once per (workspace, locale) by the caller and reused across every
// block; a nil gate governs nothing.
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

	// termsGoverned and voiceGoverned are per locale: governance binds at the
	// locale, so one answer applies to the project-wide locale and to every
	// collection's. They decide which clean blocks can be compliant, which are
	// not governed, and what the compliance basis names.
	termsGoverned := map[string]bool{}
	voiceGoverned := map[string]bool{}

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
			loc := model.LocaleID(localeStr)
			termsGoverned[localeStr] = gate.termsGoverned(ctx, loc)
			voiceGoverned[localeStr] = gate.voiceGoverned(ctx, loc)
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
			p.TermsNotChecked += c.TermsNotChecked
			p.NotChecked += c.NotChecked
			project[loc] = p
		}
	}

	stamp := func(ls *store.LocaleTranslationStats, c store.ShipGateCounts, b basisCounts) {
		if shipCandidates[ls.Locale] {
			ls.FailingChecks = c.Failing
			ls.TermsNotCheckedBlocks = c.TermsNotChecked
		}
		ls.StaleBlocks = b.Stale
		ls.StaleAwaitingDraftBlocks = b.Owed
		ls.StaleAwaitingReviewBlocks = b.Stale - b.Owed
		ls.RejectedAwaitingDraftBlocks = b.RejectedOwed
		ls.BasisUnknownBlocks = b.BasisUnknown
		ls.ShipState, ls.ShipGates = store.EvaluateShipState(store.ShipStateInputs{
			TranslatedBlocks:      ls.TranslatedBlocks,
			TotalBlocks:           ls.TotalBlocks,
			ApprovedBlocks:        ls.ApprovedBlocks,
			FailingChecks:         ls.FailingChecks,
			StaleBlocks:           ls.StaleBlocks,
			RejectedBlocks:        ls.RejectedAwaitingDraftBlocks,
			TermsNotCheckedBlocks: ls.TermsNotCheckedBlocks,
			TermsGoverned:         termsGoverned[ls.Locale],
		})
		// A clean block not below the voice bar is compliant when it has a result
		// for every dimension governing its locale, and not checked when it lacks
		// one. Where nothing beyond the checks governs the locale it is not
		// governed. A failing or below-bar block has a verdict either way.
		compliant, notGoverned := c.Clean-c.CleanBelowBar-c.NotChecked, 0
		if !termsGoverned[ls.Locale] && !voiceGoverned[ls.Locale] {
			compliant, notGoverned = 0, compliant
		}
		applyCompliance(ls, compliant, c.NotChecked, notGoverned, voiceGoverned[ls.Locale], termsGoverned[ls.Locale])
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
	// would repeat the same work for every page of the corpus. Scores are kept
	// only for a locale a voice profile governs, because no bar applies anywhere
	// else.
	scored := make(map[string]map[string]scoredBlock, len(locales))
	voiceGoverned := make(map[string]bool, len(locales))
	var governedLocales []string
	for _, l := range locales {
		if !gate.voiceGoverned(ctx, model.LocaleID(l)) {
			continue
		}
		voiceGoverned[l] = true
		governedLocales = append(governedLocales, l)
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
			Scores:        shipGateScores(scored),
			VoiceGoverned: governedLocales,
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
		// error-severity finding OR its target violates the terminology for the
		// locale, and the two are treated identically for both FailingChecks and
		// the compliance rate. A terminology verdict that is unchecked or not
		// governed fails nothing, and the rollup counts it in its own state.
		// gate.compliance is offline (in-memory snapshot).
		termVerdict := gate.compliance(ctx, block, loc)
		fails := blockFailsChecks(ctx, block, loc) || termVerdict == store.TermComplianceViolation
		sb, isScored := scored[localeStr][block.ID]
		verdict := store.ShipGateVerdict{
			BlockID: block.ID,
			Locale:  localeStr,
			Basis:   basis,
			Fails:   fails,
			Terms:   termVerdict,
		}
		rollup.Add(collByItem[itemName], localeStr, verdict, store.ShipGateVoice{
			Governed: voiceGoverned[localeStr],
			Scored:   isScored,
			BelowBar: isScored && sb.score < sb.bar,
		})
		if keepsVerdicts {
			verdicts = append(verdicts, verdict)
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

// basisCounts is one scope's stale / unknown-basis decision counts, the part of
// the stale count the convergence loop still owes a draft for, and the
// rejections it owes a draft for outside that count.
type basisCounts struct {
	Stale        int
	BasisUnknown int
	Owed         int
	RejectedOwed int
}

// owed is every unit the convergence loop still owes a draft for. Owed is a
// subset of Stale and RejectedOwed holds only the rejections Stale does not, so
// the two are disjoint and each unit is counted once.
func (c basisCounts) owed() int { return c.Owed + c.RejectedOwed }

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
		if t.Stale == 0 && t.BasisUnknown == 0 && t.Owed == 0 && t.RejectedOwed == 0 {
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
		p.Owed += t.Owed
		p.RejectedOwed += t.RejectedOwed
		out.project[loc] = p

		cid := collByItem[t.ItemName]
		if out.byColl[cid] == nil {
			out.byColl[cid] = map[string]basisCounts{}
		}
		c := out.byColl[cid][loc]
		c.Stale += t.Stale
		c.BasisUnknown += t.BasisUnknown
		c.Owed += t.Owed
		c.RejectedOwed += t.RejectedOwed
		out.byColl[cid][loc] = c
	}
	return out, nil
}

// applyCompliance stamps the derived compliance fields onto one locale scope. A
// scope with nothing translated gets no rate (nothing to rate, and the additive
// fields stay omitted). notCheckedCount is the translated blocks no bar failed
// that lack a result for a dimension governing the locale, and
// notGovernedCount those in a locale nothing beyond the checks governs. Both
// are reported and count toward neither side of the rate, which is compliant
// over the blocks with a verdict and is absent when none has one. The counts
// are clamped to the translated denominator so a stats/block-read skew can
// never report a rate above 1. The basis names the dimensions governing the
// locale: rule-based checks always, plus terms and voice where they govern it.
func applyCompliance(ls *store.LocaleTranslationStats, compliantCount, notCheckedCount, notGovernedCount int, voice, terms bool) {
	if ls.TranslatedBlocks <= 0 {
		return
	}
	ls.NotCheckedBlocks = min(max(notCheckedCount, 0), ls.TranslatedBlocks)
	ls.NotGovernedBlocks = min(max(notGovernedCount, 0), ls.TranslatedBlocks-ls.NotCheckedBlocks)
	withVerdict := ls.TranslatedBlocks - ls.NotCheckedBlocks - ls.NotGovernedBlocks
	ls.CompliantBlocks = min(max(compliantCount, 0), withVerdict)
	ls.ComplianceBasis = store.ComplianceBasisFor(voice, terms)
	if withVerdict > 0 {
		rate := float64(ls.CompliantBlocks) / float64(withVerdict)
		ls.ComplianceRate = &rate
	}
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
	issues, err := runChecksOnBlock(ctx, block, pointChecks{TargetLocale: loc})
	if err != nil {
		return true
	}
	for _, issue := range issues {
		if issue.Severity == "error" {
			return true
		}
	}
	return false
}

// blockCompliantAndPassing reports whether a translated block+locale is clean
// enough to ship without a person's review: it passes the rule-based checks with
// no error-severity finding and holds a passing result for every other dimension
// governing its locale, terminology via the shared gate and the voice bar where
// a voice profile governs. A dimension that does not govern the locale is no
// bar, and a governing dimension with no result for the block is never passing.
// This is exactly the per-block compliant predicate applyShipStates aggregates
// into the compliance rate (#1365); the bulk approve-passing endpoint reuses it
// to pick which pending drafts to auto-approve. `scored` is one locale's score
// map (latestVoiceScores(...)[normalize(locale)]).
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
	// approveBlockerTermsNotChecked is the terminology bar with no verdict: terms
	// or voice profile rules govern the locale and the target holds no text.
	// Approving it would claim evidence nobody gathered.
	approveBlockerTermsNotChecked approveBlocker = "terms_not_checked"
	approveBlockerVoice           approveBlocker = "voice"
	// approveBlockerVoiceNotChecked is the voice bar with no verdict: a voice
	// profile governs the locale and nothing has scored the block.
	approveBlockerVoiceNotChecked approveBlocker = "voice_not_checked"
)

// blockApproveBlocker is the one predicate behind blockCompliantAndPassing: it
// applies the bars in gate order and names the first one the target misses or
// has no verdict for, or approveBlockerNone when it clears them all. A bar that
// does not govern the locale is skipped. Order is significant only for
// attribution: a target missing two bars is reported against the first, so the
// reported reasons sum to the skipped count.
func blockApproveBlocker(ctx context.Context, block *model.Block, loc model.LocaleID, scored map[string]scoredBlock, gate *termGate) approveBlocker {
	if blockFailsChecks(ctx, block, loc) {
		return approveBlockerChecks
	}
	switch gate.compliance(ctx, block, loc) {
	case store.TermComplianceViolation:
		return approveBlockerTerms
	case store.TermComplianceUnchecked:
		return approveBlockerTermsNotChecked
	}
	switch gate.voiceCompliance(ctx, block.ID, loc, scored) {
	case store.VoiceComplianceBelowBar:
		return approveBlockerVoice
	case store.VoiceComplianceUnchecked:
		return approveBlockerVoiceNotChecked
	}
	return approveBlockerNone
}

// voiceCompliance is a block's standing against the voice bar for loc. No bar
// applies in a locale no voice profile governs. Where one does, a block nothing
// has scored is unchecked, and a scored block is measured against the bar of the
// profile that scored it. `scored` is the locale's score map.
func (g *termGate) voiceCompliance(ctx context.Context, blockID string, loc model.LocaleID, scored map[string]scoredBlock) store.VoiceCompliance {
	if !g.voiceGoverned(ctx, loc) {
		return store.VoiceComplianceNotGoverned
	}
	sb, ok := scored[blockID]
	switch {
	case !ok:
		return store.VoiceComplianceUnchecked
	case sb.score < sb.bar:
		return store.VoiceComplianceBelowBar
	default:
		return store.VoiceCompliancePassing
	}
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
