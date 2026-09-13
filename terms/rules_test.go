package terms_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
)

func TestRulesFromConcepts(t *testing.T) {
	concepts := []terms.Concept{
		{ID: "vessel", Terms: []terms.Term{
			{Text: "ship", Locale: "en-GB", Status: model.TermDeprecated},
			{Text: "vessel", Locale: "en-GB", Status: model.TermPreferred, Forms: []string{"vessels"}},
			{Text: "fartøy", Locale: "nb", Status: model.TermPreferred, Forms: []string{"fartøyet", "fartøyer"}},
			{Text: "skip", Locale: "nb", Status: model.TermAdmitted, Forms: []string{"skipet"}},
			{Text: "båt", Locale: "nb", Status: model.TermProposed},
			{Text: "farkost", Locale: "nb", Status: model.TermDeprecated},
			{Text: "pram", Locale: "nb", Status: model.TermForbidden},
		}},
		{ID: "compass", DoNotTranslate: true, Terms: []terms.Term{{Text: "Compass", Locale: "en-GB"}}},
		{ID: "berth", Terms: []terms.Term{
			{Text: "berth", Locale: "en-GB", Status: model.TermPreferred},
			{Text: "Liegeplatz", Locale: "de", Status: model.TermPreferred},
		}},
		{ID: "harbour", Terms: []terms.Term{{Text: "harbour", Locale: "de"}}},
	}

	assert.Equal(t, []profile.TermRule{
		{
			Term:             "vessel",
			Forms:            []string{"vessels"},
			ConceptID:        "vessel",
			Replacement:      "fartøy",
			ReplacementForms: []string{"fartøyet", "fartøyer"},
			Accepted:         []profile.Rendering{{Text: "skip", Forms: []string{"skipet"}}},
		},
		{Term: "Compass", ConceptID: "compass", DoNotTranslate: true},
	}, terms.RulesFromConcepts(concepts, "en-GB", "nb"),
		"the head term keys the rule, proposed, deprecated and forbidden terms are not renderings, and a concept with no nb term imposes nothing")

	de := terms.RulesFromConcepts(concepts, "en-GB", "de")
	assert.Len(t, de, 2)
	assert.Equal(t, "Liegeplatz", de[1].Replacement)
	assert.Nil(t, de[1].Accepted)
}
