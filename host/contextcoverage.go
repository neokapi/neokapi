package host

import "fmt"

// Context coverage describes the evidence returned by a lookup. Results with
// little or no context include advice on what to observe while working, without
// presenting unrecorded guidance as established rules.

// ContextCoverage says how much of a project's context stood behind one
// answer. It describes the answer rather than the project: a search that found
// nothing reports an empty answer whatever else the project holds.
type ContextCoverage string

const (
	// CoverageEmpty: neither kind of material stood behind the answer.
	CoverageEmpty ContextCoverage = "empty"
	// CoverageThin: one of the two kinds did.
	CoverageThin ContextCoverage = "thin"
	// CoverageCovered: both did.
	CoverageCovered ContextCoverage = "covered"
)

// coverageOf classifies the kinds of established material in a result.
// Location results count voice profiles, terms and established rules; search
// results count matching terms and prior wording. Two or more kinds are covered,
// one is thin, and none is empty.
//
// Pending suggestions raise empty to thin but never count as established material
// for covered status.
func coverageOf(kinds int, suggestions bool) ContextCoverage {
	switch {
	case kinds >= 2:
		return CoverageCovered
	case kinds == 1 || suggestions:
		return CoverageThin
	default:
		return CoverageEmpty
	}
}

// countKinds counts the kinds of material present.
func countKinds(present ...bool) int {
	n := 0
	for _, p := range present {
		if p {
			n++
		}
	}
	return n
}

// coverageAdvice is the second half of a thin answer: what to notice while
// working, so the next person to ask gets a better answer than this one.
const coverageAdvice = "Notice as you work what this project calls its own things, " +
	"which spellings it keeps to, who the text addresses, and how formal it is."

// contextPointCoverageNote returns advice for empty or thin location results.
// Pending suggestions are identified separately from established rules.
func contextPointCoverageNote(c ContextCoverage, suggestions int) string {
	switch c {
	case CoverageEmpty:
		return "This project records nothing for this location yet. " + coverageAdvice
	case CoverageThin:
		if suggestions > 0 {
			return fmt.Sprintf(
				"This project records little for this location, and %s here %s waiting to be kept or dropped (`kapi context log`). %s",
				plural(suggestions, "suggested rule", "suggested rules"), verb(suggestions, "is", "are"), coverageAdvice)
		}
		return "This project records little for this location. " + coverageAdvice
	default:
		return ""
	}
}

// plural renders a count with the noun that agrees with it.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// verb picks the verb form that agrees with a count.
func verb(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// contextSearchCoverageNote returns advice only for an empty search result.
// A thin search already found relevant material, whereas a thin location result
// can still benefit from guidance on missing context.
func contextSearchCoverageNote(c ContextCoverage, query string) string {
	if c != CoverageEmpty {
		return ""
	}
	return "This project records nothing about " + query + " yet. " + coverageAdvice
}
