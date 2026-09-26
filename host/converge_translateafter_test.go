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

// newTranslateAfterProject writes a one-file project whose converge flow translates
// via the deterministic demo provider (no network, no key, no spend) and sets
// defaults.translate_after to the given level. It returns the app, a flag-carrying
// command, and the recipe path — the fixture the translate-after converge tests
// share.
func newTranslateAfterProject(t *testing.T, level string) (*App, *EnvCommand, string) {
	return newTranslateAfterProjectWith(t, level,
		`{"greeting":"Hello world","farewell":"Goodbye now"}`)
}

// newTranslateAfterProjectWith is newTranslateAfterProject with an explicit source JSON
// body, so a test can control which blocks settle clean vs. flag a source-side
// check finding.
func newTranslateAfterProjectWith(t *testing.T, level, sourceJSON string) (*App, *EnvCommand, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"),
		[]byte(sourceJSON), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "TranslateAfterTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Flow:            "translate",
			TranslateAfter:  level,
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

// TestConvergeTranslateAfter_HoldsWhenSourceBelowLevel: with translate_after: established,
// no file-read source block can reach `established` (settlement promotes clean
// source only to `written`), so every block is held — nothing is translated and
// the run surfaces source_not_ready with a blocked-on-source count. This is the
// local parity with the server holding an un-settled source (epic 019).
func TestConvergeTranslateAfter_HoldsWhenSourceBelowLevel(t *testing.T) {
	a, cmd, recipe := newTranslateAfterProject(t, string(model.TranslateAfterEstablished))
	out, events := runConverge(t, a, cmd, recipe)

	assert.Equal(t, 2, out.BlockedOnSource, "both source blocks held below the established level")
	assert.Equal(t, string(model.TranslateAfterEstablished), out.TranslateAfter)
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

// TestConvergeTranslateAfter_NoneDraftsFreely: translate_after: none is the opt-out —
// no settle, no hold; every block translates exactly as before source-first.
func TestConvergeTranslateAfter_NoneDraftsFreely(t *testing.T) {
	a, cmd, recipe := newTranslateAfterProject(t, string(model.TranslateAfterNone))
	out, events := runConverge(t, a, cmd, recipe)

	assert.Equal(t, 0, out.BlockedOnSource)
	assert.Empty(t, out.StallReason)
	assert.Equal(t, 2, frTranslated(t, recipe), "source drafted freely at level none")

	for _, ev := range events {
		assert.NotEqual(t, convergence.StageSettleSource, ev.Stage, "the none opt-out emits no settle event")
	}
}

// TestConvergeTranslateAfter_AtLevelTranslates: with the DEFAULT level (written),
// clean source reaches `written` on settlement, reaches the level, and translates
// normally — nothing is held.
func TestConvergeTranslateAfter_AtLevelTranslates(t *testing.T) {
	// Empty translate_after → resolves to the default (written).
	a, cmd, recipe := newTranslateAfterProject(t, "")
	out, _ := runConverge(t, a, cmd, recipe)

	assert.Equal(t, 0, out.BlockedOnSource, "clean source reaches written and reaches the default level")
	assert.Equal(t, string(model.TranslateAfterWritten), out.TranslateAfter)
	assert.Empty(t, out.StallReason)
	assert.Equal(t, 2, frTranslated(t, recipe), "written source translated")
}

// TestConvergeTranslateAfter_PartialTranslatesReadyReportsHeld: with the default
// (written) level over a mixed source — one clean block (settles to written →
// translates) and one whitespace-only block (a major source-side check finding
// keeps it unsettled → held) — the run translates the ready
// block, holds the un-ready one, reports the held count, and is NOT
// source_not_ready (partial progress advanced), mirroring the server's
// partial-item handling (epic 019).
func TestConvergeTranslateAfter_PartialTranslatesReadyReportsHeld(t *testing.T) {
	a, cmd, recipe := newTranslateAfterProjectWith(t, string(model.TranslateAfterWritten),
		`{"greeting":"Hello world","blank":"   "}`)
	out, _ := runConverge(t, a, cmd, recipe)

	assert.Equal(t, 1, out.BlockedOnSource, "the whitespace-only block is held below the written level")
	assert.Empty(t, out.StallReason, "a partial run is not source_not_ready — ready work advanced")
	assert.Equal(t, 1, frTranslated(t, recipe), "the clean block translated; the held block did not")
}

// TestConvergeTranslateAfter_HeldRunDoesNotMaterialize: a full source-hold with
// defaults.materialize: on-converge must NOT write localized files — the run
// produced no translations, so materializing source-fallback output would be
// exactly the silent skip → junk output the translate_after hold prevents (matches the
// server, which skips its post-run work on source_not_ready).
func TestConvergeTranslateAfter_HeldRunDoesNotMaterialize(t *testing.T) {
	a, cmd, recipe := newTranslateAfterProject(t, string(model.TranslateAfterEstablished))
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
