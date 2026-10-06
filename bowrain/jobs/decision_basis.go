package jobs

import (
	"context"
	"log/slog"
	"strings"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// The basis of a target the platform holds, and the one question the producers
// ask of it.
//
// A translation records the source it was made from on the edition itself
// (model.Edition.Derived). The stream home stamps it whenever a change set
// writes the translation (a draft the job produced, a person's edit, a pushed
// write), so the record of the write that left a translation's current
// revision is where its basis lives, as it is locally (design 5.3, WP7). A
// decision in the ledger carries the source it blessed, by revision
// (unit_decisions.basis). One comparison answers for both: the recorded basis
// against the revision of the source the block carries now (store.BasisStale).
// Equal means the translation renders the source the project holds; different
// means the source has changed under it, its wording or an inline code.
//
// Two boundaries hold the answer to the units the platform is entitled to speak
// for:
//
//   - A target with neither a decision nor a recorded derivation is left
//     alone. Nothing says what it translates, so re-drafting it would discard
//     somebody's work on a guess.
//   - A decision's basis is the decision's. A draft never replaces it, because
//     that would erase an approval a source edit is supposed to withdraw
//     rather than delete.
//
// The second boundary leaves a decided unit stale for as long as its re-review
// takes, and a producer that read only the decision would draft it again on
// every pass and pay for it each time. So the row carries a second mark beside
// the decision, the source the platform's latest draft was made against
// (store.DraftBasis), and the producers read the two together: a stale decided
// unit is owed a draft until that mark names the source the block holds now,
// and from then on it waits on a reviewer. A source changed again moves the
// block's revision away from the mark, and the unit is owed once more.
//
// The mark carries the second question a producer has to ask as well. Comparing
// the revisions answers "does this translation render the source the project
// holds", and it cannot answer "does the project stand behind it": a reviewer
// rejecting a translation of the CURRENT source moves neither revision, so the
// row reads exactly like a settled one. The verdict is therefore read beside the
// basis, and a rejection clears the mark (server.reviewLedger.clearDraftBasis),
// which is what buys such a unit exactly one more draft. Once the pass stamps
// the mark again the unit waits on the next verdict, and a second rejection
// owes a second draft, so the two marks together admit no loop.

// decisionUnitKey is the ledger's own key, rendered for an in-memory index: the
// item whose identity namespace the unit lives in, the unit, and the variant in
// its text form.
type decisionUnitKey struct {
	item    string
	unit    string
	variant string
}

// ledgerRecord is one unit's ledger row as the producers read it: the wire
// record, and the platform's own mark of the source it last drafted the unit
// against. The mark is empty for a unit the platform has never drafted.
type ledgerRecord struct {
	venue.UnitDecision
	draftBasis string
}

// decisionLedger indexes stream decisions by unit and variant. A nil ledger
// indicates unavailable decision data: a target is then graded by the
// derivation its edition records alone.
type decisionLedger map[decisionUnitKey]ledgerRecord

// loadDecisionLedger reads the stream's recorded decisions into the index the
// producers share. A store without the ledger capability, or a read that fails,
// yields a nil ledger rather than an error: the basis makes a producer bolder
// (it re-drafts what it can prove is stale), so losing it costs freshness, never
// correctness.
func loadDecisionLedger(ctx context.Context, cs store.ContentStore, projectID, stream string) decisionLedger {
	ds, ok := cs.(store.DecisionStore)
	if !ok {
		return nil
	}
	records, err := ds.ListUnitDecisions(ctx, projectID, stream)
	if err != nil {
		slog.WarnContext(ctx, "read the decision ledger; this pass treats every target as one it has no record of",
			"project", projectID, "stream", stream, "error", err)
		return nil
	}
	if len(records) == 0 {
		return nil
	}
	index := make(decisionLedger, len(records))
	for _, d := range records {
		index[decisionUnitKey{item: d.ItemName, unit: d.Unit, variant: d.Variant}] = ledgerRecord{UnitDecision: d}
	}
	// The draft marks sit on the same rows. A read that fails leaves every
	// record unmarked, which reads as "no draft yet": the pass re-drafts what
	// it might already have drafted, and pays for it, rather than skipping
	// work it cannot prove was done.
	drafts, err := ds.ListDraftBases(ctx, projectID, stream)
	if err != nil {
		slog.WarnContext(ctx, "read the draft bases; this pass treats every stale translation as one it has not drafted",
			"project", projectID, "stream", stream, "error", err)
		return index
	}
	for _, d := range drafts {
		key := decisionUnitKey{item: d.ItemName, unit: d.Unit, variant: d.Variant}
		rec, ok := index[key]
		if !ok {
			continue
		}
		rec.draftBasis = d.Basis
		index[key] = rec
	}
	return index
}

// record returns the ledger's record for a block's variant.
func (l decisionLedger) record(sb *venue.StoredBlock, key model.EditionKey) (ledgerRecord, bool) {
	if len(l) == 0 || sb == nil || sb.SourceID == "" || sb.ItemName == "" {
		return ledgerRecord{}, false
	}
	d, ok := l[decisionUnitKey{item: sb.ItemName, unit: sb.SourceID, variant: variantText(key)}]
	return d, ok
}

// needsDraft reports whether a locale still has work on this block. Three ways
// a unit is owed one: it carries no target for the locale; it carries one whose
// recorded basis names a revision of the source other than the one the block
// holds; or a reviewer turned the translation down. The draft mark answers all three the
// same way, because a unit the platform has already drafted against the source
// the block holds now is waiting on a person whatever the row's verdict says.
//
// The verdict is read beside the basis because a rejection of a translation of
// the CURRENT source moves neither revision: the row records the source the
// block still carries, and grading it alone reported the unit as settled while
// it sat at `draft` with a reviewer's refusal on it, waiting for somebody to
// edit the source before the loop would look at it again (#2564).
//
// This is the predicate the recycle pass partitions on and the estimate prices
// from, so a quote and the run it precedes describe the same set of units. The
// server's convergence derive counts the same units from the ledger's grouped
// tally (store.DecisionBasisTally.Owed plus RejectedOwed, which are disjoint),
// so a locale is pending on production exactly when a job for it would produce.
func (l decisionLedger) needsDraft(sb *venue.StoredBlock, locale model.LocaleID) bool {
	if sb == nil || sb.Block == nil {
		return false
	}
	if !hasLocaleTarget(sb.Block, locale) {
		return true
	}
	current := sb.SourceRevision
	auth := sb.Block.Authoritative(model.AuthorityPolicy{})
	for key, t := range sb.Block.EachEdition {
		if key.Locale != locale || key == auth {
			continue
		}
		if len(t.Runs) == 0 || model.RunsText(t.Runs) == "" {
			continue
		}
		d, ok := l.record(sb, key)
		if !ok {
			// No decision: the basis is the one the write that left the
			// translation recorded on it. A target with none is one nothing
			// says the source of, and re-drafting it on a silence would
			// discard somebody's work on a guess.
			if b, ok := derivedBasis(t, auth); ok && store.BasisStale(b, current) {
				return true
			}
			continue
		}
		if store.BasisCurrent(d.draftBasis, current) {
			// Already drafted against the source the block holds now. Whatever
			// the row carries, the unit is a reviewer's to move; another pass
			// would change nothing but the bill. A rejection clears this mark,
			// which is what buys the rejected unit exactly one more draft.
			continue
		}
		if d.ReviewState == venue.ReviewStateRejected {
			return true
		}
		if !store.BasisStale(d.Basis, current) {
			// A basis naming the source the block still holds, or none (a
			// translation written outside kapi). Unknown is not stale, and
			// reading it as stale would re-draft a translation on a silence.
			continue
		}
		return true
	}
	return false
}

// variantText renders an EditionKey the way the ledger stores it ("fr",
// "fr;tone=…").
func variantText(k model.EditionKey) string {
	b, err := k.MarshalText()
	if err != nil {
		return string(k.Locale)
	}
	return string(b)
}

// derivedBasis returns the revision of the source a translation records it
// was made from, when its derivation names the edition the block is written
// in (auth) or the bare language of one.
func derivedBasis(t model.Edition, auth model.EditionKey) (string, bool) {
	d := t.Derived
	if d == nil || d.Rev == "" {
		return "", false
	}
	if d.From != auth && !d.From.IsZero() && (d.From.Tone != "" || d.From.Channel != "" || d.From.Locale != auth.Locale) {
		return "", false
	}
	return d.Rev, true
}

// recordDraftMarks stamps, for every unit the ledger holds a row for and
// this pass wrote a target of, the source the draft was made against
// (store.DraftBasis). The basis of the draft itself needs no record here: the
// stream home stamped it on the edition the change set wrote. The mark is
// what keeps the next pass from drafting a stale decided unit again while
// the decision waits on a reviewer. A unit with no row gets none: a row
// carrying only a mark would read as a decision with an unknown basis.
//
// Best-effort. A failure here costs the next pass its view of this one's output,
// never this pass's output, so it is logged and the run continues.
func recordDraftMarks(
	ctx context.Context,
	cs store.ContentStore,
	projectID, stream string,
	ledger decisionLedger,
	byBlockID map[string]*venue.StoredBlock,
	blocks []*model.Block,
	locale model.LocaleID,
) {
	ds, ok := cs.(store.DecisionStore)
	if !ok || len(blocks) == 0 || len(ledger) == 0 {
		return
	}
	variant := variantText(model.Variant(locale))

	var drafts []store.DraftBasis
	for _, b := range blocks {
		if b == nil {
			continue
		}
		sb := byBlockID[b.ID]
		if sb == nil || sb.SourceID == "" || sb.ItemName == "" {
			continue
		}
		if strings.TrimSpace(model.RunsText(localeTargetRuns(b, locale))) == "" {
			continue // nothing was produced
		}
		key := decisionUnitKey{item: sb.ItemName, unit: sb.SourceID, variant: variant}
		prev, had := ledger[key]
		basis := sb.SourceRevision
		if !had || prev.draftBasis == basis {
			continue
		}
		drafts = append(drafts, store.DraftBasis{
			ItemName: sb.ItemName, Unit: sb.SourceID, Variant: variant, Basis: basis,
		})
		// The chunks that follow in this job read the ledger they were
		// handed, so it has to say what the store now says.
		prev.draftBasis = basis
		ledger[key] = prev
	}
	if len(drafts) > 0 {
		if err := ds.RecordDraftBases(ctx, projectID, stream, drafts); err != nil {
			slog.WarnContext(ctx, "record the source this pass drafted against; the next pass may draft the same translations again",
				"project", projectID, "stream", stream, "locale", string(locale), "records", len(drafts), "error", err)
		}
	}
}
