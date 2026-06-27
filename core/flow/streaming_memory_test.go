package flow_test

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats/splicedlines"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// writeSplicedFile writes a splicedlines document of n non-continuation lines to
// path and returns its byte size. The content never lives as one big string the
// caller retains, so it does not inflate the heap baseline of a later run.
func writeSplicedFile(t *testing.T, path string, n int) int {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create input: %v", err)
	}
	bw := bufio.NewWriter(f)
	size := 0
	for i := range n {
		c, _ := fmt.Fprintf(bw, "This is translatable line number %d with several words of content\n", i)
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

// runSplicedRoundTripPeak runs a splicedlines → splicedlines round-trip through
// the streaming FileRunner path and returns the peak heap-in-use delta observed
// during the run (sampled by a background goroutine). GC is tightened so the
// sampled peak tracks the live working set rather than GC sawtooth slack.
func runSplicedRoundTripPeak(t *testing.T, inPath, outPath string) uint64 {
	t.Helper()
	ctx := context.Background()

	defer debug.SetGCPercent(debug.SetGCPercent(20))

	runner := flow.NewFileRunner(flow.FileRunnerConfig{SourceLocale: model.LocaleEnglish})

	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)

	var peak uint64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		var m runtime.MemStats
		for {
			select {
			case <-stop:
				return
			default:
			}
			runtime.ReadMemStats(&m)
			if m.HeapAlloc > atomic.LoadUint64(&peak) {
				atomic.StoreUint64(&peak, m.HeapAlloc)
			}
			time.Sleep(300 * time.Microsecond)
		}
	}()

	reader := splicedlines.NewReader()
	writer := splicedlines.NewWriter()
	passthrough := &tool.BaseTool{ToolName: "passthrough"}
	if err := runner.RunFileWithReaderWriter(ctx, "roundtrip", []tool.Tool{passthrough}, inPath, outPath, "", reader, writer); err != nil {
		close(stop)
		<-done
		t.Fatalf("round-trip: %v", err)
	}
	close(stop)
	<-done

	p := atomic.LoadUint64(&peak)
	if p < base.HeapAlloc {
		return 0
	}
	return p - base.HeapAlloc
}

// TestStreamingRoundTripBoundedMemory asserts the streaming splicedlines
// round-trip holds peak memory to a bounded window — sublinear in input size,
// not ~ file size. The old buffered path collected the whole Part slice plus the
// whole file in memory, so its peak was ≥ file size; the streaming path keeps a
// small window, so a 10× larger input must not give a ~10× larger peak and the
// peak must stay well under the (multi-MB) file size.
func TestStreamingRoundTripBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("memory regression test skipped in -short")
	}

	// Guard: the format must actually be on the streaming path, else the test
	// would assert nothing meaningful.
	var r any = splicedlines.NewReader()
	var w any = splicedlines.NewWriter()
	if _, ok := r.(interface{ StreamingReader() bool }); !ok {
		t.Fatal("splicedlines reader is not a StreamingReader")
	}
	if _, ok := w.(interface{ StreamingWriter() bool }); !ok {
		t.Fatal("splicedlines writer is not a StreamingWriter")
	}

	const smallN = 20_000
	const largeN = 200_000 // 10× the input

	dir := t.TempDir()
	smallIn := filepath.Join(dir, "small.txt")
	largeIn := filepath.Join(dir, "large.txt")
	smallSize := writeSplicedFile(t, smallIn, smallN)
	largeSize := writeSplicedFile(t, largeIn, largeN)

	smallPeak := runSplicedRoundTripPeak(t, smallIn, filepath.Join(dir, "small.out"))
	largePeak := runSplicedRoundTripPeak(t, largeIn, filepath.Join(dir, "large.out"))

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
