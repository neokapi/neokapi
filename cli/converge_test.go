package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/neokapi/neokapi/core/formats" // register JSON
)

// demoProviderApp builds a fresh App whose AI tools resolve to the offline
// demo provider (no keys, no network): the shared ai.provider default plus the
// same AI-defaults preprocessor Init wires, minus the credential store. Tests
// that exercise the built-in default flow (recycle → translate) use it so the
// translate step runs hermetically.
func demoProviderApp(t *testing.T) *App {
	t.Helper()
	a := processOnlyApp(t)
	a.Config = config.NewAppConfig()
	a.Config.Set(config.KeyAIProvider, "demo")
	a.ToolReg.SetConfigPreprocessor(func(toolName string, requires []string, cfg map[string]any) (map[string]any, error) {
		return ApplyAIDefaults(a.Config, toolName, requires, cfg), nil
	})
	return a
}

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
		Version: project.CurrentVersion,
		Name:    "ConvergeTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: targets,
			Flow:            "pseudo",
		},
		Collections: []project.Collection{{
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
// runConverge drives the shared convergence engine the way `kapi up` does
// (bare `kapi run` no longer converges — run takes a flow name only).
func runConverge(t *testing.T, a *App, recipe string, opts ConvergeOptions) (string, error) {
	t.Helper()
	cmd := NewEnvCommand(context.Background(), "up")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// Bind the fixture recipe explicitly: downstream content memory/terms resolution
	// reads the project flag, and without it the upward walk would bind to
	// the repo's dogfood .kapi on a dev machine (and nothing on CI).
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	proj, perr := a.LoadProjectInteractive(cmd.Context(), recipe, LoadProjectInteractiveOptions{AssumeYes: a.AssumeYes})
	if perr != nil {
		return out.String(), perr
	}
	a.InitRegistries()
	if opts.MaxPasses == 0 {
		opts.MaxPasses = ConvergeMaxPassesDefault
	}
	err := a.RunDefaultFlowConverge(cmd, proj, recipe, opts)
	return out.String(), err
}

// TestConverge_MaterializesFilesAndConverges: the no-arg `kapi run` runs the
// default flow over every target locale, writes the localized files, and reports
// converged when the (presence-baseline) ship gate is met.
func TestConverge_MaterializesFilesAndConverges(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": {Pct: 100}})

	out, err := runConverge(t, a, recipe, ConvergeOptions{})
	require.NoError(t, err, out)

	// Both target files materialized (file-derived coverage reads them).
	for _, f := range []string{"a.json", "b.json"} {
		target := filepath.Join(root, "src/locales/nb-NO", f)
		data, rerr := os.ReadFile(target)
		require.NoError(t, rerr, "convergence must materialize %s", f)
		assert.NotEmpty(t, data)
	}

	assert.Contains(t, out, "Up to date: every gated scope is shippable",
		"a present target meets translated:100 (presence baseline)")
	assert.Contains(t, out, "✓ shippable")
}

// TestConverge_AllTargetLocales: convergence iterates every target language, not
// just the first.
func TestConverge_AllTargetLocales(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO", "de-DE"}, gate.Gate{"translated": {Pct: 100}})

	out, err := runConverge(t, a, recipe, ConvergeOptions{})
	require.NoError(t, err, out)

	for _, loc := range []string{"nb-NO", "de-DE"} {
		target := filepath.Join(root, "src/locales", loc, "a.json")
		_, rerr := os.Stat(target)
		require.NoError(t, rerr, "convergence must write %s", loc)
	}
	assert.Contains(t, out, "over 2 locale(s)")
	assert.Contains(t, out, "Up to date")
}

// TestConverge_NoDefaultFlowUsesBuiltin: with no defaults.flow, the no-arg run
// synthesizes the built-in default flow (#1078 G6) — content memory reuse then AI translate
// — instead of erroring, and reports it as "default (built-in)".
func TestConverge_NoDefaultFlowUsesBuiltin(t *testing.T) {
	a := demoProviderApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": {Pct: 100}})
	// Strip the default flow AND the flows map: the recipe carries zero flow YAML.
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Flow = ""
	proj.Flows = nil
	require.NoError(t, project.Save(recipe, proj))

	out, runErr := runConverge(t, a, recipe, ConvergeOptions{})
	require.NoError(t, runErr, out)
	assert.Contains(t, out, `Ran flow "default (built-in)"`)
	assert.Contains(t, out, "Up to date: every gated scope is shippable")
}

// TestConverge_MissingExplicitDefaultFlow: an explicitly configured
// defaults.flow that names no flow in the recipe still errors (the built-in
// default only fills the empty case).
func TestConverge_MissingExplicitDefaultFlow(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": {Pct: 100}})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Flow = "nope"
	require.NoError(t, project.Save(recipe, proj))

	_, runErr := runConverge(t, a, recipe, ConvergeOptions{})
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), `default flow "nope" not found`)
}

// TestConverge_UntilGateParksUnreachableGate: a gate the deterministic flow
// cannot satisfy (reviewed needs a human) parks after the pass cap — never an error.
func TestConverge_UntilGateParksUnreachableGate(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"reviewed": {Pct: 100}})

	out, err := runConverge(t, a, recipe, ConvergeOptions{UntilGate: true, MaxPasses: 2})
	require.NoError(t, err, "parked work is reported, never a build failure")

	assert.Contains(t, out, "parked (needs human)", "reviewed:100 cannot be reached by an automated flow")
	assert.Contains(t, out, "Not yet up to date")
}
