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
// convergence porcelain (`up` in the Work group), and the retired fold
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
		assert.Nil(t, byName[gone], "%s must not be registered: its home moved (check --ship, models ollama, init --list-presets, plugin registry)", gone)
	}

	// Porcelain grouping: Work and Assets hold the everyday verbs, the verbs
	// for content in other languages and the content memory they reuse sit
	// under Languages, and the plumbing sits under Advanced.
	for name, group := range map[string]string{
		"init": "work", "add": "work", "status": "work", "apply": "work", "check": "work",
		"translate": "languages", "pseudo-translate": "languages", "memory": "languages",
		"terms": "assets", "voice": "assets", "models": "assets", "credentials": "assets",
		"run": "advanced", "flows": "advanced", "extract": "advanced", "merge": "advanced",
		"resolve": "advanced", "completion": "advanced", "telemetry": "advanced",
		"version": "", "update": "",
	} {
		c := byName[name]
		require.NotNil(t, c, name)
		assert.Equal(t, group, c.GroupID, "group of %s", name)
	}
}

// TestHelpGroups_RenderInPorcelainOrder: outside a project the default root
// help renders Work, then Assets, and neither Languages nor Advanced;
// `kapi help --all` renders Work, Assets, Languages, Advanced in that order.
// Neither view leaves a dangling GroupID (cobra panics on an undefined group id
// at Execute time, so a clean run of each is itself the wiring gate).
func TestHelpGroups_RenderInPorcelainOrder(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	render := func(t *testing.T, args ...string) string {
		t.Helper()
		a := processOnlyApp(t)
		root := &cobra.Command{Use: "kapi", Short: "test"}
		AddCommandGroups(a, root)
		for _, c := range KapiCommandSet(a) {
			root.AddCommand(c)
		}
		SetupStyledHelp(root)
		SetupHelpTopics(root)

		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		require.NoError(t, root.Execute())
		return out.String()
	}
	indexes := func(help string, headings ...string) []int {
		var at []int
		for _, h := range headings {
			at = append(at, strings.Index(help, "\n"+h+"\n"))
		}
		return at
	}

	help := render(t, "--help")
	at := indexes(help, "Work:", "Languages:", "Assets:", "Advanced:")
	require.NotEqual(t, -1, at[0], "Work renders")
	require.NotEqual(t, -1, at[2], "Assets renders")
	assert.Less(t, at[0], at[2], "Work renders before Assets")
	assert.Equal(t, -1, at[1], "Languages is left out outside a project")
	assert.Equal(t, -1, at[3], "Advanced is left out of the default page")

	all := render(t, "help", "--all")
	at = indexes(all, "Work:", "Assets:", "Languages:", "Advanced:", "Additional Commands:")
	for i, h := range []string{"Work:", "Assets:", "Languages:", "Advanced:", "Additional Commands:"} {
		require.NotEqual(t, -1, at[i], "%s renders in kapi help --all", h)
	}
	assert.Less(t, at[0], at[1], "Work renders before Assets")
	assert.Less(t, at[1], at[2], "Assets renders before Languages")
	assert.Less(t, at[2], at[3], "Languages renders before Advanced")
	assert.Less(t, at[3], at[4], "Advanced renders before Additional Commands")

	for _, view := range []string{help, all} {
		for _, gone := range []string{"\nProcessing:", "Project & Content:", "Info & Management:", "\nTranslate:"} {
			assert.NotContains(t, view, gone, "the pre-porcelain group headings are gone")
		}
		for _, hidden := range []string{"\n  verify ", "\n  ollama ", "\n  presets ", "\n  registry ", "\n  engine "} {
			assert.NotContains(t, view, hidden, "hidden aliases and plumbing stay out of help")
		}
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
		"visible root commands grew past the budget; new verbs belong in a group (kapi exec, an assets subtree) or hidden plumbing; visible now: %v", names)
}
