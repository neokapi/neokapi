package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

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
func AttachContributions(parent *cobra.Command, host *Host, onWarn func(msg string)) {
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
			onWarn(fmt.Sprintf("plugin %q contributes to command %q, which is not a built-in command — skipping", route.Plugin.Name(), cc.Command))
			continue
		}

		for _, fl := range cc.Flags {
			if target.Flags().Lookup(fl.Name) != nil {
				onWarn(fmt.Sprintf("plugin %q flag --%s on command %q collides with an existing flag — skipping that flag", route.Plugin.Name(), fl.Name, cc.Command))
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
func dispatchContribution(cmd *cobra.Command, p *Plugin, cc manifest.CommandContribution) error {
	args := []string{"command", cc.Handler}
	for _, fl := range cc.Flags {
		if !cmd.Flags().Changed(fl.Name) {
			continue
		}
		args = append(args, forwardFlag(cmd, fl)...)
	}

	dir := contributionDir(cmd)
	return runContributionSubprocess(cmd.Context(), p, args, dir)
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

// runContributionSubprocess execs the plugin's contribution handler with the
// engaged flags. ctx is the cobra command's context (carrying signal/
// cancellation wiring); it is propagated to exec.CommandContext so that a
// SIGTERM/SIGINT to the kapi process (which cobra translates into a cancelled
// command context) terminates the plugin child instead of leaving it running
// until it finishes on its own.
func runContributionSubprocess(ctx context.Context, p *Plugin, args []string, dir string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, p.BinaryPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = dir

	env := os.Environ()
	env = append(env, "KAPI_PLUGIN_DIR="+p.Dir)
	env = append(env, "KAPI_PLUGIN_NAME="+p.Name())
	env = append(env, "KAPI_PLUGIN_VERSION="+p.Version())
	env = append(env, "KAPI_PROJECT_DIR="+dir)
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		// If the parent context was cancelled (e.g. SIGTERM/SIGINT to kapi),
		// exec.CommandContext has already killed the child. Don't mistake the
		// resulting non-zero exit for a real plugin exit code: surface the
		// context error so the caller stops cleanly.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("plugin %q contribution %q: %w", p.Name(), args[1], ctxErr)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return withPluginExitCode(exitErr.ExitCode(), fmt.Errorf("plugin %q contribution %q: %w", p.Name(), args[1], err))
		}
		return fmt.Errorf("plugin %q contribution %q: %w", p.Name(), args[1], err)
	}
	return nil
}
