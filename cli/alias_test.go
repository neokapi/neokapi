package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModelsAbsorbsOllama: `kapi models ollama` is the canonical home of the
// Ollama runtime management commands.
func TestModelsAbsorbsOllama(t *testing.T) {
	a := &App{}
	models := NewModelsCmd(a)
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
// convergence porcelain — `up` in the Work group — and the retired fold
// spellings (verify, ollama, presets, registry) are gone outright: this
// project breaks compatibility instead of carrying transition aliases.
func TestKapiCommandSet_PorcelainLayout(t *testing.T) {
	a := processOnlyApp(t)
	byName := map[string]*cobra.Command{}
	for _, c := range KapiCommandSet(a) {
		byName[c.Name()] = c
	}

	up := byName["up"]
	require.NotNil(t, up, "kapi up is registered")
	assert.Equal(t, "work", up.GroupID)

	for _, gone := range []string{"verify", "ollama", "presets", "registry"} {
		assert.Nil(t, byName[gone], "%s must not be registered — its home moved (check --ship, models ollama, init --list-presets, plugin registry)", gone)
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
	AddCommandGroups(a, root)
	for _, c := range KapiCommandSet(a) {
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
	for _, hidden := range []string{"\n  verify ", "\n  ollama ", "\n  presets ", "\n  registry ", "\n  engine "} {
		assert.NotContains(t, help, hidden, "hidden aliases and plumbing stay out of help")
	}
}

// TestRootHelpBudget pins the visible root surface (surface strategy A6):
// demoted tool verbs, deprecated aliases, and machine plumbing are hidden,
// and the visible command count must not silently creep back up. Raising the
// budget is a deliberate product decision, not a side effect.
func TestRootHelpBudget(t *testing.T) {
	a := processOnlyApp(t)
	root := &cobra.Command{Use: "kapi", Short: "test"}
	AddCommandGroups(a, root)
	for _, c := range KapiCommandSet(a) {
		root.AddCommand(c)
	}

	visible := 0
	var names []string
	for _, c := range root.Commands() {
		if c.Hidden || !c.IsAvailableCommand() {
			continue
		}
		visible++
		names = append(names, c.Name())
	}
	assert.LessOrEqualf(t, visible, 40,
		"visible root commands grew past the budget — new verbs belong in a group (kapi tool, an assets subtree) or hidden plumbing; visible now: %v", names)
}
