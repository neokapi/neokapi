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

// TestLocalePresets_AppliedPerLocale verifies that defaults.locales.<lang>.tools
// overrides the project-wide preset for that locale only: de uses its own
// pseudo-translate prefix, fr falls back to the project default.
func TestLocalePresets_AppliedPerLocale(t *testing.T) {
	a := processOnlyApp(t)

	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	proj := &project.KapiProject{
		Version: "v1",
		Name:    "LocalePresets",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"de-DE", "fr-FR"},
			Flow:            "pseudo",
			Tools: map[string]map[string]any{
				"pseudo-translate": {"prefix": "X-", "suffix": ""},
			},
			Locales: map[string]project.LocaleDefaults{
				"de-DE": {Tools: map[string]map[string]any{
					"pseudo-translate": {"prefix": "DE-"},
				}},
			},
		},
		Content: []project.ContentCollection{{
			Path:   "en.json",
			Format: &project.FormatSpec{Name: "json"},
			Target: "{lang}.json",
		}},
		Flows: map[string]*flow.StepsSpec{
			"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
		},
	}
	recipe := filepath.Join(real, "app.kapi")
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(real, "en.json"), []byte(`{"greeting":"Hello"}`), 0o644))

	out, err := runConverge(t, a, recipe)
	require.NoError(t, err, out)

	de, err := os.ReadFile(filepath.Join(real, "de-DE.json"))
	require.NoError(t, err, "de-DE materialized")
	assert.Contains(t, string(de), "DE-", "de uses its per-locale prefix")

	fr, err := os.ReadFile(filepath.Join(real, "fr-FR.json"))
	require.NoError(t, err, "fr-FR materialized")
	assert.Contains(t, string(fr), "X-", "fr falls back to the project-wide prefix")
	assert.NotContains(t, string(fr), "DE-", "the de override does not leak into fr")
}
