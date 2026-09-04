package jobs

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
)

// The basis of a target the platform holds, and the one question the producers
// ask of it.
//
// A record in the decision ledger carries the SOURCE it was made against
// (unit_decisions.content_hash). That is true of a reviewer's approval and of
// the plain basis a producer writes when it puts a translation somewhere, so one
// comparison answers for both: the recorded basis against the source hash the
// block carries now. Equal means the translation renders the sentence the
// project holds; different means the sentence has been rewritten under it.
//
// Two boundaries hold the answer to the units the platform is entitled to speak
// for, and they are the same two the local loop applies (host/basisrecord.go):
//
//   - A target the ledger has no record of is left alone. The platform did not
//     write it and cannot say what it translates, so re-drafting it would
//     discard somebody's work on a guess.
//   - A basis is never written over a decision. The decision's basis is the
//     decision's, and replacing it would erase an approval a source edit is
//     supposed to withdraw rather than delete.
//
// The second boundary leaves a decided unit stale for as long as its re-review
// takes, and a producer that read only the decision would draft it again on
// every pass and pay for it each time. So the row carries a second mark beside
// the decision, the source the platform's latest draft was made against
// (store.DraftBasis), and the producers read the two together: a stale unit is
// owed a draft until that mark names the source the block holds now, and from
// then on it waits on a reviewer. A source rewritten again moves the block's
// hash away from the mark, and the unit is owed once more. The local loop gets
// the same guarantee from its content memory, which absorbs the re-drafted
// pairing and answers the next pass from it (host/recordabsorb.go).

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

// decisionLedger indexes a stream's recorded decisions by unit and variant.
//
// A nil ledger answers for nothing, which is the honest reading when the store
// keeps no ledger or the read failed: every target then reads as one the
// platform has no record of, and stays where it is.
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
		slog.WarnContext(ctx, "read the draft bases; this pass treats every stale unit as one it has not drafted",
			"project", projectID, "stream", stream, "error", err)
		return index
	}
	for _, d := range drafts {
		key := decisionUnitKey{item: d.ItemName, unit: d.Unit, variant: d.Variant}
		rec, ok := index[key]
		if !ok {
			continue
		}
		rec.draftBasis = d.SourceHash
		index[key] = rec
	}
	return index
}

// record returns the ledger's record for a block's variant.
func (l decisionLedger) record(sb *venue.StoredBlock, key model.VariantKey) (ledgerRecord, bool) {
	if len(l) == 0 || sb == nil || sb.SourceID == "" || sb.ItemName == "" {
		return ledgerRecord{}, false
	}
	d, ok := l[decisionUnitKey{item: sb.ItemName, unit: sb.SourceID, variant: variantText(key)}]
	return d, ok
}

// needsDraft reports whether a locale still has work on this block: it carries
// no target for the locale, or it carries one whose recorded basis names source
// wording the block no longer holds and that the platform has not yet drafted
// against the wording it holds now.
//
// This is the predicate the recycle pass partitions on and the estimate prices
// from, so a quote and the run it precedes describe the same set of units. The
// server's convergence derive counts the same units from the ledger's grouped
// tally (store.DecisionBasisTally.Owed), so a locale is pending on production
// exactly when a job for it would produce.
func (l decisionLedger) needsDraft(sb *venue.StoredBlock, locale model.LocaleID) bool {
	if sb == nil || sb.Block == nil {
		return false
	}
	if !hasLocaleTarget(sb.Block, locale) {
		return true
	}
	current := blockSourceHash(sb)
	if current == "" {
		return false
	}
	for key, t := range sb.Block.Targets {
		if key.Locale != locale || t == nil || len(t.Runs) == 0 || model.RunsText(t.Runs) == "" {
			continue
		}
		d, ok := l.record(sb, key)
		if !ok || d.ContentHash == "" {
			// No record, or one made before a basis was tracked. Unknown is not
			// stale, and reading it as stale would re-draft a translation on a
			// silence.
			continue
		}
		if d.ContentHash == current {
			continue
		}
		if d.draftBasis == current {
			// Stale, and already drafted against the source the block holds
			// now. The decision it carries is withdrawn until a person looks
			// at the new draft; another pass would change nothing but the
			// bill.
			continue
		}
		return true
	}
	return false
}

// blockSourceHash is the source hash to grade a record against: the one the
// store holds, falling back to the block's own source when a caller's store did
// not stamp it. Both are model.ComputeContentHash of the source text, which is
// the value a basis records (core/state.SourceHash).
func blockSourceHash(sb *venue.StoredBlock) string {
	if sb == nil {
		return ""
	}
	if sb.ContentHash != "" {
		return sb.ContentHash
	}
	if sb.Block == nil {
		return ""
	}
	return model.ComputeContentHash(sb.Block.SourceText())
}

// variantText renders a VariantKey the way the ledger stores it ("fr",
// "fr;tone=…").
func variantText(k model.VariantKey) string {
	b, err := k.MarshalText()
	if err != nil {
		return string(k.Locale)
	}
	return string(b)
}

// recordProducedBasis records, for every target this pass wrote, the source it
// was translated from. It is what lets the NEXT pass tell a translation of the
// current wording from one left over from wording that has since been rewritten,
// and it is the venue's half of what host/basisrecord.go does for the local
// loop.
//
// Two marks per unit. An undecided unit gets a basis record: a basis and
// nothing else, no rung, no review state, no decider, so the store projects no
// status and the target keeps the rung its producer put it on. Every unit,
// decided or not, gets its draft basis stamped beside whatever the row holds,
// which for a decided unit is the only mark this pass may leave: the decision
// stays, stale, until a reviewer replaces it, and the stamp is what keeps the
// next pass from drafting the unit again.
//
// Best-effort. A failure here costs the next pass its view of this one's output,
// never this pass's output, so it is logged and the run continues.
func recordProducedBasis(
	ctx context.Context,
	cs store.ContentStore,
	projectID, stream string,
	ledger decisionLedger,
	byBlockID map[string]*venue.StoredBlock,
	blocks []*model.Block,
	locale model.LocaleID,
) {
	ds, ok := cs.(store.DecisionStore)
	if !ok || len(blocks) == 0 {
		return
	}
	variant := variantText(model.Variant(locale))
	now := time.Now().UTC().Format(time.RFC3339)

	var records []venue.UnitDecision
	var drafts []store.DraftBasis
	for _, b := range blocks {
		if b == nil {
			continue
		}
		sb := byBlockID[b.ID]
		if sb == nil || sb.SourceID == "" || sb.ItemName == "" {
			// A block stored without an item has no unit identity for the
			// ledger to key on.
			continue
		}
		text := model.RunsText(localeTargetRuns(b, locale))
		if strings.TrimSpace(text) == "" {
			continue // nothing was produced; a record would claim a translation
		}
		key := decisionUnitKey{item: sb.ItemName, unit: sb.SourceID, variant: variant}
		prev, had := ledger[key]
		basis := blockSourceHash(sb)
		target := state.TargetHash(text)
		next := prev
		if !had {
			next.ItemName, next.Unit, next.Variant = sb.ItemName, sb.SourceID, variant
		}
		switch {
		case had && prev.ReviewState != "":
			// Decided: the basis is the decision's. Only the draft mark moves.
		case had && prev.ContentHash == basis && prev.TargetHash == target:
			// Already recorded for this exact pairing.
		default:
			records = append(records, venue.UnitDecision{
				ItemName:    sb.ItemName,
				Unit:        sb.SourceID,
				Variant:     variant,
				TargetHash:  target,
				ContentHash: basis,
				Updated:     now,
			})
			next.ContentHash, next.TargetHash, next.Updated = basis, target, now
		}
		if !had || prev.draftBasis != basis {
			drafts = append(drafts, store.DraftBasis{
				ItemName: sb.ItemName, Unit: sb.SourceID, Variant: variant, SourceHash: basis,
			})
			next.draftBasis = basis
		}
		if ledger != nil {
			// The chunks that follow in this job read the ledger they were
			// handed, so it has to say what the store now says.
			ledger[key] = next
		}
	}
	// The basis records first: a unit the ledger did not hold gets its row
	// here, and the stamp below lands on rows and creates none.
	if len(records) > 0 {
		if _, err := ds.UpsertUnitDecisions(ctx, projectID, stream, records); err != nil {
			slog.WarnContext(ctx, "record the basis for the targets this pass wrote; the next pass reads them as targets it has no record of",
				"project", projectID, "stream", stream, "locale", string(locale), "records", len(records), "error", err)
		}
	}
	if len(drafts) > 0 {
		if err := ds.RecordDraftBases(ctx, projectID, stream, drafts); err != nil {
			slog.WarnContext(ctx, "record the source this pass drafted against; the next pass may draft the same units again",
				"project", projectID, "stream", stream, "locale", string(locale), "records", len(drafts), "error", err)
		}
	}
}
