package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIndentedOpeningRunsKeepTheirBackslashBreak covers emphasis whose openers
// sit behind a space on a continuation line and are followed by a backslash
// hard break: "0", then " ***" and the break, then "0**", a code span and "*".
// The break's backslash is what keeps the openers left-flanking and the line
// from being a thematic break, and dropped it left a line the block path
// escaped as a thematic break on one pass and the next pass respelled.
func TestIndentedOpeningRunsKeepTheirBackslashBreak(t *testing.T) {
	t.Parallel()
	in := "0\n ***\\\n0**`*0*0`*"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	require.Equal(t, "0\n *__\\\n0__`*0*0`*\n", assertRebuildKeepsItsBlocks(t, in))
}
