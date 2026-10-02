package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/terms"
)

// termCheckRun is the run `kapi exec term-check` builds for files, holding
// the targets to rules, in French.
func termCheckRun(a *App, files []string, rules []profile.TermRule) ToolRunConfig {
	return ToolRunConfig{
		ToolName:   "term-check",
		Files:      files,
		TargetLang: "fr",
		JSONOutput: true,
		NewTool: func() (tool.Tool, error) {
			return a.ToolReg.NewToolWithConfig("term-check", map[string]any{"term_rules": rules}, "fr")
		},
		NewCollector: NewFindingsCollectorFor(a.ToolReg.Schema("term-check"), "fr"),
	}
}

// A translation kept in a file of its own holds no source, so term-check run
// on it compared nothing and printed "No findings.", the documented usage
// included. --target pairs the source file with its translation the way
// `kapi check --target` does, and the run reports the violation; the
// translation file alone reports that it compared nothing and how to pair it.
func TestExecTermCheckPairsASourceWithItsTranslation(t *testing.T) {
	dir := t.TempDir()
	en := filepath.Join(dir, "en.json")
	fr := filepath.Join(dir, "fr.json")
	require.NoError(t, os.WriteFile(en, []byte(`{"title": "Open the dashboard to sign in."}`), 0o644))
	require.NoError(t, os.WriteFile(fr, []byte(`{"title": "Ouvrez le panneau pour vous connecter."}`), 0o644))
	rules := []profile.TermRule{{Term: "dashboard", Replacement: "tableau de bord"}}
	a := &App{SourceLang: "en"}
	a.InitRegistries()

	report := func(t *testing.T, rc ToolRunConfig) findingsReport {
		t.Helper()
		out, err := captureStdout(t, func() error { return a.RunToolOnFiles(t.Context(), rc) })
		require.NoError(t, err)
		var r findingsReport
		require.NoError(t, json.Unmarshal([]byte(out), &r), out)
		return r
	}

	t.Run("the source paired with its translation", func(t *testing.T) {
		rc := termCheckRun(a, []string{en}, rules)
		rc.TargetFile = fr
		r := report(t, rc)
		require.Len(t, r.Findings, 1)
		d := r.Findings[0]
		assert.Equal(t, "term-check.terminology", d.Rule)
		assert.True(t, d.Fails)
		assert.Contains(t, d.Message, "tableau de bord")
		assert.Equal(t, "en.json", filepath.Base(d.Location.File), "the finding sits in the source file")
		assert.Equal(t, "title", d.Location.Block)
		assert.Empty(t, r.DidNotRunCause)
	})

	t.Run("a kept rendering passes", func(t *testing.T) {
		kept := filepath.Join(dir, "fr-kept.json")
		require.NoError(t, os.WriteFile(kept, []byte(`{"title": "Ouvrez le tableau de bord pour vous connecter."}`), 0o644))
		rc := termCheckRun(a, []string{en}, rules)
		rc.TargetFile = kept
		r := report(t, rc)
		assert.Empty(t, r.Findings)
		assert.Empty(t, r.DidNotRunCause, "the check compared the pair")
	})

	t.Run("the translation file alone compared nothing", func(t *testing.T) {
		r := report(t, termCheckRun(a, []string{fr}, rules))
		assert.Empty(t, r.Findings)
		assert.Equal(t, check.CauseContentNotChecked, r.DidNotRunCause)
		require.Len(t, r.DidNotRun, 1)
		assert.Contains(t, r.DidNotRun[0], "--target")
	})

	for name, tc := range map[string]struct {
		rc   func() ToolRunConfig
		want string
	}{
		"two files": {func() ToolRunConfig {
			rc := termCheckRun(a, []string{en, fr}, rules)
			rc.TargetFile = fr
			return rc
		}, "exactly one positional file"},
		"no target language": {func() ToolRunConfig {
			rc := termCheckRun(a, []string{en}, rules)
			rc.TargetFile, rc.TargetLang = fr, ""
			return rc
		}, "--target-lang"},
		"a translation that does not exist": {func() ToolRunConfig {
			rc := termCheckRun(a, []string{en}, rules)
			rc.TargetFile = filepath.Join(dir, "de.json")
			return rc
		}, "does not exist"},
	} {
		t.Run(name, func(t *testing.T) {
			err := a.RunToolOnFiles(t.Context(), tc.rc())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// Only a check that compares a source with its translation, and writes no
// file, pairs a translation file.
func TestReadsTargetsFollowsTheIOContract(t *testing.T) {
	a := &App{}
	a.InitRegistries()
	for _, name := range []string{"term-check", "dnt-check", "placeholder-check"} {
		assert.True(t, ReadsTargets(a.ToolReg.Schema(registry.ToolID(name))), name)
	}
	for _, name := range []string{"pseudo-translate", "xml-validation", "search-replace", "translate"} {
		assert.False(t, ReadsTargets(a.ToolReg.Schema(registry.ToolID(name))), name)
	}
	assert.False(t, ReadsTargets(nil))
}

// A terms store named with --termstore that does not exist is refused by every
// surface that reads one, named as the caller gave it with the command that
// creates it. `kapi exec term-check` read it as an empty vocabulary and passed
// what `kapi check` refused, and the refusal printed a resolved path with no
// next step.
func TestAMissingNamedTermsStoreIsRefused(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	t.Chdir(dir)
	src := filepath.Join(dir, "en.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"title": "Open the dashboard."}`), 0o644))

	for _, tc := range []struct {
		given string
		want  string
	}{
		{"nosuch", "`kapi terms import <file> --name nosuch`"},
		{"./missing.db", "`kapi terms import <file> --file ./missing.db`"},
	} {
		t.Run(tc.given, func(t *testing.T) {
			check := executionCommand(t)
			check.Flags().String("termstore", tc.given, "")
			_, err := (&App{SourceLang: "en"}).ComputeCheck(check, []string{src})
			require.Error(t, err)
			assert.Contains(t, err.Error(), `terms store "`+tc.given+`" does not exist`)
			assert.Contains(t, err.Error(), tc.want)

			exec := NewEnvCommand(t.Context(), "term-check")
			exec.Flags().String("termstore", tc.given, "")
			_, err = (&App{SourceLang: "en"}).ResolveTermRules(exec, "fr")
			require.Error(t, err, "exec refuses the store kapi check refuses")
			assert.Contains(t, err.Error(), `terms store "`+tc.given+`" does not exist`)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	// A store that exists is read.
	held := filepath.Join(dir, "held.db")
	writeNamedTermsStore(t, held, terms.Concept{ID: "dashboard", Terms: []terms.Term{
		{Text: "dashboard", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		{Text: "tableau de bord", Locale: model.LocaleFrench, Status: model.TermPreferred},
	}})
	exec := NewEnvCommand(t.Context(), "term-check")
	exec.Flags().String("termstore", held, "")
	rules, err := (&App{SourceLang: "en"}).ResolveTermRules(exec, "fr")
	require.NoError(t, err)
	assert.Len(t, rules, 1)
}
