package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScaffoldRecipe_NeokapiI18nCleanLayout exercises the framework scaffold
// path end to end: resolve the neokapi-i18n preset into content + bindings,
// build the recipe YAML, then parse it back through the real loader. It proves
// `kapi init --framework neokapi-i18n` emits a valid recipe carrying the clean
// nested layout — source under i18n/src/, per-locale targets under i18n/{lang}/,
// with brand voice + terms source bound under i18n/.
func TestScaffoldRecipe_NeokapiI18nCleanLayout(t *testing.T) {
	content, err := FrameworkContent(preset.NeokapiI18nPresetName)
	require.NoError(t, err)
	require.Len(t, content, 1)
	assert.Equal(t, "i18n/src/**/*.kbf", content[0].Path)
	assert.Equal(t, "i18n/{lang}/{path}.kbf", content[0].Target)

	brandVoiceProfile, termsSource := FrameworkBindings(preset.NeokapiI18nPresetName)
	assert.Equal(t, "i18n/brand-voice.yaml", brandVoiceProfile)
	assert.Equal(t, "i18n/termbase.ktb", termsSource)

	yaml := ScaffoldRecipe("MyApp", "en", []string{"de", "fr", "nb"}, content, brandVoiceProfile, termsSource)

	// Parse the emitted recipe through the real loader — it must be valid.
	dir := t.TempDir()
	recipePath := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipePath, yaml, 0o644))

	proj, err := project.Load(recipePath)
	require.NoError(t, err, "scaffolded recipe must load + validate:\n%s", yaml)

	require.Len(t, proj.Content, 1)
	assert.Equal(t, "i18n/src/**/*.kbf", proj.Content[0].Path)
	assert.Equal(t, "i18n/{lang}/{path}.kbf", proj.Content[0].Target)

	require.NotNil(t, proj.Defaults.BrandVoice)
	assert.Equal(t, "i18n/brand-voice.yaml", proj.Defaults.BrandVoice.ProfileFile)
	assert.Equal(t, "i18n/termbase.ktb", proj.Defaults.TermsSource)
}
