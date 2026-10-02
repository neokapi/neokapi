package model

// A run sequence rebuilt from text, by ParseRunsPlaceholderText or
// ParseRunsEditText, gets its inline codes back from the reference runs it was
// read against. carryNoTranslate gives it back the other thing the text form
// leaves out: which text was marked TextRun.NoTranslate.
//
// The text between two codes is a segment, named by the code that opens it.
// Each segment of the rebuilt runs is aligned with the reference segment of the
// same name, code point by code point (alignRunes). A code point that matches
// one in the reference takes its flag; text in between takes the flag
// ApplyTextEdits gives a replacement (replacementNoTranslate): set when every
// code point it replaced was, or, for an insertion, when both neighbours are.

// segmentKey names a segment by the code run that precedes it and how many
// times that code has occurred so far. The first segment, before any code, has
// the zero key.
type segmentKey struct {
	kind RunKind
	id   string
	nth  int
}

// textSegment is the text of the consecutive text runs after one code.
type textSegment struct {
	runes []rune
	flags []bool
}

// hasNoTranslate reports whether any top-level text run is marked.
func hasNoTranslate(runs []Run) bool {
	for _, r := range runs {
		if r.Text != nil && r.Text.NoTranslate {
			return true
		}
	}
	return false
}

// segmentsOf splits runs into their top-level text segments, keyed by the code
// that opens each.
func segmentsOf(runs []Run) map[segmentKey]*textSegment {
	out := map[segmentKey]*textSegment{}
	seen := map[segmentKey]int{}
	key := segmentKey{}
	for _, r := range runs {
		if r.Text == nil {
			k := segmentKey{kind: r.Kind(), id: r.RunID()}
			key = segmentKey{kind: k.kind, id: k.id, nth: seen[k]}
			seen[k]++
			continue
		}
		seg := out[key]
		if seg == nil {
			seg = &textSegment{}
			out[key] = seg
		}
		for _, c := range r.Text.Text {
			seg.runes = append(seg.runes, c)
			seg.flags = append(seg.flags, r.Text.NoTranslate)
		}
	}
	return out
}

// carryNoTranslate returns runs with each top-level text run marked
// NoTranslate where the reference marked the text it matches, split where the
// flag changes. Runs whose reference marks nothing are returned unchanged.
func carryNoTranslate(runs, ref []Run) []Run {
	if !hasNoTranslate(ref) {
		return runs
	}
	refSegs := segmentsOf(ref)
	out := make([]Run, 0, len(runs))
	seen := map[segmentKey]int{}
	key := segmentKey{}
	for i := 0; i < len(runs); {
		r := runs[i]
		if r.Text == nil {
			k := segmentKey{kind: r.Kind(), id: r.RunID()}
			key = segmentKey{kind: k.kind, id: k.id, nth: seen[k]}
			seen[k]++
			out = append(out, r)
			i++
			continue
		}
		j := i
		var text []rune
		for j < len(runs) && runs[j].Text != nil {
			text = append(text, []rune(runs[j].Text.Text)...)
			j++
		}
		flags := make([]bool, len(text))
		if seg := refSegs[key]; seg != nil {
			flags = alignedFlags(seg.runes, seg.flags, text)
		}
		out = appendFlaggedText(out, text, flags)
		i = j
	}
	return out
}

// alignedFlags returns the NoTranslate flag of each code point of text, which
// was rebuilt from old whose code points carry oldFlags.
func alignedFlags(old []rune, oldFlags []bool, text []rune) []bool {
	flags := make([]bool, len(text))
	match := alignRunes(old, text)
	prevOld, prevNew := -1, -1
	fill := func(oldEnd, newEnd int) {
		if prevNew+1 >= newEnd {
			return
		}
		f := replacementNoTranslate(oldFlags, prevOld+1, oldEnd)
		for k := prevNew + 1; k < newEnd; k++ {
			flags[k] = f
		}
	}
	for i, j := range match {
		if j < 0 {
			continue
		}
		fill(i, j)
		flags[j] = oldFlags[i]
		prevOld, prevNew = i, j
	}
	fill(len(old), len(text))
	return flags
}
