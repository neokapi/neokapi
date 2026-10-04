package venue

import "github.com/neokapi/neokapi/core/model"

// SourceRevision is the revision of b's source as a venue holds it and grades
// a decision's basis against: the source's runs under the project's source
// language, source. It is the revision a project read takes of the same
// content (model.Block.SourceRevisions lists it first, since a project read
// files every block under the project's language), so a basis a checkout
// records and the revision a venue stamps on the block it stores are one
// value for one content, inline codes included.
//
// A block that holds a translation filed under the source language itself (a
// bilingual file whose two languages are one) knows its source by the zero
// key, as model.Block does, and the revision is taken under that key.
//
// The block's own SourceLocale plays no part: a format reader names a
// language of its own or none, and a venue keeps no language per block, so
// both ends take the revision under the project's language.
func SourceRevision(b *model.Block, source model.LocaleID) string {
	if b == nil {
		return ""
	}
	k := model.Variant(source)
	if !k.IsZero() {
		if _, held := b.Edition(k); held && !b.IsSourceEdition(k) {
			k = model.EditionKey{}
		}
	}
	return model.RunsRevision(k, b.SourceRuns())
}

// RecordHash is the transfer hash of b: its content and context hashes and
// the revision of its source under the project's source language, folded by
// model.ComputeRecordHash. A push sends a block whose record hash the venue
// does not hold, so a change to an inline code alone is transferred as a
// change to the wording is. A venue folds the same three from the columns it
// stores.
func RecordHash(b *model.Block, source model.LocaleID) string {
	id := model.ComputeIdentity(b)
	return model.ComputeRecordHash(id.ContentHash, id.ContextHash, SourceRevision(b, source))
}
