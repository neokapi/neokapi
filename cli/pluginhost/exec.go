package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/spf13/cobra"
)

// AttachCommands adds one cobra command to parent for each Mode-A
// command provided by any plugin. The command's RunE execs the plugin
// binary with stdin/stdout/stderr inherited; the plugin is responsible
// for argument and flag parsing.
//
// Conflicting commands (between plugins, or between a plugin and a
// built-in) are reported via onConflict and not registered.
func AttachCommands(parent *cobra.Command, host *Host, onConflict func(msg string)) {
	if host == nil {
		return
	}
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
		parent.AddCommand(buildCobraCommand(route))
	}
}

// buildCobraCommand synthesizes a cobra.Command from one CommandRoute.
// It uses RunE with cobra's "DisableFlagParsing" so the plugin sees
// the raw argv — kapi only routes; the plugin parses.
func buildCobraCommand(route *CommandRoute) *cobra.Command {
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
			return execPluginCommand(route, args)
		},
	}

	// Stitch declared subcommand names onto the cobra tree as
	// pass-through children, so the right --help and shell completion
	// shows up. Each child inherits DisableFlagParsing and execs the
	// plugin with the full subcommand path.
	for _, sub := range c.Subcommands {
		subName := sub
		subCmd := &cobra.Command{
			Use:                subName,
			Short:              c.Name + " subcommand",
			DisableFlagParsing: true,
			Annotations:        map[string]string{"plugin": route.Plugin.Name()},
			RunE: func(_ *cobra.Command, args []string) error {
				return execPluginSubcommand(route, subName, args)
			},
		}
		cmd.AddCommand(subCmd)
	}
	return cmd
}

// execPluginCommand runs a top-level Mode-A command:
//
//	<binary> command <name> [args]
func execPluginCommand(route *CommandRoute, args []string) error {
	cmdArgs := append([]string{"command", route.Command.Name}, args...)
	return runSubprocess(route.Plugin, cmdArgs)
}

// execPluginSubcommand runs a Mode-A subcommand under a parent command:
//
//	<binary> command <parent> <subcommand> [args]
func execPluginSubcommand(route *CommandRoute, subName string, args []string) error {
	cmdArgs := append([]string{"command", route.Command.Name, subName}, args...)
	return runSubprocess(route.Plugin, cmdArgs)
}

func runSubprocess(p *Plugin, args []string) error {
	cmd := exec.CommandContext(context.Background(), p.BinaryPath, args...)
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
