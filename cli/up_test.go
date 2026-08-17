package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runUp invokes `kapi up --project <recipe> [flags]`, capturing combined
// output. Mirrors runConverge (converge_test.go) for the porcelain command.
func runUp(t *testing.T, a *App, recipe string, flags ...string) (string, error) {
	t.Helper()
	cmd := NewUpCmd(a)
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
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO", "de-DE"}, gate.Gate{"translated": {Pct: 100}})

	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)

	for _, loc := range []string{"nb-NO", "de-DE"} {
		target := filepath.Join(root, "src/locales", loc, "a.json")
		_, rerr := os.Stat(target)
		require.NoError(t, rerr, "up must write %s", loc)
	}
	assert.Contains(t, out, "over 2 locale(s)")
	assert.Contains(t, out, "Up to date: every gated scope is shippable")
}

// TestUp_BuiltinDefaultFlow: a recipe with NO defaults.flow and NO flows map
// converges through the built-in default flow (#1078 G6: recycle → translate),
// reported as "default (built-in)".
func TestUp_BuiltinDefaultFlow(t *testing.T) {
	a := demoProviderApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": {Pct: 100}})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Flow = ""
	proj.Flows = nil
	require.NoError(t, project.Save(recipe, proj))

	out, upErr := runUp(t, a, recipe)
	require.NoError(t, upErr, out)
	assert.Contains(t, out, `Ran flow "default (built-in)"`)
	assert.Contains(t, out, "Up to date: every gated scope is shippable")
	for _, f := range []string{"a.json", "b.json"} {
		_, statErr := os.Stat(filepath.Join(root, "src/locales/nb-NO", f))
		require.NoError(t, statErr, "the built-in default must materialize %s", f)
	}
}

// TestUp_PlanLabelsBuiltinDefaultFlow: `up --plan` on a flowless recipe labels
// the plan with the built-in default, mirroring what the run reports.
func TestUp_PlanLabelsBuiltinDefaultFlow(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": {Pct: 100}})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Flow = ""
	proj.Flows = nil
	require.NoError(t, project.Save(recipe, proj))

	out, planErr := runUp(t, a, recipe, "--plan")
	require.NoError(t, planErr, out)
	assert.Contains(t, out, `Plan for flow "default (built-in)"`)
}

// TestUp_SinglePass: --passes 1 runs exactly one pass (the old bare
// `kapi run` behavior) with no until-gate loop.
func TestUp_SinglePass(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": {Pct: 100}})

	out, err := runUp(t, a, recipe, "--passes", "1")
	require.NoError(t, err, out)
	assert.Contains(t, out, "in 1 pass.", "a single pass must be reported")
	assert.Contains(t, out, "Up to date")
}

// TestUp_ParksUnreachableGate: a gate the deterministic flow cannot satisfy
// (reviewed needs a human) parks after the loop stalls — reported, never a
// build failure. This is the default `up` behavior (no flag needed).
func TestUp_ParksUnreachableGate(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"reviewed": {Pct: 100}})

	out, err := runUp(t, a, recipe)
	require.NoError(t, err, "parked work is reported, never a build failure")
	assert.Contains(t, out, "parked (needs human)")
	assert.Contains(t, out, "Not yet up to date")
}

// TestUp_PassesCapsLoop: --passes N caps the until-gate loop at N passes.
func TestUp_PassesCapsLoop(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"reviewed": {Pct: 100}})

	out, err := runUp(t, a, recipe, "--passes", "2")
	require.NoError(t, err, out)
	// The pseudo flow stalls after pass 1 (reviewed needs a human); the loop
	// stops on no-progress before hitting the cap and parks the locale.
	assert.Contains(t, out, "parked (needs human)")
}

// TestUp_RejectsNegativePasses: a negative --passes is a usage error.
func TestUp_RejectsNegativePasses(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": {Pct: 100}})

	_, err := runUp(t, a, recipe, "--passes", "-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--passes")
}

// TestUp_RequiresProject: outside a project, up errors actionably.
func TestUp_RequiresProject(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	a := processOnlyApp(t)
	cmd := NewUpCmd(a)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs a project")
}

// openStore opens a project's store directly, for tests inspecting what a run
// left in it.
func openStore(t *testing.T, root string) *projectdb.DB {
	t.Helper()
	db, err := projectdb.Open(t.Context(), project.Layout{
		Root: root, StateDir: filepath.Join(root, project.StateDirName),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// storeBlockTexts reads every translatable block's source text from a
// project's block cache.
func storeBlockTexts(t *testing.T, root string) []string {
	t.Helper()
	sess, err := openStore(t, root).BlocksAutocommit().Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()
	var texts []string
	tr := true
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Translatable: &tr}) {
		require.NoError(t, berr)
		var sb strings.Builder
		for _, r := range b.Source {
			if r.Text != nil {
				sb.WriteString(r.Text.Text)
			}
		}
		texts = append(texts, sb.String())
	}
	return texts
}

// TestUp_AutoExtractsOnDrift: `kapi up` populates the project block store on
// first run (missing store = drift), stamps it, and — after a source edit
// between runs — re-extracts so the store mirrors the edited working tree.
func TestUp_AutoExtractsOnDrift(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "into the project store", "first up must auto-extract (missing store)")

	assert.False(t, openStore(t, root).BlockStoreStale(t.Context()),
		"auto-extract must stamp the store version")
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
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	out, err := runUp(t, a, recipe, "--no-extract")
	require.NoError(t, err, out)
	assert.NotContains(t, out, "into the project store")

	// The stamp is a row now, not a sidecar, so "never stamped" reads as the
	// store reporting itself stale — the same conclusion, asked of the table.
	db := openStore(t, root)
	assert.True(t, db.BlockStoreStale(t.Context()), "--no-extract must not stamp the store")
	has, herr := db.HasBlocks(t.Context())
	require.NoError(t, herr)
	assert.False(t, has, "--no-extract must not extract either")
}

// TestUp_MaterializePolicyManualByDefault: with the default policy (manual),
// a converged up does not run the post-loop materialize step.
func TestUp_MaterializePolicyManualByDefault(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "Up to date", out)
	assert.NotContains(t, out, "Materialized", "defaults.materialize is manual — no post-loop write")
}

// TestUp_MaterializeOnConverge: with defaults.materialize: on-converge, a
// gated-green locale ends the run with its localized files on disk carrying the
// translation.
//
// What it must NOT do is rewrite them from a store that holds no target for the
// locale. The convergence pass writes the target files itself, so on this path
// the store can legitimately hold nothing; materializing from it then wrote the
// SOURCE text over every translation the pass had just produced, and a test that
// only stat'd the files called it a success.
func TestUp_MaterializeOnConverge(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Materialize = project.MaterializeOnConverge
	require.NoError(t, project.Save(recipe, proj))

	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "Up to date", out)
	for _, f := range []string{"a.json", "b.json"} {
		body, rerr := os.ReadFile(filepath.Join(root, "src/locales/nb-NO", f))
		require.NoError(t, rerr)
		assert.NotContains(t, string(body), "Hello, world.",
			"%s must hold the pseudo-translation, not the source it was written over", f)
	}
}

// TestUp_MaterializeFlagForces: --materialize forces the post-loop write even
// when the recipe policy is manual — and, like the policy, writes what the store
// holds rather than source fallback over the run's own output.
func TestUp_MaterializeFlagForces(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	out, err := runUp(t, a, recipe, "--materialize")
	require.NoError(t, err, out)
	assert.Contains(t, out, "Up to date", out)
	body, rerr := os.ReadFile(filepath.Join(root, "src/locales/nb-NO", "a.json"))
	require.NoError(t, rerr)
	assert.NotContains(t, string(body), "Hello, world.")
}

// TestUp_MaterializeSkipsParkedLocale: a locale short of its gate (parked)
// does not materialize — its content isn't at the bar yet.
//
// The assertion that matters is the second one. Reading the run's own report
// only asks whether the gate was consulted; #1936 is that it was consulted after
// the pass had already written the files, so the report said withheld while the
// unreviewed draft sat in the tree a site build globs. What a consumer sees is
// the filesystem, so that is what this asks.
func TestUp_MaterializeSkipsParkedLocale(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"reviewed": gate.Threshold{Pct: 100}})

	out, err := runUp(t, a, recipe, "--materialize")
	require.NoError(t, err, out)
	assert.Contains(t, out, "parked (needs human)", out)
	assert.NotContains(t, out, "Materialized", "a parked locale must not materialize")

	for _, f := range []string{"a.json", "b.json"} {
		_, serr := os.Stat(filepath.Join(root, "src/locales/nb-NO", f))
		assert.True(t, os.IsNotExist(serr),
			"%s: the gate says withheld, so nothing that reads target files may find one", f)
	}
}

// TestRun_BareRunErrors: `kapi run` takes a flow name only — the bare form
// errors and points at `kapi up` (hard cutover; run never converges).
func TestRun_BareRunErrors(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": {Pct: 100}})

	cmd := NewRunCmd(a, RunCmdOptions{})
	cmd.SetArgs([]string{"--project", recipe})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err := cmd.Execute()
	require.Error(t, err, "bare run must not converge")
	assert.Contains(t, err.Error(), "kapi up")
}

// TestUp_FirstRunInlineWizard: a provider-less `kapi up` on a TTY runs the
// compact provider wizard inline, persists the choice, and then continues the
// original command to convergence.
func TestUp_FirstRunInlineWizard(t *testing.T) {
	a := processOnlyApp(t)
	saved := map[string]string{}
	wiz := AISetupIO{
		In:        strings.NewReader("\nn\n"), // accept default (Claude Code), skip live check
		Out:       io.Discard,
		IsTTY:     func() bool { return true },
		Detect:    func(context.Context) AIDetection { return AIDetection{ClaudeCode: true} },
		LiveCheck: func(context.Context, string, string, string) error { return nil },
		SetDefault: func(provider, model string) error {
			saved["ai.provider"], saved["ai.model"] = provider, model
			return nil
		},
	}
	a.AISetupIOOverride = &wiz
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": {Pct: 100}})

	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)

	// The wizard persisted the pick…
	assert.Equal(t, "claude-code", saved["ai.provider"])
	// …and the original command continued to run the loop.
	_, rerr := os.Stat(filepath.Join(root, "src/locales/nb-NO", "a.json"))
	require.NoError(t, rerr, "up must continue after the inline wizard")
	assert.Contains(t, out, "Up to date")
}

// TestUp_ConfiguredSkipsWizard: with a provider already configured the wizard
// never engages (no prompts in output).
func TestUp_ConfiguredSkipsWizard(t *testing.T) {
	a := processOnlyApp(t)
	wiz := AISetupIO{
		IsTTY:  func() bool { return true },
		Detect: func(context.Context) AIDetection { return AIDetection{DefaultProvider: "ollama"} },
	}
	a.AISetupIOOverride = &wiz
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": {Pct: 100}})

	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)
	assert.NotContains(t, out, "No AI provider is configured")
}
