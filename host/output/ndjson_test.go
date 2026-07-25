package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nthWriteFailer fails on the nth Write (1-based) with err, and succeeds
// otherwise. Constructing the failure at the writer rather than by replacing the
// encoder keeps json.Encoder in the picture, so the test exercises the same
// wrapping a real io.Writer failure goes through.
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

func (w *nthWriteFailer) lines() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Split(strings.TrimSuffix(w.buf.String(), "\n"), "\n")
}

// TestNDJSONStream_CompleteStreamReportsNothing is the baseline: a stream that
// wrote every record has nothing to report, so the ordinary run stays exit 0.
func TestNDJSONStream_CompleteStreamReportsNothing(t *testing.T) {
	var buf strings.Builder
	s := NewNDJSONStream(&buf)
	for i := range 3 {
		require.NoError(t, s.Encode(map[string]int{"i": i}))
	}

	assert.NoError(t, s.Err())
	assert.NoError(t, s.Report("kapi up --json"))
	assert.Len(t, strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n"), 3)
}

// TestNDJSONStream_ClosedConsumerIsQuiet is the benign case: `kapi up --json |
// head` closes the pipe, and the run must not be failed because the reader
// stopped reading. Every EPIPE spelling that reaches Go is covered, including the
// os.File and io.Pipe wrappings — which is why the detection is errors.Is and not
// a string match.
func TestNDJSONStream_ClosedConsumerIsQuiet(t *testing.T) {
	for name, err := range map[string]error{
		"bare syscall.EPIPE":       syscall.EPIPE,
		"os.PathError wrapping":    &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EPIPE},
		"io.ErrClosedPipe":         io.ErrClosedPipe,
		"fmt.Errorf %w over EPIPE": fmt.Errorf("write /dev/stdout: %w", syscall.EPIPE),
	} {
		t.Run(name, func(t *testing.T) {
			w := &nthWriteFailer{n: 3, err: err}
			s := NewNDJSONStream(w)
			for i := range 6 {
				require.NoError(t, s.Encode(map[string]int{"i": i}),
					"a consumer that went away is not a failure to report")
			}

			assert.True(t, IsClosedConsumer(err), "the error must be recognised structurally")
			assert.NoError(t, s.Err(), "a closed consumer must not be remembered as truncation")
			assert.NoError(t, s.Report("kapi up --json"), "the run must not fail because the reader stopped")
			assert.Equal(t, 3, w.writes,
				"once the consumer is gone the stream stops writing instead of re-raising")
		})
	}
}

// TestNDJSONStream_TruncationIsReportedOnce is the loud case: a write failure
// that is NOT a closed consumer — a full disk is the canonical one — left a
// listening consumer with a partial stream, so the command must fail, once,
// naming what was lost.
func TestNDJSONStream_TruncationIsReportedOnce(t *testing.T) {
	diskFull := &os.PathError{Op: "write", Path: "/out.ndjson", Err: syscall.ENOSPC}
	w := &nthWriteFailer{n: 3, err: diskFull}
	s := NewNDJSONStream(w)

	var failed int
	for i := range 6 {
		if err := s.Encode(map[string]int{"i": i}); err != nil {
			failed++
		}
	}

	assert.Equal(t, 1, failed,
		"the failure is reported once, not once per remaining event (json.Encoder latches its write error)")
	require.Error(t, s.Err())
	require.ErrorIs(t, s.Err(), syscall.ENOSPC, "the underlying cause must survive")

	report := s.Report("kapi up --json")
	require.Error(t, report, "a listening consumer handed a truncated stream must fail the command")
	assert.Contains(t, report.Error(), "kapi up --json", "the report names the stream")
	assert.Contains(t, report.Error(), "truncated")
	assert.Contains(t, report.Error(), "2 record(s) written, 4 lost",
		"the counts matter: a machine reader needs to know its 2 is not the total")
	require.ErrorIs(t, report, syscall.ENOSPC)
	assert.Len(t, w.lines(), 2, "only the records that got through are on the wire")
}

// TestNDJSONStream_FirstErrorWins asserts a stream failing repeatedly reports the
// first cause, not the last — the first failure is the one that explains where the
// consumer's view diverged.
func TestNDJSONStream_FirstErrorWins(t *testing.T) {
	first := errors.New("first failure")
	w := &alwaysFailWriter{err: first}
	s := NewNDJSONStream(w)
	for range 3 {
		_ = s.Encode(map[string]int{"i": 1})
	}
	require.ErrorIs(t, s.Err(), first)
	assert.Contains(t, s.Report("x").Error(), "0 record(s) written, 3 lost")
}

type alwaysFailWriter struct{ err error }

func (w *alwaysFailWriter) Write([]byte) (int, error) { return 0, w.err }

// TestNDJSONStream_ConcurrentEmittersSerialize asserts the stream is safe for the
// concurrent per-file goroutines behind --progress=jsonl: every line is whole and
// parseable, which is the property a line-delimited consumer depends on.
func TestNDJSONStream_ConcurrentEmittersSerialize(t *testing.T) {
	var buf syncBuffer
	s := NewNDJSONStream(&buf)

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() { s.Emit(map[string]int{"i": i}) })
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	require.Len(t, lines, 50)
	for _, line := range lines {
		var m map[string]int
		require.NoError(t, json.Unmarshal([]byte(line), &m), "every line must be a whole object: %q", line)
	}
	assert.NoError(t, s.Err())
}

type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestIsClosedConsumer_RejectsOtherFailures is the discriminating control: the
// failures that mean "the write failed while somebody was still listening" must
// NOT be classified as a departed consumer, or the fix degrades to "never fail on
// an encode error" — which is the bug.
func TestIsClosedConsumer_RejectsOtherFailures(t *testing.T) {
	for name, err := range map[string]error{
		"nil":                nil,
		"disk full":          &os.PathError{Op: "write", Path: "/x", Err: syscall.ENOSPC},
		"permission denied":  &os.PathError{Op: "write", Path: "/x", Err: syscall.EACCES},
		"unencodable value":  errors.New("json: unsupported type: chan int"),
		"broken pipe, prose": errors.New("write |1: broken pipe"),
	} {
		t.Run(name, func(t *testing.T) {
			assert.False(t, IsClosedConsumer(err),
				"only a structurally-identified closed consumer is benign")
		})
	}
}

// TestNDJSONStream_RealClosedPipeIsQuiet drives a real os.Pipe whose read end is
// closed — the actual `| head` mechanics rather than a synthesised error — and
// asserts the stream goes quiet with nothing to report.
func TestNDJSONStream_RealClosedPipeIsQuiet(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer func() { _ = w.Close() }()
	require.NoError(t, r.Close()) // the consumer walked away

	s := NewNDJSONStream(w)
	for range 4 {
		require.NoError(t, s.Encode(map[string]string{"type": "progress"}))
	}
	assert.NoError(t, s.Report("kapi up --json"),
		"a real broken pipe must not fail the run")
}
