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
// source gate, under the dogfood isolation contract (CLAUDE.md): every root this
// run could otherwise inherit — the developer's ~/.config/kapi, the user and
// system plugin roots, the shared caches — is pinned to a throwaway dir, and
// project discovery is off, so the repo's own dogfood recipe can never be found.
func newSourceSettleProject(t *testing.T, sourceGate string) (*App, *EnvCommand, string, string) {
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
			SourceGate:      sourceGate,
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
	a, cmd, recipe, dir := newSourceSettleProject(t, string(model.SourceGateChecked))
	src := filepath.Join(dir, "src", "en.json")
	require.NoError(t, os.Chmod(src, 0o000))
	t.Cleanup(func() { _ = os.Chmod(src, 0o644) })

	out, err := runSourceSettleConverge(t, a, cmd, recipe)
	require.Error(t, err, "a source the gate cannot read must fail the run")
	assert.Contains(t, err.Error(), "settle source",
		"the failure is attributed to the source settle, not to a coverage internal")
	assert.Contains(t, err.Error(), string(model.SourceGateChecked),
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
	a, cmd, recipe, dir := newSourceSettleProject(t, string(model.SourceGateChecked))
	out, err := runSourceSettleConverge(t, a, cmd, recipe)
	require.NoError(t, err)
	assert.True(t, out.Converged, "a clean source settles and the run converges")
	assert.Zero(t, out.BlockedOnSource, "clean source blocks are not held at the checked gate")
	_, statErr := os.Stat(filepath.Join(dir, "src", "fr.json"))
	require.NoError(t, statErr, "the locale is written")
}
