package model

import "strings"

// ObjectReplacement is the sentinel rune standing in for one inline-code run in
// [RunsHygieneText]: U+FFFC OBJECT REPLACEMENT CHARACTER, whose Unicode purpose
// is exactly this — "a character that stands in for an object embedded in
// text". It is non-whitespace, non-control, and belongs to no word, so it reads
// as content to every shape predicate without being mistaken for one.
const ObjectReplacement = '￼'

// objectReplacement is [ObjectReplacement] as a string. Its byte width is the
// per-inline-code cost [HygieneView.RealOffset] subtracts to get back to
// [RunsText] coordinates, where a code contributes nothing.
const objectReplacement = string(ObjectReplacement)

// RunsHygieneText flattens a Run sequence for content-*shape* inspection: text
// runs verbatim, and every inline-code run (Ph / PcOpen / PcClose / Sub)
// collapsed to a single [ObjectReplacement] rune.
//
// This is the flattening any whitespace, adjacency, or emptiness rule must use.
// [RunsText] drops inline code, which silently changes the shape of the content
// and produces false positives:
//
//	[ph{p.price}][text " each"]   RunsText → " each"        → leading whitespace
//	                              hygiene  → "￼ each"  → a separator, correctly
//
//	[text "Hello "][ph][text " world"]  RunsText → "Hello  world" → double space
//	                                    hygiene  → "Hello ￼ world" → correct
//
//	[ph{p.price}]                 RunsText → ""             → "content is empty"
//	                              hygiene  → "￼"       → a placeholder, not empty
//
// A placeholder is content: it occupies a position, and the space beside it is a
// legitimate separator rather than stray whitespace on the block's boundary. The
// sentinel keeps that position visible while contributing no text of its own, so
// a placeholder's *equivalent* text can never leak into a word- or
// character-level judgement the way FlattenRuns' `{equiv}` rendering would.
//
// A rule that reports a *range* rather than a yes/no needs [NewHygieneView],
// which carries the same flattening plus the mapping back to run-anchored spans.
func RunsHygieneText(runs []Run) string {
	return NewHygieneView(runs).Text()
}

// HygieneView is the run-aware view of a Run sequence that content-shape rules
// inspect: the [RunsHygieneText] flattening, plus the mapping from offsets in it
// back to [RunsText] coordinates — the space every stand-off overlay, content
// hash and TM key is anchored in.
//
// Both halves are needed together. A rule that only answers yes/no can read the
// flattened string on its own, but one that reports *where* (a QA overlay
// highlighting the offending characters) must map its offsets back, because the
// sentinel occupies bytes the anchor coordinate space does not have. Reading the
// shape from one string and anchoring to another is how a range ends up
// silently shifted by the width of every preceding placeholder.
type HygieneView struct {
	runs []Run
	text string
	// sentinels holds the byte offset in text at which each inline-code
	// sentinel starts, ascending. Empty for content with no inline code, which
	// is the case where the two coordinate spaces coincide.
	sentinels []int
}

// NewHygieneView builds the hygiene view of a Run sequence. It is the single
// walk behind [RunsHygieneText], so the string and the mapping cannot drift.
func NewHygieneView(runs []Run) *HygieneView {
	v := &HygieneView{runs: runs}
	var buf strings.Builder
	v.flatten(&buf, runs)
	v.text = buf.String()
	return v
}

func (v *HygieneView) flatten(buf *strings.Builder, runs []Run) {
	for _, r := range runs {
		switch r.Kind() {
		case RunKindText:
			buf.WriteString(r.Text.Text)
		case RunKindPh, RunKindPcOpen, RunKindPcClose, RunKindSub:
			v.sentinels = append(v.sentinels, buf.Len())
			buf.WriteString(objectReplacement)
		case RunKindPlural:
			if form, ok := r.Plural.Forms[PluralOther]; ok {
				v.flatten(buf, form)
				continue
			}
			for _, form := range r.Plural.Forms {
				v.flatten(buf, form)
				break
			}
		case RunKindSelect:
			if form, ok := r.Select.Cases["other"]; ok {
				v.flatten(buf, form)
				continue
			}
			for _, form := range r.Select.Cases {
				v.flatten(buf, form)
				break
			}
		}
	}
}

// Text returns the flattening a shape rule reads.
func (v *HygieneView) Text() string { return v.text }

// Runs returns the Run sequence the view was built from.
func (v *HygieneView) Runs() []Run { return v.runs }

// RealOffset maps a byte offset into [HygieneView.Text] to the equivalent byte
// offset into [RunsText]. Each inline-code run contributes one sentinel rune to
// the hygiene text and nothing to RunsText, so the mapping is a running
// subtraction of the sentinel bytes passed on the way. An offset that lands
// *inside* a sentinel clamps to that code's leading edge — a code is atomic, so
// there is no position within it to point at. Offsets outside the text clamp to
// its ends.
func (v *HygieneView) RealOffset(off int) int {
	if off <= 0 {
		return 0
	}
	if off > len(v.text) {
		off = len(v.text)
	}
	dropped := 0
	for _, s := range v.sentinels {
		if off <= s {
			break
		}
		if off < s+len(objectReplacement) {
			return s - dropped
		}
		dropped += len(objectReplacement)
	}
	return off - dropped
}

// Range maps a half-open byte span of [HygieneView.Text] to the run-anchored
// range covering it, so a shape rule can report where it fired.
func (v *HygieneView) Range(start, end int) RunRange {
	return RunRangeForBytes(v.runs, v.RealOffset(start), v.RealOffset(end))
}
