package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// Exit codes following Unix conventions.
const (
	ExitOK     = 0
	ExitError  = 1
	ExitUsage  = 2
	ExitGate   = 3   // a quality/brand gate failed (distinct from operational error)
	ExitSignal = 130 // 128 + SIGINT(2)
)

// ErrQualityGate signals that a quality/brand gate (e.g. `kapi brand check
// --min-score`) failed. Commands return it so skills and CI can distinguish a
// failed gate (ExitGate) from an operational error (ExitError). Output is still
// written normally before the command returns this sentinel.
var ErrQualityGate = errors.New("quality gate failed")

// SignalContext returns a context that is cancelled on SIGINT or SIGTERM,
// along with a stop function that must be called to release resources.
func SignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

// ExitCode determines the appropriate exit code for the given error.
// It returns ExitSignal for context cancellation (Ctrl-C), ExitUsage for
// usage/flag errors, and ExitError for all other errors.
func ExitCode(_ *cobra.Command, err error) int {
	if err == nil {
		return ExitOK
	}

	// Signal-based cancellation (Ctrl-C).
	if errors.Is(err, context.Canceled) {
		return ExitSignal
	}

	// Quality/brand gate failure gets a distinct code.
	if errors.Is(err, ErrQualityGate) {
		return ExitGate
	}

	return ExitError
}

// Run executes a Cobra root command with signal-aware context and proper
// exit codes. Both kapi and bowrain main() should call this. The optional
// cleanup functions are called before exiting (regardless of success/failure).
func Run(cmd *cobra.Command, cleanup ...func()) {
	ctx, stop := SignalContext(context.Background())

	cmd.SetContext(ctx)
	err := cmd.ExecuteContext(ctx)

	stop()

	for _, fn := range cleanup {
		fn()
	}

	if err != nil {
		code := ExitCode(cmd, err)

		// Print the error ourselves since SilenceErrors is set.
		if code != ExitSignal {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		}

		os.Exit(code)
	}
}
