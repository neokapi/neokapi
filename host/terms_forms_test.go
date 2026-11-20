package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
