package host

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

// coverageOf grades an answer by how many of its two kinds of material it
// carries. The by-location answer counts the voice profile in force and the
// terms bound at the point; the by-content answer counts the terms it matched
// and the prior wording it found.
//
// Two kinds and three grades, because the grade is read by a model deciding
// how much to lean on the answer, and a finer scale would be a number nobody
// could act on differently.
func coverageOf(first, second bool) ContextCoverage {
	switch {
	case first && second:
		return CoverageCovered
	case first || second:
		return CoverageThin
	default:
		return CoverageEmpty
	}
}

// coverageAdvice is the second half of a thin answer: what to notice while
// working, so the next person to ask gets a better answer than this one.
const coverageAdvice = "Notice as you work what this project calls its own things, " +
	"which spellings it keeps to, who the text addresses, and how formal it is."

// contextPointCoverageNote is the note a thin by-location answer carries.
// Empty for an answer with both kinds of material behind it.
func contextPointCoverageNote(c ContextCoverage) string {
	switch c {
	case CoverageEmpty:
		return "This project records nothing for this location yet. " + coverageAdvice
	case CoverageThin:
		return "This project records little for this location. " + coverageAdvice
	default:
		return ""
	}
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
