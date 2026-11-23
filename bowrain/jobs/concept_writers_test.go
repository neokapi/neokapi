package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/terms/termstest"
	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/expand"
)

// concept "save": an English source term with a Norwegian one to give forms.
func savedConcept(t *testing.T) sqlterms.Store {
	t.Helper()
	st := termstest.Memory()
	_, err := st.CreateConcept(t.Context(), fw.Concept{
		ID:         "save",
		Definition: "as read",
		Terms: []fw.Term{
			{Text: "Save", Locale: "en", Status: model.TermPreferred},
			{Text: "lagre", Locale: "nb", Status: model.TermPreferred},
		},
	}, "")
	require.NoError(t, err)
	return st
}

func definitionOf(t *testing.T, st sqlterms.Store, id string) (fw.Concept, string) {
	t.Helper()
	c, _, ok, err := st.GetConceptAt(t.Context(), id)
	require.NoError(t, err)
	require.True(t, ok)
	return c, c.Definition
}

// The forms job reads the concept, asks a model, and writes the forms. A
// person who saves the concept while it ran keeps their edit: the job loses
// the race, reads again, and puts the forms on the person's version.
func TestFileTermForms_KeepsAnEditMadeWhileItRan(t *testing.T) {
	st := savedConcept(t)
	raced := termstest.Interleaved(st, termstest.SetDefinition(t, "a person's edit"))

	n, err := fileTermForms(t.Context(), raced, []expand.Proposal{{
		ConceptID: "save", Locale: "nb", Term: "lagre", Forms: []string{"lagrer", "lagret"},
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, n, "one term gained forms")

	c, def := definitionOf(t, st, "save")
	assert.Equal(t, "a person's edit", def, "the person's edit stays")
	assert.Equal(t, []string{"lagrer", "lagret"}, c.Terms[1].Forms, "and the forms land on it")
}

// The term-proposal job files a proposed translation on the concept as it
// stands when written, so an edit saved while the model answered stays.
func TestFileTermProposals_KeepsAnEditMadeWhileItRan(t *testing.T) {
	st := termstest.Memory()
	_, err := st.CreateConcept(t.Context(), fw.Concept{
		ID: "save", Definition: "as read",
		Terms: []fw.Term{{Text: "Save", Locale: "en", Status: model.TermPreferred}},
	}, "")
	require.NoError(t, err)
	raced := termstest.Interleaved(st, termstest.SetDefinition(t, "a person's edit"))

	n, err := fileTermProposals(t.Context(), raced, "en", "de", []aitools.TermTranslation{{ID: "save", Term: "Speichern"}})
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	c, def := definitionOf(t, st, "save")
	assert.Equal(t, "a person's edit", def, "the person's edit stays")
	assert.NotEmpty(t, c.Properties, "and the proposal is filed on it")
}

// A store whose writes name no revision is refused outright, before the
// job writes anything.
func TestConceptJobs_RefuseAStoreWithoutRevisions(t *testing.T) {
	plain := fw.NewInMemoryStore()
	_, err := fileTermForms(t.Context(), plain, []expand.Proposal{{ConceptID: "x", Locale: "nb", Term: "x", Forms: []string{"y"}}})
	require.Error(t, err)
	_, err = fileTermProposals(t.Context(), plain, "en", "de", []aitools.TermTranslation{{ID: "x", Term: "y"}})
	require.Error(t, err)
}
