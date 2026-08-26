package tools

import (
	"slices"
	"strings"
)

// The classifier's own view of an edit, rendered so it can be read.
//
// A verdict on its own is a number by another name: "substantive" is no more
// inspectable than 95%. What makes it checkable is seeing WHICH words moved and
// which characters were ignored, because that is the entire claim. Punctuation,
// case and quote shape are shown and do not count; a word appearing, vanishing
// or changing is what tips a verdict.
//
// So this shares the classifier's tokenizer rather than approximating it. A
// diff drawn by a second, similar-looking splitter would eventually disagree
// with the verdict beside it, and a reader would have no way to tell which one
// was lying.

// SpanOp is what happened to one piece of text between the two sources.
type SpanOp string

const (
	// SpanSame: present in both, character for character.
	SpanSame SpanOp = "same"
	// SpanCosmetic: present in both and written differently, or punctuation
	// that appeared or vanished. Shown, and counts for nothing.
	SpanCosmetic SpanOp = "cosmetic"
	// SpanAdded: a word in the current source with no counterpart in the prior.
	SpanAdded SpanOp = "added"
	// SpanRemoved: a word in the prior source with no counterpart in the current.
	SpanRemoved SpanOp = "removed"
)

// Span is a run of text and what became of it.
type Span struct {
	Text string `json:"text"`
	Op   SpanOp `json:"op"`
}

// Diff is one edit shown from both sides, plus the verdict it produces.
//
// Prior carries same, cosmetic and removed spans; Current carries same,
// cosmetic and added. Concatenating either side's Text reproduces that source
// exactly, so a rendering cannot quietly drop content.
type Diff struct {
	Kind    EditKind `json:"kind"`
	Prior   []Span   `json:"prior"`
	Current []Span   `json:"current"`
}

// DiffEdit aligns two sources word by word and reports what changed.
//
// The alignment is a longest common subsequence over the comparable word forms,
// with any separator matching any other, so punctuation that moved lines up
// with the punctuation it replaced instead of showing as a word going missing.
//
// Kind agrees with ClassifyEdit by construction: an alignment leaves a word
// unmatched exactly when the two word sequences differ.
func DiffEdit(prior, current string) Diff {
	pt, ct := tokenize(prior), tokenize(current)
	matched := lcsPairs(pt, ct)

	d := Diff{Kind: ClassifyEdit(prior, current)}
	priorMatch := make(map[int]int, len(matched))
	currentMatch := make(map[int]int, len(matched))
	for _, m := range matched {
		priorMatch[m[0]] = m[1]
		currentMatch[m[1]] = m[0]
	}

	d.Prior = spansFor(pt, ct, priorMatch, SpanRemoved)
	d.Current = spansFor(ct, pt, currentMatch, SpanAdded)
	return d
}

// spansFor renders one side of an alignment. unmatched is what an unpaired word
// on this side is called: removed when this side is the prior, added when it is
// the current.
func spansFor(side, other []token, match map[int]int, unmatched SpanOp) []Span {
	out := make([]Span, 0, len(side))
	for i, tk := range side {
		j, paired := match[i]
		switch {
		case paired && tk.raw == other[j].raw:
			out = append(out, Span{Text: tk.raw, Op: SpanSame})
		case paired:
			out = append(out, Span{Text: tk.raw, Op: SpanCosmetic})
		case tk.word:
			out = append(out, Span{Text: tk.raw, Op: unmatched})
		case strings.TrimSpace(tk.raw) == "":
			// The space an inserted word arrived with. Marking it would draw a
			// second highlight for one edit, in a place a reader cannot see
			// anything happening.
			out = append(out, Span{Text: tk.raw, Op: SpanSame})
		default:
			// Punctuation that appeared or vanished, which is exactly what the
			// classifier declines to count.
			out = append(out, Span{Text: tk.raw, Op: SpanCosmetic})
		}
	}
	return out
}

// ContainsWords reports whether needle appears in haystack as whole words.
//
// It is Contains with the word boundary the rest of this file is built on, and
// it exists because the substring version is wrong in the way that matters: the
// Norwegian for a shopping basket is "kurven", the word a model reaches for
// instead is "handlekurven", and one contains the other. A consistency check
// written with strings.Contains reports the drift as a match and passes.
//
// Comparison is the classifier's: case-folded, NFC, intra-word hyphens and
// apostrophes ignored. A multi-word needle must appear as a contiguous run.
func ContainsWords(haystack, needle string) bool {
	want := words(needle)
	if len(want) == 0 {
		return false
	}
	have := words(haystack)
	for i := 0; i+len(want) <= len(have); i++ {
		if slices.Equal(have[i:i+len(want)], want) {
			return true
		}
	}
	return false
}

// lcsPairs returns index pairs of a longest common subsequence, comparing
// tokens by their normalized key.
//
// The inputs are one block of content, so the quadratic table is bounded by
// what a person writes in a paragraph.
func lcsPairs(a, b []token) [][2]int {
	n, m := len(a), len(b)
	table := make([][]int, n+1)
	for i := range table {
		table[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i].key == b[j].key {
				table[i][j] = table[i+1][j+1] + 1
				continue
			}
			table[i][j] = max(table[i+1][j], table[i][j+1])
		}
	}
	var out [][2]int
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case a[i].key == b[j].key:
			out = append(out, [2]int{i, j})
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			i++
		default:
			j++
		}
	}
	return out
}
