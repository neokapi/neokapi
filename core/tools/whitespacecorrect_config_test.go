package tools_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
)

// whitespace-correct was registered with a zero-arg factory only. A registry
// with no ConfigFactory calls that factory and throws the step's config map
// away without a word (registry.ToolRegistry.NewToolWithConfig), and the
// factory pinned TargetLocale to English — so every knob the tool declares was
// dead from a flow, and `--target-lang nb` could not move the tool off `en`.
// Both halves are silent: the run reports success and nothing has changed.
//
// These tests drive the real registry path a flow step takes, not the
// constructor, because the constructor was never the broken part.

// wsToolFromStep builds whitespace-correct exactly as a flow step does:
// through the registry, with the step's config map and the run's target
// language.
func wsToolFromStep(t *testing.T, config map[string]any, targetLang string) tool.Tool {
	t.Helper()
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)
	tl, err := reg.NewToolWithConfig("whitespace-correct", config, targetLang)
	require.NoError(t, err)
	require.NotNil(t, tl)
	return tl
}

// wsRunStep sends one block with an `nb` target through a registry-built tool
// and returns the target runs it emitted.
func wsRunStep(t *testing.T, config map[string]any, targetLang string, source, target []model.Run) []model.Run {
	t.Helper()
	b := &model.Block{ID: "u1", Translatable: true, Source: source}
	b.SetTargetRuns(model.LocaleID("nb"), target)

	tl := wsToolFromStep(t, config, targetLang)
	out := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: b})
	blk, ok := out.Resource.(*model.Block)
	require.True(t, ok)
	return blk.TargetRuns(model.LocaleID("nb"))
}

// A step's config must reach the tool. The knobs are set against their
// defaults in both directions, so "config applied" and "config dropped, so
// defaults applied" cannot produce the same answer.
func TestWhitespaceCorrect_StepConfigReachesTheTool(t *testing.T) {
	source := []model.Run{wsTx("Save "), wsPh("1", "strong"), wsTx("now")}
	target := []model.Run{wsTx("Lagre  "), wsPh("1", "strong"), wsTx("nå   ")}

	t.Run("every correction off is a no-op", func(t *testing.T) {
		got := wsRunStep(t, map[string]any{
			"normalizeSpaces":       false,
			"matchSourceWhitespace": false,
			"trimTrailing":          false,
		}, "nb", source, target)
		assert.Equal(t,
			[]string{"text:Lagre  ", "ph:strong", "text:nå   "},
			runShapes(got),
			"a step that turns every correction off must change nothing")
	})

	t.Run("trimTrailing on, the rest off", func(t *testing.T) {
		got := wsRunStep(t, map[string]any{
			"normalizeSpaces":       false,
			"matchSourceWhitespace": false,
			"trimTrailing":          true,
		}, "nb", source, target)
		assert.Equal(t,
			[]string{"text:Lagre  ", "ph:strong", "text:nå"},
			runShapes(got),
			"trimTrailing (default false) must apply; normalizeSpaces (default true) must stay off")
	})

	t.Run("defaults when the step says nothing", func(t *testing.T) {
		got := wsRunStep(t, nil, "nb", source, target)
		assert.Equal(t,
			[]string{"text:Lagre ", "ph:strong", "text:nå"},
			runShapes(got),
			"normalizeSpaces collapses the double space; matchSourceWhitespace strips the trailing run the source has no edge for")
	})
}

// The locale comes from the run. The zero-arg factory hardcoded `en`, so a
// tool built for an `nb` run never saw the `nb` target — the one it was asked
// to correct — and left it exactly as it found it.
func TestWhitespaceCorrect_TargetLocaleComesFromTheRun(t *testing.T) {
	source := []model.Run{wsTx("Save now")}
	target := []model.Run{wsTx("Lagre  nå")}

	got := wsRunStep(t, nil, "nb", source, target)
	assert.Equal(t, []string{"text:Lagre nå"}, runShapes(got),
		"the run's --target-lang is the locale the tool corrects")
}

// A locale spelled in the step config must not outrank the run: one flow
// serves every locale it is run for.
func TestWhitespaceCorrect_RunLocaleOutranksConfigLocale(t *testing.T) {
	source := []model.Run{wsTx("Save now")}
	target := []model.Run{wsTx("Lagre  nå")}

	got := wsRunStep(t, map[string]any{"targetLocale": "de"}, "nb", source, target)
	assert.Equal(t, []string{"text:Lagre nå"}, runShapes(got),
		"the run's target language wins over a locale written into the step config")
}

// The config factory is reachable from the registry at all — the registration
// this whole class of defect turns on.
func TestWhitespaceCorrect_RegistryAppliesConfigNotDefaults(t *testing.T) {
	tl := wsToolFromStep(t, map[string]any{"trimLeading": true, "correctComma": false}, "nb")
	bt, ok := tl.(*tool.BaseTool)
	require.True(t, ok)
	cfg, ok := bt.Cfg.(*tools.WhitespaceCorrectConfig)
	require.True(t, ok)

	assert.Equal(t, model.LocaleID("nb"), cfg.TargetLocale)
	assert.True(t, cfg.TrimLeading, "a knob the step set")
	assert.False(t, cfg.CorrectComma, "a defaulted-true knob the step turned off")
	assert.True(t, cfg.NormalizeSpaces, "a knob the step did not mention keeps its default")
}
