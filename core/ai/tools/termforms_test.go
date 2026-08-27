package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The filter is what makes a model's answer safe to put in a gate. A wrong form
// accuses text that broke no rule, and the author never wrote it down
// themselves, so every check below drops rather than keeps when unsure.

func TestFilterKeepsRealInflections(t *testing.T) {
	got := filterExpansions(
		[]string{"utilize", "løsning"},
		[]TermExpansion{
			{Term: "utilize", Forms: []string{"utilizes", "utilized", "utilizing"}},
			{Term: "løsning", Forms: []string{"løsningen", "løsninger", "løsningene"}},
		})
	assert.Len(t, got, 2)
	assert.Equal(t, []string{"utilizes", "utilized", "utilizing"}, got[0].Forms)
	assert.Equal(t, []string{"løsningen", "løsninger", "løsningene"}, got[1].Forms)
}

// TestFilterDropsSynonyms is the check that earns its place. Asked for forms of
// "leverage", a model will offer "harness" and "exploit", which are other words
// with other meanings. Matching them would flag prose the profile permits.
func TestFilterDropsSynonyms(t *testing.T) {
	got := filterExpansions([]string{"leverage"},
		[]TermExpansion{{Term: "leverage", Forms: []string{"leverages", "leveraged", "harness", "exploit", "utilise"}}})
	assert.Equal(t, []string{"leverages", "leveraged"}, got[0].Forms)

	reasons := map[string]string{}
	for _, r := range got[0].Rejected {
		reasons[r.Form] = r.Reason
	}
	assert.Equal(t, "not a form of the term", reasons["harness"])
	assert.Equal(t, "not a form of the term", reasons["exploit"])
	assert.Equal(t, "not a form of the term", reasons["utilise"], "a different spelling is a different rule")
}

// TestFilterDropsTheTermAndDuplicates: the term is matched already, and a
// duplicate form would report one occurrence twice.
func TestFilterDropsTheTermAndDuplicates(t *testing.T) {
	got := filterExpansions([]string{"utilize"},
		[]TermExpansion{{Term: "utilize", Forms: []string{"utilize", "utilizes", "utilizes", "Utilizes"}}})
	assert.Equal(t, []string{"utilizes"}, got[0].Forms)
}

// TestFilterDropsUnaskedTerms: a model that volunteers a rule the profile never
// declared would be writing policy. Only what was asked comes back.
func TestFilterDropsUnaskedTerms(t *testing.T) {
	got := filterExpansions([]string{"utilize"},
		[]TermExpansion{
			{Term: "utilize", Forms: []string{"utilizes"}},
			{Term: "synergy", Forms: []string{"synergies"}},
		})
	assert.Len(t, got, 1)
	assert.Equal(t, "utilize", got[0].Term)
}

// TestFilterDeclinesShortTerms.
//
// `Go` is the case that sank generated inflections: core/profile asserts a rule
// about the language does not match inside "going". No prefix rule helps here,
// because for `Go` it admits "gone" (a real form of the verb) and "Golang" (a
// different word) alike.
//
// So a term of one or two characters gets nothing. In a voice profile such a
// term is almost always a product or a technology, where every inflection of
// the ordinary word sharing its spelling is a false accusation, and an author
// who does mean the verb can write the forms in one line.
func TestFilterDeclinesShortTerms(t *testing.T) {
	got := filterExpansions([]string{"Go"},
		[]TermExpansion{{Term: "Go", Forms: []string{"gone", "went", "Golang"}}})
	assert.Empty(t, got[0].Forms)
	assert.Len(t, got[0].Rejected, 3, "and says so, rather than returning a quietly empty list")
}

// TestFilterHonoursTheCap: a long tail of rare forms costs precision at check
// time for violations nobody writes.
func TestFilterHonoursTheCap(t *testing.T) {
	many := []string{"runs", "running", "runner", "runners", "runny", "runnable", "runtime", "runway", "rundown"}
	got := filterExpansions([]string{"run"}, []TermExpansion{{Term: "run", Forms: many}})
	assert.Len(t, got[0].Forms, MaxFormsPerTerm)
}

// TestFilterDropsUnmatchableForms: a form the matcher cannot look for would sit
// in the profile doing nothing, and a reader would think it was covered.
func TestFilterDropsUnmatchableForms(t *testing.T) {
	got := filterExpansions([]string{"utilize"},
		[]TermExpansion{{Term: "utilize", Forms: []string{"utilizes", "   ", "utili\nzes"}}})
	assert.Equal(t, []string{"utilizes"}, got[0].Forms)
}
