package venue

import (
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/model"
)

// SourceRevision is the revision of b's source as a venue holds it and grades
// a decision's basis against: the source's runs under the project's source
// language, source. It depends on those runs and that language alone. The
// language the block's reader declared plays no part, and neither do the
// translations the block holds, so two writes of one source give one revision
// whichever of its translations each carries.
//
// A checkout takes the revision of the same content under whichever key its
// reader filed the source by: the project's language for a project read, the
// language a format declares for the change service, or no language. It
// accepts any of them as current (model.Block.SourceRevisions), and the
// venue's revision is always among them. A push sends a basis as the venue
// takes it (Basis), so a checkout's record and the venue's revision are one
// value for one content, inline codes included.
func SourceRevision(b *model.Block, source model.LocaleID) string {
	if b == nil {
		return ""
	}
	return model.RunsRevision(model.Variant(source), b.SourceRuns())
}

// Basis is a basis a checkout recorded, as a venue holding b grades it: when
// basis is a revision of b's source under any key a reader of the document
// takes one under (model.Block.SourceRevisions), it is the venue's revision
// of that source (SourceRevision). Any other basis names another source, and
// is returned as it is: it reads stale on the venue as it does on the
// checkout. An empty basis stays empty.
func Basis(b *model.Block, source model.LocaleID, basis string) string {
	if b == nil || basis == "" {
		return basis
	}
	if slices.Contains(b.SourceRevisions(source), basis) {
		return SourceRevision(b, source)
	}
	return basis
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

// SourceLanguageMismatch reports why a push that takes source revisions under
// pushed cannot land on a project written in project, or "" when the two name
// one language. A venue stamps each source revision under its project's
// language and grades every basis by equality with it, so a push under another
// language would read every decision stale and send every block again on each
// push.
func SourceLanguageMismatch(pushed, project model.LocaleID) string {
	if pushed.IsEmpty() || project.IsEmpty() ||
		model.NormalizeLocale(pushed) == model.NormalizeLocale(project) {
		return ""
	}
	return fmt.Sprintf("the recipe's source language is %s, and the project on the server is written in %s; "+
		"set defaults.source_language to %s in kapi.yaml, or push to a project whose source language is %s",
		pushed, project, project, pushed)
}
