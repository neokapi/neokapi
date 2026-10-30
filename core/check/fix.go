package check

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// MetaSpelled is the metadata key that marks a finding whose words a
// character reference spells an edge of. Its words are quoted as the page shows
// them and no span of the block's text holds them, so it gets no fix: a
// replacement found by its text could land on another use of the words, or be
// refused as ambiguous with the rest of the change set.
const MetaSpelled = "spelled"

// Fix is the replace_text that applies a finding's replacement
// (Metadata["replacement"]) to the edition at names, guarded by rev, the
// revision the check read. runs are the edition's runs as the check read them.
//
// The finding's run range names the words when at is the block's own edition.
// Otherwise the quoted text does, which the change service refuses as
// ambiguous when it occurs more than once.
//
// A replacement is plain text, so it keeps an inline code only at either end
// of the words. Fix returns nil where a replacement would delete a link, a
// bold span or a placeholder among the words, for words in a code span
// (model.TypeInlineCode) or a URL (URLSpans), for a finding with no
// replacement, for words nothing locates, and for a finding marked
// MetaSpelled. A lower-case replacement may take a capital
// (KeepInitialCapital).
func Fix(f Finding, runs []model.Run, at change.Ref, rev string) *change.Op {
	replacement := ""
	if f.Metadata != nil {
		replacement = f.Metadata["replacement"]
	}
	if replacement == "" || rev == "" || f.Metadata[MetaSpelled] != "" {
		return nil
	}
	var sel change.Selection
	switch {
	case at.Edition.IsZero() && f.Position.Kind == model.AnchorRange && f.Position.Start != f.Position.End:
		seq, ok := model.ResolveRunPath(runs, f.Position.Path)
		if !ok || !f.Position.InBounds(seq) {
			return nil
		}
		start, end := ownOffset(seq, f.Position.Start), ownOffset(seq, f.Position.End)
		if codeInside(seq, start, end) || inInlineCode(seq, start, end) {
			return nil
		}
		text := model.SequenceText(seq)
		from, to := byteOffset(text, start), byteOffset(text, end)
		if InURL(text, from, to) {
			return nil
		}
		replacement = KeepInitialCapital(text[from:to], replacement, sentenceStart(text, from))
		sel = change.Selection{Path: f.Position.Path, Range: &change.Span{Start: f.Position.Start, End: f.Position.End}}
	case f.OriginalText != "":
		if !foundInTextAlone(runs, f.OriginalText) {
			return nil
		}
		find := f.OriginalText
		replacement = KeepInitialCapital(find, replacement, false)
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

// KeepInitialCapital is replacement as it replaces words. A lower-case
// replacement takes a capital when the words begin a sentence (atSentence) or
// their first word is Title-case, so "E-mail" with "email" becomes "Email".
//
// Otherwise the replacement lands as written: a replacement with a capital
// after its first letter ("iPhone", "macOS"), and words that differ from it
// only in case ("Kapi" for "kapi"). So do words that start with an acronym
// ("URL") or carry a capital in a later word ("Kapi CLI"), whose capitals
// belong to a name rather than to the sentence.
func KeepInitialCapital(words, replacement string, atSentence bool) string {
	r, size := utf8.DecodeRuneInString(replacement)
	w, _ := utf8.DecodeRuneInString(words)
	switch {
	case !unicode.IsLower(r), !unicode.IsUpper(w),
		hasUpper(replacement[size:]), strings.EqualFold(words, replacement):
		return replacement
	}
	first, rest, _ := strings.Cut(words, " ")
	if isAcronym(first) || hasUpper(rest) || !atSentence && !titleCase(first) {
		return replacement
	}
	return string(unicode.ToUpper(r)) + replacement[size:]
}

// sentenceStart reports whether byte offset pos of text begins a sentence:
// nothing but space before it, or a full stop, question mark, exclamation
// mark, colon or line break before the space.
func sentenceStart(text string, pos int) bool {
	before := strings.TrimRight(text[:pos], " \t")
	if before == "" {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(before)
	return strings.ContainsRune(".!?:\n", r)
}

// titleCase reports whether word is one capital followed by no other.
func titleCase(word string) bool {
	r, size := utf8.DecodeRuneInString(word)
	return unicode.IsUpper(r) && !hasUpper(word[size:])
}

// isAcronym reports whether word holds two letters or more, every one a
// capital, such as "URL".
func isAcronym(word string) bool {
	letters := 0
	for _, r := range word {
		if !unicode.IsLetter(r) {
			continue
		}
		if !unicode.IsUpper(r) {
			return false
		}
		letters++
	}
	return letters > 1
}

// hasUpper reports whether s holds a capital.
func hasUpper(s string) bool { return strings.ContainsFunc(s, unicode.IsUpper) }

// byteOffset is the byte offset of the code point at index n of text.
func byteOffset(text string, n int) int {
	for i := range text {
		if n == 0 {
			return i
		}
		n--
	}
	return len(text)
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

// inInlineCode reports whether [start, end) of seq's own text lies inside an
// inline code span: a paired code of type model.TypeInlineCode that opens at
// or before start and closes at or after end.
func inInlineCode(seq []model.Run, start, end int) bool {
	at := 0
	open := map[string]int{}
	for _, r := range seq {
		switch {
		case r.Text != nil:
			at += utf8.RuneCountInString(r.Text.Text)
		case r.PcOpen != nil && r.PcOpen.Type == model.TypeInlineCode:
			open[r.PcOpen.ID] = at
		case r.PcClose != nil && r.PcClose.Type == model.TypeInlineCode:
			if from, ok := open[r.PcClose.ID]; ok && from <= start && end <= at {
				return true
			}
			delete(open, r.PcClose.ID)
		}
	}
	return false
}

// foundInTextAlone reports whether words occur in the text runs hold
// themselves, and no occurrence has an inline code inside it or sits in a URL,
// as the change service finds them.
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
		if codeInside(runs, start, start+n) || inInlineCode(runs, start, start+n) || InURL(text, i+j, i+j+len(words)) {
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
