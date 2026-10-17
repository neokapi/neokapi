package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDegradedOpeningRunsKeepTheirBackslashBreak covers a quote whose pairs
// have no spelling that reads back and are written as text: ">00", then "*"
// and a backslash break, "****" and a backslash break, and "*0******0". The
// line of four asterisks keeps the break's backslash on the pass that wrote
// the runs as markup, and the pass after, reading them as text, dropped it
// and escaped the run as a thematic break instead. A backslash break after a
// line of delimiter runs is kept whatever wrote them.
func TestDegradedOpeningRunsKeepTheirBackslashBreak(t *testing.T) {
	t.Parallel()
	in := ">00\n*\\\n****\\\n*0******0"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	assert.Equal(t, ">00\n\\*\n****\\\n\\_0\\_*****0\n", assertRebuildKeepsItsBlocks(t, in))
}
