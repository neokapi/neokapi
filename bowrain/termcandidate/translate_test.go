package termcandidate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

func keptConcept() terms.Concept {
	return terms.Concept{
		ID:    "c1",
		Terms: []terms.Term{{Text: "workspace", Locale: "en", Status: model.TermPreferred}},
	}
}

// TestNeedsTranslation: a kept term with no usable term in the target
// waits for a proposal; a candidate, a do-not-translate concept, one with a
// target term and one with a proposal filed do not.
func TestNeedsTranslation(t *testing.T) {
	cp := keptConcept()
	assert.True(t, NeedsTranslation(cp, "en", "nb"))
	assert.False(t, NeedsTranslation(cp, "en", "en-GB"), "the source language is not a target")

	cand := cp
	cand.Terms = []terms.Term{{Text: "workspace", Locale: "en", Status: model.TermProposed}}
	assert.False(t, NeedsTranslation(cand, "en", "nb"), "a candidate is not a term yet")

	dnt := cp
	dnt.DoNotTranslate = true
	assert.False(t, NeedsTranslation(dnt, "en", "nb"))

	translated := cp
	translated.Terms = append(translated.Terms, terms.Term{Text: "arbeidsområde", Locale: "nb", Status: model.TermPreferred})
	assert.False(t, NeedsTranslation(translated, "en", "nb"))

	require.True(t, SetTranslation(&cp, "en", "nb", " arbeidsområde "))
	assert.Equal(t, "arbeidsområde", TranslationFor(cp, "nb"))
	assert.False(t, NeedsTranslation(cp, "en", "nb"), "a proposal is filed")
	assert.Equal(t, []Translation{{Locale: "nb", Text: "arbeidsområde"}}, Translations(cp))
	_, ok := terms.RuleForConcept(cp, "en", "nb")
	assert.False(t, ok, "a proposal governs nothing until it is kept")
}

// TestKeepTranslation adds the reviewer's wording as the preferred term and
// clears the proposal.
func TestKeepTranslation(t *testing.T) {
	cp := keptConcept()
	require.True(t, SetTranslation(&cp, "en", "nb", "arbeidsområde"))
	KeepTranslation(&cp, "nb", "arbeidsflate")
	assert.Empty(t, TranslationFor(cp, "nb"))
	rule, ok := terms.RuleForConcept(cp, "en", "nb")
	require.True(t, ok)
	assert.Equal(t, "arbeidsflate", rule.Replacement)

	// Keeping a text the locale already holds promotes it, and the
	// earlier preferred term steps down.
	cp.Terms = append(cp.Terms, terms.Term{Text: "Arbeidsområde", Locale: "nb", Status: model.TermAdmitted})
	KeepTranslation(&cp, "nb", "arbeidsområde")
	require.Len(t, cp.Terms, 3)
	assert.Equal(t, model.TermAdmitted, cp.Terms[1].Status)
	assert.Equal(t, model.TermPreferred, cp.Terms[2].Status)
}

// TestRejectTranslation clears the proposal and keeps its text from being
// proposed again.
func TestRejectTranslation(t *testing.T) {
	cp := keptConcept()
	require.True(t, SetTranslation(&cp, "en", "nb", "Arbeidsflate"))
	RejectTranslation(&cp, "nb")
	assert.Empty(t, TranslationFor(cp, "nb"))
	assert.Equal(t, []string{"arbeidsflate"}, RejectedTranslations(cp, "nb"))
	assert.True(t, NeedsTranslation(cp, "en", "nb"))
	assert.False(t, SetTranslation(&cp, "en", "nb", "arbeidsflate"), "a rejected text is not filed again")
	assert.True(t, SetTranslation(&cp, "en", "nb", "arbeidsområde"))
}

// TestExamples gathers up to the cap per term, matching inflections, and
// reports full once every term has its examples.
func TestExamples(t *testing.T) {
	e := NewExamples("en", []string{"workspace", "flow", "Workspace"}, 2)
	assert.Equal(t, []string{"workspace", "flow"}, e.Terms())
	e.Add("Open the workspace.")
	e.Add("Two workspaces share one key.")
	e.Add("Every workspace has an owner.")
	assert.False(t, e.Full())
	e.Add("A flow runs tools.")
	e.Add("Flows run in parallel.")
	assert.True(t, e.Full())
	assert.Equal(t, []string{"Open the workspace.", "Two workspaces share one key."}, e.For("Workspace"))
	assert.Len(t, e.For("flow"), 2)
	assert.Nil(t, e.For("other"))
}
