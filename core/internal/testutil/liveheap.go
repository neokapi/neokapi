package testutil

import (
	"bytes"
	"fmt"
	"runtime"
	"runtime/metrics"
)

// LiveHeap returns the bytes the heap holds live once every other goroutine
// is blocked.
//
// A streaming pipeline keeps working while a test measures it: the reader
// parses ahead into its channels and grows its bookkeeping. A collection run
// beside that work counts the reader's allocations during the mark as live,
// and can catch a growing table holding both its old and new arrays. The
// 40,000-message TS document behind TestStreamingPair_BoundedWriterTS peaked
// between 1.0 and 2.4 MiB that way, and its bound fails at 2.4 MiB.
//
// Once the sampler stops forwarding parts, every stage of the pipeline blocks
// on a full channel or an empty pipe. LiveHeap waits for that, then collects,
// so a measurement depends on the document and the sample point, not on where
// the other goroutines were.
//
// It panics when the other goroutines never all block: a stage does not stop
// when the next stage stops consuming.
func LiveHeap() uint64 {
	waitQuiescent()
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// quiescentAttempts bounds the wait. Each attempt yields the processor, so a
// runnable stage gets to run until it blocks.
const quiescentAttempts = 10_000_000

// waitQuiescent reads the scheduler's own counts first, which costs no
// stop-the-world. They are approximate, so a count that says nothing else
// runs is confirmed against a goroutine dump, which stops the world.
func waitQuiescent() {
	buf := make([]byte, 1<<20)
	sched := []metrics.Sample{
		{Name: "/sched/goroutines/running:goroutines"},
		{Name: "/sched/goroutines/runnable:goroutines"},
	}
	busy := []byte("the scheduler reports another goroutine running or runnable")
	for range quiescentAttempts {
		metrics.Read(sched)
		if sched[0].Value.Uint64() <= 1 && sched[1].Value.Uint64() == 0 {
			n := runtime.Stack(buf, true)
			if busy = busyGoroutine(buf[:n]); busy == nil {
				return
			}
		}
		runtime.Gosched()
	}
	panic(fmt.Sprintf("testutil.LiveHeap: a goroutine kept running, so the heap cannot be measured at rest: %s", busy))
}

// busyGoroutine returns the header of the first goroutine in a runtime.Stack
// dump, after the caller's own, that is running or waiting to run, or nil
// when every one is blocked.
func busyGoroutine(dump []byte) []byte {
	first := true
	for line := range bytes.Lines(dump) {
		if !bytes.HasPrefix(line, []byte("goroutine ")) {
			continue
		}
		if first {
			first = false // the caller
			continue
		}
		open := bytes.IndexByte(line, '[')
		end := bytes.IndexByte(line, ']')
		if open < 0 || end < open {
			continue
		}
		state, _, _ := bytes.Cut(line[open+1:end], []byte(","))
		switch string(state) {
		case "running", "runnable", "preempted":
			return bytes.TrimSpace(line)
		}
	}
	return nil
}
