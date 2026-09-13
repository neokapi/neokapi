package host

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
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

	proposals, err := ProposeTermForms(context.Background(), mock, expansionConcepts(), TermsExpandOptions{SourceLocale: "en-US"})
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"nb:varsel", "nb:kaiplass", "nb:varsling", "de:Liegeplatz"}, *asked,
		"source terms, terms that declare forms and do-not-translate names are not asked about")
	assert.Len(t, mock.ChatStructuredCalls, 2, "one call per language")

	byTerm := map[string]FormsProposal{}
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
	_, err := ProposeTermForms(context.Background(), mock, expansionConcepts(), TermsExpandOptions{
		SourceLocale: "en",
		Locales:      []model.LocaleID{"nb"},
		Overwrite:    true,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"nb:varsel", "nb:kaiplass", "nb:fartøy", "nb:varsling"}, *asked)

	assert.Equal(t, 0, ExpandableTerms(expansionConcepts(), TermsExpandOptions{SourceLocale: "en", Locales: []model.LocaleID{"sv"}}))
	assert.Equal(t, 4, ExpandableTerms(expansionConcepts(), TermsExpandOptions{SourceLocale: "en"}))
}

func TestProposeTermForms_BatchesALargeLanguage(t *testing.T) {
	var concepts []terms.Concept
	answers := map[string][]string{}
	for i := range termsExpandBatch + 5 {
		text := fmt.Sprintf("begrep%02d", i)
		answers[text] = []string{text + "et"}
		concepts = append(concepts, terms.Concept{ID: text, Terms: []terms.Term{{Text: text, Locale: "nb"}}})
	}
	mock, _ := formsModel(t, map[string]map[string][]string{"nb": answers})
	proposals, err := ProposeTermForms(context.Background(), mock, concepts, TermsExpandOptions{SourceLocale: "en"})
	require.NoError(t, err)
	assert.Len(t, mock.ChatStructuredCalls, 2)
	require.Len(t, proposals, termsExpandBatch+5)
	for _, p := range proposals {
		assert.Equal(t, []string{p.Term + "et"}, p.Forms)
	}
}

func TestApplyFormsProposals_KeepsDeclaredFormsUnlessOverwriting(t *testing.T) {
	proposals := []FormsProposal{
		{ConceptID: "alert", Locale: "nb", Term: "varsel", Forms: []string{"varsler", " varsler "}},
		{ConceptID: "vessel", Locale: "nb", Term: "fartøy", Forms: []string{"fartøyer"}},
		{ConceptID: "berth", Locale: "nb", Term: "kaiplass"},
	}

	concepts := expansionConcepts()
	changed, n := ApplyFormsProposals(concepts, proposals, false)
	assert.Equal(t, 1, n)
	assert.Equal(t, map[string]bool{"alert": true}, changed)
	assert.Equal(t, []string{"varsler"}, concepts[0].Terms[1].Forms)
	assert.Equal(t, []string{"fartøyet"}, concepts[2].Terms[1].Forms, "declared forms stay without overwrite")

	concepts = expansionConcepts()
	_, n = ApplyFormsProposals(concepts, proposals, true)
	assert.Equal(t, 2, n)
	assert.Equal(t, []string{"fartøyer"}, concepts[2].Terms[1].Forms)
}

func TestTermsFormsTarget_BundleRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terms.json")
	data, err := ktb.Marshal(ktb.FromConcepts(expansionConcepts()))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))

	target, err := openBundleTarget(path)
	require.NoError(t, err)
	defer target.Close()
	concepts, err := target.Concepts(context.Background())
	require.NoError(t, err)

	written, err := target.Save(context.Background(), concepts, nil)
	require.NoError(t, err)
	assert.False(t, written, "nothing changed, nothing written")

	changed, _ := ApplyFormsProposals(concepts, []FormsProposal{{ConceptID: "alert", Locale: "nb", Term: "varsel", Forms: []string{"varsler"}}}, false)
	written, err = target.Save(context.Background(), concepts, changed)
	require.NoError(t, err)
	assert.True(t, written)

	reread, err := os.ReadFile(path)
	require.NoError(t, err)
	file, err := ktb.Unmarshal(reread)
	require.NoError(t, err)
	for _, c := range file.Concepts {
		if c.ID == "alert" {
			assert.Equal(t, []string{"varsler"}, c.TargetTerms("nb")[0].Forms)
		}
	}

	_, err = openBundleTarget(filepath.Join(t.TempDir(), "missing.terms.json"))
	assert.Error(t, err)
}
