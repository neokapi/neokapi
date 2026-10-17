package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildEscapesACharacterReferenceFormedAcrossRuns covers an ampersand
// whose character reference is completed by a later text run once the inline
// HTML between them is dropped: "&A<A>;" was written as "&A;", which the
// reader reads as one character reference by its shape, and a paragraph of
// one reference is no block at all. The ampersand is escaped where the output
// as a whole spells a reference from it.
func TestRebuildEscapesACharacterReferenceFormedAcrossRuns(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"named reference", "&A<A>;"},
		{"known named reference", "&amp<A>;"},
		{"numeric reference", "&#1<A>;"},
		{"reference in a sentence", "a &A<A>; b"},
		// Control: an ampersand that completes nothing stays bare.
		{"bare ampersand", "a <A>& b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
	assert.Equal(t, "\\&A;\n", assertRebuildKeepsItsBlocks(t, "&A<A>;"))
	assert.Equal(t, "a & b\n", assertRebuildKeepsItsBlocks(t, "a <A>& b"))
}
