package testutil

import (
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// AssertBoundedMemory checks the property a streaming reader or writer
// promises: it does not retain the document.
//
// A bound relative to document size punishes fixed overhead on a small
// document and gets easier as the input grows.
//
// Bounded means peak memory does not grow with the document. The check
// measures two input sizes and asserts two things:
//
//   - Marginal retention: the extra heap for the extra document is a small
//     fraction of it. A whole-document buffer retains about all of it; a
//     streaming reader about none. Fixed overhead cancels out of the
//     difference.
//
//   - No scaling: 20x the document must not mean 3x the memory. A floor keeps
//     a peak that is mostly fixed overhead from being judged on a ratio of
//     noise.
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

	for _, problem := range boundedMemoryProblems(smallPeak, smallSize, largePeak, largeSize, 0) {
		t.Errorf("%s: %s", label, problem)
	}
}

// AssertBoundedMemoryPerBlock is AssertBoundedMemory for a reader that must
// remember every block it has named. A format whose keys may repeat (a TMX
// `tuid`, a Qt TS message with no `id`) needs a digest of each name and ID to
// give a repeat its ordinal in one pass, so its peak grows with the number of
// blocks by the size of those two model.DigestSets. The growth check allows
// the measured growth of two such sets, plus an eighth, and nothing more. A
// reader that keeps a map per block or any of a block's content fails it.
func AssertBoundedMemoryPerBlock(t *testing.T, label string, smallPeak, smallSize, smallBlocks, largePeak, largeSize, largeBlocks uint64) {
	t.Helper()

	if largeSize <= smallSize || largeBlocks <= smallBlocks {
		t.Fatalf("%s: bounded-memory check needs a larger second document (small=%d bytes/%d blocks, large=%d bytes/%d blocks)",
			label, smallSize, smallBlocks, largeSize, largeBlocks)
	}

	t.Logf("%s peakΔ: small(%d KiB, %d blocks)=%d KiB, large(%d KiB, %d blocks)=%d KiB",
		label, smallSize/1024, smallBlocks, smallPeak/1024, largeSize/1024, largeBlocks, largePeak/1024)

	allowance := identityAllowance(smallBlocks, largeBlocks)
	for _, problem := range boundedMemoryProblems(smallPeak, smallSize, largePeak, largeSize, allowance) {
		t.Errorf("%s: %s", label, problem)
	}
}

// identityAllowance is how much two model.DigestSets grow between holding
// small and large digests, plus an eighth for the slack of a sample.
func identityAllowance(small, large uint64) uint64 {
	grown := uint64(digestSetFootprint(large) - digestSetFootprint(small))
	return 2 * grown * 9 / 8
}

// digestSetFootprint fills a model.DigestSet with n digests and returns the
// bytes its table holds.
func digestSetFootprint(n uint64) int {
	var s model.DigestSet
	for i := range n {
		s.Add(i + 1)
	}
	return s.Footprint()
}

// boundedMemoryProblems is the decision, separated from the reporting so the
// bound itself can be tested. A loosened bound that no longer catches a
// whole-document buffer would be worse than the brittle one it replaced.
//
// allowance is how many bytes the large peak may exceed the growth bound by:
// the identity sets of AssertBoundedMemoryPerBlock, or zero.
func boundedMemoryProblems(smallPeak, smallSize, largePeak, largeSize, allowance uint64) []string {
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
	if floor := max(smallPeak, minPeakFloor); largePeak > floor*maxPeakGrowth+allowance {
		problems = append(problems, fmt.Sprintf(
			"peak heap %d KiB scaled with input (small doc peaked at %d KiB; "+
				"a %dx bigger document must not mean %dx the memory, beyond %d KiB for block identity)",
			largePeak/1024, smallPeak/1024, largeSize/max(smallSize, 1), maxPeakGrowth, allowance/1024))
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
	// The samples are taken with the pipeline stopped at a Gate, so a small
	// peak varies by tens of KiB, not by the MiB a concurrent sample did.
	minPeakFloor = 128 << 10 // 128 KiB

	// maxPeakGrowth is how much peak heap may grow when the document grows 20x.
	maxPeakGrowth = 3
)
