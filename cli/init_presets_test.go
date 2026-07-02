package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInit_ListPresets: `kapi init --list-presets` prints the preset catalog
// (absorbing `kapi presets list`) and scaffolds nothing.
func TestInit_ListPresets(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	a := &App{}
	cmd := a.NewInitCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--list-presets"})
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "Framework Presets", "the preset catalog renders")
	assert.Contains(t, out.String(), "react-i18next", "framework presets are listed")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "--list-presets must not scaffold anything")
}

// TestInit_PresetScaffolds: `kapi init --preset <name>` scaffolds from the
// named framework preset (the porcelain spelling of --framework).
func TestInit_PresetScaffolds(t *testing.T) {
	dir := t.TempDir()

	a := &App{}
	cmd := a.NewInitCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--dir", dir, "--preset", "react-i18next", "--target-locale", "nb"})
	require.NoError(t, cmd.Execute())

	recipe, err := os.ReadFile(filepath.Join(dir, filepath.Base(dir)+".kapi"))
	require.NoError(t, err)
	assert.Contains(t, string(recipe), "public/locales", "the preset's content mapping is scaffolded")
	assert.Contains(t, string(recipe), "- nb")
}

// TestInit_PresetAndFrameworkAreExclusive: --preset is the alias of
// --framework; passing both is a usage error.
func TestInit_PresetAndFrameworkAreExclusive(t *testing.T) {
	a := &App{}
	cmd := a.NewInitCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--preset", "react-i18next", "--framework", "vue-i18n"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "none of the others can be")
}

// TestInit_UnknownPresetErrs: an unknown preset name surfaces the available
// framework presets.
func TestInit_UnknownPresetErrs(t *testing.T) {
	dir := t.TempDir()
	a := &App{}
	cmd := a.NewInitCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--dir", dir, "--preset", "no-such-stack"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown framework")
}
