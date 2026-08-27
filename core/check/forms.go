package check

import "sort"

// A rule names a word, and prose uses that word in many shapes. Matching only
// the exact string meant a profile forbidding `utilize` passed "the platform
// utilizes your data", and one forbidding `løsning` passed "løsningen",
// "løsninger" and "løsningene" — every form that actually occurs.
//
// The shapes are declared, not guessed. An earlier attempt generated them from
// English suffix rules, which worked for English, produced non-words for
// Norwegian ("løsninges", "utnytted"), reached none of the forms that language
// actually uses, and needed a minimum term length to stop `Go` matching inside
// "going". Morphology is per-language knowledge, and the tools that do it
// properly — LanguageTool, Acrolinx — carry a linguistic pack per language.
//
// So the forms come from the profile, filled in at authoring time by
// `kapi voice expand`, which asks a model for them once in the profile's own
// language and writes them into a diff a person reviews. The knowledge is the
// model's; the matching stays exact, free, deterministic and language-neutral.
// See issue #2226.

// FindTermForms returns the byte ranges where any of a rule's surface forms
// occurs in text.
//
// Longest match wins where forms overlap, so a rule declaring both "utilize"
// and "utilizes" reports "utilizes" once as the eight-character hit rather than
// twice.
func FindTermForms(text string, forms []string) [][2]int {
	if len(forms) == 1 {
		return FindTerm(text, forms[0])
	}
	var all [][2]int
	for _, f := range forms {
		all = append(all, FindTerm(text, f)...)
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
