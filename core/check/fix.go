package check

import (
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// Fix is the change operation that applies a finding's replacement to the
// edition it was raised on: a replace_text of the words the finding objects
// to with the wording its rule asks for (Metadata["replacement"]), guarded by
// rev, the revision of the edition the check read. at names that edition, and
// runs are its runs as the check read them.
//
// The words are named by the finding's run range when it has one and at is
// the block's own edition, the runs a position is anchored to; otherwise by
// the text the finding quotes, which the change service refuses as ambiguous
// when it occurs more than once.
//
// A replacement is plain text, so it keeps an inline code only at either end
// of the words it replaces: a link, a bold span or a placeholder that sits
// among the words would be deleted with them. A finding whose words hold one
// has no fix, and is edited by hand. Fix returns nil for it, for a finding
// with no replacement, and for one with nothing that locates its words.
func Fix(f Finding, runs []model.Run, at change.Ref, rev string) *change.Op {
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
		seq, ok := model.ResolveRunPath(runs, f.Position.Path)
		if !ok || !f.Position.InBounds(seq) {
			return nil
		}
		if codeInside(seq, ownOffset(seq, f.Position.Start), ownOffset(seq, f.Position.End)) {
			return nil
		}
		sel = change.Selection{Path: f.Position.Path, Range: &change.Span{Start: f.Position.Start, End: f.Position.End}}
	case f.OriginalText != "":
		if !foundInTextAlone(runs, f.OriginalText) {
			return nil
		}
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

// ownOffset is the code-point offset of p into the text seq holds itself
// (model.SequenceText), in which every run but a text run has zero width.
func ownOffset(seq []model.Run, p model.RunPos) int {
	n := 0
	for i := 0; i < p.Run && i < len(seq); i++ {
		if seq[i].Text != nil {
			n += utf8.RuneCountInString(seq[i].Text.Text)
		}
	}
	if p.Run < len(seq) && seq[p.Run].Text != nil {
		n += p.Offset
	}
	return n
}

// codeInside reports whether a run with no text of its own (an inline code, a
// plural or a select) sits strictly inside [start, end) of seq's own text,
// where a replacement of that text deletes it. One at either end stays beside
// the replacement.
func codeInside(seq []model.Run, start, end int) bool {
	at := 0
	for _, r := range seq {
		if r.Text != nil {
			at += utf8.RuneCountInString(r.Text.Text)
			continue
		}
		if at > start && at < end {
			return true
		}
	}
	return false
}

// foundInTextAlone reports whether words occur in the text runs hold
// themselves, and no occurrence has an inline code inside it, as the change
// service finds them.
func foundInTextAlone(runs []model.Run, words string) bool {
	text := model.SequenceText(runs)
	n := utf8.RuneCountInString(words)
	found := false
	for i := 0; i < len(text); {
		j := strings.Index(text[i:], words)
		if j < 0 {
			break
		}
		start := utf8.RuneCountInString(text[:i+j])
		if codeInside(runs, start, start+n) {
			return false
		}
		found = true
		i += j + len(words)
	}
	return found
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
