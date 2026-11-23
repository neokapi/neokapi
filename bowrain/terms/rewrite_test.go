package terms_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/terms/termstest"
	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/terms"
)

// A framework routine that deletes a concept (fw.DemoteRule removing the last
// term) deletes it at the revision it read. An edit landing in between
// refuses the delete, and the routine runs again over the edited concept, so
// what it decides is decided on what the concept holds.
func TestRewrite_ADeleteNamesTheRevisionItRead(t *testing.T) {
	ctx := context.Background()
	st := termstest.Memory()
	_, err := st.CreateConcept(ctx, fw.Concept{ID: "c1", Terms: []fw.Term{
		{Text: "utilize", Locale: model.LocaleEnglish, Status: model.TermForbidden},
	}}, "")
	require.NoError(t, err)

	raced := termstest.Interleaved(st, termstest.SetDefinition(t, "a person's edit"))
	var seen []string
	err = sqlterms.Rewrite(ctx, raced, func(tb fw.Terminology) error {
		c, ok, err := tb.GetConcept(ctx, "c1")
		if err != nil || !ok {
			return err
		}
		seen = append(seen, c.Definition)
		if c.Definition != "" {
			// Seen the edit: keep the concept.
			return nil
		}
		return tb.DeleteConcept(ctx, "c1")
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"", "a person's edit"}, seen, "the stale delete ran the routine again")
	c, _, ok, err := st.GetConceptAt(ctx, "c1")
	require.NoError(t, err)
	require.True(t, ok, "the edited concept stays")
	assert.Equal(t, "a person's edit", c.Definition)
}
