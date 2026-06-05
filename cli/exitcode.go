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

// ErrSilentExit requests a non-zero exit (ExitError) with no "Error:" message
// printed — for tools that use exit status as a result channel rather than a
// failure signal (e.g. `kgrep` exits 1 when nothing matched). The command is
// responsible for writing any output of its own before returning it.
var ErrSilentExit = errors.New("")

// exitCoder is implemented by any error that carries an explicit process exit
// code. ExitCode checks for this interface via errors.As so that packages
// inside cli/ (e.g. cli/pluginhost) can return a conforming type without
// importing the cli package directly (which would create a cycle).
type exitCoder interface {
	ExitCode() int
}

// exitCodeError wraps an error with an explicit process exit code. A command
// returns one (via WithExitCode) when it needs a specific code — e.g. the
// toolbox utilities' grep-style "2 on trouble" — while still printing the
// underlying message. ExitCode honors it.
type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string { return e.err.Error() }
func (e *exitCodeError) Unwrap() error { return e.err }
func (e *exitCodeError) ExitCode() int { return e.code }

// WithExitCode tags err with an explicit process exit code for ExitCode to
// return. It returns nil when err is nil.
func WithExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &exitCodeError{code: code, err: err}
}

// SignalContext returns a context that is cancelled on SIGINT or SIGTERM,
// along with a stop function that must be called to release resources.
func SignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

// ExitCode determines the appropriate exit code for the given error.
// It returns ExitSignal for context cancellation (Ctrl-C), ExitGate for a
// failed quality/brand gate, an explicit code for errors tagged via
// WithExitCode (e.g. the toolbox utilities mapping operational trouble to
// ExitUsage), and ExitError for all other errors.
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

	// An explicit exit code requested by the command (e.g. a toolbox utility
	// mapping operational trouble to ExitUsage). The check uses the exitCoder
	// interface so that sub-packages (e.g. pluginhost) can return their own
	// conforming error type without importing cli and causing a cycle.
	var coded exitCoder
	if errors.As(err, &coded) {
		return coded.ExitCode()
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

		// Print the error ourselves since SilenceErrors is set. ErrSilentExit
		// carries the exit code but suppresses the message (grep-style status).
		if code != ExitSignal && !errors.Is(err, ErrSilentExit) {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		}

		os.Exit(code)
	}
}
