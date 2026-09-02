// Package pluginattach wires manifest-driven plugin commands and command
// contributions onto a cobra command tree. It is the cobra-facing rim of
// host/pluginhost: routing, subprocess execution, and daemon dispatch stay
// in the host module; only the cobra attachment lives here.
package pluginattach

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// AttachOptions configures how AttachCommands wires plugin commands.
type AttachOptions struct {
	// OnConflict is invoked when a plugin command shadows a built-in.
	OnConflict func(msg string)

	// DaemonPool, when non-nil, enables Mode-C dispatch for source-
	// connector commands. The pool spawns daemons on demand and a
	// per-plugin SourceConnectorDispatcher (registered via
	// RegisterSourceConnectorDispatcher) handles the actual RPC.
	DaemonPool *pluginhost.DaemonPool
}

// AttachCommands adds one cobra command to parent for each Mode-A
// command provided by any plugin. The command's RunE execs the plugin
// binary with stdin/stdout/stderr inherited; the plugin is responsible
// for argument and flag parsing.
//
// When AttachOptions.DaemonPool is set AND the plugin declares a Mode-C
// daemon block AND a SourceConnectorDispatcher is registered for the
// plugin's name AND it claims this op, the command is routed via the
// daemon instead of spawning a fresh subprocess. This makes successive
// kapi push / kapi pull invocations reuse a single daemon process,
// eliminating per-call startup cost.
//
// Conflicting commands (between plugins, or between a plugin and a
// built-in) are reported via onConflict and not registered.
func AttachCommands(parent *cobra.Command, host *pluginhost.Host, onConflict func(msg string)) {
	AttachCommandsWithOptions(parent, host, AttachOptions{OnConflict: onConflict})
}

// AttachCommandsWithOptions is the AttachCommands variant that takes
// AttachOptions. Prefer this in new code.
//
// Attachment follows the no-shadowing rule (plugins extend kapi, they never
// override it):
//
//   - Every plugin command also attaches under a per-plugin group command
//     (`kapi <group> <verb>`, e.g. `kapi bowrain config`), giving docs an
//     unambiguous spelling. The group is the manifest's plugin-level `group`
//     field, falling back to the plugin name.
//   - A plugin command whose name collides with a built-in attaches under
//     the group ONLY — the built-in always keeps the top-level verb. This is
//     a supported layout, not an error, so it does not warn per invocation.
//     (Plugin-vs-plugin name collisions never reach here: pluginhost.NewHost
//     drops both routes and reports the conflict.)
//   - Commands marked hidden in the manifest (dispatch plumbing such as
//     bowrain's server-status / server-up) attach hidden: routable, absent
//     from --help and completion.
func AttachCommandsWithOptions(parent *cobra.Command, host *pluginhost.Host, opts AttachOptions) {
	if host == nil {
		return
	}
	onConflict := opts.OnConflict
	if onConflict == nil {
		onConflict = func(string) {}
	}

	// The set of command names already taken at the top level (built-ins
	// first, then plugin commands as they attach).
	taken := map[string]bool{}
	for _, c := range parent.Commands() {
		taken[c.Name()] = true
	}

	// Per-plugin group parents (`kapi bowrain …`), created lazily.
	groups := map[string]*cobra.Command{}
	groupFor := func(route *pluginhost.CommandRoute) *cobra.Command {
		name := route.Plugin.Manifest.Group
		if name == "" {
			name = route.Plugin.Name()
		}
		if g, ok := groups[name]; ok {
			return g
		}
		g := &cobra.Command{
			Use:         name,
			Short:       fmt.Sprintf("Commands from the %s plugin", route.Plugin.Name()),
			Annotations: map[string]string{"plugin": route.Plugin.Name()},
		}
		groups[name] = g
		if taken[name] {
			// A group whose name collides with a built-in cannot attach; its
			// members stay top-level-only. This should never happen for
			// well-named plugins.
			onConflict(fmt.Sprintf("plugin group %q collides with a built-in command, so the group spelling is unavailable", name))
		} else {
			parent.AddCommand(g)
			taken[name] = true
		}
		return g
	}

	for _, route := range host.CommandRoutes() {
		// Group spelling first: always available, collision-free by scope.
		group := groupFor(route)
		groupChild := buildCobraCommandWithDispatch(route, opts.DaemonPool)
		groupChild.Hidden = route.Command.Hidden
		group.AddCommand(groupChild)

		if taken[route.Command.Name] {
			// The built-in keeps the verb; the plugin's version lives in its
			// group. Supported layout — no per-invocation warning. (Routes are
			// name-unique: plugin-vs-plugin collisions are already resolved,
			// with a conflict report, in pluginhost.NewHost.)
			continue
		}
		top := buildCobraCommandWithDispatch(route, opts.DaemonPool)
		top.Hidden = route.Command.Hidden
		parent.AddCommand(top)
		taken[route.Command.Name] = true
	}

	// Drop group parents that ended up with only hidden children: an empty-
	// looking `kapi bowrain` entry in --help would be noise.
	for _, g := range groups {
		visible := false
		for _, c := range g.Commands() {
			if !c.Hidden {
				visible = true
				break
			}
		}
		if !visible {
			g.Hidden = true
		}
	}
}

// buildCobraCommandWithDispatch synthesizes a cobra.Command from one
// pluginhost.CommandRoute. It uses RunE with cobra's "DisableFlagParsing" so the
// plugin sees the raw argv — kapi only routes; the plugin parses.
//
// When pool is non-nil and the plugin declares Mode-C with a registered
// SourceConnectorDispatcher claiming this op, the command is routed
// over the daemon's gRPC connection instead of spawning a fresh Mode-A
// subprocess. When pool is nil (or no dispatcher is registered), the
// command falls through to the legacy Mode-A subprocess path.
func buildCobraCommandWithDispatch(route *pluginhost.CommandRoute, pool *pluginhost.DaemonPool) *cobra.Command {
	c := route.Command
	cmd := &cobra.Command{
		Use:                c.Name,
		Short:              c.Short,
		Long:               c.Long,
		DisableFlagParsing: true,
		// Annotate with the source plugin so --help can show "[bowrain]".
		Annotations: map[string]string{
			"plugin": route.Plugin.Name(),
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if pool != nil && pluginhost.SupportsModeCDispatch(route.Plugin.Name(), c.Name) &&
				route.Plugin.Manifest.Daemon != nil {
				return pluginhost.DispatchViaDaemon(cmd.Context(), pool, route.Plugin, c.Name, args)
			}
			return pluginhost.ExecPluginCommand(cmd.Context(), route, args)
		},
	}

	// Stitch declared subcommands onto the cobra tree as pass-through
	// children, so the right --help and shell completion shows up. Each
	// child inherits DisableFlagParsing and execs the plugin with the full
	// subcommand path. Subcommands may nest (e.g. "auth token create").
	for _, sub := range c.Subcommands {
		cmd.AddCommand(buildSubcommandTree(route, []string{c.Name}, sub))
	}
	return cmd
}

// buildSubcommandTree synthesizes a cobra subcommand (and, recursively, its
// children) from a manifest Subcommand. parentPath is the chain of command
// names from the top-level command down to (but not including) sub, so the
// plugin is invoked with the full path: <binary> command <parentPath...> <sub> [args].
func buildSubcommandTree(route *pluginhost.CommandRoute, parentPath []string, sub manifest.Subcommand) *cobra.Command {
	// path is the full command chain including this subcommand.
	path := append(append([]string{}, parentPath...), sub.Name)
	pluginName := route.Plugin.Name()
	subCmd := &cobra.Command{
		Use:                sub.Name,
		Short:              strings.Join(path, " ") + " subcommand",
		DisableFlagParsing: true,
		Annotations:        map[string]string{"plugin": pluginName},
		RunE: func(cmd *cobra.Command, args []string) error {
			// path[0] is the top-level command; the rest is the subcommand chain.
			return pluginhost.ExecPluginSubcommandPath(cmd.Context(), route, path[1:], args)
		},
	}
	for _, child := range sub.Subcommands {
		subCmd.AddCommand(buildSubcommandTree(route, path, child))
	}
	return subCmd
}

// AttachContributions wires plugin command-contributions onto kapi's built-in
// commands. For each contribution it:
//
//  1. registers the contributed flags on the matching built-in command, and
//  2. wraps the command's RunE so that, after the built-in action runs and when
//     the contribution is engaged, kapi dispatches the plugin's handler.
//
// Contributions augment a built-in command rather than replacing it, so plain
// invocations (no contributed flags) keep their original behavior. Handlers run
// as Mode-A subprocesses in the command's project directory and must be
// idempotent — kapi may dispatch the same handler on a project that is already
// in the desired state.
//
// Call this AFTER built-in commands and AttachCommands have populated the tree.
func AttachContributions(parent *cobra.Command, host *pluginhost.Host, onWarn func(msg string)) {
	if host == nil {
		return
	}
	if onWarn == nil {
		onWarn = func(string) {}
	}

	byName := map[string]*cobra.Command{}
	for _, c := range parent.Commands() {
		byName[c.Name()] = c
	}

	for _, route := range host.ContributionRoutes() {
		cc := route.Contribution
		target, ok := byName[cc.Command]
		if !ok {
			onWarn(fmt.Sprintf("plugin %q contributes to command %q, which is not a built-in command, skipping", route.Plugin.Name(), cc.Command))
			continue
		}

		for _, fl := range cc.Flags {
			if target.Flags().Lookup(fl.Name) != nil {
				onWarn(fmt.Sprintf("plugin %q flag --%s on command %q collides with an existing flag, skipping that flag", route.Plugin.Name(), fl.Name, cc.Command))
				continue
			}
			registerContributedFlag(target.Flags(), fl)
		}

		plugin := route.Plugin
		orig := target.RunE
		origRun := target.Run
		target.RunE = func(cmd *cobra.Command, args []string) error {
			switch {
			case orig != nil:
				if err := orig(cmd, args); err != nil {
					return err
				}
			case origRun != nil:
				origRun(cmd, args)
			}
			if !contributionEngaged(cmd, cc) {
				return nil
			}
			return dispatchContribution(cmd, plugin, cc)
		}
		target.Run = nil
	}
}

// registerContributedFlag adds one declared flag to a flag set.
func registerContributedFlag(fs *pflag.FlagSet, fl manifest.FlagSpec) {
	switch fl.Type {
	case "bool":
		def, _ := fl.Default.(bool)
		fs.BoolP(fl.Name, fl.Short, def, fl.Description)
	case "int":
		def := 0
		if f, ok := fl.Default.(float64); ok { // JSON numbers decode as float64
			def = int(f)
		}
		fs.IntP(fl.Name, fl.Short, def, fl.Description)
	case "stringSlice":
		fs.StringSliceP(fl.Name, fl.Short, nil, fl.Description)
	default: // "string"
		def, _ := fl.Default.(string)
		fs.StringP(fl.Name, fl.Short, def, fl.Description)
	}
}

// contributionEngaged reports whether the user invoked the contribution: the
// EngageWhen flag was set, or — when EngageWhen is empty — any contributed flag.
func contributionEngaged(cmd *cobra.Command, cc manifest.CommandContribution) bool {
	if cc.EngageWhen != "" {
		return cmd.Flags().Changed(cc.EngageWhen)
	}
	for _, fl := range cc.Flags {
		if cmd.Flags().Changed(fl.Name) {
			return true
		}
	}
	return false
}

// dispatchContribution execs the plugin handler with the engaged flags, in the
// command's resolved project directory.
//
//	<binary> command <handler> [--flag value ...]
func dispatchContribution(cmd *cobra.Command, p *pluginhost.Plugin, cc manifest.CommandContribution) error {
	args := []string{"command", cc.Handler}
	for _, fl := range cc.Flags {
		if !cmd.Flags().Changed(fl.Name) {
			continue
		}
		args = append(args, forwardFlag(cmd, fl)...)
	}

	dir := contributionDir(cmd)
	return pluginhost.RunContributionSubprocess(cmd.Context(), p, args, dir)
}

// forwardFlag renders one engaged flag as argv for the handler.
func forwardFlag(cmd *cobra.Command, fl manifest.FlagSpec) []string {
	name := "--" + fl.Name
	switch fl.Type {
	case "bool":
		if v, _ := cmd.Flags().GetBool(fl.Name); v {
			return []string{name}
		}
		return []string{name + "=false"}
	case "int":
		v, _ := cmd.Flags().GetInt(fl.Name)
		return []string{name, strconv.Itoa(v)}
	case "stringSlice":
		v, _ := cmd.Flags().GetStringSlice(fl.Name)
		return []string{name, strings.Join(v, ",")}
	default: // "string"
		v, _ := cmd.Flags().GetString(fl.Name)
		return []string{name, v}
	}
}

// contributionDir resolves the project directory the contribution should run
// in: a `--dir` flag if the built-in command has one, otherwise the cwd.
func contributionDir(cmd *cobra.Command) string {
	dir := ""
	if f := cmd.Flags().Lookup("dir"); f != nil {
		dir = f.Value.String()
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return dir
}
