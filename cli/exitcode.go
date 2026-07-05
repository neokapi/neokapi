package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

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

		// Print the error ourselves since SilenceErrors is set. ErrSilentExit
		// carries the exit code but suppresses the message (grep-style status).
		if code != ExitSignal && !errors.Is(err, ErrSilentExit) {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		}

		os.Exit(code)
	}
}
