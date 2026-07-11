package pluginattach

import (
	"testing"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func contribPlugin(name string, cc manifest.CommandContribution) *pluginhost.Plugin {
	return &pluginhost.Plugin{
		Manifest: &manifest.Manifest{
			ManifestVersion: "1",
			Plugin:          name,
			Version:         "0.0.0",
			Binary:          "bin",
			Capabilities:    manifest.Capabilities{CommandContributions: []manifest.CommandContribution{cc}},
		},
	}
}

func commandPlugin(name, group string, cmds ...manifest.Command) *pluginhost.Plugin {
	return &pluginhost.Plugin{
		Manifest: &manifest.Manifest{
			ManifestVersion: "1",
			Plugin:          name,
			Version:         "0.0.0",
			Binary:          "bin",
			Group:           group,
			Capabilities:    manifest.Capabilities{Commands: cmds},
		},
	}
}

func findCmd(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// The no-shadowing rule: a plugin command colliding with a built-in never
// takes the top-level verb — the built-in keeps it, and the plugin's version
// is reachable only under the plugin group.
func TestAttachCommands_builtinAlwaysWins(t *testing.T) {
	host := pluginhost.NewHost([]*pluginhost.Plugin{commandPlugin("bowrain", "bowrain",
		manifest.Command{Name: "config", Short: "bowrain config"},
		manifest.Command{Name: "push", Short: "push"},
	)}, nil)

	root := &cobra.Command{Use: "kapi"}
	builtinConfig := &cobra.Command{Use: "config", Short: "built-in config", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(builtinConfig)

	var warns []string
	AttachCommands(root, host, func(m string) { warns = append(warns, m) })

	assert.Empty(t, warns, "a built-in collision is a supported layout, not a per-invocation warning")
	assert.Same(t, builtinConfig, findCmd(t, root, "config"), "the built-in must keep the top-level verb")

	group := findCmd(t, root, "bowrain")
	require.NotNil(t, group, "plugin group command should attach")
	assert.NotNil(t, findCmd(t, group, "config"), "colliding command must be reachable as `kapi bowrain config`")
	assert.NotNil(t, findCmd(t, group, "push"), "non-colliding commands are also reachable in the group")
	assert.NotNil(t, findCmd(t, root, "push"), "non-colliding commands still attach top-level")
}

// Hidden manifest commands (dispatch plumbing like server-status/server-up)
// stay routable but out of --help and completion.
func TestAttachCommands_hiddenPlumbing(t *testing.T) {
	host := pluginhost.NewHost([]*pluginhost.Plugin{commandPlugin("bowrain", "bowrain",
		manifest.Command{Name: "server-up", Hidden: true, Short: "plumbing"},
	)}, nil)

	root := &cobra.Command{Use: "kapi"}
	AttachCommands(root, host, nil)

	top := findCmd(t, root, "server-up")
	require.NotNil(t, top, "hidden commands stay routable at the top level")
	assert.True(t, top.Hidden)

	group := findCmd(t, root, "bowrain")
	require.NotNil(t, group)
	assert.True(t, group.Hidden, "a group with only hidden children hides itself")
}

// A plugin with no explicit group falls back to its plugin name.
func TestAttachCommands_groupDefaultsToPluginName(t *testing.T) {
	host := pluginhost.NewHost([]*pluginhost.Plugin{commandPlugin("acme", "",
		manifest.Command{Name: "frob", Short: "frob"},
	)}, nil)

	root := &cobra.Command{Use: "kapi"}
	AttachCommands(root, host, nil)

	group := findCmd(t, root, "acme")
	require.NotNil(t, group)
	assert.False(t, group.Hidden)
	assert.NotNil(t, findCmd(t, group, "frob"))
}

// A plugin group name colliding with a built-in loses the group spelling and
// reports the conflict.
func TestAttachCommands_groupNameCollision(t *testing.T) {
	host := pluginhost.NewHost([]*pluginhost.Plugin{commandPlugin("evil", "status",
		manifest.Command{Name: "frob", Short: "frob"},
	)}, nil)

	root := &cobra.Command{Use: "kapi"}
	builtinStatus := &cobra.Command{Use: "status", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(builtinStatus)

	var warns []string
	AttachCommands(root, host, func(m string) { warns = append(warns, m) })

	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], "collides with a built-in command")
	assert.Same(t, builtinStatus, findCmd(t, root, "status"), "the built-in keeps the verb")
	assert.NotNil(t, findCmd(t, root, "frob"), "the plugin's commands still attach top-level")
}

func TestAttachContributions_registersFlagsOnBuiltin(t *testing.T) {
	host := pluginhost.NewHost([]*pluginhost.Plugin{contribPlugin("bowrain", manifest.CommandContribution{
		Command:    "init",
		Handler:    "init-connect",
		EngageWhen: "server",
		Flags: []manifest.FlagSpec{
			{Name: "server", Type: "string", Description: "server URL"},
			{Name: "anonymous", Type: "bool"},
		},
	})}, nil)

	root := &cobra.Command{Use: "kapi"}
	initCmd := &cobra.Command{Use: "init", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(initCmd)

	var warns []string
	AttachContributions(root, host, func(m string) { warns = append(warns, m) })

	assert.Empty(t, warns)
	assert.NotNil(t, initCmd.Flags().Lookup("server"), "contributed --server flag should be registered")
	assert.NotNil(t, initCmd.Flags().Lookup("anonymous"), "contributed --anonymous flag should be registered")
}

func TestAttachContributions_unknownCommandWarns(t *testing.T) {
	host := pluginhost.NewHost([]*pluginhost.Plugin{contribPlugin("p", manifest.CommandContribution{
		Command: "does-not-exist", Handler: "h",
	})}, nil)

	root := &cobra.Command{Use: "kapi"}
	var warns []string
	AttachContributions(root, host, func(m string) { warns = append(warns, m) })

	require.Len(t, warns, 1)
	assert.Contains(t, warns[0], "not a built-in command")
}

func TestContributionEngaged(t *testing.T) {
	cmd := &cobra.Command{Use: "init"}
	cmd.Flags().String("server", "", "")
	cmd.Flags().Bool("anonymous", false, "")

	withEngage := manifest.CommandContribution{
		EngageWhen: "server",
		Flags:      []manifest.FlagSpec{{Name: "server", Type: "string"}, {Name: "anonymous", Type: "bool"}},
	}
	assert.False(t, contributionEngaged(cmd, withEngage), "not engaged before any flag is set")
	require.NoError(t, cmd.Flags().Set("server", "http://localhost:8080"))
	assert.True(t, contributionEngaged(cmd, withEngage), "engaged once the engage_when flag is set")

	// Without EngageWhen, any contributed flag engages it.
	anyFlag := manifest.CommandContribution{Flags: []manifest.FlagSpec{{Name: "anonymous", Type: "bool"}}}
	assert.False(t, contributionEngaged(&cobra.Command{Use: "x"}, anyFlag))
	require.NoError(t, cmd.Flags().Set("anonymous", "true"))
	assert.True(t, contributionEngaged(cmd, anyFlag))
}
