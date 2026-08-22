package commands

import (
	"encoding/json"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/neokapi/neokapi/host/venue/transfer"

	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serverUpJSONCmd builds a cobra command with up's flag set and --json on, with
// stdout captured by w.
func serverUpJSONCmd(t *testing.T, w *nthWriteFailer) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "server-up"}
	cli.AddUpFlags(cmd)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	cmd.SetOut(w)
	return cmd
}

// emitServerEvents re-emits n server events the way runServerUp's --json sink
// does.
func emitServerEvents(stream *output.NDJSONStream, n int) {
	for i := range n {
		stream.Emit(convergence.Event{Type: convergence.EventPassStart, Pass: i + 1})
	}
}

// TestServerUpJSONDocument_ReportsTruncationOnce is a contract test for the
// server venue's NDJSON document, not a driven server run: it wires the real
// reportVoicePush, event sink, and cli.PrintUpResultStream around one stream
// exactly as runServerUp does, then breaks a write in the middle.
//
// Before this, `sink = func(ev) { _ = enc.Encode(ev) }` dropped every progress
// line, so a CI job consuming `kapi up --json` against the server received a
// stream that ended mid-run and exited 0 — the same shape as the host bug, in the
// sibling that must not be left one-sided.
func TestServerUpJSONDocument_ReportsTruncationOnce(t *testing.T) {
	w := &nthWriteFailer{n: 4, err: &os.PathError{Op: "write", Path: "/out.ndjson", Err: syscall.ENOSPC}}
	cmd := serverUpJSONCmd(t, w)
	stream := output.NewNDJSONStream(cmd.OutOrStdout())

	require.NoError(t, reportVoicePush(cmd, stream, &transfer.PushVoiceResult{Name: "Acme Voice", Action: "created", Version: 1}, true))
	emitServerEvents(stream, 6) // the 4th write fails; the rest are lost

	err := cli.PrintUpResultStream(cmd, stream, cli.ConvergeOutput{Passes: 1})
	require.Error(t, err, "a truncated server-run stream must not be reported as a finished run")
	assert.Contains(t, err.Error(), "truncated")
	require.ErrorIs(t, err, syscall.ENOSPC)
	assert.Equal(t, 1, strings.Count(err.Error(), "truncated"), "reported once, not per lost event")
}

// TestServerUpJSONDocument_ClosedConsumerExitsCleanly is the discriminating
// control: `kapi up --json | head` against the server must exit cleanly, so the
// fix is not "fail on any encode error".
func TestServerUpJSONDocument_ClosedConsumerExitsCleanly(t *testing.T) {
	w := &nthWriteFailer{n: 4, err: &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EPIPE}}
	cmd := serverUpJSONCmd(t, w)
	stream := output.NewNDJSONStream(cmd.OutOrStdout())

	require.NoError(t, reportVoicePush(cmd, stream, &transfer.PushVoiceResult{Name: "Acme Voice", Action: "created", Version: 1}, true))
	emitServerEvents(stream, 6)

	require.NoError(t, cli.PrintUpResultStream(cmd, stream, cli.ConvergeOutput{Passes: 1}),
		"a consumer that stopped reading must not fail the run")
	assert.Equal(t, 4, w.writes, "the stream stops writing once the consumer is gone")
}

// TestServerUpJSONDocument_CompleteStreamIsUnchanged asserts the document shape
// the sharing does not alter: the brand record, one line per server event, and the
// discriminated result last.
func TestServerUpJSONDocument_CompleteStreamIsUnchanged(t *testing.T) {
	w := &nthWriteFailer{n: 0}
	cmd := serverUpJSONCmd(t, w)
	stream := output.NewNDJSONStream(cmd.OutOrStdout())

	require.NoError(t, reportVoicePush(cmd, stream, &transfer.PushVoiceResult{Name: "Acme Voice", Action: "created", Version: 1}, true))
	emitServerEvents(stream, 2)
	require.NoError(t, cli.PrintUpResultStream(cmd, stream, cli.ConvergeOutput{Passes: 1}))

	lines := strings.Split(strings.TrimSuffix(w.buf.String(), "\n"), "\n")
	require.Len(t, lines, 4, "brand record + two events + the result record")
	var first, last struct {
		Type string `json:"type"`
	}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
	require.NoError(t, json.Unmarshal([]byte(lines[3]), &last))
	assert.Equal(t, "voice_profile", first.Type)
	assert.Equal(t, "result", last.Type)
}
