package testutil

import (
	"bytes"
	"io"
	"math"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lines reads src a line at a time in its own goroutine, the way a streaming
// reader does, and sends each line it completes.
func lines(src io.Reader) <-chan string {
	ch := make(chan string, 4)
	go func() {
		defer close(ch)
		var line bytes.Buffer
		buf := make([]byte, 3)
		for {
			n, err := src.Read(buf)
			for _, b := range buf[:n] {
				if b == '\n' {
					ch <- line.String()
					line.Reset()
					continue
				}
				line.WriteByte(b)
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}

func TestGateStopsTheReaderAtItsBudget(t *testing.T) {
	gate := NewGate(strings.NewReader("one\ntwo\nthree\nfour\n"))
	ch := lines(gate)

	var got []string
	collect := func(s string) { got = append(got, s) }
	require.True(t, drain(ch, gate.Open(8), collect), "the reader stops inside the input")
	assert.Equal(t, []string{"one", "two"}, got, "a budget of 8 bytes yields the lines it holds, and no more")

	require.True(t, drain(ch, gate.Open(14), collect))
	assert.Equal(t, []string{"one", "two", "three"}, got)

	assert.False(t, drain(ch, gate.Open(math.MaxInt64), collect), "the last budget runs to the end")
	assert.Equal(t, []string{"one", "two", "three", "four"}, got)
}

func TestGateReadAtWaitsForTheWholeCall(t *testing.T) {
	gate := NewGate(strings.NewReader("abcdefgh"))
	done := make(chan []byte)
	go func() {
		p := make([]byte, 5)
		_, _ = gate.ReadAt(p, 2)
		done <- p
	}()
	<-gate.Open(4) // too small for the call: the caller waits at the gate
	select {
	case <-done:
		t.Fatal("ReadAt read past its budget")
	default:
	}
	gate.Open(math.MaxInt64)
	assert.Equal(t, "cdefg", string(<-done))
}

// SamplePeak counts what the pipeline holds at each stop: a reader that keeps
// every line it reads shows a peak that grows with the input.
func TestSamplePeakCountsWhatTheReaderKeeps(t *testing.T) {
	line := strings.Repeat("x", 1023) + "\n"
	doc := strings.Repeat(line, 4096) // 4 MiB
	gate := NewGate(strings.NewReader(doc))
	var kept []string
	peak := SamplePeak(gate, int64(len(doc)), 8, lines(gate), func(s string) { kept = append(kept, s) })
	assert.Len(t, kept, 4096)
	assert.Greater(t, peak, uint64(len(doc)/2), "most of the document is live at the last stop")
}

// A pool filled before the run must not lift the baseline over every sample:
// the pipeline below holds only the lines in flight, about a kilobyte, while
// the pool holds a megabyte the samples no longer see.
func TestSamplePeakIgnoresWhatAPoolHeldBeforeTheRun(t *testing.T) {
	var pool sync.Pool
	for range 8 {
		b := make([]byte, 128<<10)
		pool.Put(&b)
	}
	line := strings.Repeat("x", 1023) + "\n"
	doc := strings.Repeat(line, 1024)
	gate := NewGate(strings.NewReader(doc))
	peak := SamplePeak(gate, int64(len(doc)), 16, lines(gate), func(string) {})
	assert.Positive(t, peak, "the lines in flight are measured above the baseline")
	runtime.KeepAlive(&pool)
}

func TestCheckPeakRefusesAPeakThatMeasuredNothing(t *testing.T) {
	assert.Panics(t, func() { checkPeak(0) })
	assert.NotPanics(t, func() { checkPeak(1) }, "a small peak is a measurement")
}
