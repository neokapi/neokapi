package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const kib = 1024

// The bound that replaced the brittle one has to still catch the thing the
// brittle one was for. Loosening a test until it goes green is worse than the
// flake it cures, so the bound is tested against both the readings that made CI
// red and the failures it must never miss.
func TestBoundedMemoryBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                                       string
		smallPeak, smallSize, largePeak, largeSize uint64
		wantProblem                                bool
	}{
		// Real CI readings from the two tests that failed. The documents grew
		// ~20x; peak heap grew 2.2x and 1.3x. These readers were bounded all
		// along. They carry ~1-1.5 MiB of fixed overhead (the SQLite
		// skeleton store), which a fraction-of-document-size bound punishes.
		{"xcstrings on CI", 785 * kib, 247 * kib, 1699 * kib, 5045 * kib, false},
		{"designtokens on CI", 1161 * kib, 224 * kib, 1546 * kib, 4587 * kib, false},
		// Readers with little fixed overhead, which passed even the old bound.
		{"arb on CI", 78 * kib, 154 * kib, 107 * kib, 3181 * kib, false},
		{"xml on CI", 87 * kib, 247 * kib, 159 * kib, 5056 * kib, false},

		// The regression the whole test exists to catch: an implementation that
		// buffers the document retains ~100% of every extra byte.
		{"buffers the whole document", 250 * kib, 250 * kib, 5000 * kib, 5000 * kib, true},
		// Retaining even half of the document is a regression.
		{"retains half the document", 250 * kib, 250 * kib, 2500 * kib, 5000 * kib, true},
		// Memory scaling with input, even under the marginal bound's radar.
		{"peak scales with input", 4000 * kib, 250 * kib, 40_000 * kib, 5000 * kib, true},

		// Fixed overhead alone, however large, is not a leak: peak that does not
		// move as the document grows 20x is exactly what bounded means.
		{"large fixed overhead, no growth", 8000 * kib, 250 * kib, 8000 * kib, 5000 * kib, false},
		// The tmx reader before its names moved to a DigestSet: 14x the peak
		// for 20x the document, which a 1 MiB floor let through.
		{"small peak that grows with the document", 118 * kib, 360 * kib, 1684 * kib, 7350 * kib, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			problems := boundedMemoryProblems(tc.smallPeak, tc.smallSize, tc.largePeak, tc.largeSize, 0)
			if tc.wantProblem {
				assert.NotEmpty(t, problems, "this profile must be reported as unbounded")
				return
			}
			assert.Empty(t, problems, "this profile is bounded and must not be reported")
		})
	}
}

// The per-block allowance covers a reader's two digest sets and no more. The
// tmx reader's peaks with its names in a DigestSet pass. Its peaks with the
// names in a Go map (the reading the old 1 MiB floor let through) fail, and so
// does a reader that keeps each block's text.
func TestBoundedMemoryPerBlockAllowance(t *testing.T) {
	t.Parallel()
	allowance := identityAllowance(2_000, 40_000)

	assert.Empty(t, boundedMemoryProblems(123*kib, 360*kib, 1212*kib, 7350*kib, allowance),
		"two digest sets are within the allowance")
	assert.NotEmpty(t, boundedMemoryProblems(118*kib, 360*kib, 1684*kib, 7350*kib, allowance),
		"a map of names is not")
	assert.NotEmpty(t, boundedMemoryProblems(123*kib, 360*kib, 1212*kib+180*38_000, 7350*kib, allowance),
		"keeping each block's text is not")
}
