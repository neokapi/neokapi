package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStrikethroughNestedInOneTildeRun is the #3216 reproducer. GFM pairs a
// closing tilde run with an opening run of the same length, and the parser
// reads "~~a~[b~" as a pair inside a pair, each opened by one of the two
// tildes. The opener was read as the whole run before the content, so every
// tilde came back doubled on the skeleton path.
func TestStrikethroughNestedInOneTildeRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"bracket after the inner closer", "~~a~[b~"},
		{"parenthesis after the inner closer", "~~a~(b~"},
		{"period after the inner closer", "~~a~.b~"},
		{"in a sentence", "An unbalanced ~~strike~[b~ here.\n"},
		// Controls: the shapes #2500 pinned, and a plain pair.
		{"shorter closing run", "An unbalanced ~~strike~ here.\n"},
		{"shorter opening run", "An unbalanced ~strike~~ here.\n"},
		{"plain pair", "A ~~struck~~ word here.\n"},
		{"single tilde pair", "A ~struck~ word here.\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}

// TestStrikethroughNestedInOneTildeRunText pins the block's text: the tildes
// are markup and the brackets are content.
func TestStrikethroughNestedInOneTildeRunText(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"a[b"}, readBlockTexts(t, "~~a~[b~"))
}
