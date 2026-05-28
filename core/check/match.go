package check

import (
	"strings"
	"unicode"
)

// FindTerm returns the byte ranges [start,end) where term occurs in text as a
// whole word: case-insensitive, with Unicode word boundaries so "use" does not
// match inside "user". A term that begins or ends with a non-word rune relaxes
// the corresponding boundary, so "C++" or "{count}" still match. Reported ranges
// index the original (not lower-cased) bytes of text. An empty term returns nil.
//
// This replaces naïve strings.Index/Contains substring matching, the source of
// false positives like flagging "use" inside "user". Morphology-aware matching
// (stems, inflections) is layered on top by the small-model checkers.
func FindTerm(text, term string) [][2]int {
	if term == "" {
		return nil
	}
	want := []rune(strings.ToLower(term))

	// Lower-case the haystack rune-by-rune while recording each rune's byte
	// offset in the ORIGINAL text, so reported ranges index the original bytes.
	runes := make([]rune, 0, len(text))
	byteAt := make([]int, 0, len(text)+1)
	for i, r := range text {
		runes = append(runes, unicode.ToLower(r))
		byteAt = append(byteAt, i)
	}
	byteAt = append(byteAt, len(text))

	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }
	headIsWord := isWord(want[0])
	tailIsWord := isWord(want[len(want)-1])

	var out [][2]int
	for i := 0; i+len(want) <= len(runes); i++ {
		matched := true
		for j, wr := range want {
			if runes[i+j] != wr {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		if i > 0 && headIsWord && isWord(runes[i-1]) {
			continue // left boundary fails: word rune precedes a word-initial term
		}
		end := i + len(want)
		if end < len(runes) && tailIsWord && isWord(runes[end]) {
			continue // right boundary fails
		}
		out = append(out, [2]int{byteAt[i], byteAt[end]})
	}
	return out
}

// ContainsTerm reports whether term occurs in text as a whole word (see
// FindTerm).
func ContainsTerm(text, term string) bool {
	return len(FindTerm(text, term)) > 0
}
