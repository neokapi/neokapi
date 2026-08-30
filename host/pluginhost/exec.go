package pluginhost

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// ExecPluginCommand runs a top-level Mode-A command:
//
//	<binary> command <name> [args]
//
// ctx is the cobra command's context (carrying signal/cancellation
// wiring); cancelling it kills the plugin subprocess.
func ExecPluginCommand(ctx context.Context, route *CommandRoute, args []string) error {
	cmdArgs := append([]string{"command", route.Command.Name}, args...)
	return runSubprocess(ctx, route.Plugin, cmdArgs)
}

// ExecPluginSubcommandPath runs a Mode-A subcommand at an arbitrary depth
// under a parent command:
//
//	<binary> command <parent> <sub...> [args]
//
// subPath is the chain of subcommand names below the top-level command
// (e.g. ["token", "create"] for `auth token create`).
//
// ctx is the cobra command's context (carrying signal/cancellation
// wiring); cancelling it kills the plugin subprocess.
func ExecPluginSubcommandPath(ctx context.Context, route *CommandRoute, subPath []string, args []string) error {
	cmdArgs := append([]string{"command", route.Command.Name}, subPath...)
	cmdArgs = append(cmdArgs, args...)
	return runSubprocess(ctx, route.Plugin, cmdArgs)
}

// runSubprocess execs the plugin binary with args, inheriting the process's
// stdio. ctx is propagated to exec.CommandContext so that a SIGTERM/SIGINT to
// the kapi process (which cobra translates into a cancelled command context)
// terminates the plugin child instead of leaving it running until it finishes
// on its own.
func runSubprocess(ctx context.Context, p *Plugin, args []string) error {
	_, err := execPlugin(ctx, p, args, false)
	return err
}

// CaptureStdout runs the route's top-level Mode-A command capturing its
// stdout, instead of inheriting the parent's stdio. It is the plumbing behind
// data-returning command contributions (e.g. the built-in `kapi status`
// shelling `server-status --json` to merge a server section). The plugin's
// stderr is discarded — a plumbing caller degrades gracefully rather than
// leaking plugin diagnostics into a structured host output.
func (r *CommandRoute) CaptureStdout(ctx context.Context, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"command", r.Command.Name}, args...)
	return execPlugin(ctx, r.Plugin, cmdArgs, true)
}

// StreamStdout runs the route's top-level Mode-A command and delivers its
// stdout to onLine one line at a time, as the child writes it, rather than
// buffering the whole document first. The trailing newline is stripped; a final
// line without one is still delivered.
//
// It is what a caller that must react to a long-running plumbing command while
// it runs uses instead of CaptureStdout: a progress feed read after the
// subprocess exits is a transcript, and a UI rendering one shows a spinner for
// the length of a convergence run and then every event at once.
//
// onLine runs on the reading goroutine, so a slow handler backpressures the
// child. The plugin's stderr is discarded, as CaptureStdout discards it.
func (r *CommandRoute) StreamStdout(ctx context.Context, onLine func([]byte), args ...string) error {
	cmdArgs := append([]string{"command", r.Command.Name}, args...)
	return streamPlugin(ctx, r.Plugin, cmdArgs, onLine)
}

// pluginCommand builds the subprocess every launch mode shares: the same
// binary, the same cancellation wiring, and the same environment.
//
// Pass useful context to the plugin via env. The plugin's argv already carries
// the user's intent; env carries kapi-side state — minus the provider API keys,
// which are the host's to spend (see env.go).
//
//nolint:contextcheck // the nil-ctx guard is an API fallback for embedded/desktop callers; ctx is otherwise threaded straight into exec.CommandContext
func pluginCommand(ctx context.Context, p *Plugin, args []string) *exec.Cmd {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, p.BinaryPath, args...)
	env := pluginEnviron()
	env = append(env, "KAPI_PLUGIN_DIR="+p.Dir)
	env = append(env, "KAPI_PLUGIN_NAME="+p.Name())
	env = append(env, "KAPI_PLUGIN_VERSION="+p.Version())
	cmd.Env = env
	return cmd
}

// pluginRunError maps a failed subprocess onto the error a caller should see.
//
// It is one function on purpose: the capturing launch mode used to be a copy of
// the inheriting one, and the copy had quietly lost exit-code propagation — a
// plugin that failed under CaptureStdout reported a generic error and kapi
// exited 1 instead of the plugin's own code. Every mode maps its failure here.
func pluginRunError(ctx context.Context, p *Plugin, err error) error {
	if err == nil {
		return nil
	}
	// If the parent context was cancelled (e.g. SIGTERM/SIGINT to kapi),
	// exec.CommandContext has already killed the child. Don't mistake the
	// resulting non-zero exit for a real plugin exit code: surface the
	// context error so the caller stops cleanly.
	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("plugin %q: %w", p.Name(), ctxErr)
		}
	}
	// Propagate exit codes cleanly: return an error that carries the plugin's
	// exit code so cli.Run's ExitCode() emits the right code via the exitCoder
	// interface, without bypassing App.Shutdown.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return withPluginExitCode(exitErr.ExitCode(), fmt.Errorf("plugin %q: %w", p.Name(), err))
	}
	return fmt.Errorf("plugin %q: %w", p.Name(), err)
}

// execPlugin runs a plugin subprocess to completion. When capture is false the
// child inherits the host's stdio; when true its stdout is buffered and
// returned (stdin closed, stderr discarded).
func execPlugin(ctx context.Context, p *Plugin, args []string, capture bool) ([]byte, error) {
	cmd := pluginCommand(ctx, p, args)

	var out bytes.Buffer
	if capture {
		cmd.Stdin = nil
		cmd.Stdout = &out
		cmd.Stderr = io.Discard
	} else {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		if ctxErr := pluginRunError(ctx, p, err); ctxErr != nil {
			// A cancelled run has no output worth returning; a failed one may.
			if ctx != nil && ctx.Err() != nil {
				return nil, ctxErr
			}
			return out.Bytes(), ctxErr
		}
	}
	return out.Bytes(), nil
}

// streamPlugin runs a plugin subprocess and hands each stdout line to onLine as
// it arrives. stdin is closed and stderr discarded, matching the capturing mode:
// a plumbing caller reads a structured document, not a terminal rendering.
func streamPlugin(ctx context.Context, p *Plugin, args []string, onLine func([]byte)) error {
	cmd := pluginCommand(ctx, p, args)
	cmd.Stdin = nil
	cmd.Stderr = io.Discard

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("plugin %q: open stdout: %w", p.Name(), err)
	}
	if err := cmd.Start(); err != nil {
		return pluginRunError(ctx, p, err)
	}

	// bufio.Reader.ReadBytes rather than a Scanner: a run's closing result
	// record carries the whole ConvergeOutput on one line, and a Scanner's
	// default 64KiB token limit would turn a large project's result into a
	// "token too long" error at the very end of an otherwise finished run.
	reader := bufio.NewReader(pipe)
	for {
		line, readErr := reader.ReadBytes('\n')
		if trimmed := bytes.TrimRight(line, "\r\n"); len(trimmed) > 0 && onLine != nil {
			onLine(trimmed)
		}
		if readErr != nil {
			break
		}
	}

	// Wait closes the pipe, so it must follow the read loop to its end.
	return pluginRunError(ctx, p, cmd.Wait())
}

// pluginExitError carries the exit code from a plugin subprocess so that
// cli.ExitCode (which checks for the exitCoder interface) can propagate it
// through the normal cobra error path — avoiding os.Exit inside RunE.
// Implementing ExitCode() satisfies cli.exitCoder without importing the cli
// package (which would create an import cycle).
type pluginExitError struct {
	code int
	err  error
}

func (e *pluginExitError) Error() string { return e.err.Error() }
func (e *pluginExitError) Unwrap() error { return e.err }
func (e *pluginExitError) ExitCode() int { return e.code }

// withPluginExitCode wraps err with an exit code, mirroring cli.WithExitCode
// but defined here to avoid an import cycle with the parent cli package.
func withPluginExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &pluginExitError{code: code, err: err}
}

// FormatHelpLine returns a single-line summary of a command route used
// by `kapi plugin list` and similar UI surfaces.
func FormatHelpLine(c manifest.Command, plugin *Plugin) string {
	if c.Short != "" {
		return fmt.Sprintf("%s — %s [%s]", c.Name, c.Short, plugin.Name())
	}
	return fmt.Sprintf("%s [%s]", c.Name, plugin.Name())
}
