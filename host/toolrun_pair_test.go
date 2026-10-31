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

	run := func(t *testing.T, rc ToolRunConfig) (findingsReport, error) {
		t.Helper()
		out, err := captureStdout(t, func() error { return a.RunToolOnFiles(t.Context(), rc) })
		var r findingsReport
		require.NoError(t, json.Unmarshal([]byte(out), &r), out)
		return r, err
	}
	report := func(t *testing.T, rc ToolRunConfig) findingsReport {
		t.Helper()
		r, err := run(t, rc)
		require.NoError(t, err)
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
		r, err := run(t, termCheckRun(a, []string{fr}, rules))
		require.ErrorIs(t, err, ErrCheckNotRun, "a run that compared nothing does not exit as a clean one")
		assert.Equal(t, ExitNotRun, ExitCode(nil, err))
		assert.Contains(t, err.Error(), "--target <file>")
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

// ExecTargetFor is the one rule for every exec tool: a tool that compares a
// source with its translation pairs a translation file with --target unless it
// writes a translation, and a tool that writes only findings writes no file a
// run does not name. `exec qa docs/contacts.md --target-lang nb` wrote the
// source over docs/nb/contacts.md through the default layout, and the message
// it printed named a --target the command did not take.
func TestExecTargetForEveryTool(t *testing.T) {
	a := &App{}
	a.InitRegistries()
	pairs := map[string]bool{"term-check": true, "dnt-check": true, "placeholder-check": true, "qa": true}
	findingsOnly := map[string]bool{"qa": true}
	seen := map[string]bool{}
	for _, e := range a.ToolReg.CLITools() {
		name := string(e.Info.Name)
		seen[name] = true
		got := ExecTargetFor(e.Schema, e.Info.WritesOutput)
		assert.Equal(t, pairs[name], got.PairsTarget, "%s: --target", name)
		assert.Equal(t, e.Info.WritesOutput && !findingsOnly[name], got.WritesByDefault, "%s: a file with no -o", name)
		assert.Equal(t, findingsOnly[name], e.Info.WritesOutput && WritesFindingsOnly(e.Schema), "%s: writes only findings", name)
		if ReadsTargets(e.Schema) && e.Info.WritesOutput {
			assert.False(t, got.WritesByDefault, "%s reads a translation it does not write, so a run names the file it writes", name)
		}
	}
	for name := range pairs {
		assert.True(t, seen[name], "%s is an exec tool", name)
	}
	assert.Equal(t, ExecTarget{PairsTarget: false, WritesByDefault: true}, ExecTargetFor(nil, true),
		"a tool with no contract keeps the default layout and pairs nothing")
}

// A tool that reads a translation reports that it compared none, and exits
// as a check that did not run (ExitNotRun), wherever it read the blocks from.
func TestExecRunThatComparedNoTranslationDidNotRun(t *testing.T) {
	assert.NoError(t, notRunError(findingsReport{}))
	assert.NoError(t, notRunError("not a findings report"))
	err := notRunError(findingsReport{untranslated: &untranslatedRun{tool: "qa", lang: "nb"}})
	require.ErrorIs(t, err, ErrCheckNotRun)
	assert.Equal(t, ExitNotRun, ExitCode(nil, err))
	assert.Contains(t, err.Error(), "qa read no nb translation; pass the translation file with --target <file>")
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
