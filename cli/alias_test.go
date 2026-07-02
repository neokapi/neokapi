package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeprecatedAlias_PrintsNoteAndForwards: a deprecated alias is hidden,
// detached from help groups, and every runnable subcommand prints the pointer
// note on stderr before running the original RunE.
func TestDeprecatedAlias_PrintsNoteAndForwards(t *testing.T) {
	a := &App{}
	cmd := deprecatedAlias(a.NewPresetsCmd(), "note: presets moved")

	assert.True(t, cmd.Hidden)
	assert.Empty(t, cmd.GroupID)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"list"})
	require.NoError(t, cmd.Execute())

	assert.Contains(t, stderr.String(), "note: presets moved", "the note goes to stderr")
	assert.Contains(t, stdout.String(), "react-i18next", "the original command still runs")
}

// TestModelsAbsorbsOllama: `kapi models ollama` is the canonical home of the
// Ollama runtime management commands.
func TestModelsAbsorbsOllama(t *testing.T) {
	a := &App{}
	models := a.NewModelsCmd()
	var ollama *cobra.Command
	for _, sub := range models.Commands() {
		if sub.Name() == "ollama" {
			ollama = sub
		}
	}
	require.NotNil(t, ollama, "models must expose the ollama subcommand")
	assert.False(t, ollama.Hidden, "the canonical home is visible")

	subs := map[string]bool{}
	for _, s := range ollama.Commands() {
		subs[s.Name()] = true
	}
	for _, want := range []string{"status", "list", "pull", "install"} {
		assert.True(t, subs[want], "models ollama %s", want)
	}
}

// TestKapiCommandSet_PorcelainLayout: the kapi command set exposes the
// convergence porcelain — `up` in the Work group — and ships the one-release
// hidden aliases (verify, ollama, presets) excluded from help.
func TestKapiCommandSet_PorcelainLayout(t *testing.T) {
	a := processOnlyApp(t)
	byName := map[string]*cobra.Command{}
	for _, c := range a.KapiCommandSet() {
		byName[c.Name()] = c
	}

	up := byName["up"]
	require.NotNil(t, up, "kapi up is registered")
	assert.Equal(t, "work", up.GroupID)

	for _, alias := range []string{"verify", "ollama", "presets"} {
		c := byName[alias]
		require.NotNil(t, c, "%s stays registered as an alias", alias)
		assert.True(t, c.Hidden, "%s is hidden from help", alias)
		assert.Empty(t, c.GroupID, "%s carries no help group", alias)
	}

	// Porcelain grouping: Work and Assets hold the everyday verbs.
	for name, group := range map[string]string{
		"init": "work", "add": "work", "status": "work", "apply": "work", "check": "work",
		"tm": "assets", "termbase": "assets", "brand": "assets", "models": "assets", "credentials": "assets",
		"run": "advanced", "flows": "advanced", "extract": "advanced", "merge": "advanced",
	} {
		c := byName[name]
		require.NotNil(t, c, name)
		assert.Equal(t, group, c.GroupID, "group of %s", name)
	}
}

// TestHelpGroups_RenderInPorcelainOrder: the sectioned --help renders Work
// first, then Assets, with Advanced after the tool groups, and no dangling
// GroupID (cobra panics on an undefined group id at Execute time, so a clean
// run of --help is itself the wiring gate).
func TestHelpGroups_RenderInPorcelainOrder(t *testing.T) {
	a := processOnlyApp(t)
	root := &cobra.Command{Use: "kapi", Short: "test"}
	a.AddCommandGroups(root)
	for _, c := range a.KapiCommandSet() {
		root.AddCommand(c)
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--help"})
	require.NoError(t, root.Execute())

	help := out.String()
	iWork := strings.Index(help, "Work:")
	iAssets := strings.Index(help, "Assets:")
	iAdvanced := strings.Index(help, "Advanced:")
	require.NotEqual(t, -1, iWork)
	require.NotEqual(t, -1, iAssets)
	require.NotEqual(t, -1, iAdvanced)
	assert.Less(t, iWork, iAssets, "Work renders before Assets")
	assert.Less(t, iAssets, iAdvanced, "Assets renders before Advanced")

	for _, gone := range []string{"\nProcessing:", "Project & Content:", "Info & Management:"} {
		assert.NotContains(t, help, gone, "the pre-porcelain group headings are gone")
	}
	for _, hidden := range []string{"\n  verify ", "\n  ollama ", "\n  presets "} {
		assert.NotContains(t, help, hidden, "hidden aliases stay out of help")
	}
}
