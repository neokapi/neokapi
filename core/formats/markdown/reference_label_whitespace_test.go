package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReferenceMatchesItsDefinitionByNormalizedLabel covers a definition
// whose label spans a line ending, "[" over "0]:0 '0'", and a full reference
// "[0][" over "0]" to it. The rebuild spells the reference's label on one
// line, "[0][ 0]", which the parser still matches: a label is compared with
// its whitespace runs as one space (CommonMark 6.6). The reader keyed the
// references it had seen by their spelling, so the definition's title block
// was gone on the pass after.
func TestReferenceMatchesItsDefinitionByNormalizedLabel(t *testing.T) {
	t.Parallel()
	in := "[\n0]:0 '0'\n[0][\n0]"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	assert.Equal(t, "[\n0]: 0 \"0\"\n\n[0][ 0]\n", assertRebuildKeepsItsBlocks(t, in))
	assert.Equal(t, []string{"0", "0"}, readBlockTexts(t, "[\n0]: 0 \"0\"\n\n[0][ 0]"))
}
