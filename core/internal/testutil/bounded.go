package testutil

import (
	"fmt"
	"testing"
)

// AssertBoundedMemory checks the property a streaming reader or writer actually
// promises: it does not retain the document.
//
// The obvious test — "peak heap must be under a quarter of the document size" —
// is not that property, and it fails on exactly the readers we most want to
// trust. A reader with a fixed overhead (the SQLite skeleton store costs 1–1.5
// MiB before it reads a byte) blows a size-relative bound on a *small* document
// and sails through it on a large one: the bound gets easier the bigger the
// input, which is backwards. It failed CI on 5 MiB documents while the readers
// were provably bounded — the document grew 20x and peak heap grew 1.3x.
//
// What "bounded" means is that peak memory does not grow *with* the document. So
// measure the growth directly, at two input sizes, and assert two things:
//
//   - Marginal retention: the extra heap used for the extra document must be a
//     small fraction of it. A reader that buffers the whole document has a
//     marginal cost near 1.0; a streaming one is near zero. Fixed overhead
//     cancels out of a difference, which is the whole point.
//
//   - No scaling: 20x the document must not mean 20x the memory, with a floor so
//     that a reader whose peak is dominated by fixed overhead isn't held to a
//     ratio of a number that is mostly noise.
//
// peaks and sizes are bytes.
func AssertBoundedMemory(t *testing.T, label string, smallPeak, smallSize, largePeak, largeSize uint64) {
	t.Helper()

	if largeSize <= smallSize {
		t.Fatalf("%s: bounded-memory check needs a larger second document (small=%d, large=%d)",
			label, smallSize, largeSize)
	}

	t.Logf("%s peakΔ: small(%d KiB)=%d KiB, large(%d KiB)=%d KiB",
		label, smallSize/1024, smallPeak/1024, largeSize/1024, largePeak/1024)

	for _, problem := range boundedMemoryProblems(smallPeak, smallSize, largePeak, largeSize) {
		t.Errorf("%s: %s", label, problem)
	}
}

// boundedMemoryProblems is the decision, separated from the reporting so the
// bound itself can be tested — a loosened bound that no longer catches a
// whole-document buffer would be worse than the brittle one it replaced.
func boundedMemoryProblems(smallPeak, smallSize, largePeak, largeSize uint64) []string {
	var problems []string

	// Marginal retention. If peak did not grow at all, the implementation is
	// trivially bounded and there is nothing to check.
	if largePeak > smallPeak {
		extraHeap := largePeak - smallPeak
		extraDoc := largeSize - smallSize
		if ratio := float64(extraHeap) / float64(extraDoc); ratio > maxMarginalRetention {
			problems = append(problems, fmt.Sprintf(
				"peak heap grew by %d KiB for %d KiB more document (%.0f%% retained; "+
					"whole-document buffering retains ~100%%, streaming ~0%%)",
				extraHeap/1024, extraDoc/1024, ratio*100))
		}
	}

	// And peak must not scale with input. The floor keeps an implementation whose
	// peak is mostly fixed overhead from being judged on a ratio of noise.
	if floor := max(smallPeak, minPeakFloor); largePeak > floor*maxPeakGrowth {
		problems = append(problems, fmt.Sprintf(
			"peak heap %d KiB scaled with input (small doc peaked at %d KiB; "+
				"a %dx bigger document must not mean %dx the memory)",
			largePeak/1024, smallPeak/1024, largeSize/max(smallSize, 1), maxPeakGrowth))
	}

	return problems
}

const (
	// maxMarginalRetention is the share of additional document a streaming
	// implementation may hold on to. Whole-document buffering sits near 1.0, so
	// this is far below any real regression while tolerating the per-part
	// allocations a streaming reader legitimately has in flight.
	maxMarginalRetention = 0.35

	// minPeakFloor stops the growth ratio from being computed against a peak
	// that is mostly fixed overhead and measurement noise.
	minPeakFloor = 1 << 20 // 1 MiB

	// maxPeakGrowth is how much peak heap may grow when the document grows 20x.
	maxPeakGrowth = 3
)
