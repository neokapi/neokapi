package flow_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/srt"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/require"
)

// writePlainFile writes an SRT document of n cues to
// path and returns its byte size. The content never lives as one big string the
// caller retains, so it does not inflate the heap baseline of a later run.
func writePlainFile(t *testing.T, path string, n int) int {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create input: %v", err)
	}
	bw := bufio.NewWriter(f)
	size := 0
	for i := range n {
		c, _ := fmt.Fprintf(bw, "%d\n00:%02d:%02d,000 --> 00:%02d:%02d,500\nThis is translatable cue number %d with several words of content\n\n",
			i+1, (i/60)%60, i%60, (i/60)%60, i%60, i)
		size += c
	}
	if err := bw.Flush(); err != nil {
		t.Fatalf("flush input: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}
	return size
}

// gatedSRTReader is the SRT reader with its input fed through a gate, so the
// test can stop the runner's pipeline at points it chooses.
type gatedSRTReader struct {
	*srt.Reader
	gates chan<- *testutil.Gate
}

func (r gatedSRTReader) Open(ctx context.Context, doc *model.RawDocument) error {
	gate := testutil.NewGate(doc.Reader)
	doc.Reader = io.NopCloser(gate)
	r.gates <- gate
	return r.Reader.Open(ctx, doc)
}

// storeSRTWriter is the SRT writer that hands the test the streaming skeleton
// store the runner wires it to.
type storeSRTWriter struct {
	*srt.Writer
	stores chan<- *format.SkeletonStore
}

func (w storeSRTWriter) SetSkeletonStore(s *format.SkeletonStore) {
	w.stores <- s
	w.Writer.SetSkeletonStore(s)
}

// runPlainRoundTripPeak runs an SRT → SRT round-trip through the streaming
// FileRunner path and returns the peak live heap above the pipeline's heap
// before any input. The reader's input is fed through a gate in 64 steps. At
// each stop the reader waits for input and the writer has caught up and waits
// on the skeleton store, so the heap is measured with no stage working.
func runPlainRoundTripPeak(t *testing.T, inPath, outPath string, size int) uint64 {
	t.Helper()
	ctx := context.Background()

	defer debug.SetGCPercent(debug.SetGCPercent(20))

	runner := flow.NewFileRunner(flow.FileRunnerConfig{SourceLocale: model.LocaleEnglish})
	gates := make(chan *testutil.Gate, 1)
	stores := make(chan *format.SkeletonStore, 1)
	reader := gatedSRTReader{Reader: srt.NewReader(), gates: gates}
	writer := storeSRTWriter{Writer: srt.NewWriter(), stores: stores}
	passthrough := &tool.BaseTool{ToolName: "passthrough"}

	done := make(chan struct{})
	var runErr error
	go func() {
		defer close(done)
		runErr = runner.RunFileWithReaderWriter(ctx, "roundtrip", []tool.Tool{passthrough}, inPath, outPath, "", reader, writer)
	}()
	var store *format.SkeletonStore
	select {
	case store = <-stores:
	case <-done:
		t.Fatalf("the runner wired no streaming skeleton store: %v", runErr)
	}
	require.True(t, store.IsStreaming(), "the runner streams the round trip")
	gate := <-gates
	peak := testutil.SamplePeakStops(gate, int64(size), 64, store, done)
	<-done
	require.NoError(t, runErr, "round-trip")
	return peak
}

// TestStreamingRoundTripBoundedMemory asserts the streaming plaintext
// round-trip holds peak memory to a bounded window, sublinear in input size
// rather than ~ file size. A buffered path holding the whole Part slice and
// the whole file would peak at ≥ file size. The streaming path keeps a small
// window, so a 10× larger input must not give a ~10× larger peak, and the peak
// must stay well under the (multi-MB) file size.
func TestStreamingRoundTripBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("memory regression test skipped in -short")
	}

	// Guard: the format must actually be on the streaming path, else the test
	// would assert nothing meaningful.
	if !format.IsStreamingReader(srt.NewReader()) {
		t.Fatal("plaintext reader is not a StreamingReader")
	}
	if !format.IsStreamingWriter(srt.NewWriter()) {
		t.Fatal("plaintext writer is not a StreamingWriter")
	}

	const smallN = 20_000
	const largeN = 200_000 // 10× the input

	dir := t.TempDir()
	smallIn := filepath.Join(dir, "small.txt")
	largeIn := filepath.Join(dir, "large.txt")
	smallSize := writePlainFile(t, smallIn, smallN)
	largeSize := writePlainFile(t, largeIn, largeN)

	smallPeak := runPlainRoundTripPeak(t, smallIn, filepath.Join(dir, "small.out"), smallSize)
	largePeak := runPlainRoundTripPeak(t, largeIn, filepath.Join(dir, "large.out"), largeSize)

	t.Logf("small: peakΔ=%d KiB (input %d KiB)", smallPeak/1024, smallSize/1024)
	t.Logf("large: peakΔ=%d KiB (input %d KiB)", largePeak/1024, largeSize/1024)

	// Bounded window: peak must be a small fraction of the file size. The large
	// input is several MB; a streaming round-trip should peak far below that.
	if largePeak > uint64(largeSize)/2 {
		t.Errorf("peak heap %d B is not bounded well below file size %d B — looks like the whole document is buffered", largePeak, largeSize)
	}

	// Sublinear: 10× input must not give ~10× peak. Allow a generous 4× to
	// absorb GC sawtooth noise while still failing a linear (≈10×) regression.
	if smallPeak > 0 && largePeak > smallPeak*4 {
		t.Errorf("peak heap scaled near-linearly with input: small=%d B large=%d B (10× input → %.1f× peak)", smallPeak, largePeak, float64(largePeak)/float64(smallPeak))
	}
}
