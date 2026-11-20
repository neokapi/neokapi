package expand

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// formsModel answers a term-forms request from a table keyed by language and
// term, the way a model given the prompt would, and records which languages
// and terms it was asked about.
func formsModel(t *testing.T, answers map[string]map[string][]string) (*aiprovider.MockProvider, *[]string) {
	t.Helper()
	var asked []string
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(_ context.Context, messages []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		var system, user strings.Builder
		for _, m := range messages {
			if m.Role == "system" {
				system.WriteString(m.Text())
			} else {
				user.WriteString(m.Text())
			}
		}
		lang := ""
		for l := range answers {
			if strings.Contains(system.String(), "in "+l+":") {
				lang = l
			}
		}
		type expansion struct {
			Term  string   `json:"term"`
			Forms []string `json:"forms"`
		}
		var resp struct {
			Terms []expansion `json:"terms"`
		}
		for term := range strings.SplitSeq(strings.TrimSpace(user.String()), "\n") {
			asked = append(asked, lang+":"+term)
			resp.Terms = append(resp.Terms, expansion{Term: term, Forms: answers[lang][term]})
		}
		b, err := json.Marshal(resp)
		if err != nil {
			return nil, err
		}
		return &aiprovider.ChatResponse{Content: string(b)}, nil
	}
	return mock, &asked
}

func expansionConcepts() []terms.Concept {
	return []terms.Concept{
		{ID: "alert", Terms: []terms.Term{
			{Text: "alert", Locale: "en", Status: model.TermPreferred},
			{Text: "varsel", Locale: "nb", Status: model.TermPreferred},
		}},
		{ID: "berth", Terms: []terms.Term{
			{Text: "berth", Locale: "en", Status: model.TermPreferred},
			{Text: "kaiplass", Locale: "nb", Status: model.TermPreferred},
			{Text: "Liegeplatz", Locale: "de", Status: model.TermPreferred},
		}},
		{ID: "vessel", Terms: []terms.Term{
			{Text: "vessel", Locale: "en"},
			{Text: "fartøy", Locale: "nb", Forms: []string{"fartøyet"}},
		}},
		{ID: "compass", DoNotTranslate: true, Terms: []terms.Term{
			{Text: "Compass", Locale: "en"},
			{Text: "Compass", Locale: "nb"},
		}},
		{ID: "notification", Terms: []terms.Term{
			{Text: "notification", Locale: "en"},
			{Text: "varsling", Locale: "nb"},
		}},
	}
}

func TestProposeTermForms_AsksTargetTermsAndFilters(t *testing.T) {
	mock, asked := formsModel(t, map[string]map[string][]string{
		"nb": {
			"varsel":   {"varsler", "varselet", "varsling", "alarm"},
			"kaiplass": {"kaiplasser", "kaiplassen"},
		},
		"de": {"Liegeplatz": {"Liegeplätze", "Anlegestelle"}},
	})

	proposals, err := Propose(context.Background(), mock, expansionConcepts(), Options{SourceLocale: "en-US"})
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"nb:varsel", "nb:kaiplass", "nb:varsling", "de:Liegeplatz"}, *asked,
		"source terms, terms that declare forms and do-not-translate names are not asked about")
	assert.Len(t, mock.ChatStructuredCalls, 2, "one call per language")

	byTerm := map[string]Proposal{}
	for _, p := range proposals {
		byTerm[p.Term] = p
	}
	assert.Equal(t, []string{"varsler", "varselet"}, byTerm["varsel"].Forms)
	reasons := map[string]string{}
	for _, r := range byTerm["varsel"].Rejected {
		reasons[r.Form] = r.Reason
	}
	assert.Equal(t, `spelled the same as the term "varsling"`, reasons["varsling"],
		"a form that opens like the term but spells another term is dropped")
	assert.Equal(t, "not a form of the term", reasons["alarm"])
	assert.Equal(t, []string{"Liegeplätze"}, byTerm["Liegeplatz"].Forms)
}

func TestProposeTermForms_LocaleSelectionAndOverwrite(t *testing.T) {
	mock, asked := formsModel(t, map[string]map[string][]string{
		"nb": {"fartøy": {"fartøyer"}, "varsel": {"varsler"}, "kaiplass": {"kaiplasser"}, "varsling": {"varslinger"}},
	})
	_, err := Propose(context.Background(), mock, expansionConcepts(), Options{
		SourceLocale: "en",
		Locales:      []model.LocaleID{"nb"},
		Overwrite:    true,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"nb:varsel", "nb:kaiplass", "nb:fartøy", "nb:varsling"}, *asked)

	assert.Equal(t, 0, Count(expansionConcepts(), Options{SourceLocale: "en", Locales: []model.LocaleID{"sv"}}))
	assert.Equal(t, 4, Count(expansionConcepts(), Options{SourceLocale: "en"}))
}

// TestProposeTermForms_NarrowsToConcepts: naming concepts asks about their
// terms alone, while the other concepts' spellings still filter the forms.
func TestProposeTermForms_NarrowsToConcepts(t *testing.T) {
	mock, asked := formsModel(t, map[string]map[string][]string{
		"nb": {"varsel": {"varsler", "varsling"}},
	})
	opts := Options{SourceLocale: "en", Concepts: []string{"alert"}}
	assert.Equal(t, 1, Count(expansionConcepts(), opts))
	proposals, err := Propose(context.Background(), mock, expansionConcepts(), opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"nb:varsel"}, *asked)
	require.Len(t, proposals, 1)
	assert.Equal(t, []string{"varsler"}, proposals[0].Forms, "the spelling of another concept's term is still dropped")
}

func TestProposeTermForms_BatchesALargeLanguage(t *testing.T) {
	var concepts []terms.Concept
	answers := map[string][]string{}
	for i := range batchSize + 5 {
		text := fmt.Sprintf("begrep%02d", i)
		answers[text] = []string{text + "et"}
		concepts = append(concepts, terms.Concept{ID: text, Terms: []terms.Term{{Text: text, Locale: "nb"}}})
	}
	mock, _ := formsModel(t, map[string]map[string][]string{"nb": answers})
	proposals, err := Propose(context.Background(), mock, concepts, Options{SourceLocale: "en"})
	require.NoError(t, err)
	assert.Len(t, mock.ChatStructuredCalls, 2)
	require.Len(t, proposals, batchSize+5)
	for _, p := range proposals {
		assert.Equal(t, []string{p.Term + "et"}, p.Forms)
	}
}

func TestApplyFormsProposals_KeepsDeclaredFormsUnlessOverwriting(t *testing.T) {
	proposals := []Proposal{
		{ConceptID: "alert", Locale: "nb", Term: "varsel", Forms: []string{"varsler", " varsler "}},
		{ConceptID: "vessel", Locale: "nb", Term: "fartøy", Forms: []string{"fartøyer"}},
		{ConceptID: "berth", Locale: "nb", Term: "kaiplass"},
	}

	concepts := expansionConcepts()
	changed, n := Apply(concepts, proposals, false)
	assert.Equal(t, 1, n)
	assert.Equal(t, map[string]bool{"alert": true}, changed)
	assert.Equal(t, []string{"varsler"}, concepts[0].Terms[1].Forms)
	assert.Equal(t, []string{"fartøyet"}, concepts[2].Terms[1].Forms, "declared forms stay without overwrite")

	concepts = expansionConcepts()
	_, n = Apply(concepts, proposals, true)
	assert.Equal(t, 2, n)
	assert.Equal(t, []string{"fartøyer"}, concepts[2].Terms[1].Forms)
}
