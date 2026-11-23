package termspush

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/terms/termstest"
	"github.com/neokapi/neokapi/core/model"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/terms"
)

// A pushed edit is a person's, made against what they pulled. It lands only
// on the concept as Apply read it: a write that lands between Apply's read
// and its write refuses the push, and the other write stays.
func TestTermsPush_AnEditLandsOnlyOnTheConceptItRead(t *testing.T) {
	st := termstest.Memory()
	ctx := t.Context()
	_, err := st.CreateConcept(ctx, terms.Concept{ID: "c-save", Domain: "ui", Terms: []terms.Term{
		{Text: "Save", Locale: "en", Status: model.TermApproved},
	}}, "")
	require.NoError(t, err)
	edits := &apiclient.TermsEdits{Updates: []apiclient.ConceptEdit{{
		ConceptID: "c-save",
		Concept: apiclient.UpdateConceptParams{
			Domain: "ui", Definition: "pushed",
			Terms: []apiclient.TermInfo{{Text: "Save", Locale: "en", Status: "approved"}},
		},
	}}}

	raced := termstest.Interleaved(st, termstest.SetDefinition(t, "a person's edit"))
	_, err = Apply(ctx, Stores{Terms: raced}, Params{WorkspaceID: "ws"}, edits)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "changed on the server while this push was applied")

	got, _, _, err := st.GetConceptAt(ctx, "c-save")
	require.NoError(t, err)
	assert.Equal(t, "a person's edit", got.Definition, "the other write stays")

	// Without a write in between, the edit lands.
	_, err = Apply(ctx, Stores{Terms: st}, Params{WorkspaceID: "ws"}, edits)
	require.NoError(t, err)
	got, _, _, err = st.GetConceptAt(ctx, "c-save")
	require.NoError(t, err)
	assert.Equal(t, "pushed", got.Definition)
}
