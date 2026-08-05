package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyFrameworkPreset_NeokapiI18nCleanLayout proves `kapi init --preset
// neokapi-i18n` scaffolds the clean nested layout: source KBF catalogs under
// i18n/src/, per-locale targets under i18n/{lang}/, and brand voice + terms
// source bound under i18n/. It then round-trips through InitProject so the
// generated state .gitignore is asserted too.
func TestApplyFrameworkPreset_NeokapiI18nCleanLayout(t *testing.T) {
	recipe := &project.Recipe{}
	require.NoError(t, applyFrameworkPreset(recipe, "neokapi-i18n"))

	require.Len(t, recipe.Content, 1)
	assert.Equal(t, "i18n/src/**/*.kbf.json", recipe.Content[0].Path)
	assert.Equal(t, "i18n/{lang}/{path}.kbf.json", recipe.Content[0].Target)
	require.NotNil(t, recipe.Content[0].Format)
	assert.Equal(t, "kbf", recipe.Content[0].Format.Name)

	require.NotNil(t, recipe.Defaults.BrandVoice)
	assert.Equal(t, "i18n/brand-voice.yaml", recipe.Defaults.BrandVoice.ProfileFile)
	assert.Equal(t, "i18n/terms.json", recipe.Defaults.TermsSource)

	// Full init round-trip: the recipe writes, the state dir scaffolds with its
	// committed context/ and its ignored work/, and the generated .gitignore is
	// the two-line rule — no globs, and nothing to negate back out.
	dir := t.TempDir()
	proj, err := project.InitProject(dir, recipe)
	require.NoError(t, err)
	require.NoError(t, writeStateGitignore(proj))

	assert.DirExists(t, proj.Layout.ContextDir(), "init scaffolds the committed context directory")

	gi, err := os.ReadFile(filepath.Join(proj.StateDir(), ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, "work/\nfilters.local.json\n", string(gi),
		"the state .gitignore is exactly the machine-state dir and the personal filters file")
}
