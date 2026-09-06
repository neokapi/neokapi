package markdown_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmphasisDelimiterRoundTrips is the #2446 reproducer. goldmark records an
// emphasis's level and not the character that spelled it, and the reader read
// the byte before the first child's segment, which only a text child has. An
// underscore emphasis around anything else came back with asterisks, and
// `_*a*_` came back as `**a**`, which re-reads as strong: the block changed
// kind. The delimiter now comes from the offset the resolver places the
// emphasis at.
func TestEmphasisDelimiterRoundTrips(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		text  string // source text of the first block
	}{
		{"underscore around a link", "_[a](b)_\n", "a"},
		{"underscore around a code span", "_`c`_\n", "c"},
		{"underscore around an autolink", "_<https://x>_\n", ""},
		{"underscore around an image", "__![i](s)__\n", "i"},
		{"underscore around an emphasis", "_*a*_\n", "a"},
		{"underscore around a strong", "_**a**_\n", "a"},
		{"asterisk around a link", "*[a](b)*\n", "a"},
		{"asterisk around an emphasis", "*_a_*\n", "a"},
		{"underscore around text", "_a_\n", "a"},
		{"strong underscore around text", "__a__\n", "a"},
		{"underscore after text", "x _[a](b)_ y\n", "x a y"},
		{"underscore after an emphasis", "*x* _[a](b)_\n", "x a"},
		{"two underscore emphases", "_[a](b)_ and _`c`_\n", "a and c"},
		{"underscore in a heading", "# _[a](b)_\n", "a"},
		{"underscore in a list item", "- _[a](b)_\n", "a"},
		{"underscore in a blockquote", "> _[a](b)_\n", "a"},
		{"underscore around a strikethrough", "_~~a~~_\n", "a"},
		{"underscore around inline HTML", "_<b>a</b>_\n", "a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before := readBlocks(t, tc.input)
			out := roundtripWithSkeleton(t, tc.input)
			require.Equal(t, tc.input, out, "skeleton path is not byte-exact")
			if len(before) == 0 {
				return
			}
			assert.Equal(t, tc.text, before[0].SourceText())
		})
	}
}

// TestUnderscoreEmphasisKeepsItsKind pins what the bug cost beyond the
// spelling: `_*a*_` written back as `**a**` re-reads as one strong emphasis
// where the source had two nested italics, so the runs of the block change.
func TestUnderscoreEmphasisKeepsItsKind(t *testing.T) {
	t.Parallel()
	out := roundtripWithSkeleton(t, "_*a*_\n")
	require.Equal(t, "_*a*_\n", out)

	blocks := readBlocks(t, out)
	require.Len(t, blocks, 1)
	var types []string
	for _, r := range blocks[0].Source {
		if r.PcOpen != nil {
			types = append(types, r.PcOpen.Type)
		}
	}
	assert.Equal(t, []string{"fmt:italic", "fmt:italic"}, types,
		"two nested italics, not one bold")
}

// TestEmphasisDelimiterFixture walks the committed fixture.
func TestEmphasisDelimiterFixture(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/emphasis-delimiters.md")
	require.NoError(t, err)
	assertSkeletonByteExact(t, string(data))
}
