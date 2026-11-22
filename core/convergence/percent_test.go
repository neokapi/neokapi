package convergence

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Percent rounds to the nearest whole percent and never reports a part as
// all or none: 999 of 1000 is 99, and 1 of 1000 is 1.
func TestPercent(t *testing.T) {
	for _, tc := range []struct{ n, total, want int }{
		{0, 0, 100},
		{0, 10, 0},
		{10, 10, 100},
		{1, 3, 33},
		{2, 3, 67},
		{999, 1000, 99},
		{995, 1000, 99},
		{1, 1000, 1},
		{4, 1000, 1},
		{5, 1000, 1},
		{500, 1000, 50},
	} {
		assert.Equal(t, tc.want, Percent(tc.n, tc.total), "%d of %d", tc.n, tc.total)
	}
}
