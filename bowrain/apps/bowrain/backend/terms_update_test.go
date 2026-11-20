package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalConceptUpdateKeepsGovernance(t *testing.T) {
	a := newTestApp(t)
	concept := terms.Concept{
		ID: "product", Definition: "Before", Advisory: true, DoNotTranslate: true,
		Source: terms.TermSourceBrandVocabulary,
		Terms: []terms.Term{{Text: "Sample", Locale: "en", Status: model.TermApproved,
			CompetitorTerm: true, Validity: &graph.Validity{Tags: map[string]string{"market": "UK"}},
		}},
	}
	require.NoError(t, a.tb.AddConcept(t.Context(), concept))

	require.NoError(t, a.UpdateConcept(UpdateConceptRequest{ConceptID: concept.ID, Definition: "After",
		Terms: []TermInfo{{Text: "Sample", Locale: "en", Status: "approved", Note: "Revised"}},
	}))
	got, ok, err := a.tb.GetConcept(t.Context(), concept.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "After", got.Definition)
	assert.True(t, got.Advisory)
	assert.True(t, got.DoNotTranslate)
	assert.Equal(t, concept.Source, got.Source)
	concept.Terms[0].Note = "Revised"
	assert.Equal(t, concept.Terms, got.Terms)
}
