package commands

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
)

// A local edit that only declares forms is an ordinary change the push sends
// up, and a term whose forms match its baseline is not a change at all.
func TestOrdinaryConceptChanged_SeesFormsEdits(t *testing.T) {
	pulled := terms.Concept{
		ID:     "alert",
		Domain: "ui",
		Terms:  []terms.Term{{Text: "varsel", Locale: "nb", Status: model.TermApproved}},
	}
	base := buildBaseline([]terms.Concept{pulled}, nil).Concepts["alert"]
	assert.False(t, ordinaryConceptChanged(pulled, base, pulled.Terms))

	edited := pulled
	edited.Terms = []terms.Term{{Text: "varsel", Locale: "nb", Status: model.TermApproved, Forms: []string{"varsler"}}}
	assert.True(t, ordinaryConceptChanged(edited, base, edited.Terms))

	rebased := buildBaseline([]terms.Concept{edited}, nil).Concepts["alert"]
	assert.False(t, ordinaryConceptChanged(edited, rebased, edited.Terms), "forms survive the baseline snapshot")
	assert.Equal(t, []string{"varsler"}, baselineTermToTerm(rebased.Terms[0]).Forms)
	assert.Equal(t, []string{"varsler"}, termsToInfo(edited.Terms)[0].Forms)
}
