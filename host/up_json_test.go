package host

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/host/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upJSONCmd builds a command with --json set and the given stdout.
func upJSONCmd(t *testing.T, jsonOut bool, w *nthWriteFailer) *EnvCommand {
	t.Helper()
	cmd := NewEnvCommand(context.Background(), "up")
	AddUpFlags(cmd)
	if jsonOut {
		require.NoError(t, cmd.Flags().Set("json", "true"))
	}
	cmd.SetOut(w)
	return cmd
}

// emitUpEvents streams n convergence events into the stream the way ExecuteUp's
// onEvent callback does.
func emitUpEvents(stream *output.NDJSONStream, n int) {
	for i := range n {
		stream.Emit(convergence.Event{Type: convergence.EventPassStart, Pass: i + 1})
	}
}

// TestUpJSON_ReportsTruncatedEventStream is the regression for host/up.go's
// dropped `_ = enc.Encode(ev)`. A consumer of `kapi up --json` used to receive a
// stream that ended mid-run with no error line while kapi exited 0, so it read
// the run as having finished with fewer locales than it processed.
//
// The asymmetry was in the same file: PrintUpResult DID check the closing record's
// error, so a failure on the last line was reported while every progress line
// before it was eaten. One stream now carries both.
func TestUpJSON_ReportsTruncatedEventStream(t *testing.T) {
	w := &nthWriteFailer{n: 3, err: &os.PathError{Op: "write", Path: "/out.ndjson", Err: syscall.ENOSPC}}
	cmd := upJSONCmd(t, true, w)

	stream := output.NewNDJSONStream(cmd.OutOrStdout())
	emitUpEvents(stream, 6) // the write at index 2 fails; the rest are lost

	err := PrintUpResultStream(cmd, stream, ConvergeOutput{Passes: 1})
	require.Error(t, err,
		"a consumer handed a truncated NDJSON stream must not be told the run succeeded")
	assert.Contains(t, err.Error(), "kapi up --json")
	assert.Contains(t, err.Error(), "truncated")
	assert.ErrorIs(t, err, syscall.ENOSPC)
}

// TestUpJSON_ClosedConsumerExitsCleanly is the discriminating control and the
// benign case the founder's policy turns on: `kapi up --json | head` closes the
// pipe, and the run must NOT be failed because the reader stopped reading. Note
// that PrintUpResultStream also covers the closing record, which used to return
// the EPIPE straight out of ExecuteUp as a run failure.
func TestUpJSON_ClosedConsumerExitsCleanly(t *testing.T) {
	w := &nthWriteFailer{n: 3, err: &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EPIPE}}
	cmd := upJSONCmd(t, true, w)

	stream := output.NewNDJSONStream(cmd.OutOrStdout())
	emitUpEvents(stream, 6)

	require.NoError(t, PrintUpResultStream(cmd, stream, ConvergeOutput{Passes: 1}),
		"a closed pipe is ordinary use, not a failed run")
	assert.Equal(t, 3, w.writes, "the stream stops writing once the consumer is gone")
}

// TestUpJSON_ClosingRecordEPIPEIsQuiet isolates the closing record: with the event
// phase clean and only the result line hitting a closed pipe, the command must
// still exit 0. Before this, PrintUpResult returned that EPIPE verbatim.
func TestUpJSON_ClosingRecordEPIPEIsQuiet(t *testing.T) {
	w := &nthWriteFailer{n: 1, err: &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EPIPE}}
	cmd := upJSONCmd(t, true, w)

	assert.NoError(t, PrintUpResult(cmd, ConvergeOutput{Passes: 1}),
		"`kapi up --json | head` must not report an error for the record head never read")
}

// TestUpJSON_ClosingRecordHardFailureIsLoud is the other half: the same record
// failing for a reason that is not a departed consumer must still fail the
// command, so the fix is not "never fail on an encode error".
func TestUpJSON_ClosingRecordHardFailureIsLoud(t *testing.T) {
	w := &nthWriteFailer{n: 1, err: &os.PathError{Op: "write", Path: "/out.ndjson", Err: syscall.ENOSPC}}
	cmd := upJSONCmd(t, true, w)

	err := PrintUpResult(cmd, ConvergeOutput{Passes: 1})
	require.Error(t, err)
	assert.ErrorIs(t, err, syscall.ENOSPC)
}

// TestUpJSON_CompleteStreamIsUnchanged is the baseline: the NDJSON document is
// still one event per line closed by a discriminated result record, and an
// ordinary run reports nothing.
func TestUpJSON_CompleteStreamIsUnchanged(t *testing.T) {
	w := &nthWriteFailer{n: 0}
	cmd := upJSONCmd(t, true, w)

	stream := output.NewNDJSONStream(cmd.OutOrStdout())
	emitUpEvents(stream, 2)
	require.NoError(t, PrintUpResultStream(cmd, stream, ConvergeOutput{Passes: 1}))

	lines := strings.Split(strings.TrimSuffix(w.String(), "\n"), "\n")
	require.Len(t, lines, 3, "two events plus the closing result record")
	var last struct {
		Type string `json:"type"`
	}
	require.NoError(t, json.Unmarshal([]byte(lines[2]), &last))
	assert.Equal(t, "result", last.Type, "the stream still closes with the discriminated result")
}

// TestUpJSON_TextModeUnaffected asserts the text path never touches the stream, so
// a run without --json cannot acquire a new failure mode.
func TestUpJSON_TextModeUnaffected(t *testing.T) {
	w := &nthWriteFailer{n: 0}
	cmd := upJSONCmd(t, false, w)

	require.NoError(t, PrintUpResultStream(cmd, nil, ConvergeOutput{Passes: 1}))
	assert.NotContains(t, w.String(), `"type":"result"`, "text mode renders the summary, not NDJSON")
}
