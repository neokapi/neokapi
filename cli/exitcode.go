package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/spf13/cobra"
)

// Run executes a Cobra root command with signal-aware context and proper
// exit codes. Both kapi and bowrain main() should call this. The optional
// cleanup functions are called before exiting (regardless of success/failure).
func Run(cmd *cobra.Command, cleanup ...func()) {
	ctx, stop := SignalContext(context.Background())

	cmd.SetContext(ctx)
	// ExecuteContextC returns the command that actually ran — its flag set
	// carries the parsed persistent output flags (--json/--jq), which the
	// error envelope below needs. The root's own flag set would not.
	executed, err := cmd.ExecuteContextC(ctx)

	stop()

	for _, fn := range cleanup {
		fn()
	}

	if err != nil {
		code := ExitCode(cmd, err)

		// Print the error ourselves since SilenceErrors is set. ErrSilentExit
		// carries the exit code but suppresses the message (grep-style status).
		if code != ExitSignal && !errors.Is(err, ErrSilentExit) {
			if executed == nil {
				executed = cmd
			}
			PrintCommandError(executed, err, code)
		}

		os.Exit(code)
	}
}

// PrintCommandError renders a command failure on stderr. In text mode it is
// the historical plain line ("Error: <message>"); under --json (or --jq /
// --output-format=json) it is the structured envelope
// {"error": "<message>", "code": "<symbol>"} so scripted callers never have
// to parse prose. Exit codes are unchanged either way — the code symbol
// mirrors them (see ErrorCodeString).
func PrintCommandError(cmd *cobra.Command, err error, code int) {
	if output.ResolveFormat(cmd) == output.FormatJSON {
		output.PrintError(cmd, err, ErrorCodeString(code))
		return
	}
	fmt.Fprintf(os.Stderr, "Error: %s\n", err)
}

// ErrorCodeString maps a process exit code to the stable machine-readable
// symbol carried in the JSON error envelope: "error" (1), "usage" (2),
// "gate" (3), "signal" (130); any other code renders as its decimal string.
func ErrorCodeString(code int) string {
	switch code {
	case ExitError:
		return "error"
	case ExitUsage:
		return "usage"
	case ExitGate:
		return "gate"
	case ExitSignal:
		return "signal"
	default:
		return strconv.Itoa(code)
	}
}
