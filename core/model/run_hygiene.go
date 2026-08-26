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

// TypeInlineCode is the canonical vocabulary type of an inline code span —
// markdown backticks, HTML `<code>`/`<kbd>`/`<samp>`/`<tt>`, an AsciiDoc
// monospace pair, an OOXML code run. A paired run carrying it wraps content
// whose bytes are the point, so [RunsHygieneText] stands the whole span as one
// sentinel rather than exposing what is inside it to a prose rule.
const TypeInlineCode = "fmt:code"

// RunsHygieneText flattens a Run sequence for content-*shape* inspection: text
// runs verbatim, and every inline-code run (Ph / PcOpen / PcClose / Sub)
// collapsed to a single [ObjectReplacement] rune. A [TypeInlineCode] pair
// collapses together with everything between it, so a code span is one sentinel
// and its contents are not prose.
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
//	[pcOpen`][text "x  y"][pcClose`]  RunsText → "x  y"      → double space
//	                                  hygiene  → "￼"    → a code span, correctly
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

// RunsHaveContent reports whether a Run sequence holds content: text beyond
// whitespace, or an inline code. It is the one presence predicate — "is there
// anything here?" — that every gate asking it must call.
//
// Presence is a *shape* question, so it reads the [RunsHygieneText] flattening.
// [RunsText] drops inline code, which answers it wrongly for a block whose only
// run is a placeholder:
//
//	[ph{p.price}]   RunsText → ""    → "nothing here"
//	                hygiene  → "￼"   → a placeholder, which is content
//
// That wrong answer is not a cosmetic one wherever a gate keys off it: a
// placeholder-only unit reads as unauthored and untranslated, so it never
// reaches a rung of either ladder and can never ship; the same unit reads as
// missing to `kapi up`, which re-translates it every pass; and as unproduced to
// the guardrails, which then never check it.
//
// A rule that wants the string, not the verdict, reads [RunsHygieneText]
// directly; one that reports a range needs [NewHygieneView].
func RunsHaveContent(runs []Run) bool {
	return strings.TrimSpace(RunsHygieneText(runs)) != ""
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
	// sentinels holds one entry per inline-code sentinel in text, ascending by
	// offset. Empty for content with no inline code, which is the case where the
	// two coordinate spaces coincide.
	sentinels []hygieneSentinel
}

// hygieneSentinel records where one sentinel sits in the hygiene text and how
// much of the [RunsText] coordinate space the content it stands for occupies.
// A bare inline code contributes no text there (real = 0); a collapsed
// [TypeInlineCode] span contributes the text between its two halves, which is
// what makes the two coordinate spaces diverge by more than the sentinel's own
// width.
type hygieneSentinel struct {
	at   int
	real int
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
	for i := 0; i < len(runs); i++ {
		r := runs[i]
		switch r.Kind() {
		case RunKindText:
			buf.WriteString(r.Text.Text)
		case RunKindPcOpen:
			if end, ok := codeSpanEnd(runs, i); ok {
				// The span, its contents and its closing half stand as one
				// sentinel: what is inside a code span is bytes the author meant
				// literally, not prose a shape rule may judge.
				v.writeSentinel(buf, len(RunsText(runs[i+1:end])))
				i = end
				continue
			}
			v.writeSentinel(buf, 0)
		case RunKindPh, RunKindPcClose, RunKindSub:
			v.writeSentinel(buf, 0)
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

// writeSentinel appends one sentinel to the flattening and records how much of
// the [RunsText] coordinate space the content it stands for occupies.
func (v *HygieneView) writeSentinel(buf *strings.Builder, real int) {
	v.sentinels = append(v.sentinels, hygieneSentinel{at: buf.Len(), real: real})
	buf.WriteString(objectReplacement)
}

// codeSpanEnd reports the index of the closing half of the [TypeInlineCode] span
// opened at runs[open], when runs[open] opens one and the pair closes within the
// sequence. An unpaired opening half is not a span: it collapses on its own, the
// way any other inline code does, so an unbalanced sequence loses no text from
// the flattening.
func codeSpanEnd(runs []Run, open int) (int, bool) {
	r := runs[open]
	if r.PcOpen == nil || r.PcOpen.Type != TypeInlineCode {
		return 0, false
	}
	for i := open + 1; i < len(runs); i++ {
		if c := runs[i].PcClose; c != nil && c.ID == r.PcOpen.ID {
			return i, true
		}
	}
	return 0, false
}

// Text returns the flattening a shape rule reads.
func (v *HygieneView) Text() string { return v.text }

// Runs returns the Run sequence the view was built from.
func (v *HygieneView) Runs() []Run { return v.runs }

// RealOffset maps a byte offset into [HygieneView.Text] to the equivalent byte
// offset into [RunsText]. A sentinel occupies its own width in the hygiene text
// and whatever the content it stands for occupies in RunsText — nothing for a
// bare inline code, the enclosed text for a collapsed code span — so the mapping
// is a running correction by that difference for every sentinel passed on the
// way. An offset that lands *inside* a sentinel clamps to its leading edge: a
// code is atomic, so there is no position within it to point at. Offsets outside
// the text clamp to its ends.
func (v *HygieneView) RealOffset(off int) int {
	if off <= 0 {
		return 0
	}
	if off > len(v.text) {
		off = len(v.text)
	}
	delta := 0
	for _, s := range v.sentinels {
		if off <= s.at {
			break
		}
		if off < s.at+len(objectReplacement) {
			return s.at + delta
		}
		delta += s.real - len(objectReplacement)
	}
	return off + delta
}

// Range maps a half-open byte span of [HygieneView.Text] to the run-anchored
// range covering it, so a shape rule can report where it fired.
func (v *HygieneView) Range(start, end int) Anchor {
	return RangeAnchorForBytes(v.runs, v.RealOffset(start), v.RealOffset(end))
}
