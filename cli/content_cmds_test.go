package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddRm_LocalRecipeEditing verifies the core `kapi add`/`kapi rm` commands
// edit the local .kapi recipe's content collections / exclude list, and — the
// key boundary property — preserve a platform `server:` block (unknown to the
// framework, round-tripped via Extras) across the save.
func TestAddRm_LocalRecipeEditing(t *testing.T) {
	a := processOnlyApp(t)

	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := filepath.Join(real, "app.kapi")

	const initial = `version: v1
name: AddRmTest
defaults:
  source_language: en-US
  target_languages:
    - fr-FR
server:
  url: https://example.test
content:
  - path: existing/*.json
    format:
      name: json
`
	require.NoError(t, os.WriteFile(recipe, []byte(initial), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(real, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(real, "src", "page.html"), []byte("<p>hi</p>"), 0o644))

	run := func(cmd *cobra.Command, patterns ...string) (string, error) {
		cmd.SetArgs(append(patterns, "--project", recipe))
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		err := cmd.Execute()
		return out.String(), err
	}

	// add: a new pattern, format auto-detected from the .html extension.
	out, err := run(a.NewAddCmd(), "src/**/*.html")
	require.NoError(t, err)
	assert.Contains(t, out, "src/**/*.html")

	// add: an already-tracked pattern is skipped.
	out, err = run(a.NewAddCmd(), "existing/*.json")
	require.NoError(t, err)
	assert.Contains(t, out, "Already tracked")

	// The recipe now tracks the new pattern with the detected format, AND the
	// platform server: block survived the save (Extras round-trip).
	raw, err := os.ReadFile(recipe)
	require.NoError(t, err)
	s := string(raw)
	assert.Contains(t, s, "src/**/*.html")
	assert.Contains(t, s, "html") // detected format recorded
	assert.Contains(t, s, "server:")
	assert.Contains(t, s, "https://example.test")

	// rm: a tracked bare entry removes the mapping; server: still preserved.
	out, err = run(a.NewRmCmd(), "src/**/*.html")
	require.NoError(t, err)
	assert.Contains(t, out, "Removed")
	raw, err = os.ReadFile(recipe)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "src/**/*.html")
	assert.Contains(t, string(raw), "server:")

	// rm: a non-tracked pattern is added to the exclude list.
	out, err = run(a.NewRmCmd(), "legacy/*.md")
	require.NoError(t, err)
	assert.Contains(t, out, "Excluded")
	raw, err = os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "legacy/*.md")

	// No project found is an actionable error.
	noProj := a.NewAddCmd()
	noProj.SetArgs([]string{"x/*.json", "--project", filepath.Join(real, "missing.kapi")})
	noProj.SetOut(&bytes.Buffer{})
	noProj.SetErr(&bytes.Buffer{})
	require.Error(t, noProj.Execute())
}

// TestCoreKapi_RefusesRecipeRequiringUnregisteredPlugin proves the boundary
// gate from the core side: a recipe that declares `requires: bowrain` (as a
// server-connected project does) is refused by plain kapi, where the bowrain
// extension is not registered — so a server: project does not silently work
// without the plugin.
func TestCoreKapi_RefusesRecipeRequiringUnregisteredPlugin(t *testing.T) {
	a := processOnlyApp(t)
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := filepath.Join(real, "app.kapi")
	const yaml = `version: v1
name: NeedsBowrain
requires:
  bowrain: "*"
server:
  url: https://example.test
`
	require.NoError(t, os.WriteFile(recipe, []byte(yaml), 0o644))

	cmd := a.NewAddCmd()
	cmd.SetArgs([]string{"src/*.json", "--project", recipe})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err = cmd.Execute()
	require.Error(t, err, "core kapi must refuse a recipe requiring an unregistered plugin")
	assert.Contains(t, err.Error(), "bowrain")
}

// TestLs_ListsTrackedFiles verifies core `kapi ls` lists the files the project's
// content tracks, honors a path filter, and shows block/word counts with --stats.
func TestLs_ListsTrackedFiles(t *testing.T) {
	a := processOnlyApp(t)
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := filepath.Join(real, "app.kapi")

	const yaml = `version: v1
name: LsTest
defaults:
  source_language: en-US
content:
  - path: src/*.json
    format:
      name: json
`
	require.NoError(t, os.WriteFile(recipe, []byte(yaml), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(real, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(real, "src", "a.json"), []byte(`{"greeting":"Hello there world"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(real, "src", "b.json"), []byte(`{"bye":"Goodbye"}`), 0o644))

	run := func(args ...string) (string, error) {
		cmd := a.NewLsCmd()
		cmd.SetArgs(append(args, "--project", recipe))
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		execErr := cmd.Execute()
		return out.String(), execErr
	}

	// Plain ls lists both files with their format.
	out, err := run()
	require.NoError(t, err)
	assert.Contains(t, out, "src/a.json")
	assert.Contains(t, out, "src/b.json")
	assert.Contains(t, out, "json")
	assert.Contains(t, out, "2 file(s)")

	// A path filter narrows the listing.
	out, err = run("src/a.json")
	require.NoError(t, err)
	assert.Contains(t, out, "src/a.json")
	assert.NotContains(t, out, "src/b.json")

	// --stats adds block/word columns + a totals summary.
	out, err = run("--stats")
	require.NoError(t, err)
	assert.Contains(t, out, "BLOCKS")
	assert.Contains(t, out, "WORDS")
	assert.Contains(t, out, "blocks, ")
}
