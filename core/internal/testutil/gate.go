package testutil

import (
	"fmt"
	"io"
	"math"
	"runtime"
	"sync"
)

// Gate feeds a streaming pipeline its input in measured steps, so a memory
// test can stop the pipeline at a point it chooses and measure the heap with
// nothing running.
//
// A streaming reader keeps parsing ahead while a test samples it. A
// collection run beside that work counts the reader's allocations during the
// mark as live, and can catch a growing table holding both its arrays: the
// 40,000-message TS document behind TestStreamingPair_BoundedWriterTS peaked
// anywhere from 1.0 to 2.4 MiB that way, against a bound that fails at 2.4.
//
// The gate gives the reader its input up to a byte budget. When the reader
// asks for more than the budget allows, it blocks inside the gate, and the
// channel Open returned closes. The reader has then sent every part that
// input yields. Once the test has received those parts, nothing in the
// pipeline is working, by construction rather than by observation.
type Gate struct {
	mu      sync.Mutex
	cond    *sync.Cond
	r       io.Reader
	ra      io.ReaderAt
	used    int64
	budget  int64
	stopped chan struct{}
	closed  bool
}

// NewGate gates r, with a budget of nothing. A reader whose Open reads input
// needs a budget first: call Open with it before opening the reader.
func NewGate(r io.Reader) *Gate {
	g := &Gate{r: r, stopped: make(chan struct{})}
	if ra, ok := r.(io.ReaderAt); ok {
		g.ra = ra
	}
	g.cond = sync.NewCond(&g.mu)
	return g
}

// Open raises the budget to budget bytes from the start of the input and
// returns a channel that closes when the reader has used it and waits for
// more. A budget of math.MaxInt64 lets the rest of the input through; its
// channel never closes, and the pipeline's own end is the signal.
func (g *Gate) Open(budget int64) <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.budget = budget
	g.stopped = make(chan struct{})
	g.closed = false
	g.cond.Broadcast()
	return g.stopped
}

// stop closes the current epoch's channel once. The caller holds mu.
func (g *Gate) stop() {
	if !g.closed {
		close(g.stopped)
		g.closed = true
	}
}

// wait blocks until n more bytes fit the budget. The caller holds mu.
func (g *Gate) wait(n int64) {
	for g.used+n > g.budget {
		g.stop()
		g.cond.Wait()
	}
}

// Read reads at most what the budget allows, waiting at the gate for more.
func (g *Gate) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	g.mu.Lock()
	g.wait(1)
	n := min(int64(len(p)), g.budget-g.used)
	g.used += n
	g.mu.Unlock()
	got, err := g.r.Read(p[:n])
	g.mu.Lock()
	g.used -= n - int64(got)
	g.mu.Unlock()
	return got, err
}

// ReadAt reads len(p) bytes at off once they fit the budget. A reader of a
// random-access document reads it in calls of this kind, so its budget counts
// the bytes those calls ask for, in the order the reader asks.
func (g *Gate) ReadAt(p []byte, off int64) (int, error) {
	g.mu.Lock()
	if g.budget != math.MaxInt64 {
		g.wait(int64(len(p)))
	}
	g.used += int64(len(p))
	g.mu.Unlock()
	return g.ra.ReadAt(p, off)
}

// Measure collects once and returns the bytes the heap holds live. Call it
// only while the pipeline is stopped at a gate, where nothing allocates
// during the collection.
func Measure() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// measureBaseline is Measure with a second collection first. A sync.Pool
// keeps what it held before a collection in a victim cache until the next
// one, so a single collection counts whatever setup left in a pool as live.
// archive/zip's compressor pool, filled while a test wrote its fixture, can
// put 1 MiB into a baseline that way. Every later sample then sits below it
// and the peak comes out as zero. Samples keep one collection, so a pool the pipeline
// fills counts against it, never for it.
func measureBaseline() uint64 {
	runtime.GC()
	return Measure()
}

// minPeak is the least a pipeline that has read input holds above its
// baseline: at least the part in hand. A smaller peak means the samples did
// not see the pipeline at all, and it would pass any bound.
const minPeak = 1 << 10

// checkPeak panics on a peak too small to have measured anything.
func checkPeak(peak uint64) {
	if peak < minPeak {
		panic(fmt.Sprintf("testutil: the measured peak is %d bytes, below the %d a pipeline holding one part reaches: the baseline was taken over something the samples no longer hold", peak, minPeak))
	}
}

// SamplePeak drives a pipeline fed through g in samples equal steps of total
// input bytes. It returns the largest live heap it measured above the
// pipeline's heap before any input. handle receives every value ch carries,
// in order; it must not keep them. ch is drained to its end before
// SamplePeak returns.
func SamplePeak[T any](g *Gate, total int64, samples int, ch <-chan T, handle func(T)) uint64 {
	return samplePeak(g, total, samples, ch, handle, func() {})
}

// StreamingStore is the side of a streaming skeleton store that tells a test
// the writer has caught up. *format.SkeletonStore satisfies it.
type StreamingStore interface {
	SetWaitHook(func())
	Waiting() bool
}

// SamplePeakPair is SamplePeak for a reader feeding a writer through a
// streaming skeleton store. handle passes parts on to the writer through a
// buffered channel, so it never waits on the writer. At each stop the writer
// also catches up: it writes everything the reader gave it and waits on the
// store for the next entry, and only then is the heap measured.
func SamplePeakPair[T any](g *Gate, total int64, samples int, ch <-chan T, handle func(T), store StreamingStore) uint64 {
	waited := make(chan struct{}, 1)
	store.SetWaitHook(func() {
		select {
		case waited <- struct{}{}:
		default:
		}
	})
	defer store.SetWaitHook(nil)
	return samplePeak(g, total, samples, ch, handle, func() {
		for !store.Waiting() {
			<-waited
		}
	})
}

// SamplePeakStops is SamplePeak for a pipeline the code under test drives,
// such as a runner that owns the stages between the reader and the writer.
// At each stop the reader waits at g and the writer has caught up and waits
// on store; the parts the runner holds in its channels are then a fixed set.
// done closes when the pipeline has finished; SamplePeakStops returns then,
// or after its last stop.
func SamplePeakStops(g *Gate, total int64, samples int, store StreamingStore, done <-chan struct{}) uint64 {
	waited := make(chan struct{}, 1)
	store.SetWaitHook(func() {
		select {
		case waited <- struct{}{}:
		default:
		}
	})
	defer store.SetWaitHook(nil)
	// stop waits for the reader to use its budget and the writer to catch
	// up, and reports false when the pipeline ends first.
	stop := func(stopped <-chan struct{}) bool {
		select {
		case <-stopped:
		case <-done:
			return false
		}
		for !store.Waiting() {
			select {
			case <-waited:
			case <-done:
				return false
			}
		}
		return true
	}
	if !stop(g.Open(0)) {
		panic(errNoStop)
	}
	base := measureBaseline()
	var peak uint64
	stops := 0
	for i := 1; i < samples; i++ {
		if !stop(g.Open(total * int64(i) / int64(samples))) {
			break
		}
		stops++
		if h := Measure(); h > base && h-base > peak {
			peak = h - base
		}
	}
	g.Open(math.MaxInt64)
	if stops == 0 {
		panic(errNoStop)
	}
	checkPeak(peak)
	return peak
}

// errNoStop is the panic of a sampler whose pipeline finished without
// stopping at the gate. Its peak would be zero and pass any bound, so the
// test fails instead.
const errNoStop = "testutil: the pipeline finished without stopping at the gate, so nothing was measured: does its reader read the gated input?"

func samplePeak[T any](g *Gate, total int64, samples int, ch <-chan T, handle func(T), settle func()) uint64 {
	if !drain(ch, g.Open(0), handle) {
		panic(errNoStop)
	}
	settle()
	base := measureBaseline()
	var peak uint64
	stops := 0
	for i := 1; i <= samples; i++ {
		budget := total * int64(i) / int64(samples)
		if i == samples {
			budget = math.MaxInt64
		}
		if !drain(ch, g.Open(budget), handle) {
			break
		}
		stops++
		settle()
		if h := Measure(); h > base && h-base > peak {
			peak = h - base
		}
	}
	g.Open(math.MaxInt64)
	for v := range ch {
		handle(v)
	}
	if stops == 0 {
		panic(errNoStop)
	}
	checkPeak(peak)
	return peak
}

// drain hands handle everything ch delivers until stopped closes and ch has
// nothing waiting. It reports false when ch closes.
func drain[T any](ch <-chan T, stopped <-chan struct{}, handle func(T)) bool {
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				return false
			}
			handle(v)
		case <-stopped:
			for {
				select {
				case v, ok := <-ch:
					if !ok {
						return false
					}
					handle(v)
				default:
					return true
				}
			}
		}
	}
}
