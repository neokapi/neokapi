//go:build !js

package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// RunContributionSubprocess execs the plugin's contribution handler with the
// engaged flags. ctx is the cobra command's context, which carries the signal
// and cancellation wiring. It is propagated to exec.CommandContext, so a
// SIGTERM or SIGINT to kapi (which cobra turns into a cancelled context) ends
// the plugin child instead of leaving it to run until it finishes.
//
//nolint:contextcheck // the nil-ctx guard is an API fallback for embedded/desktop callers; ctx is otherwise threaded straight into exec.CommandContext
func RunContributionSubprocess(ctx context.Context, p *Plugin, args []string, dir string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := CheckLaunch(p); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, p.BinaryPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = dir

	env := pluginEnviron()
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
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return withPluginExitCode(exitErr.ExitCode(), fmt.Errorf("plugin %q contribution %q: %w", p.Name(), args[1], err))
		}
		return fmt.Errorf("plugin %q contribution %q: %w", p.Name(), args[1], err)
	}
	return nil
}
