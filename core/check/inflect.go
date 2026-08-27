package check

import (
	"sort"
	"strings"
)

// A forbidden term is a word, and prose uses words inflected.
//
// Matching only the exact string meant a profile forbidding `utilize` passed
// "the platform utilizes your data": the matcher requires a word boundary after
// the term, and the `s` is not one. The words a voice profile actually forbids
// are overwhelmingly verbs — utilize, leverage, enable, empower, streamline —
// and prose reaches for them inflected at least as often as bare. See #2226.
//
// The failure direction was the bad one. The check reported nothing and scored
// 100/100, which reads as "on brand".

// inflectionSuffixes are the regular endings appended to a consonant-final stem.
//
// `-d` alone is deliberately absent. It would turn a two-letter term into a real
// and unrelated word: `ad` + `d` is "add", and a rule about advertising would
// start flagging arithmetic. An e-final term reaches its `-ed` form through the
// e-drop below instead, which is where English puts it anyway.
var inflectionSuffixes = []string{"s", "es", "ed", "ing"}

// eDropSuffixes are the endings that replace a term's final `e`.
var eDropSuffixes = []string{"ed", "ing", "es"}

// minInflectedLen is the shortest term that gets inflected.
//
// A short stem plus a suffix is usually a different word, not an inflection of
// this one. `Go` is the case that proves it: a rule about the language would
// start matching "going", which the whole-word rule exists to prevent and which
// core/profile's own test asserts against. Two more that would have followed:
// `ad` + s + ed, `bid` + ing → "biding".
//
// Four covers every word a voice profile actually forbids — utilize, leverage,
// seamless, empower, streamline, revolutionary — and excludes the range where a
// generated form is more likely to be a different word than an inflection.
const minInflectedLen = 4

// Inflections returns term and its regular English inflections, deduplicated
// and in a stable order.
//
// Regular only. Consonant doubling (ship → shipping) and irregular forms
// (run → ran) are not generated: guessing them costs false positives on short
// terms, and a term whose inflection this misses is no worse off than it was
// before inflection existed. A rule that needs the exact string says so with
// `exact: true`.
//
// Some generated forms are not words (ship → "shipes"). That is harmless: a
// non-word matches nothing, and filtering them would need a dictionary this
// package has no business carrying.
func Inflections(term string) []string {
	base := strings.TrimSpace(term)
	if base == "" {
		return nil
	}
	forms := []string{base}

	// A term ending in a word character is the only kind a suffix can extend.
	// "C++" and "{count}" take their own edge rule in the matcher and gain
	// nothing from an appended "s".
	last := rune(base[len(base)-1])
	if !wordRune(last) {
		return forms
	}
	if len([]rune(base)) < minInflectedLen {
		return forms
	}

	if stem, cut := strings.CutSuffix(base, "e"); cut {
		forms = append(forms, base+"s")
		for _, suf := range eDropSuffixes {
			forms = append(forms, stem+suf)
		}
	} else {
		for _, suf := range inflectionSuffixes {
			forms = append(forms, base+suf)
		}
	}
	return dedupe(forms)
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// FindTermInflected returns the byte ranges where term or one of its regular
// inflections occurs in text.
//
// Longest match wins where forms overlap, so "utilizes" is reported once as the
// eight-character hit rather than twice — once for "utilize" and again for the
// form that contains it.
func FindTermInflected(text, term string) [][2]int {
	var all [][2]int
	for _, form := range Inflections(term) {
		all = append(all, FindTerm(text, form)...)
	}
	return longestNonOverlapping(all)
}

// longestNonOverlapping keeps the longest hit at each position and drops any
// that overlaps one already kept.
func longestNonOverlapping(hits [][2]int) [][2]int {
	if len(hits) < 2 {
		return hits
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i][0] != hits[j][0] {
			return hits[i][0] < hits[j][0]
		}
		return hits[i][1]-hits[i][0] > hits[j][1]-hits[j][0]
	})
	out := hits[:0]
	end := -1
	for _, h := range hits {
		if h[0] < end {
			continue
		}
		out = append(out, h)
		end = h[1]
	}
	return out
}
