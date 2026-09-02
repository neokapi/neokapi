package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSourceGateProject writes a one-file project whose converge flow translates
// via the deterministic demo provider (no network, no key, no spend) and sets
// defaults.source_gate to the given level. It returns the app, a flag-carrying
// command, and the recipe path — the fixture the source-gate converge tests
// share.
func newSourceGateProject(t *testing.T, gate string) (*App, *EnvCommand, string) {
	return newSourceGateProjectWith(t, gate,
		`{"greeting":"Hello world","farewell":"Goodbye now"}`)
}

// newSourceGateProjectWith is newSourceGateProject with an explicit source JSON
// body, so a test can control which blocks settle clean vs. flag a source-side
// check finding.
func newSourceGateProjectWith(t *testing.T, gate, sourceJSON string) (*App, *EnvCommand, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"),
		[]byte(sourceJSON), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "SourceGateTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Flow:            "translate",
			SourceGate:      gate,
		},
		Collections: []project.Collection{
			{Name: "docs", Path: "src/en.json", Target: "src/{lang}.json"},
		},
		Flows: map[string]*flow.StepsSpec{
			"translate": {Steps: []flow.FlowStep{
				{Tool: "translate", Config: map[string]any{"provider": "demo"}},
			}},
		},
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
	return a, cmd, recipe
}

// runConverge drives one converge run over the fixture and returns its captured
// structured output plus the events it emitted.
func runConverge(t *testing.T, a *App, cmd *EnvCommand, recipe string) (ConvergeOutput, []convergence.Event) {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	var out ConvergeOutput
	var events []convergence.Event
	err = a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{
		UntilGate: true,
		MaxPasses: 3,
		noExtract: true,
		noChecks:  true,
		capture:   &out,
		onEvent:   func(ev convergence.Event) { events = append(events, ev) },
	})
	require.NoError(t, err)
	return out, events
}

// frTranslated reports how many fr values carry a real demo translation. The
// demo provider stamps each translation with a distinctive `⟦fr⟧` marker, so a
// value carrying it was TRANSLATED — as opposed to a source-fallback value the
// writer echoes when a block was held and never produced a target. 0 means
// nothing was translated (the file may still exist with source-fallback
// values).
func frTranslated(t *testing.T, recipe string) int {
	t.Helper()
	path := filepath.Join(filepath.Dir(recipe), "src", "fr.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	return strings.Count(string(data), "⟦fr⟧")
}

// TestConvergeSourceGate_HoldsWhenSourceBelowGate: with source_gate: approved,
// no file-read source block can reach `approved` (settlement promotes clean
// source only to `checked`), so every block is held — nothing is translated and
// the run surfaces source_not_ready with a blocked-on-source count. This is the
// local parity with the server holding an un-settled source (epic 019).
func TestConvergeSourceGate_HoldsWhenSourceBelowGate(t *testing.T) {
	a, cmd, recipe := newSourceGateProject(t, string(model.SourceGateApproved))
	out, events := runConverge(t, a, cmd, recipe)

	assert.Equal(t, 2, out.BlockedOnSource, "both source blocks held below the approved gate")
	assert.Equal(t, string(model.SourceGateApproved), out.SourceGate)
	assert.Equal(t, convergence.StallSourceNotReady, out.StallReason)
	assert.False(t, out.Converged)
	assert.Equal(t, 0, frTranslated(t, recipe), "no translation was produced")

	// The settle_source stage event carries the held count.
	var sawSettle bool
	for _, ev := range events {
		if ev.Stage == convergence.StageSettleSource {
			sawSettle = true
			assert.Equal(t, 2, ev.BlockedOnSource)
		}
	}
	assert.True(t, sawSettle, "a settle_source stage event was emitted")
}

// TestConvergeSourceGate_NoneDraftsFreely: source_gate: none is the opt-out —
// no settle, no hold; every block translates exactly as before source-first.
func TestConvergeSourceGate_NoneDraftsFreely(t *testing.T) {
	a, cmd, recipe := newSourceGateProject(t, string(model.SourceGateNone))
	out, events := runConverge(t, a, cmd, recipe)

	assert.Equal(t, 0, out.BlockedOnSource)
	assert.Empty(t, out.StallReason)
	assert.Equal(t, 2, frTranslated(t, recipe), "source drafted freely under the none gate")

	for _, ev := range events {
		assert.NotEqual(t, convergence.StageSettleSource, ev.Stage, "the none opt-out emits no settle event")
	}
}

// TestConvergeSourceGate_AtGateTranslates: with the DEFAULT gate (checked),
// clean source reaches `checked` on settlement, clears the gate, and translates
// normally — nothing is held.
func TestConvergeSourceGate_AtGateTranslates(t *testing.T) {
	// Empty source_gate → resolves to the default (checked).
	a, cmd, recipe := newSourceGateProject(t, "")
	out, _ := runConverge(t, a, cmd, recipe)

	assert.Equal(t, 0, out.BlockedOnSource, "clean source reaches checked and clears the default gate")
	assert.Equal(t, string(model.SourceGateChecked), out.SourceGate)
	assert.Empty(t, out.StallReason)
	assert.Equal(t, 2, frTranslated(t, recipe), "checked source translated")
}

// TestConvergeSourceGate_PartialTranslatesReadyReportsHeld: with the default
// (checked) gate over a mixed source — one clean block (settles to checked →
// translates) and one whitespace-only block (a major source-side check finding
// keeps it at the authored baseline → held) — the run translates the ready
// block, holds the un-ready one, reports the held count, and is NOT
// source_not_ready (partial progress advanced), mirroring the server's
// partial-item handling (epic 019).
func TestConvergeSourceGate_PartialTranslatesReadyReportsHeld(t *testing.T) {
	a, cmd, recipe := newSourceGateProjectWith(t, string(model.SourceGateChecked),
		`{"greeting":"Hello world","blank":"   "}`)
	out, _ := runConverge(t, a, cmd, recipe)

	assert.Equal(t, 1, out.BlockedOnSource, "the whitespace-only block is held below the checked gate")
	assert.Empty(t, out.StallReason, "a partial run is not source_not_ready — ready work advanced")
	assert.Equal(t, 1, frTranslated(t, recipe), "the clean block translated; the held block did not")
}

// TestConvergeSourceGate_HeldRunDoesNotMaterialize: a full source-hold with
// defaults.materialize: on-converge must NOT write localized files — the run
// produced no translations, so materializing source-fallback output would be
// exactly the silent skip → junk output the source gate prevents (matches the
// server, which skips its post-run work on source_not_ready).
func TestConvergeSourceGate_HeldRunDoesNotMaterialize(t *testing.T) {
	a, cmd, recipe := newSourceGateProject(t, string(model.SourceGateApproved))
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	var out ConvergeOutput
	require.NoError(t, a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{
		UntilGate:   true,
		MaxPasses:   3,
		noExtract:   true,
		noChecks:    true,
		materialize: true, // force the materialize step
		capture:     &out,
	}))

	assert.Equal(t, convergence.StallSourceNotReady, out.StallReason)
	assert.Equal(t, 0, out.MaterializedFiles, "a source-held run materializes nothing")
}
