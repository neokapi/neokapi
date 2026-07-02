package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runUp invokes `kapi up --project <recipe> [flags]`, capturing combined
// output. Mirrors runConverge (converge_test.go) for the porcelain command.
func runUp(t *testing.T, a *App, recipe string, flags ...string) (string, error) {
	t.Helper()
	cmd := a.NewUpCmd()
	args := append([]string{"--project", recipe}, flags...)
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

// TestUp_ConvergesByDefault: `kapi up` runs the default flow across every
// target locale with until-gate looping on by default, materializes the
// targets, and reports converged when the gate is met.
func TestUp_ConvergesByDefault(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO", "de-DE"}, gate.Gate{"translated": 100})

	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)

	for _, loc := range []string{"nb-NO", "de-DE"} {
		target := filepath.Join(root, "src/locales", loc, "a.json")
		_, rerr := os.Stat(target)
		require.NoError(t, rerr, "up must write %s", loc)
	}
	assert.Contains(t, out, "over 2 locale(s)")
	assert.Contains(t, out, "Converged: every gated scope is shippable")
}

// TestUp_SinglePass: --passes 1 runs exactly one pass (the old bare
// `kapi run` behavior) with no until-gate loop.
func TestUp_SinglePass(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": 100})

	out, err := runUp(t, a, recipe, "--passes", "1")
	require.NoError(t, err, out)
	assert.Contains(t, out, "in 1 pass.", "a single pass must be reported")
	assert.Contains(t, out, "Converged")
}

// TestUp_ParksUnreachableGate: a gate the deterministic flow cannot satisfy
// (reviewed needs a human) parks after the loop stalls — reported, never a
// build failure. This is the default `up` behavior (no flag needed).
func TestUp_ParksUnreachableGate(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"reviewed": 100})

	out, err := runUp(t, a, recipe)
	require.NoError(t, err, "parked work is reported, never a build failure")
	assert.Contains(t, out, "parked (needs human)")
	assert.Contains(t, out, "Not fully converged")
}

// TestUp_PassesCapsLoop: --passes N caps the until-gate loop at N passes.
func TestUp_PassesCapsLoop(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"reviewed": 100})

	out, err := runUp(t, a, recipe, "--passes", "2")
	require.NoError(t, err, out)
	// The pseudo flow stalls after pass 1 (reviewed needs a human); the loop
	// stops on no-progress before hitting the cap and parks the locale.
	assert.Contains(t, out, "parked (needs human)")
}

// TestUp_RejectsNegativePasses: a negative --passes is a usage error.
func TestUp_RejectsNegativePasses(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": 100})

	_, err := runUp(t, a, recipe, "--passes", "-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--passes")
}

// TestUp_RequiresProject: outside a project, up errors actionably.
func TestUp_RequiresProject(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	a := processOnlyApp(t)
	cmd := a.NewUpCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs a project")
}

// TestRun_BareRunPointsAtUp: the no-argument `kapi run` keeps working but
// prints the one-release pointer to `kapi up` on stderr.
func TestRun_BareRunPointsAtUp(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": 100})

	out, err := runConverge(t, a, recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "note: `kapi up` is the new home of the no-argument run; `kapi run` keeps custom-flow semantics.")
	assert.Contains(t, out, "Converged", "the bare run still converges")
}
