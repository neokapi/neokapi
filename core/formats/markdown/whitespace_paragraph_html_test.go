package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParagraphOfWhitespaceAndInlineHTMLEmitsNoBlock covers a quote whose
// paragraph is a vertical tab and, on a marked second line, an inline tag:
// ">" and a vertical tab over "><A>". The tag is markup with no text and the
// rebuild drops it, so the block's only content was the continuation marker
// baked into its text, and rebuilt as a quote of a vertical tab it read back
// as no block. A paragraph of whitespace and inline HTML emits no block, as
// one of whitespace alone does; its bytes replay from the skeleton.
func TestParagraphOfWhitespaceAndInlineHTMLEmitsNoBlock(t *testing.T) {
	t.Parallel()
	for _, in := range []string{">\v\n><A>", "\v<A>", "<A> \n<B>"} {
		require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
		assert.Empty(t, readBlockTexts(t, in), "input %q", in)
	}
	// Text beside the tag is content.
	assert.Equal(t, []string{"a"}, readBlockTexts(t, "<A>a"))
}
