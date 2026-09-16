package commands

import (
	"context"
	"errors"
	"time"

	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// Whether the work happened and whether this client watched it happen are two
// questions, and `up` answered both with one exit code. A run that went on to
// completion while the connection between here and the server closed was
// reported as a failed command: the nightly of 2026-09-16 lost four minutes of
// silence to a gateway, failed, and left the server finishing the run it had
// asked for.
//
// So the exit code answers the first question and these records answer the
// second. A reader who loses the stream still holds the run's id, which is what
// `kapi status` takes.

// Watch reasons. A complete watch saw the run's terminal frame; the others say
// what ended it.
const (
	watchDone        = "done"
	watchTimeout     = "timeout"
	watchStreamError = "stream_error"
)

// runStateDeadline bounds the re-read of a run's state after a watch ended
// early. The state in the record has to be what the server says now rather than
// the last frame this client saw, and a dropped watch must not turn into
// another long wait.
const runStateDeadline = 10 * time.Second

// watchOutcome is what happened to this client's view of the run.
type watchOutcome struct {
	// Complete reports that the stream reached the run's terminal event.
	Complete bool `json:"complete"`
	// Reason is done, timeout or stream_error.
	Reason string `json:"reason"`
	// Detail carries what went wrong when the state could not be re-read, so a
	// reader knows the state beside it is the last one seen.
	Detail string `json:"detail,omitempty"`
}

// runRef names a run the way `kapi status --json` names one, so the two
// surfaces agree and a consumer that lost the stream can look this run up or
// resume it.
type runRef struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Passes int    `json:"passes"`
}

// watchResult reads a stream error as an outcome for the record, and an error
// only when the command itself must stop.
//
// A deadline is the caller's own --timeout and has always meant "pull what
// landed". A gateway or transport failure says nothing about the run, so it
// reads the same way. Cancellation is the exception: a person pressing Ctrl-C
// asked this command to stop, and cli.Run turns that into a cancelled context,
// so carrying on to pull and report would ignore them.
func watchResult(err error) (watchOutcome, error) {
	switch {
	case err == nil:
		return watchOutcome{Complete: true, Reason: watchDone}, nil
	case errors.Is(err, context.Canceled):
		return watchOutcome{}, err
	case errors.Is(err, context.DeadlineExceeded):
		return watchOutcome{Reason: watchTimeout}, nil
	default:
		return watchOutcome{Reason: watchStreamError, Detail: err.Error()}, nil
	}
}

// runStateNow re-reads the run so the record carries the state the server holds
// rather than the last frame this client saw. It is bounded, and a failure
// leaves the run as last seen and says so in the outcome.
func runStateNow(ctx context.Context, client *apiclient.BowrainClient, run *apiclient.ConvergenceRun, watch *watchOutcome) *apiclient.ConvergenceRun {
	if client == nil || run == nil {
		return run
	}
	readCtx := ctx
	if !watch.Complete {
		// A watch that ended early gets one short attempt: the state in the
		// record has to be accounted for, and a dropped watch must not become
		// another long wait. A complete watch is in no hurry and keeps the
		// caller's own context.
		var cancel context.CancelFunc
		readCtx, cancel = context.WithTimeout(ctx, runStateDeadline)
		defer cancel()
	}

	current, err := client.GetConvergenceRun(readCtx, run.ID)
	if err != nil {
		if !watch.Complete && watch.Detail == "" {
			watch.Detail = "the run's state could not be re-read: " + err.Error()
		}
		return run
	}
	return current
}
