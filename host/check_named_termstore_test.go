package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// writeNamedTermsStore writes a standalone terms store holding concepts.
func writeNamedTermsStore(t *testing.T, path string, concepts ...terms.Concept) {
	t.Helper()
	store, err := terms.NewSQLiteStore(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()
	for _, c := range concepts {
		require.NoError(t, store.AddConcept(t.Context(), c))
	}
}

// A terms store named with --termstore governs `kapi check` outside a project.
// The check's terms resolver answered nil for every file outside a project, so
// the named store was never opened and the check printed "terms none loaded"
// beside a pass, for the source checks and for a translation alike.
func TestCheckReadsANamedTermsStoreOutsideAProject(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	store := filepath.Join(dir, "terms.db")
	writeNamedTermsStore(t, store,
		terms.Concept{ID: "dashboard", Terms: []terms.Term{
			{Text: "dashboard", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "tableau de bord", Locale: model.LocaleFrench, Status: model.TermPreferred},
		}},
		terms.Concept{ID: "sign-in", Terms: []terms.Term{
			{Text: "sign in", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "log in", Locale: model.LocaleEnglish, Status: model.TermForbidden},
		}},
	)
	src := filepath.Join(dir, "login.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"title":"Log in to open the dashboard."}`), 0o644))
	target := filepath.Join(dir, "fr.json")
	require.NoError(t, os.WriteFile(target, []byte(`{"title":"Connectez-vous pour ouvrir le panneau."}`), 0o644))

	rules := func(r check.Report) map[string]bool {
		out := map[string]bool{}
		for _, d := range r.Findings {
			out[d.Rule] = out[d.Rule] || d.Fails
		}
		return out
	}

	t.Run("the source is held to the store's vocabulary", func(t *testing.T) {
		cmd := executionCommand(t)
		cmd.Flags().String("target-lang", "fr", "")
		cmd.Flags().String("termstore", store, "")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{src})
		require.NoError(t, err)
		require.NotNil(t, report.Execution)
		require.Len(t, report.Execution.Contexts, 1)
		assert.True(t, report.Execution.Contexts[0].TermsApplied, "the named store is loaded")
		assert.Contains(t, rules(report), "terms.vocabulary", "the forbidden term is a finding")

		var out bytes.Buffer
		require.NoError(t, (checkReport{report}).FormatText(&out))
		assert.Contains(t, out.String(), "terms loaded")
		assert.NotContains(t, out.String(), "terms none loaded")
	})

	t.Run("a translation is held to the store's renderings", func(t *testing.T) {
		cmd := executionCommand(t)
		cmd.Flags().String("target", target, "")
		cmd.Flags().String("target-lang", "fr", "")
		cmd.Flags().String("termstore", store, "")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{src})
		require.NoError(t, err)
		assert.True(t, rules(report)["terms.terminology"], "the missing rendering fails")
		assert.Equal(t, check.VerdictFailed, report.Verdict)
	})

	t.Run("a named store that does not exist is refused", func(t *testing.T) {
		cmd := executionCommand(t)
		cmd.Flags().String("termstore", filepath.Join(dir, "missing.db"), "")
		_, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{src})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing.db")
		assert.Contains(t, err.Error(), "does not exist")
	})
}
