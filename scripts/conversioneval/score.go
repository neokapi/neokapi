package main

import (
	"strings"
	"unicode"
)

// The metric is multiset word recall.
//
// Words rather than characters, because whitespace and line wrapping differ
// between every converter and none of those differences is content loss.
// A multiset rather than a set, because a converter that drops one of three
// identical paragraphs has lost content that set membership would not notice.
//
//	recall = Σ min(count_in_output, count_in_truth) / Σ count_in_truth
//
// Words shorter than two characters are dropped from both sides. A one-letter
// token matches something in almost any output, so counting them inflates every
// score toward each other and hides the differences the eval is for.
//
// Recall and not precision: a converter that emits extra text is doing
// something different from one that loses it, and this measures loss. Extra
// output shows up in the length ratio instead, where a reader can see it
// without it being scored as an error.

const minWordLen = 2

// words normalizes text to a comparable token list.
//
// Case is folded and punctuation is dropped, because "Blizzard." and "Blizzard"
// are the same word to every question this eval asks, and converters disagree
// about where a full stop attaches.
func words(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := fields[:0]
	for _, f := range fields {
		if len([]rune(f)) >= minWordLen {
			out = append(out, f)
		}
	}
	return out
}

// counts is a word multiset.
func counts(ws []string) map[string]int {
	m := make(map[string]int, len(ws))
	for _, w := range ws {
		m[w]++
	}
	return m
}

// Recall is the score, plus the pieces it was computed from so a reader can
// check the arithmetic rather than take it.
type Recall struct {
	TruthWords  int     `json:"truthWords"`
	OutputWords int     `json:"outputWords"`
	Matched     int     `json:"matched"`
	Recall      float64 `json:"recall"`
	// Missing is the words the output lost, most-frequent first and capped.
	// A score with no example of what it lost cannot be acted on.
	Missing []MissingWord `json:"missing,omitempty"`
}

// MissingWord is one word the output dropped, and how many times.
type MissingWord struct {
	Word  string `json:"word"`
	Times int    `json:"times"`
}

const maxMissingReported = 12

// score compares one converter's output against the document's own text.
func score(truth []string, output string) Recall {
	tw := words(strings.Join(truth, " "))
	ow := words(output)
	tc, oc := counts(tw), counts(ow)

	matched := 0
	var missing []MissingWord
	for w, n := range tc {
		got := min(oc[w], n)
		matched += got
		if lost := n - got; lost > 0 {
			missing = append(missing, MissingWord{Word: w, Times: lost})
		}
	}
	r := Recall{TruthWords: len(tw), OutputWords: len(ow), Matched: matched}
	if r.TruthWords > 0 {
		r.Recall = float64(matched) / float64(r.TruthWords)
	}
	r.Missing = topMissing(missing)
	return r
}

// topMissing sorts by how much was lost and caps the list. Ties break on the
// word so a rerun produces identical bytes.
func topMissing(m []MissingWord) []MissingWord {
	if len(m) == 0 {
		return nil
	}
	sortMissing(m)
	if len(m) > maxMissingReported {
		m = m[:maxMissingReported]
	}
	return m
}
