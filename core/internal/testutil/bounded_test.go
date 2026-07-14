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
		// along — they just carry ~1-1.5 MiB of fixed overhead (the SQLite
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			problems := boundedMemoryProblems(tc.smallPeak, tc.smallSize, tc.largePeak, tc.largeSize)
			if tc.wantProblem {
				assert.NotEmpty(t, problems, "this profile must be reported as unbounded")
				return
			}
			assert.Empty(t, problems, "this profile is bounded and must not be reported")
		})
	}
}
