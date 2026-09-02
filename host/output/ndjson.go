package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"syscall"
)

// NDJSONStream writes one JSON object per line — the shape `kapi up --json` and
// `--progress=jsonl` speak to machine consumers — and remembers the first write
// failure that matters, so a stream that stops mid-flight is never mistaken for a
// complete one.
//
// The two failure modes are deliberately not treated alike:
//
//   - **The consumer went away.** `kapi up --json | head` is ordinary use. The
//     stream stops writing and Err stays nil: failing a run because the reader
//     stopped reading would be worse than the silence it replaces. (On raw
//     stdout/stderr the Go runtime terminates the process on SIGPIPE before the
//     write ever returns, which is the same user-visible outcome — quiet. This
//     covers every other writer, where the write returns EPIPE instead: a relayed
//     pipe, an embedded or desktop run, a network connection.)
//   - **Anything else** — a full disk, an unencodable value — means the consumer
//     is still listening and has been handed a truncated stream. Err reports the
//     first failure so the caller can fail loudly, once, naming what was lost.
//
// A stream is safe for concurrent emitters; the per-file goroutines behind
// `--progress=jsonl` share one.
type NDJSONStream struct {
	mu      sync.Mutex
	enc     *json.Encoder
	written int
	dropped int
	// stopped records that the stream has given up writing, either because the
	// consumer went away or because a write failed. Further Encode calls would
	// raise the same error for no benefit — json.Encoder latches its own write
	// error, so every subsequent Encode would fail identically — and re-raising
	// would turn "report once" into one report per remaining event.
	stopped bool
	err     error // first non-benign failure; nil when the consumer merely left
}

// NewNDJSONStream returns a stream writing NDJSON to w.
func NewNDJSONStream(w io.Writer) *NDJSONStream {
	return &NDJSONStream{enc: json.NewEncoder(w)}
}

// Encode writes v as one line. It returns the failure only when the consumer is
// still listening — a closed consumer yields nil, because there is nobody left to
// report to and no run to fail. Callers that cannot return an error use Emit.
func (s *NDJSONStream) Encode(v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		s.dropped++
		return nil
	}
	if err := s.enc.Encode(v); err != nil {
		s.stopped = true
		s.dropped++
		if IsClosedConsumer(err) {
			return nil
		}
		s.err = err
		return err
	}
	s.written++
	return nil
}

// Emit writes v as one line and discards the return value. Use it for the
// void-returning event sinks (RunEventSink, the convergence onEvent callback),
// whose caller consults Report when the run finishes.
func (s *NDJSONStream) Emit(v any) { _ = s.Encode(v) }

// Err reports the first failure that left a listening consumer with a truncated
// stream, or nil when the stream was complete — or when the consumer simply went
// away.
func (s *NDJSONStream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Report returns the error to fail the command with when the stream truncated,
// and nil otherwise. name identifies the stream to the operator (e.g.
// "kapi up --json"); the message counts what got through and what did not,
// because a machine consumer that read N records needs to know N is not the
// total.
func (s *NDJSONStream) Report(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		return nil
	}
	return fmt.Errorf("%s: the event stream truncated after %d record(s) written, %d lost: %w",
		name, s.written, s.dropped, s.err)
}

// IsClosedConsumer reports whether err means the reader of a stream went away,
// rather than the write itself having failed.
//
// Detected structurally, never by matching the message text: a broken pipe
// arrives as syscall.EPIPE from an os.File, as io.ErrClosedPipe from an io.Pipe,
// and with platform-specific wrapping in between — which is exactly why
// errors.Is is the only reliable test.
func IsClosedConsumer(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, io.ErrClosedPipe) ||
		isPlatformClosedConsumer(err)
}
