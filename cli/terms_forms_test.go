package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTermsBundle(t *testing.T, concepts []terms.Concept) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "glossary.terms.json")
	data, err := ktb.Marshal(ktb.FromConcepts(concepts))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

func runTermsCmd(t *testing.T, a *App, args ...string) (string, error) {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "1") // no upward walk into a real recipe
	root := NewTermsCmd(a)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestTermsValidate_WarnsAboutTargetTermsWithoutForms(t *testing.T) {
	path := writeTermsBundle(t, []terms.Concept{{ID: "alert", Terms: []terms.Term{
		{Text: "alert", Locale: "en", Status: model.TermPreferred},
		{Text: "varsel", Locale: "nb", Status: model.TermPreferred},
		{Text: "Alarm", Locale: "de", Status: model.TermPreferred, Forms: []string{"Alarme"}},
	}}})

	out, err := runTermsCmd(t, &App{}, "validate", path, "-s", "en")
	require.NoError(t, err, "warnings do not fail the command")
	assert.Contains(t, out, "VALID")
	assert.Contains(t, out, `alert nb "varsel": no forms declared in nb`)

	out, err = runTermsCmd(t, &App{}, "validate", path, "-s", "en", "--json")
	require.NoError(t, err)
	var res output.TermsValidateOutput
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.True(t, res.Valid)
	assert.Equal(t, 1, res.Concepts)
	require.Len(t, res.Problems, 1)
	assert.True(t, res.Problems[0].Warning)
}

func TestTermsValidate_FailsOnErrors(t *testing.T) {
	path := writeTermsBundle(t, []terms.Concept{{ID: "berth", Terms: []terms.Term{{Text: "berth", Locale: "en", Status: "retired"}}}})
	out, err := runTermsCmd(t, &App{}, "validate", path, "-s", "en")
	require.ErrorIs(t, err, ErrSilentExit)
	assert.Contains(t, out, "INVALID")
	assert.Contains(t, out, `unknown status "retired"`)
}

func TestTermsExpand_NothingToAskNeedsNoModel(t *testing.T) {
	path := writeTermsBundle(t, []terms.Concept{{ID: "alert", Terms: []terms.Term{
		{Text: "alert", Locale: "en"},
		{Text: "varsel", Locale: "nb", Forms: []string{"varsler"}},
	}}})
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	// No provider is configured: a run with every target term already expanded
	// must finish without building one.
	out, err := runTermsCmd(t, &App{}, "expand", path, "-s", "en")
	require.NoError(t, err)
	assert.Contains(t, out, "No terms to expand")

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)

	_, err = runTermsCmd(t, &App{}, "expand", filepath.Join(t.TempDir(), "terms.csv"))
	assert.ErrorContains(t, err, "is not a terms bundle")
}
