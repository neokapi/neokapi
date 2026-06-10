package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeToolPreset(t *testing.T) {
	t.Run("no preset returns config unchanged", func(t *testing.T) {
		cfg := map[string]any{"prefix": "step"}
		assert.Equal(t, cfg, mergeToolPreset(nil, cfg))
	})
	t.Run("preset supplies defaults", func(t *testing.T) {
		got := mergeToolPreset(map[string]any{"prefix": "preset", "suffix": "!"}, nil)
		assert.Equal(t, map[string]any{"prefix": "preset", "suffix": "!"}, got)
	})
	t.Run("step keys win per key", func(t *testing.T) {
		preset := map[string]any{"prefix": "preset", "suffix": "!"}
		cfg := map[string]any{"prefix": "step"}
		got := mergeToolPreset(preset, cfg)
		assert.Equal(t, map[string]any{"prefix": "step", "suffix": "!"}, got)
		// Neither input is mutated.
		assert.Equal(t, map[string]any{"prefix": "preset", "suffix": "!"}, preset)
		assert.Equal(t, map[string]any{"prefix": "step"}, cfg)
	})
}

func TestApplyProjectBindings_ToolPresets(t *testing.T) {
	a := &App{projectBindings: &projectBindings{
		toolPresets: map[string]map[string]any{
			"pseudo-translate": {"prefix": "«", "suffix": "»"},
		},
	}}

	t.Run("preset applies to a bare step", func(t *testing.T) {
		got := a.applyProjectBindings("pseudo-translate", nil, nil)
		assert.Equal(t, map[string]any{"prefix": "«", "suffix": "»"}, got)
	})
	t.Run("step config overrides preset per key", func(t *testing.T) {
		got := a.applyProjectBindings("pseudo-translate", nil, map[string]any{"prefix": "["})
		assert.Equal(t, map[string]any{"prefix": "[", "suffix": "»"}, got)
	})
	t.Run("other tools are untouched", func(t *testing.T) {
		cfg := map[string]any{"mode": "upper"}
		assert.Equal(t, cfg, a.applyProjectBindings("case-transform", nil, cfg))
	})
}

// presetProjectFixture writes a `.kapi` project whose defaults.tools preset
// pins a pseudo-translate prefix, with two flows: one bare step (inherits the
// preset) and one overriding the prefix (step wins).
func presetProjectFixture(t *testing.T) (recipe, srcAbs, root string) {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	recipe = filepath.Join(real, "app.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "PresetTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR"},
			Tools: map[string]map[string]any{
				"pseudo-translate": {"prefix": "«PRESET» "},
			},
		},
		Content: []project.ContentCollection{
			{
				Path:   "src/locales/en/*.json",
				Format: &project.FormatSpec{Name: "json"},
				Target: "src/locales/{lang}/*.json",
			},
		},
		Flows: map[string]*flow.StepsSpec{
			"pseudo": {
				Steps: []flow.FlowStep{{Tool: "pseudo-translate"}},
			},
			"pseudo-override": {
				Steps: []flow.FlowStep{{
					Tool:   "pseudo-translate",
					Config: map[string]any{"prefix": "[STEP] "},
				}},
			},
		},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))

	srcAbs = filepath.Join(real, "src/locales/en/messages.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(srcAbs), 0o755))
	require.NoError(t, os.WriteFile(srcAbs, []byte(`{"greeting":"Hello"}`), 0o644))
	return recipe, srcAbs, real
}

// TestRun_ProjectToolPreset_Applies runs the bare-step flow end to end and
// asserts the project preset reaches the tool.
func TestRun_ProjectToolPreset_Applies(t *testing.T) {
	a := processOnlyApp(t)
	recipe, srcAbs, root := presetProjectFixture(t)

	outPreset := filepath.Join(root, "out-preset.json")
	out, err := runRunCmd(t, a, recipe, "pseudo", "-i", srcAbs, "-o", outPreset)
	require.NoError(t, err, "run output: %s", out)
	data, err := os.ReadFile(outPreset)
	require.NoError(t, err)
	assert.Contains(t, string(data), "«PRESET» ", "the project preset prefix reaches the tool")
}

// TestRun_ProjectToolPreset_StepOverrides runs the overriding flow on a fresh
// project and asserts the step config wins per key over the preset.
func TestRun_ProjectToolPreset_StepOverrides(t *testing.T) {
	a := processOnlyApp(t)
	recipe, srcAbs, root := presetProjectFixture(t)

	outOverride := filepath.Join(root, "out-override.json")
	out, err := runRunCmd(t, a, recipe, "pseudo-override", "-i", srcAbs, "-o", outOverride)
	require.NoError(t, err, "run output: %s", out)
	data, err := os.ReadFile(outOverride)
	require.NoError(t, err)
	assert.Contains(t, string(data), "[STEP] ", "the step config overrides the preset")
	assert.NotContains(t, string(data), "«PRESET» ")
}
