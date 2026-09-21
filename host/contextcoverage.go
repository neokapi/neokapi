package host

import "fmt"

// What an answer with nothing in it has to say.
//
// Most projects meet kapi with no voice profile and no terms. The honest
// answer to "what applies here" is then a short one, and for a while it was so
// short that a caller read it as "there is nothing to do here" and carried on
// without the project's context for the rest of the session.
//
// So an answer states its own coverage, and a thin one says what is worth
// noticing while the work is done. It states no rule: there are none, and
// inventing one is the only outcome worse than saying nothing.

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

// coverageOf grades an answer by how many kinds of material stand behind it,
// and separately by whether anything unconfirmed does.
//
// kinds counts what is in force: for a location, the voice profile, the terms
// bound at the point, and the rules the project has confirmed and widened
// (C-11); for a query, the terms it matched and the prior wording it found.
// Two or more is `covered`, one is `thin`.
//
// candidates are proposals nobody has decided on. They hold nothing to
// anything, so they never make an answer `covered`. They do lift `empty` to
// `thin`, because a candidate is evidence that someone looked here: `empty`
// then means what it says, that nothing at all has been recorded.
//
// Three grades, because the grade is read by a model deciding how much to lean
// on the answer, and a finer scale would be a number nobody could act on
// differently.
func coverageOf(kinds int, candidates bool) ContextCoverage {
	switch {
	case kinds >= 2:
		return CoverageCovered
	case kinds == 1 || candidates:
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

// contextPointCoverageNote is the note a thin by-location answer carries.
// Empty for an answer with two kinds of material behind it.
//
// A candidate is named as a candidate. It holds nothing to anything until
// someone confirms it, and an answer that listed it beside the rules in force
// would be handing a writer a rule the project has not agreed to.
func contextPointCoverageNote(c ContextCoverage, candidates int) string {
	switch c {
	case CoverageEmpty:
		return "This project records nothing for this location yet. " + coverageAdvice
	case CoverageThin:
		if candidates > 0 {
			return fmt.Sprintf(
				"This project records little for this location, and %s here %s waiting to be confirmed or discarded (`kapi context log`). %s",
				plural(candidates, "candidate rule", "candidate rules"), verb(candidates, "is", "are"), coverageAdvice)
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

// contextSearchCoverageNote is the note a by-content answer carries when it
// found nothing at all.
//
// It fires on an empty answer and on no other, which is where the two
// primitives differ. A location graded thin is half-governed, and a writer
// there gains from knowing what to watch for. A search graded thin found the
// word and answered the question asked, so the same sentence would be a
// lecture delivered on a successful call. The grade is on the answer either
// way, for a caller that wants to branch on it.
func contextSearchCoverageNote(c ContextCoverage, query string) string {
	if c != CoverageEmpty {
		return ""
	}
	return "This project records nothing about " + query + " yet. " + coverageAdvice
}
