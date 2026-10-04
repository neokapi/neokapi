package store

import (
	"errors"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// ErrBlockChanged reports that a block changed after the caller read it. A
// write made against that read is refused, and the caller is answered with the
// block as it now stands.
var ErrBlockChanged = errors.New("block changed since it was read")

// TargetRevision identifies one locale's target on a block as a reader sees
// it: the edition revision of that target (model.RunsRevision), a function of
// the target's content alone. A single write names the revision it read, and
// lands only while the target still has it (see BlockStore.UpdateBlock). A
// locale with no target reads as model.AbsentRevision, so two first
// translations of one block cannot overwrite each other.
//
// The source and the target's status are not part of it: a fix to the source
// wording does not refuse a translator's save, and a review decision binds to
// the revision it saw rather than moving it. The same token is valid wherever
// the content is held, a project file as well as a stream row.
func TargetRevision(sb *venue.StoredBlock, locale model.LocaleID) string {
	if sb == nil || sb.Block == nil {
		return model.AbsentRevision
	}
	t, ok := sb.Block.TargetEdition(locale)
	if !ok {
		return model.AbsentRevision
	}
	return model.RunsRevision(model.Variant(locale), t.Runs)
}

// VariantRevision is the revision of a translation filed under variant, a
// variant in EditionKey text form as the ledger and the translations table
// store it, with content runs. A variant that does not parse yields "", which
// no decision's revision equals.
func VariantRevision(variant string, runs []model.Run) string {
	key, err := model.ParseEditionKey(variant)
	if err != nil {
		return ""
	}
	return model.RunsRevision(key, runs)
}

// BasisCurrent reports whether a record whose basis is basis was made against
// the source a block holds at sourceRevision (StoredBlock.SourceRevision): the
// two are one revision. A record that names no basis names no source, so it is
// never current, and neither is any basis against a block whose source
// revision was never stamped.
func BasisCurrent(basis, sourceRevision string) bool {
	return basis != "" && basis == sourceRevision
}

// BasisStale reports whether a record whose basis is basis was made against a
// source other than the one a block holds at sourceRevision: it names a
// revision, and not that one. A record that names no basis (a translation
// written outside kapi, made from no recorded source) is not stale: its basis
// is unknown, as a checkout grades it.
//
// BasisCurrent and BasisStale are the grading rules of the decision ledger.
// Both stores' SQL encode the same tests over their columns.
func BasisStale(basis, sourceRevision string) bool {
	return basis != "" && basis != sourceRevision
}
