package host

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The source-first settle phase used to swallow both its unit-resolution and its
// settle error (`uerr == nil` / `herr == nil`). With the settle error dropped,
// blockedOnSource and totalSource both stayed 0 — so finishConverge's
// `blockedOnSource >= totalSource` test never fired, the run was labelled
// converged rather than source_not_ready, and the materialize guard that stops
// source-fallback output being written "as if caught up" was disarmed. The stated
// justification ("degrades to the gate-off behavior") did not hold either: the
// settle reads the same source files ComputeShipCoverage reads a moment later
// through the same readBlocks, so the run failed anyway — just later, and with a
// message pointing at coverage internals instead of the source file.

// newSourceSettleProject writes a one-file, one-locale project with an explicit
// translate_after level, under the dogfood isolation contract (CLAUDE.md): every root this
// run could otherwise inherit — the developer's ~/.config/kapi, the user and
// system plugin roots, the shared caches — is pinned to a throwaway dir, and
// project discovery is off, so the repo's own dogfood recipe can never be found.
func newSourceSettleProject(t *testing.T, translateAfter string) (*App, *EnvCommand, string, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"),
		[]byte(`{"greeting":"Hello world","farewell":"Goodbye now"}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "SourceSettleTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Flow:            "translate",
			TranslateAfter:  translateAfter,
		},
		Collections: []project.Collection{
			{Name: "app", Path: "src/en.json", Target: "src/{lang}.json"},
		},
		Flows: map[string]*flow.StepsSpec{
			"translate": {Steps: []flow.FlowStep{
				{Tool: "translate", Config: map[string]any{"provider": "demo"}},
			}},
		},
		ShipGate: gate.Gate{"translated": {Pct: 100}},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))
	t.Chdir(dir)

	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	return a, cmd, recipe, dir
}

func runSourceSettleConverge(t *testing.T, a *App, cmd *EnvCommand, recipe string) (ConvergeOutput, error) {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	var out ConvergeOutput
	rerr := a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{
		UntilGate: true,
		MaxPasses: 2,
		noChecks:  true,
		capture:   &out,
	})
	return out, rerr
}

// TestConverge_SourceSettleFailure_FailsNamingTheSourceStage: an unreadable source
// file makes the settle fail. The run must stop there, attributed to the source
// settle and to the gate it was evaluating, instead of continuing with a silently
// zeroed hold count and surfacing an unrelated coverage error later.
func TestConverge_SourceSettleFailure_FailsNamingTheSourceStage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file modes")
	}
	a, cmd, recipe, dir := newSourceSettleProject(t, string(model.TranslateAfterWritten))
	src := filepath.Join(dir, "src", "en.json")
	require.NoError(t, os.Chmod(src, 0o000))
	t.Cleanup(func() { _ = os.Chmod(src, 0o644) })

	out, err := runSourceSettleConverge(t, a, cmd, recipe)
	require.Error(t, err, "a source the gate cannot read must fail the run")
	assert.Contains(t, err.Error(), "settle source",
		"the failure is attributed to the source settle, not to a coverage internal")
	assert.Contains(t, err.Error(), string(model.TranslateAfterWritten),
		"and names the gate it was evaluating")
	assert.False(t, out.Converged, "a run that failed never reports convergence")
	assert.NotEqual(t, convergence.StallSourceNotReady, out.StallReason)

	// Nothing was written for the locale: the run stopped before producing output.
	_, statErr := os.Stat(filepath.Join(dir, "src", "fr.json"))
	assert.True(t, os.IsNotExist(statErr), "no target is written when the settle failed")
}

// TestConverge_SourceSettleClean_StillConverges is the control: with a readable
// source and a gate the settlement can satisfy, the same fixture converges and
// writes its target — so the strictness above rejects only real faults.
func TestConverge_SourceSettleClean_StillConverges(t *testing.T) {
	a, cmd, recipe, dir := newSourceSettleProject(t, string(model.TranslateAfterWritten))
	out, err := runSourceSettleConverge(t, a, cmd, recipe)
	require.NoError(t, err)
	assert.True(t, out.Converged, "a clean source settles and the run converges")
	assert.Zero(t, out.BlockedOnSource, "clean source blocks are not held at the written gate")
	_, statErr := os.Stat(filepath.Join(dir, "src", "fr.json"))
	require.NoError(t, statErr, "the locale is written")
}

// settleLineRun converges a project whose recipe is written from the given
// targets and collections, and returns the settle phase's run-log line beside
// the extraction line the same run printed.
func settleLineRun(t *testing.T, targets []model.LocaleID, colls []project.Collection, files map[string]string) (settle, extract string, out ConvergeOutput) {
	t.Helper()
	a, cmd, recipe, dir := newSourceSettleProject(t, string(model.TranslateAfterEstablished))
	for rel, body := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644))
	}
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.TargetLanguages = targets
	proj.Collections = colls
	require.NoError(t, project.Save(recipe, proj))
	proj, err = project.Load(recipe)
	require.NoError(t, err)

	rerr := a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{
		UntilGate: true,
		MaxPasses: 2,
		noChecks:  true,
		capture:   &out,
		onEvent: func(ev convergence.Event) {
			if ev.Type != convergence.EventLog {
				return
			}
			switch ev.Stage {
			case convergence.StageSettleSource:
				settle = ev.Message
			case convergence.StageSync:
				extract = ev.Message
			}
		},
	})
	require.NoError(t, rerr)
	return settle, extract, out
}

// A project with no target language settles its whole source, and the settle
// line reports it beside the extraction of the same files.
func TestConverge_SettleLineCountsTheSourceOfAMonolingualProject(t *testing.T) {
	settle, extract, out := settleLineRun(t, nil,
		[]project.Collection{{Name: "app", SourceOnly: true, Content: []project.ContentItem{{Path: "src/en.json"}}}}, nil)
	assert.Equal(t, `Settled source: 2 translatable block(s). No target language reads them, so translate_after "established" holds nothing.`, settle)
	assert.Equal(t, "Extracted 2 block(s) from 1 file(s) into the project store (first extraction).", extract)
	assert.Zero(t, out.BlockedOnSource)
}

// Beside a translated collection, a source-only one is settled with the rest
// of the source, and the hold is counted over the files that feed a target.
func TestConverge_SettleLineNamesTheHoldOverTranslatedFiles(t *testing.T) {
	settle, _, out := settleLineRun(t, []model.LocaleID{"fr"},
		[]project.Collection{
			{Name: "app", Path: "src/en.json", Target: "src/{lang}.json"},
			{Name: "notes", SourceOnly: true, Content: []project.ContentItem{{Path: "notes/en.json"}}},
		},
		map[string]string{"notes/en.json": `{"one":"First note","two":"Second note","three":"Third note"}`})
	assert.Equal(t, `Settled source: 5 translatable block(s). Of the 2 with a target language, 2 held below translate_after "established".`, settle)
	assert.Equal(t, 2, out.BlockedOnSource, "the source-only blocks hold no translation")
	assert.Equal(t, convergence.StallSourceNotReady, out.StallReason)
}

// When every source file feeds a target, the line keeps its one-count form.
func TestSettleSourceLine_EveryFileFeedsATarget(t *testing.T) {
	assert.Equal(t, `Settled source: 3 translatable block(s), 1 held below translate_after "written".`,
		settleSourceLine(settledSource{states: []string{"a", "b", "c"}, held: 1, heldOf: 3}, model.TranslateAfterWritten))
}
