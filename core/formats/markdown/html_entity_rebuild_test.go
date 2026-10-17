package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildSpellsAnEntityAsRead covers a character reference on the rebuild
// path. Its placeholder's equivalent is the decoded character, and a block
// that is one reference renders no text of its own, so writing the character
// turned the block into a text block on re-read: "<div>&ac;<A>0" read as one
// block and its rebuild as two.
func TestRebuildSpellsAnEntityAsRead(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"entity alone in an HTML block before a tag", "<div>&ac;<A>0", "&ac;\n\n0\n"},
		{"entity alone in a paragraph", "&amp;\n\n0", "&amp;\n\n0\n"},
		{"entity before dropped inline HTML", "&amp;<A>0", "&amp;0\n"},
		{"angle bracket reference", "a &lt; b", "a &lt; b\n"},
		{"ampersand reference", "a &amp; b", "a &amp; b\n"},
		{"numeric reference", "a &#38; b", "a &#38; b\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assert.Equal(t, tc.want, assertRebuildKeepsItsBlocks(t, tc.input))
		})
	}
}
