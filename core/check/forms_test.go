package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFindTermFormsMatchesEveryDeclaredShape.
//
// A rule names a word and prose uses that word in many shapes. This is the
// English case; the point of declaring rather than deriving is that the
// Norwegian one below works the same way, which no English suffix rule reaches.
func TestFindTermFormsMatchesEveryDeclaredShape(t *testing.T) {
	forms := []string{"utilize", "utilizes", "utilized", "utilizing"}
	text := "We utilize it, they utilized it, she is utilizing it, and it utilizes them."
	assert.Len(t, FindTermForms(text, forms), 4)
}

// TestFindTermFormsIsLanguageNeutral.
//
// The forms that made the generated approach untenable. Norwegian inflects a
// noun by suffixing the definite and plural articles, which no combination of
// English `-s/-es/-ed/-ing` produces, and a generated `løsninges` matches
// nothing at all.
func TestFindTermFormsIsLanguageNeutral(t *testing.T) {
	forms := []string{"løsning", "løsningen", "løsninger", "løsningene"}
	text := "en løsning, løsningen, flere løsninger, alle løsningene"
	assert.Len(t, FindTermForms(text, forms), 4)

	verb := []string{"utnytte", "utnytter", "utnyttet", "utnyttes"}
	assert.Len(t, FindTermForms("vi utnytter det, vi utnyttet det", verb), 2)
}

// TestFindTermFormsReportsAnOccurrenceOnce.
//
// Declared forms overlap by construction: "utilize" is inside "utilizes". A
// naive union reports the same occurrence under both, which would double-count
// a single violation and underline only part of the word.
func TestFindTermFormsReportsAnOccurrenceOnce(t *testing.T) {
	text := "the platform utilizes your data"
	hits := FindTermForms(text, []string{"utilize", "utilizes"})
	assert.Len(t, hits, 1)
	assert.Equal(t, "utilizes", text[hits[0][0]:hits[0][1]])
}

// TestFindTermFormsKeepsTheWordBoundary: declaring more shapes widens what is
// matched, it does not relax the whole-word rule. `Go` must still not match
// inside "going", which is what the generated forms broke.
func TestFindTermFormsKeepsTheWordBoundary(t *testing.T) {
	assert.Empty(t, FindTermForms("we are going home", []string{"Go"}))
	assert.Len(t, FindTermForms("written in Go today", []string{"Go"}), 1)
	assert.Empty(t, FindTermForms("the user signed in", []string{"use", "uses", "used"}))
}

// TestFindTermFormsOfOneIsFindTerm: a rule that declares no extra shapes costs
// nothing and behaves exactly as it always did.
func TestFindTermFormsOfOneIsFindTerm(t *testing.T) {
	text := "we utilize it"
	assert.Equal(t, FindTerm(text, "utilize"), FindTermForms(text, []string{"utilize"}))
	assert.Empty(t, FindTermForms("it utilizes data", []string{"utilize"}))
}
