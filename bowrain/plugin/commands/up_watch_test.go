package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// The exit code answers whether the work happened. Losing the stream is this
// client's problem and the run's own state is the answer, so a watch that ended
// early is reported rather than raised.

func TestWatchResult_AStreamErrorIsReportedNotRaised(t *testing.T) {
	outcome, err := watchResult(errors.New("subscribe convergence run events failed (HTTP 504)"))

	require.NoError(t, err, "a gateway's answer says nothing about the run and must not fail the command")
	assert.False(t, outcome.Complete)
	assert.Equal(t, watchStreamError, outcome.Reason)
	assert.Contains(t, outcome.Detail, "504", "the record says what ended the watch")
}

func TestWatchResult_ATimeoutKeepsItsMeaning(t *testing.T) {
	outcome, err := watchResult(fmt.Errorf("stream: %w", context.DeadlineExceeded))

	require.NoError(t, err, "--timeout has always meant pull what landed")
	assert.False(t, outcome.Complete)
	assert.Equal(t, watchTimeout, outcome.Reason)
}

func TestWatchResult_ACompleteWatchSaysSo(t *testing.T) {
	outcome, err := watchResult(nil)

	require.NoError(t, err)
	assert.True(t, outcome.Complete)
	assert.Equal(t, watchDone, outcome.Reason)
}

// A person pressing Ctrl-C asked this command to stop, and cli.Run turns that
// into a cancelled context. Carrying on to pull and report would ignore them.
func TestWatchResult_CancellationStopsTheCommand(t *testing.T) {
	_, err := watchResult(fmt.Errorf("stream: %w", context.Canceled))

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// The state in the record is what the server says now, not the last frame this
// client saw: a run that finished after the watch dropped reports as finished.
func TestRunStateNow_ReReadsTheRunAfterAnEarlyEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ConvergenceRun{ID: "r1", State: "converged", Passes: 3})
	}))
	defer srv.Close()

	client := apiclient.NewProjectBearerClient(srv.URL, "proj1", "tok")
	watch := watchOutcome{Reason: watchStreamError}
	got := runStateNow(context.Background(), client, &apiclient.ConvergenceRun{ID: "r1", State: "running", Passes: 1}, &watch)

	require.NotNil(t, got)
	assert.Equal(t, "converged", got.State, "the server's answer, not the last frame seen")
	assert.Equal(t, 3, got.Passes)
}

// A re-read that fails leaves the run as last seen and says so, because exit 0
// is only defensible when the record's state is accounted for.
func TestRunStateNow_AFailedReReadKeepsTheLastSeenStateAndSaysSo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := apiclient.NewProjectBearerClient(srv.URL, "proj1", "tok")
	watch := watchOutcome{Reason: watchStreamError}
	last := &apiclient.ConvergenceRun{ID: "r1", State: "running", Passes: 1}
	got := runStateNow(context.Background(), client, last, &watch)

	assert.Equal(t, "running", got.State, "the last state this client saw stands")
	assert.Contains(t, watch.Detail, "could not be re-read")
}

// The run id is data: a consumer that loses the stream still holds the handle
// `kapi status` takes, spelled as `kapi status --json` spells it.
func TestServerUpJSONDocument_NamesTheRunAtStartAndInTheResult(t *testing.T) {
	w := &nthWriteFailer{n: 0} // never fails: capture the whole document
	cmd := serverUpJSONCmd(t, w)
	stream := output.NewNDJSONStream(cmd.OutOrStdout())

	// The stream is whatever this client is working on: nothing on the server-up
	// path calls SetStream, and the client answers "main" when nothing set one.
	client := apiclient.NewProjectBearerClient("http://example.invalid", "proj1", "tok")
	emitRunStarted(stream, &apiclient.ConvergenceRun{ID: "r1", State: "running"}, client.Stream())
	emitServerEvents(stream, 2)
	require.NoError(t, writeServerUpResult(cmd, stream, cli.ConvergeOutput{Passes: 2, Converged: true},
		&apiclient.ConvergenceRun{ID: "r1", State: "converged", Passes: 2},
		watchOutcome{Complete: true, Reason: watchDone}))

	var started, result map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(w.buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec), line)
		switch rec["type"] {
		case "run_started":
			started = rec
		case "result":
			result = rec
		}
	}

	require.NotNil(t, started, "the stream names the run when it starts")
	assert.Equal(t, "r1", started["id"])
	assert.Equal(t, client.Stream(), started["stream"], "the stream the client reports, not a literal")

	require.NotNil(t, result, "the closing record is still a result record")
	assert.Equal(t, true, result["converged"], "the fields CI already reads keep their spelling")
	assert.EqualValues(t, 2, result["passes"])

	run, ok := result["run"].(map[string]any)
	require.True(t, ok, "the result names the run")
	assert.Equal(t, "r1", run["id"])
	assert.Equal(t, "converged", run["state"])

	watch, ok := result["watch"].(map[string]any)
	require.True(t, ok, "the result says whether this client watched the whole run")
	assert.Equal(t, true, watch["complete"])
	assert.Equal(t, watchDone, watch["reason"])
}

// An incomplete watch is visible in the record and, by default, is not a
// failure. Strict pipelines opt in.
func TestServerUpJSONDocument_AnIncompleteWatchIsRecorded(t *testing.T) {
	w := &nthWriteFailer{n: 0}
	cmd := serverUpJSONCmd(t, w)
	stream := output.NewNDJSONStream(cmd.OutOrStdout())

	require.NoError(t, writeServerUpResult(cmd, stream, cli.ConvergeOutput{Passes: 1},
		&apiclient.ConvergenceRun{ID: "r1", State: "running", Passes: 1},
		watchOutcome{Reason: watchStreamError, Detail: "HTTP 504"}))

	var result map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(w.buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec), line)
		if rec["type"] == "result" {
			result = rec
		}
	}
	require.NotNil(t, result)

	watch, ok := result["watch"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, watch["complete"])
	assert.Equal(t, watchStreamError, watch["reason"])
	assert.Equal(t, "HTTP 504", watch["detail"])
}

// --fail-on-incomplete-watch is the smallest thing that serves a strict
// pipeline, and it is off by default because kapi-action treats any non-zero
// exit as a failed run and stops delivering what did land.
func TestIncompleteWatchError_OnlyWithTheFlag(t *testing.T) {
	incomplete := watchOutcome{Reason: watchStreamError, Detail: "HTTP 504"}

	require.NoError(t, incompleteWatchError(incomplete, false),
		"by default an incomplete watch is reported, not raised")

	err := incompleteWatchError(incomplete, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kapi status")

	require.NoError(t, incompleteWatchError(watchOutcome{Complete: true, Reason: watchDone}, true),
		"a complete watch is never a failure, flag or not")
}
