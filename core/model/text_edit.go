package model

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// TextEdit replaces the half-open range [Start, End) of a run sequence's own
// text (SequenceText) with Replacement. Offsets count Unicode code points, the
// unit the wire, the TypeScript mirror and Anchor share; a detector that
// reports byte offsets converts them with RangeAnchorForBytes. Edits passed to
// ApplyTextEdits must be sorted by Start and non-overlapping.
type TextEdit struct {
	Start       int
	End         int
	Replacement string
}

// HasStructuredRuns reports whether a run sequence contains plural or select
// runs. Their text lives in nested forms that a position in the sequence's own
// text does not reach; an edit inside one addresses the form by its RunPath.
func HasStructuredRuns(runs []Run) bool {
	for _, r := range runs {
		if r.Plural != nil || r.Select != nil {
			return true
		}
	}
	return false
}

// SequenceText returns the text a run sequence holds itself: the text of its
// text runs, in order. Every other run, a plural or a select included, has
// zero width. For a sequence with no plural or select it equals RunsText;
// TextEdit offsets index it.
func SequenceText(runs []Run) string {
	var b strings.Builder
	for _, r := range runs {
		if r.Text != nil {
			b.WriteString(r.Text.Text)
		}
	}
	return b.String()
}

// ApplyTextEdits rewrites a run sequence by applying code-point edits to its
// own text (SequenceText), then repositioning the inline codes that survive.
// Edits must be sorted by Start and be non-overlapping; malformed input returns
// the runs unchanged.
//
// Inline-code preservation follows the vocabulary editing constraints carried
// on each code (RunConstraints.Deletable, resolved from the span vocabulary
// when a run carries none of its own — see vocabulary.go). This mirrors the
// Okapi Framework, where a Code is deleteable or not and a translator may drop
// only the deleteable ones:
//
//   - A paired code (PcOpen/PcClose) is a span over the text. After the edit its
//     endpoints are remapped: if the span still covers text it is kept and stays
//     balanced; if it collapses to nothing it is removed when deletable (an
//     emptied bold span disappears rather than leaving an empty <b></b>) and
//     kept (empty) only when non-deletable.
//   - A standalone code (Ph/Sub) that falls strictly inside a replaced range is
//     removed when deletable and kept (at the range boundary) when not — so a
//     line break, a variable, or a subblock reference survives an edit that
//     deletes the text around it. A plural or a select is a standalone code
//     that is never deletable.
//   - Codes outside every edited range are shifted but otherwise untouched. Text
//     inside a span is editable and the span follows it; text replacing a span's
//     whole content keeps the span around the new text.
//
// Run flags survive. Text the edits leave alone keeps its TextRun.NoTranslate,
// and rebuilt text runs split where the flag changes. Replacement text is
// marked NoTranslate when every code point it replaces was; an insertion that
// replaces nothing is marked when the code points on both sides of it are.
func ApplyTextEdits(runs []Run, edits []TextEdit) []Run {
	if len(edits) == 0 {
		return runs
	}

	// The sequence's own text as code points, each with its run's flag, and
	// the inline-code runs with their code-point position and original index.
	type codeAt struct {
		pos    int
		runIdx int
		run    Run
	}
	var (
		old   []rune
		oldNT []bool
		codes []codeAt
	)
	for i, r := range runs {
		if r.Text == nil {
			codes = append(codes, codeAt{pos: len(old), runIdx: i, run: r})
			continue
		}
		for _, c := range r.Text.Text {
			old = append(old, c)
			oldNT = append(oldNT, r.Text.NoTranslate)
		}
	}

	// Reject overlapping or out-of-range edits, then build the edited text.
	cursor := 0
	for _, e := range edits {
		if e.Start < cursor || e.End < e.Start || e.End > len(old) {
			return runs
		}
		cursor = e.End
	}
	newText := make([]rune, 0, len(old))
	newNT := make([]bool, 0, len(old))
	cursor = 0
	for _, e := range edits {
		newText = append(newText, old[cursor:e.Start]...)
		newNT = append(newNT, oldNT[cursor:e.Start]...)
		flag := replacementNoTranslate(oldNT, e.Start, e.End)
		for _, c := range e.Replacement {
			newText = append(newText, c)
			newNT = append(newNT, flag)
		}
		cursor = e.End
	}
	newText = append(newText, old[cursor:]...)
	newNT = append(newNT, oldNT[cursor:]...)

	// A placement inserts a code into newText at newPos; seq keeps the original
	// document order stable when several codes land on one position.
	type placement struct {
		newPos int
		seq    int
		run    Run
	}
	var places []placement

	// Pair PcOpen with its PcClose by ID and resolve each as a span; anything
	// left over (placeholders, subs, structures, unbalanced halves) is handled
	// standalone.
	openAt := make(map[string]int, len(codes))
	paired := make([]bool, len(codes))
	for i, c := range codes {
		switch {
		case c.run.PcOpen != nil:
			openAt[c.run.PcOpen.ID] = i
		case c.run.PcClose != nil:
			oi, ok := openAt[c.run.PcClose.ID]
			if !ok {
				continue // unbalanced close: treat as standalone below
			}
			delete(openAt, c.run.PcClose.ID)
			paired[oi], paired[i] = true, true
			op := codes[oi]
			newOpen := mapEditedPos(edits, op.pos, biasRight)
			newClose := mapEditedPos(edits, c.pos, biasLeft)
			switch {
			case newClose > newOpen:
				// Span still covers text: keep both halves, balanced.
				places = append(places,
					placement{newOpen, op.runIdx, op.run},
					placement{newClose, c.runIdx, c.run})
			case !runDeletable(op.run):
				// Collapsed but must survive: keep an empty pair in place.
				places = append(places,
					placement{newOpen, op.runIdx, op.run},
					placement{newOpen, c.runIdx, c.run})
			}
			// Collapsed and deletable: drop both halves.
		}
	}

	// Standalone codes: placeholders, subs, structures, and any unbalanced pc
	// halves.
	for i, c := range codes {
		if paired[i] {
			continue
		}
		if editStrictlyContains(edits, c.pos) && runDeletable(c.run) {
			continue // sat in deleted text and may go with it
		}
		places = append(places, placement{mapEditedPos(edits, c.pos, biasLeft), c.runIdx, c.run})
	}

	sort.SliceStable(places, func(a, b int) bool {
		if places[a].newPos != places[b].newPos {
			return places[a].newPos < places[b].newPos
		}
		return places[a].seq < places[b].seq
	})

	out := make([]Run, 0, len(runs)+len(places))
	tc := 0
	for _, pl := range places {
		out = appendFlaggedText(out, newText[tc:pl.newPos], newNT[tc:pl.newPos])
		out = append(out, pl.run)
		tc = pl.newPos
	}
	out = appendFlaggedText(out, newText[tc:], newNT[tc:])
	return mergeAdjacentRuns(out)
}

// replacementNoTranslate reports whether text replacing [start, end) of a
// sequence whose code points carry the flags nt is itself marked NoTranslate:
// when every replaced code point was, or, for an insertion, when the code
// points on both sides of it are.
func replacementNoTranslate(nt []bool, start, end int) bool {
	if start < end {
		for _, f := range nt[start:end] {
			if !f {
				return false
			}
		}
		return true
	}
	return start > 0 && start < len(nt) && nt[start-1] && nt[start]
}

// appendFlaggedText appends text as text runs, starting a new run wherever the
// NoTranslate flag changes.
func appendFlaggedText(out []Run, text []rune, nt []bool) []Run {
	for i := 0; i < len(text); {
		j := i + 1
		for j < len(text) && nt[j] == nt[i] {
			j++
		}
		out = append(out, Run{Text: &TextRun{Text: string(text[i:j]), NoTranslate: nt[i]}})
		i = j
	}
	return out
}

// Position bias for a code that falls strictly inside a replaced range:
// biasLeft collapses it to the replacement's start, biasRight to its end. A
// span's open uses biasRight and its close biasLeft, so replacement text is not
// drawn into a span whose boundary the edit consumed, while an edit wholly
// inside a span keeps the span around the new text.
const (
	biasLeft = iota
	biasRight
)

// mapEditedPos maps a code-point position in the original text to the
// corresponding position in the edited text.
func mapEditedPos(edits []TextEdit, p, bias int) int {
	delta := 0
	for _, e := range edits {
		n := utf8.RuneCountInString(e.Replacement)
		if p >= e.End {
			delta += n - (e.End - e.Start)
			continue
		}
		if p <= e.Start {
			break
		}
		// e.Start < p < e.End: strictly inside a replaced range.
		newStart := e.Start + delta
		if bias == biasLeft {
			return newStart
		}
		return newStart + n
	}
	return p + delta
}

// editStrictlyContains reports whether p lies strictly inside some replaced
// range (boundary positions do not count).
func editStrictlyContains(edits []TextEdit, p int) bool {
	for _, e := range edits {
		if p > e.Start && p < e.End {
			return true
		}
	}
	return false
}

// runDeletable reports whether an inline-code run may be removed when the text
// it applies to is edited away. It reads the run's own RunConstraints when set,
// otherwise resolves the default from the vocabulary by semantic type. Sub runs
// reference a subblock and are never deletable, nor is a plural or a select;
// text runs are not codes.
func runDeletable(r Run) bool {
	switch {
	case r.Ph != nil:
		if r.Ph.Constraints != nil {
			return r.Ph.Constraints.Deletable
		}
		return vocabDeletable(r.Ph.Type)
	case r.PcOpen != nil:
		if r.PcOpen.Constraints != nil {
			return r.PcOpen.Constraints.Deletable
		}
		return vocabDeletable(r.PcOpen.Type)
	}
	return false
}

func vocabDeletable(typeName string) bool {
	if info := defaultEditVocab().LookupOrFallback(typeName); info != nil {
		return info.Constraints.Deletable
	}
	return false
}

// mergeAdjacentRuns coalesces consecutive text runs with the same NoTranslate
// flag (which an edit can produce where a replacement abuts untouched text)
// into one, allocating fresh TextRun values so the caller's input is never
// mutated in place.
func mergeAdjacentRuns(runs []Run) []Run {
	out := make([]Run, 0, len(runs))
	for _, r := range runs {
		if r.Text != nil && len(out) > 0 {
			if last := out[len(out)-1].Text; last != nil && last.NoTranslate == r.Text.NoTranslate {
				out[len(out)-1].Text = &TextRun{Text: last.Text + r.Text.Text, NoTranslate: last.NoTranslate}
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// defaultEditVocab is the process-wide default vocabulary used to resolve a
// code's editing constraints when a run carries none of its own. It is the
// shared DefaultVocabulary registry.
func defaultEditVocab() *VocabularyRegistry { return DefaultVocabulary() }
