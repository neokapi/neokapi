package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
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

// storeBlockTexts reads every translatable block's source text from a
// project's block store.
func storeBlockTexts(t *testing.T, root string) []string {
	t.Helper()
	storePath := filepath.Join(root, ".kapi", "cache", "blocks.db")
	store, err := sqlitestore.New(storePath)
	require.NoError(t, err)
	defer store.Close()
	sess, err := store.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()
	var texts []string
	tr := true
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Translatable: &tr}) {
		require.NoError(t, berr)
		var text string
		for _, r := range b.Source {
			if r.Text != nil {
				text += r.Text.Text
			}
		}
		texts = append(texts, text)
	}
	return texts
}

// TestUp_AutoExtractsOnDrift: `kapi up` populates the project block store on
// first run (missing store = drift), stamps it, and — after a source edit
// between runs — re-extracts so the store mirrors the edited working tree.
func TestUp_AutoExtractsOnDrift(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": 100})

	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "into the project store", "first up must auto-extract (missing store)")

	storePath := filepath.Join(root, ".kapi", "cache", "blocks.db")
	assert.False(t, project.BlockStoreStale(storePath), "auto-extract must stamp the store version")
	assert.Contains(t, storeBlockTexts(t, root), "Hello, world.")

	// No drift → the next up does not re-extract.
	a2 := processOnlyApp(t)
	out2, err := runUp(t, a2, recipe)
	require.NoError(t, err, out2)
	assert.NotContains(t, out2, "into the project store", "clean store must not re-extract")

	// Edit a source file → the next up re-extracts and the store reflects it.
	require.NoError(t, os.WriteFile(filepath.Join(root, "src/locales/en/a.json"),
		[]byte(`{"greeting":"Hello, edited world."}`), 0o644))
	a3 := processOnlyApp(t)
	out3, err := runUp(t, a3, recipe)
	require.NoError(t, err, out3)
	assert.Contains(t, out3, "source file(s) changed", "edited source must trigger a re-extract")
	texts := storeBlockTexts(t, root)
	assert.Contains(t, texts, "Hello, edited world.")
	assert.NotContains(t, texts, "Hello, world.", "re-extraction is a full rebuild — no stale blocks")
}

// TestUp_NoExtractOptsOut: --no-extract skips the drift check entirely; the
// store is never stamped by the convergence loop.
func TestUp_NoExtractOptsOut(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": 100})

	out, err := runUp(t, a, recipe, "--no-extract")
	require.NoError(t, err, out)
	assert.NotContains(t, out, "into the project store")
	_, statErr := os.Stat(project.BlockStoreVersionStampPath(filepath.Join(root, ".kapi", "cache", "blocks.db")))
	assert.True(t, os.IsNotExist(statErr), "--no-extract must not stamp the store")
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
