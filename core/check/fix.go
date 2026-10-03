package check

import (
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// Fix is the change operation that applies a finding's replacement to the
// edition it was raised on: a replace_text of the words the finding objects
// to with the wording its rule asks for (Metadata["replacement"]), guarded by
// rev, the revision of the edition the check read. at names that edition.
//
// The words are named by the finding's run range when it has one and at is
// the block's own edition, the runs a position is anchored to; otherwise by
// the text the finding quotes, which the change service refuses as ambiguous
// when it occurs more than once. A finding with no replacement, or with
// nothing that locates its words, has no fix and Fix returns nil.
func Fix(f Finding, at change.Ref, rev string) *change.Op {
	replacement := ""
	if f.Metadata != nil {
		replacement = f.Metadata["replacement"]
	}
	if replacement == "" || rev == "" {
		return nil
	}
	var sel change.Selection
	switch {
	case at.Edition.IsZero() && f.Position.Kind == model.AnchorRange && f.Position.Start != f.Position.End:
		sel = change.Selection{Path: f.Position.Path, Range: &change.Span{Start: f.Position.Start, End: f.Position.End}}
	case f.OriginalText != "":
		find := f.OriginalText
		sel = change.Selection{Find: &find}
	default:
		return nil
	}
	return &change.Op{
		Kind:    change.KindReplaceText,
		At:      at,
		IfMatch: rev,
		Body:    &change.ReplaceText{Edits: []change.TextEdit{{Selection: sel, Text: replacement}}},
	}
}

// SourceRevision is the revision of b's own edition as the change service
// reads it, for a fix's if_match. A block read for a check may carry no
// language of its own, and the service's read names the edition by the
// language the document is written in, so source names it here.
func SourceRevision(b *model.Block, source model.LocaleID) string {
	if b.SourceLocale == "" && source != "" {
		return model.RunsRevision(model.EditionKey{Locale: source}, b.SourceRuns())
	}
	return model.EditionRevision(b, model.EditionKey{})
}
