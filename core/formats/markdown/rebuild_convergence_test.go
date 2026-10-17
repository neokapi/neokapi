package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRebuildWritesWhatItsReReadWrites covers a paragraph's rendered text as
// a whole: it is the spelling its own re-read settles on. A pass spells
// markup from the parser's rules and checks the spelling against the parser,
// but a re-read hands the next pass a different run structure for the same
// text where a delimiter was re-spelled, and that pass could spell it
// differently again: ">____!" and a backslash break over ">_!__" was written
// with its literal underscore bare, and the pass after, holding the inner
// pair as "_", escaped it. The text is now read back and rendered again until
// a pass renders what it read.
func TestRebuildWritesWhatItsReReadWrites(t *testing.T) {
	t.Parallel()
	for _, in := range []string{">____!\\\n>_!__", ">!___!\\\n>_!_", "a *b* c"} {
		require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
		assertRebuildKeepsItsBlocks(t, in)
	}
}
