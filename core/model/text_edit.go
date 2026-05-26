package model

import (
	"sort"
	"strings"
	"sync"
)

// TextEdit replaces the half-open byte range [Start,End) of a run sequence's
// flattened text (RunsText) with Replacement. Edits passed to ApplyTextEdits
// must be sorted by Start and non-overlapping.
type TextEdit struct {
	Start       int
	End         int
	Replacement string
}

// HasStructuredRuns reports whether a run sequence contains plural or select
// runs. Their flattened text comes from nested forms, so a byte offset into the
// flattening does not map back to a single position — ApplyTextEdits supports
// only flat sequences, and callers should guard with this.
func HasStructuredRuns(runs []Run) bool {
	for _, r := range runs {
		if r.Plural != nil || r.Select != nil {
			return true
		}
	}
	return false
}

// ApplyTextEdits rewrites a flat run sequence by applying byte-range edits to
// its flattened text, then repositioning the inline codes that survive. Edits
// must be sorted by Start and be non-overlapping; malformed input returns the
// runs unchanged.
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
//     deletes the text around it.
//   - Codes outside every edited range are shifted but otherwise untouched. Text
//     inside a span is editable and the span follows it; text replacing a span's
//     whole content keeps the span around the new text.
//
// Only flat sequences are supported (no plural/select); guard with
// HasStructuredRuns.
func ApplyTextEdits(runs []Run, edits []TextEdit) []Run {
	if len(edits) == 0 {
		return runs
	}
	text := RunsText(runs)

	// Build the edited text, rejecting overlapping or out-of-range edits.
	var nb strings.Builder
	cursor := 0
	for _, e := range edits {
		if e.Start < cursor || e.End < e.Start || e.End > len(text) {
			return runs
		}
		nb.WriteString(text[cursor:e.Start])
		nb.WriteString(e.Replacement)
		cursor = e.End
	}
	nb.WriteString(text[cursor:])
	newText := nb.String()

	// Gather inline-code runs with their byte position and original index.
	type codeAt struct {
		pos    int
		runIdx int
		run    Run
	}
	var codes []codeAt
	pos := 0
	for i, r := range runs {
		if r.Text != nil {
			pos += len(r.Text.Text)
			continue
		}
		codes = append(codes, codeAt{pos: pos, runIdx: i, run: r})
	}

	// A placement inserts a code into newText at newPos; seq keeps the original
	// document order stable when several codes land on one position.
	type placement struct {
		newPos int
		seq    int
		run    Run
	}
	var places []placement

	// Pair PcOpen with its PcClose by ID and resolve each as a span; anything
	// left over (placeholders, subs, unbalanced halves) is handled standalone.
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

	// Standalone codes: placeholders, subs, and any unbalanced pc halves.
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
		if pl.newPos > tc {
			out = append(out, Run{Text: &TextRun{Text: newText[tc:pl.newPos]}})
		}
		out = append(out, pl.run)
		tc = pl.newPos
	}
	if tc < len(newText) {
		out = append(out, Run{Text: &TextRun{Text: newText[tc:]}})
	}
	return mergeAdjacentRuns(out)
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

// mapEditedPos maps a byte position in the original flattened text to the
// corresponding position in the edited text.
func mapEditedPos(edits []TextEdit, p, bias int) int {
	delta := 0
	for _, e := range edits {
		if p >= e.End {
			delta += len(e.Replacement) - (e.End - e.Start)
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
		return newStart + len(e.Replacement)
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
// reference a subblock and are never deletable; text runs are not codes.
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

// mergeAdjacentRuns coalesces consecutive text runs (which an edit can produce
// where a replacement abuts untouched text) into one, allocating fresh TextRun
// values so the caller's input is never mutated in place.
func mergeAdjacentRuns(runs []Run) []Run {
	out := make([]Run, 0, len(runs))
	for _, r := range runs {
		if r.Text != nil && len(out) > 0 && out[len(out)-1].Text != nil {
			out[len(out)-1].Text = &TextRun{Text: out[len(out)-1].Text.Text + r.Text.Text}
			continue
		}
		out = append(out, r)
	}
	return out
}

// defaultEditVocab is the process-wide default vocabulary used to resolve a
// code's editing constraints when a run carries none of its own. Loaded once.
var defaultEditVocab = sync.OnceValue(func() *VocabularyRegistry {
	r := NewVocabularyRegistry()
	_ = r.LoadDefaults()
	return r
})
