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
