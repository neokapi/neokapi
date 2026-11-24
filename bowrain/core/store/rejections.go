package store

import (
	"context"

	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/venue"
)

// RejectionTally is how many drafts of one source a unit has had turned down
// for one variant: each rejected draft, and each draft that came back in
// wording a reviewer had rejected (a repeat, which the platform does not
// keep). It sits on the unit's ledger row beside the latest decision, because
// the row keeps only that decision and a count read from it would always be
// one.
type RejectionTally struct {
	ItemName string
	Unit     string
	// Variant is the locale (and optional tone/channel) in EditionKey text
	// form, as the ledger stores it.
	Variant string
	// Basis is the revision of the source the counted drafts were made from
	// (StoredBlock.SourceRevision). A rejection of a draft of another source
	// starts the count again.
	Basis string
	Count int
}

// Holds reports whether the unit has reached the stop for the source the
// block holds now (sourceRevision): tool.MaxRejections drafts of it turned
// down. The platform then drafts it no more, and a person writes its
// translation. A changed source is drafted again.
func (t RejectionTally) Holds(sourceRevision string) bool {
	return t.Basis != "" && t.Basis == sourceRevision && t.Count >= tool.MaxRejections
}

// CountRejection is the unit's tally after next replaces the decision old (nil
// for a unit with no row). Only a rejection of a draft the row did not already
// hold as rejected adds one: the same rejected draft restated with an edited
// note is the same rejection. Any other decision leaves the tally as it is.
// next.Basis names the source the rejected draft was made from; a store fills
// in the block's current source revision for a rejection that names none.
func CountRejection(old *venue.UnitDecision, tally RejectionTally, next venue.UnitDecision) RejectionTally {
	if next.ReviewState != venue.ReviewStateRejected {
		return tally
	}
	if old != nil && old.ReviewState == venue.ReviewStateRejected && old.Revision == next.Revision {
		return tally
	}
	return addRejection(tally, next.Basis)
}

// addRejection adds one turned-down draft of the source basis to tally.
func addRejection(tally RejectionTally, basis string) RejectionTally {
	if tally.Count > 0 && tally.Basis == basis {
		tally.Count++
		return tally
	}
	tally.Basis, tally.Count = basis, 1
	return tally
}

// RejectionLedger is the optional capability that keeps the tallies. Both
// persistent stores implement it; a producer that finds none drafts without a
// stop, as it did before the tallies existed.
type RejectionLedger interface {
	// RecordRepeatedDrafts adds one to the tally of each named unit for the
	// source its repeated draft was made from (DraftBasis.Basis). A unit with
	// no ledger row is not given one.
	RecordRepeatedDrafts(ctx context.Context, projectID, stream string, drafts []DraftBasis) error
	// ListRejectionTallies returns every tally on the stream with a count
	// above zero.
	ListRejectionTallies(ctx context.Context, projectID, stream string) ([]RejectionTally, error)
}
