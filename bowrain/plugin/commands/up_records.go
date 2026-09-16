package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// The run's id travels as data, not only as a sentence on stderr. A consumer
// that loses the stream still holds the handle `kapi status` takes, and both
// surfaces spell a run the same way: id, state, passes.

// emitRunStarted names the run on the NDJSON document as soon as the server has
// one, so a consumer holds the handle before anything can interrupt the watch.
// The stream is the one this client is working on, which the client answers for.
func emitRunStarted(stream *output.NDJSONStream, run *apiclient.ConvergenceRun, streamName string) {
	if stream == nil || run == nil {
		return
	}
	stream.Emit(struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		Stream string `json:"stream,omitempty"`
	}{Type: "run_started", ID: run.ID, Stream: streamName})
}

// writeServerUpResult closes the document the way cli.PrintUpResultStream does,
// with the run and the watch beside the result.
//
// The body is the same cli.ConvergeOutput a local run produces, embedded and
// unchanged, because that type is shared with the local venue and every field a
// CI consumer reads lives in it. What a server run knows on top is who the run
// was and whether this client saw all of it, and those are the venue's own
// concern rather than the shared summary's.
func writeServerUpResult(cmd *cobra.Command, stream *output.NDJSONStream, out cli.ConvergeOutput, run *apiclient.ConvergenceRun, watch watchOutcome) error {
	if !flagBool(cmd, "json") {
		return out.FormatText(cmd.OutOrStdout())
	}
	if stream == nil {
		stream = output.NewNDJSONStream(cmd.OutOrStdout())
	}
	var ref *runRef
	if run != nil {
		ref = &runRef{ID: run.ID, State: run.State, Passes: run.Passes}
	}
	// The record's own failure is carried by the stream; Report renders it with
	// the count of what got through, for every path.
	_ = stream.Encode(struct {
		Type string `json:"type"`
		cli.ConvergeOutput
		Run   *runRef       `json:"run,omitempty"`
		Watch *watchOutcome `json:"watch,omitempty"`
	}{Type: "result", ConvergeOutput: out, Run: ref, Watch: &watch})
	return stream.Report("kapi up --json")
}

// incompleteWatchError fails the command for a watch that ended early, and only
// when the caller asked for that.
//
// It is off by default because an incomplete watch is not a bad outcome: the
// run goes on, the result record says what the server holds, and kapi-action
// treats any non-zero exit as a failed run and stops delivering what did land.
// A pipeline that would rather stop than proceed on a partial view opts in.
func incompleteWatchError(watch watchOutcome, failOnIncomplete bool) error {
	if watch.Complete || !failOnIncomplete {
		return nil
	}
	return fmt.Errorf("the run's event stream ended before the run did (%s); the run may still be going, and `kapi status` reports where it got to", watch.Reason)
}

// watchEndedLine tells a person what to do with a run this command stopped
// watching. A CI log is often the only thing a reader has.
func watchEndedLine(watch watchOutcome, runID string) string {
	switch watch.Reason {
	case watchTimeout:
		return fmt.Sprintf("Stopped waiting for run %s; pulling what landed. `kapi status` reports where it got to.", runID)
	default:
		return fmt.Sprintf("Lost the event stream for run %s; pulling what landed. The run goes on, and `kapi status` reports where it got to.", runID)
	}
}
