package host

import (
	"github.com/neokapi/neokapi/host/output"
)

// progressFlagName is the NDJSON progress flag shared by run/extract/merge.
const progressFlagName = "progress"

// addProgressFlag registers --progress on a long-running verb. The only
// mode is "jsonl": stream machine-readable progress events to stderr, one
// JSON object per line, using the FlowRunEvent vocabulary (cli/flowrun.go) —
// the same event shapes the desktop run sink receives.
func AddProgressFlag(cmd Command) {
	cmd.Flags().String(progressFlagName, "",
		"stream progress events to stderr as NDJSON (value: jsonl); events use the flow-run event vocabulary")
}

// progressSink resolves the --progress flag into a RunEventSink plus the check to
// run when the command finishes. It returns a nil sink (the discard sink) unless
// --progress=jsonl was passed. Events go to stderr so stdout stays reserved for
// the command's result (text or --json). The sink is safe for concurrent
// emitters.
//
// RunEventSink cannot return an error — the desktop and embedded runs consume the
// same signature — so a failed write is remembered on the stream and surfaced by
// the returned check, which the caller must consult before returning success. A
// progress feed that stops while the run continues shows a watching UI a stalled
// run that is in fact fine, and the exit code is the only channel left when the
// failing writer *is* stderr.
func progressSink(cmd Command) (RunEventSink, func() error) {
	mode, _ := cmd.Flags().GetString(progressFlagName)
	if mode != "jsonl" {
		return nil, func() error { return nil }
	}
	stream := output.NewNDJSONStream(cmd.ErrOrStderr())
	return func(ev FlowRunEvent) { stream.Emit(ev) },
		func() error { return stream.Report("--progress=jsonl") }
}
