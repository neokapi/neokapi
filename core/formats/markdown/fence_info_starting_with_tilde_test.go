package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFenceInfoStartingWithTheFenceCharacterIsSeparated covers a tilde fence
// whose info string begins with a tilde: "~~~ ~" then a tab, "~" and a
// backtick, over "0". The backtick keeps the fence tildes, and written with
// no space the info string's tilde lengthened the opening fence to four, which
// the closing fence of three did not close, so every pass grew the block by
// one closing fence.
func TestFenceInfoStartingWithTheFenceCharacterIsSeparated(t *testing.T) {
	t.Parallel()
	in := "~~~ ~\t~`\n0"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	assert.Equal(t, "~~~ ~\t~`\n0\n~~~\n", assertRebuildKeepsItsBlocks(t, in))
}
