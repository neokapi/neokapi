package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/spf13/cobra"
)

// AttachOptions configures how AttachCommands wires plugin commands.
type AttachOptions struct {
	// OnConflict is invoked when a plugin command shadows a built-in.
	OnConflict func(msg string)

	// DaemonPool, when non-nil, enables Mode-C dispatch for source-
	// connector commands. The pool spawns daemons on demand and a
	// per-plugin SourceConnectorDispatcher (registered via
	// RegisterSourceConnectorDispatcher) handles the actual RPC.
	DaemonPool *DaemonPool
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
func AttachCommands(parent *cobra.Command, host *Host, onConflict func(msg string)) {
	AttachCommandsWithOptions(parent, host, AttachOptions{OnConflict: onConflict})
}

// AttachCommandsWithOptions is the AttachCommands variant that takes
// AttachOptions. Prefer this in new code.
func AttachCommandsWithOptions(parent *cobra.Command, host *Host, opts AttachOptions) {
	if host == nil {
		return
	}
	onConflict := opts.OnConflict
	if onConflict == nil {
		onConflict = func(string) {}
	}

	// Build the set of built-in command names so we can shadow-warn.
	builtin := map[string]bool{}
	for _, c := range parent.Commands() {
		builtin[c.Name()] = true
	}

	for _, route := range host.CommandRoutes() {
		if builtin[route.Command.Name] {
			onConflict(fmt.Sprintf("command %q from plugin %q is shadowed by a built-in command", route.Command.Name, route.Plugin.Name()))
			continue
		}
		parent.AddCommand(buildCobraCommandWithDispatch(route, opts.DaemonPool))
	}
}

// buildCobraCommandWithDispatch synthesizes a cobra.Command from one
// CommandRoute. It uses RunE with cobra's "DisableFlagParsing" so the
// plugin sees the raw argv — kapi only routes; the plugin parses.
//
// When pool is non-nil and the plugin declares Mode-C with a registered
// SourceConnectorDispatcher claiming this op, the command is routed
// over the daemon's gRPC connection instead of spawning a fresh Mode-A
// subprocess. When pool is nil (or no dispatcher is registered), the
// command falls through to the legacy Mode-A subprocess path.
func buildCobraCommandWithDispatch(route *CommandRoute, pool *DaemonPool) *cobra.Command {
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
			if pool != nil && SupportsModeCDispatch(route.Plugin.Name(), c.Name) &&
				route.Plugin.Manifest.Daemon != nil {
				return DispatchViaDaemon(cmd.Context(), pool, route.Plugin, c.Name, args)
			}
			return execPluginCommand(cmd.Context(), route, args)
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
func buildSubcommandTree(route *CommandRoute, parentPath []string, sub manifest.Subcommand) *cobra.Command {
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
			return execPluginSubcommandPath(cmd.Context(), route, path[1:], args)
		},
	}
	for _, child := range sub.Subcommands {
		subCmd.AddCommand(buildSubcommandTree(route, path, child))
	}
	return subCmd
}

// execPluginCommand runs a top-level Mode-A command:
//
//	<binary> command <name> [args]
//
// ctx is the cobra command's context (carrying signal/cancellation
// wiring); cancelling it kills the plugin subprocess.
func execPluginCommand(ctx context.Context, route *CommandRoute, args []string) error {
	cmdArgs := append([]string{"command", route.Command.Name}, args...)
	return runSubprocess(ctx, route.Plugin, cmdArgs)
}

// execPluginSubcommandPath runs a Mode-A subcommand at an arbitrary depth
// under a parent command:
//
//	<binary> command <parent> <sub...> [args]
//
// subPath is the chain of subcommand names below the top-level command
// (e.g. ["token", "create"] for `auth token create`).
//
// ctx is the cobra command's context (carrying signal/cancellation
// wiring); cancelling it kills the plugin subprocess.
func execPluginSubcommandPath(ctx context.Context, route *CommandRoute, subPath []string, args []string) error {
	cmdArgs := append([]string{"command", route.Command.Name}, subPath...)
	cmdArgs = append(cmdArgs, args...)
	return runSubprocess(ctx, route.Plugin, cmdArgs)
}

// runSubprocess execs the plugin binary with args, inheriting the
// process's stdio. ctx is propagated to exec.CommandContext so that a
// SIGTERM/SIGINT to the kapi process (which cobra translates into a
// cancelled command context) terminates the plugin child instead of
// leaving it running until it finishes on its own.
func runSubprocess(ctx context.Context, p *Plugin, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, p.BinaryPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Pass useful context to the plugin via env. The plugin's argv
	// already carries the user's intent; env carries kapi-side state.
	env := os.Environ()
	env = append(env, "KAPI_PLUGIN_DIR="+p.Dir)
	env = append(env, "KAPI_PLUGIN_NAME="+p.Name())
	env = append(env, "KAPI_PLUGIN_VERSION="+p.Version())
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		// If the parent context was cancelled (e.g. SIGTERM/SIGINT to
		// kapi), exec.CommandContext has already killed the child. Don't
		// mistake the resulting non-zero exit for a real plugin exit code:
		// surface the context error so the caller stops cleanly.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("plugin %q: %w", p.Name(), ctxErr)
		}
		// Propagate exit codes cleanly: cobra surfaces *exec.ExitError
		// with the right exit code through SilenceErrors+SilenceUsage.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("plugin %q: %w", p.Name(), err)
	}
	return nil
}

// FormatHelpLine returns a single-line summary of a command route used
// by `kapi plugin list` and similar UI surfaces.
func FormatHelpLine(c manifest.Command, plugin *Plugin) string {
	if c.Short != "" {
		return fmt.Sprintf("%s — %s [%s]", c.Name, c.Short, plugin.Name())
	}
	return fmt.Sprintf("%s [%s]", c.Name, plugin.Name())
}
