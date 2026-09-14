package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/project"
)

// A target language no ship gate matches has no bar to clear, so kapi reports it
// as not gated. These are end-to-end runs of the smallest such project: one JSON
// source, a recycle-only flow, and nothing in the content memory, so the language
// sits at 0% translated. The same project with a gate, and with a failing check,
// shows the two ways a scope is withheld.

// notGatedProject writes that project with the given materialize policy and
// ship gate; a nil gate declares none.
func notGatedProject(t *testing.T, materialize string, shipGate gate.Gate) (*App, *EnvCommand, string, string) {
	t.Helper()
	a, cmd, recipe, dir := recipeFormatProject(t,
		map[string]string{"src/en.json": `{"greeting":"Hello world"}`},
		[]project.Collection{{
			Name: "app",
			Content: []project.ContentItem{{
				Path:   "src/en.json",
				Format: &project.FormatSpec{Name: "json"},
				Target: "src/{lang}.json",
			}},
		}}, nil)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Materialize = materialize
	proj.ShipGate = shipGate
	require.NoError(t, project.Save(recipe, proj))
	return a, cmd, recipe, dir
}

// upWithChecks runs the loop as `kapi up` does, bound checks included, and
// returns the structured result.
func upWithChecks(t *testing.T, a *App, cmd *EnvCommand, recipe string) ConvergeOutput {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	var out ConvergeOutput
	require.NoError(t, a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{
		UntilGate: true,
		MaxPasses: 3,
		capture:   &out,
	}))
	return out
}

func statusCommand(t *testing.T, recipe string, flags map[string]string) (*App, *EnvCommand) {
	t.Helper()
	cmd := NewEnvCommand(context.Background(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	for k, v := range flags {
		require.NoError(t, cmd.Flags().Set(k, v))
	}
	return &App{}, cmd
}

func statusJSONFor(t *testing.T, recipe string) StatusOutput {
	t.Helper()
	a, cmd := statusCommand(t, recipe, map[string]string{"json": "true"})
	out, err := captureStdout(t, func() error { return a.RunStatus(cmd, nil) })
	require.NoError(t, err)
	var parsed StatusOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed), out)
	return parsed
}

func statusTextFor(t *testing.T, recipe string) string {
	t.Helper()
	a, cmd := statusCommand(t, recipe, nil)
	out, err := captureStdout(t, func() error { return a.RunStatus(cmd, nil) })
	require.NoError(t, err)
	return out
}

// shipJSONFor runs `kapi status --ship --emit ship.json` and returns the
// entries as written to the file.
func shipJSONFor(t *testing.T, recipe, dir string) map[string]map[string]any {
	t.Helper()
	path := filepath.Join(dir, "ship.json")
	a, cmd := statusCommand(t, recipe, map[string]string{"ship": "true", "emit": path})
	_, err := captureStdout(t, func() error { return a.RunStatus(cmd, nil) })
	require.NoError(t, err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var m map[string]map[string]any
	require.NoError(t, json.Unmarshal(raw, &m), string(raw))
	return m
}

// TestNotGated_StatusMakesNoClaimAtZeroPercent: before any run, the language is
// untranslated and ungated. `kapi status` and ship.json name it not gated, and
// the two-field reading still offers it.
func TestNotGated_StatusMakesNoClaimAtZeroPercent(t *testing.T) {
	_, _, recipe, dir := notGatedProject(t, "", nil)

	nb, ok := localeCoverage(statusJSONFor(t, recipe), "nb")
	require.True(t, ok)
	assert.Equal(t, 0, nb.Pct["translated"])
	assert.False(t, nb.Gated)
	assert.True(t, nb.Shippable, "nothing withholds the scope")
	assert.Equal(t, ShipStateNotGated, nb.ShipState)

	ship := shipJSONFor(t, recipe, dir)
	assert.Equal(t, "not_gated", ship["nb"]["state"], "ship.json says not gated explicitly")
	assert.Equal(t, true, ship["nb"]["shippable"], "a picker reading only shippable still offers nb")

	text := statusTextFor(t, recipe)
	assert.Contains(t, shipLineFor(text, "nb/app"), "not gated", text)
	assert.Contains(t, text, "No ship gates are declared.", text)
}

// TestNotGated_UpReportsNotGated: under `materialize: on-converge` the run's
// draft stays out of the delivered tree, which re-derives at 0% translated. The
// run converges as before and says the language is not gated, never shippable.
func TestNotGated_UpReportsNotGated(t *testing.T) {
	a, cmd, recipe, dir := notGatedProject(t, project.MaterializeOnConverge, nil)

	out := upWithChecks(t, a, cmd, recipe)
	assert.True(t, out.Converged, "nothing withholds the language, so the run converges as it did")
	require.Len(t, out.Locales, 1)
	nb := out.Locales[0]
	assert.Equal(t, 0, nb.Pct["translated"])
	assert.False(t, nb.Gated)
	assert.Equal(t, ShipStateNotGated, nb.ShipState)

	text := convergeText(t, out)
	assert.Contains(t, shipLineFor(text, "nb"), "not gated", text)
	assert.NotContains(t, text, "shippable", text)
	assert.Contains(t, text, "Up to date. No ship gates are declared.", text)

	assert.Equal(t, "not_gated", shipJSONFor(t, recipe, dir)["nb"]["state"])
}

// TestNotGated_FailingChecksWithholdTheScope: with the default delivery policy the
// pass writes a source-fallback nb.json, which fails the identical-target check.
// A failing check withholds a scope with no gate, so the language is withheld
// and parked on every surface.
func TestNotGated_FailingChecksWithholdTheScope(t *testing.T) {
	a, cmd, recipe, dir := notGatedProject(t, "", nil)

	out := upWithChecks(t, a, cmd, recipe)
	assert.False(t, out.Converged)
	require.Len(t, out.Locales, 1)
	nb := out.Locales[0]
	assert.Positive(t, nb.FailingChecks, "the source-fallback target fails the identical-target check")
	assert.True(t, nb.Parked)
	assert.False(t, nb.Gated)
	assert.Equal(t, ShipStateWithheld, nb.ShipState)
	assert.NotContains(t, convergeText(t, out), "not gated")

	lc, ok := localeCoverage(statusJSONFor(t, recipe), "nb")
	require.True(t, ok)
	assert.False(t, lc.Gated)
	assert.False(t, lc.Shippable)
	assert.Equal(t, ShipStateWithheld, lc.ShipState)

	ship := shipJSONFor(t, recipe, dir)
	assert.Equal(t, "withheld", ship["nb"]["state"])
	assert.Equal(t, false, ship["nb"]["shippable"])
}

// TestGated_ZeroPercentIsWithheld: the same project with `ship_gate: {translated:
// 100}` is gated, and at 0% it is withheld on every surface.
func TestGated_ZeroPercentIsWithheld(t *testing.T) {
	a, cmd, recipe, dir := notGatedProject(t, "", gate.Gate{"translated": {Pct: 100}})

	nb, ok := localeCoverage(statusJSONFor(t, recipe), "nb")
	require.True(t, ok)
	assert.True(t, nb.Gated)
	assert.False(t, nb.Shippable)
	assert.Equal(t, ShipStateWithheld, nb.ShipState)
	assert.Equal(t, "translated", nb.Blocking)
	assert.Contains(t, shipLineFor(statusTextFor(t, recipe), "nb/app"), "blocked: translate")

	ship := shipJSONFor(t, recipe, dir)
	assert.Equal(t, "withheld", ship["nb"]["state"])
	assert.Equal(t, false, ship["nb"]["shippable"])

	out := upWithChecks(t, a, cmd, recipe)
	assert.False(t, out.Converged)
	require.Len(t, out.Locales, 1)
	assert.True(t, out.Locales[0].Gated)
	assert.Equal(t, ShipStateWithheld, out.Locales[0].ShipState)
}

// TestGated_ClearedGateIsShippable: a gated language that clears its gate is the
// one ship.json and `kapi up` call shippable.
func TestGated_ClearedGateIsShippable(t *testing.T) {
	a, cmd, recipe, dir := parkedProject(t, gate.Gate{"translated": {Pct: 100}})

	out := converge(t, a, cmd, recipe)
	require.True(t, out.Converged)
	require.Len(t, out.Locales, 1)
	assert.True(t, out.Locales[0].Gated)
	assert.Equal(t, ShipStateShippable, out.Locales[0].ShipState)
	assert.Contains(t, convergeText(t, out), "Up to date: every gated scope is shippable.")

	ship := shipJSONFor(t, recipe, dir)
	assert.Equal(t, "shippable", ship["nb"]["state"])
	assert.Equal(t, true, ship["nb"]["shippable"])
}
