package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/neokapi/neokapi/core/formats" // register JSON
)

// convergeFixture writes a project whose default flow is an inline pseudo-translate
// flow, with the given target locales and ship gate. Two source files exercise
// the multi-file path; coverage is file-derived from the materialized targets.
func convergeFixture(t *testing.T, targets []model.LocaleID, shipGate gate.Gate) (recipe, root string) {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	recipe = filepath.Join(real, "app.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "ConvergeTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: targets,
			Flow:            "pseudo",
		},
		Content: []project.ContentCollection{{
			Path:   "src/locales/en/*.json",
			Format: &project.FormatSpec{Name: "json"},
			Target: "src/locales/{lang}/*.json",
		}},
		Flows: map[string]*flow.StepsSpec{
			"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
		},
		ShipGate: shipGate,
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))

	srcDir := filepath.Join(real, "src/locales/en")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.json"), []byte(`{"greeting":"Hello, world."}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "b.json"), []byte(`{"farewell":"Goodbye."}`), 0o644))
	return recipe, real
}

// runConverge invokes `kapi run --project <recipe> [flags]` with NO flow
// argument (the convergence path), capturing combined output.
func runConverge(t *testing.T, a *App, recipe string, flags ...string) (string, error) {
	t.Helper()
	cmd := a.NewRunCmd(RunCmdOptions{})
	args := append([]string{"--project", recipe}, flags...)
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

// TestConverge_MaterializesFilesAndConverges: the no-arg `kapi run` runs the
// default flow over every target locale, writes the localized files, and reports
// converged when the (presence-baseline) ship gate is met.
func TestConverge_MaterializesFilesAndConverges(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": 100})

	out, err := runConverge(t, a, recipe)
	require.NoError(t, err, out)

	// Both target files materialized (file-derived coverage reads them).
	for _, f := range []string{"a.json", "b.json"} {
		target := filepath.Join(root, "src/locales/nb-NO", f)
		data, rerr := os.ReadFile(target)
		require.NoError(t, rerr, "convergence must materialize %s", f)
		assert.NotEmpty(t, data)
	}

	assert.Contains(t, out, "Converged: every gated scope is shippable",
		"a present target meets translated:100 (presence baseline)")
	assert.Contains(t, out, "✓ shippable")
}

// TestConverge_AllTargetLocales: convergence iterates every target language, not
// just the first.
func TestConverge_AllTargetLocales(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO", "de-DE"}, gate.Gate{"translated": 100})

	out, err := runConverge(t, a, recipe)
	require.NoError(t, err, out)

	for _, loc := range []string{"nb-NO", "de-DE"} {
		target := filepath.Join(root, "src/locales", loc, "a.json")
		_, rerr := os.Stat(target)
		require.NoError(t, rerr, "convergence must write %s", loc)
	}
	assert.Contains(t, out, "over 2 locale(s)")
	assert.Contains(t, out, "Converged")
}

// TestConverge_NoDefaultFlow: with no defaults.flow, the no-arg run is an
// actionable error (not a silent no-op).
func TestConverge_NoDefaultFlow(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": 100})
	// Strip the default flow.
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Flow = ""
	require.NoError(t, project.Save(recipe, proj))

	_, runErr := runConverge(t, a, recipe)
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "default flow")
}

// TestConverge_UntilGateParksUnreachableGate: a gate the deterministic flow
// cannot satisfy (reviewed needs a human) parks after the pass cap — never an error.
func TestConverge_UntilGateParksUnreachableGate(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"reviewed": 100})

	out, err := runConverge(t, a, recipe, "--until-gate", "--max-passes", "2")
	require.NoError(t, err, "parked work is reported, never a build failure")

	assert.Contains(t, out, "parked (needs human)", "reviewed:100 cannot be reached by an automated flow")
	assert.Contains(t, out, "Not fully converged")
}
