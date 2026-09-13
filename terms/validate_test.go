package terms_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
)

func TestValidateConcepts(t *testing.T) {
	concepts := []terms.Concept{
		{ID: "alert", Terms: []terms.Term{
			{Text: "alert", Locale: "en", Status: model.TermPreferred},
			{Text: "varsel", Locale: "nb-NO", Status: model.TermPreferred},
			{Text: "Alarm", Locale: "de", Status: model.TermPreferred, Forms: []string{"Alarme"}},
			{Text: "警报", Locale: "zh-Hans", Status: model.TermPreferred},
			{Text: "alarm", Locale: "nb", Status: model.TermDeprecated},
		}},
		{ID: "kapi", DoNotTranslate: true, Terms: []terms.Term{
			{Text: "kapi", Locale: "en"},
			{Text: "kapi", Locale: "nb"},
		}},
		{ID: "berth", Terms: []terms.Term{
			{Text: "berth", Locale: "en", Status: "retired"},
			{Text: "kaiplass", Locale: "nb", Forms: []string{"kaiplasser", "Varsel"}},
		}},
		{ID: "berth", Terms: []terms.Term{{Text: "quay", Locale: "en"}}},
		{Terms: []terms.Term{{Text: "orphan", Locale: "en"}}},
		{ID: "broken", Terms: []terms.Term{
			{Text: " ", Locale: "nb"},
			{Text: "nowhere"},
		}},
	}

	type finding struct {
		concept, term string
		warning       bool
	}
	var got []finding
	for _, p := range terms.ValidateConcepts(concepts, "en-US") {
		got = append(got, finding{p.ConceptID, p.Term, p.Warning})
	}
	assert.Equal(t, []finding{
		{"alert", "varsel", true},
		{"berth", "berth", false},
		{"berth", "kaiplass", true},
		{"berth", "", false},
		{"", "", false},
		{"broken", " ", false},
		{"broken", "nowhere", false},
	}, got)

	messages := map[finding]string{}
	for _, p := range terms.ValidateConcepts(concepts, "en") {
		messages[finding{p.ConceptID, p.Term, p.Warning}] = p.Message
	}
	assert.Contains(t, messages[finding{"alert", "varsel", true}], "no forms declared in nb")
	assert.Contains(t, messages[finding{"berth", "kaiplass", true}], `the form "Varsel" is spelled the same as the term "varsel" of concept alert`)
	assert.Contains(t, messages[finding{"berth", "berth", false}], `unknown status "retired"`)
	assert.Contains(t, messages[finding{"berth", "", false}], "used more than once")
}

func TestValidateConcepts_SourceTermsNeedNoForms(t *testing.T) {
	concepts := []terms.Concept{{ID: "varsel", Terms: []terms.Term{{Text: "varsel", Locale: "nb"}}}}
	assert.Empty(t, terms.ValidateConcepts(concepts, "nb"), "a term in the source language is not a target rendering")
	assert.Len(t, terms.ValidateConcepts(concepts, "en"), 1)
}

func TestLanguageInflects(t *testing.T) {
	for loc, want := range map[model.LocaleID]bool{
		"nb-NO": true, "de": true, "en": true, "fi": true, "pt_BR": true,
		"zh-Hant": false, "ja": false, "th": false, "id": false,
		"qps": false, "qaa": false, "und": false, "": false,
	} {
		assert.Equal(t, want, terms.LanguageInflects(loc), "LanguageInflects(%q)", loc)
	}
	assert.Equal(t, "nb", terms.BaseLanguage("NB_no"))
	assert.Empty(t, terms.BaseLanguage(""))
}
