package host

import (
	"context"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nthWriteFailer fails the nth Write (1-based) with err and succeeds otherwise.
// The failure is at the writer, not a replaced encoder, so json.Encoder stays in
// the path and the test exercises the same wrapping a real stderr failure has.
type nthWriteFailer struct {
	mu     sync.Mutex
	n      int
	err    error
	writes int
	buf    strings.Builder
}

func (w *nthWriteFailer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes++
	if w.writes == w.n {
		return 0, w.err
	}
	w.buf.Write(p)
	return len(p), nil
}

func (w *nthWriteFailer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// progressCmd builds a command with --progress=jsonl and the given stderr.
func progressCmd(t *testing.T, mode string, errW *nthWriteFailer) *EnvCommand {
	t.Helper()
	cmd := NewEnvCommand(context.Background(), "run")
	AddProgressFlag(cmd)
	require.NoError(t, cmd.Flags().Set(progressFlagName, mode))
	cmd.SetErr(errW)
	return cmd
}

// TestProgressSink_ReportsTruncation asserts the --progress=jsonl sink no longer
// drops a write failure. RunEventSink cannot return an error, so before this the
// feed simply stopped while the run continued — a watching UI showed a stalled run
// that was in fact fine, and the command exited 0.
func TestProgressSink_ReportsTruncation(t *testing.T) {
	errW := &nthWriteFailer{n: 3, err: &os.PathError{Op: "write", Path: "/dev/stderr", Err: syscall.ENOSPC}}
	cmd := progressCmd(t, "jsonl", errW)

	sink, report := progressSink(cmd)
	require.NotNil(t, sink, "--progress=jsonl must produce a sink")
	for i := range 6 {
		sink(FlowRunEvent{Type: FlowEventProgress, FileIndex: i, FileCount: 6})
	}

	err := report()
	require.Error(t, err, "a truncated progress feed must fail the command")
	assert.Contains(t, err.Error(), "--progress=jsonl", "the report names the stream")
	assert.Contains(t, err.Error(), "truncated")
	assert.ErrorIs(t, err, syscall.ENOSPC)
}

// TestProgressSink_ClosedConsumerDoesNotFailTheRun is the discriminating control:
// a consumer that stopped reading the progress feed must not fail a run that is
// otherwise fine. "Fail on any encode error" is not an acceptable substitute for
// the fix above.
func TestProgressSink_ClosedConsumerDoesNotFailTheRun(t *testing.T) {
	errW := &nthWriteFailer{n: 3, err: &os.PathError{Op: "write", Path: "/dev/stderr", Err: syscall.EPIPE}}
	cmd := progressCmd(t, "jsonl", errW)

	sink, report := progressSink(cmd)
	for i := range 6 {
		sink(FlowRunEvent{Type: FlowEventProgress, FileIndex: i, FileCount: 6})
	}

	require.NoError(t, report(), "a reader that walked away must not fail the run")
	assert.Equal(t, 3, errW.writes, "the sink stops writing once the consumer is gone")
}

// TestProgressSink_CompleteFeedReportsNothing is the baseline: an ordinary run
// writes every event and reports nothing, so --progress=jsonl stays exit 0.
func TestProgressSink_CompleteFeedReportsNothing(t *testing.T) {
	errW := &nthWriteFailer{n: 0} // never fails
	cmd := progressCmd(t, "jsonl", errW)

	sink, report := progressSink(cmd)
	for i := range 4 {
		sink(FlowRunEvent{Type: FlowEventProgress, FileIndex: i, FileCount: 4})
	}

	require.NoError(t, report())
	assert.Len(t, strings.Split(strings.TrimSuffix(errW.String(), "\n"), "\n"), 4)
}

// TestProgressSink_WithoutFlagIsInert asserts the no-flag path is unchanged: a nil
// sink (the discard sink the emit wrappers check for) and a check that reports
// nothing, so no command gains a failure mode by adopting the new signature.
func TestProgressSink_WithoutFlagIsInert(t *testing.T) {
	cmd := NewEnvCommand(context.Background(), "run")
	AddProgressFlag(cmd)

	sink, report := progressSink(cmd)
	assert.Nil(t, sink, "no --progress → the discard sink")
	require.NotNil(t, report, "the check must be callable unconditionally")
	require.NoError(t, report())
}
