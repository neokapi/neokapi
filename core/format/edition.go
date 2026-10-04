package format

import "github.com/neokapi/neokapi/core/model"

// AuthoritativeRuns returns the runs of the block's authoritative edition
// under the default policy: the edition the block was read in, which a writer
// emits wherever it writes no target. The slice is the block's own; treat it
// as read-only.
func AuthoritativeRuns(b *model.Block) []model.Run {
	e, _ := b.Edition(b.Authoritative(model.AuthorityPolicy{}))
	return e.Runs
}
