//go:build parity

package roundtrip_test

import (
	"testing"

	"github.com/neokapi/neokapi/cli/parity/roundtrip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMarkdownCanonicalFoldsTableCellPadding pins the table-row rule: the
// source's padding around each pipe (which native replays) and Okapi's
// single-space rewrite of it canonicalise to the same bytes, while a row
// that lost a cell still differs.
func TestMarkdownCanonicalFoldsTableCellPadding(t *testing.T) {
	norm := func(s string) string {
		out, err := roundtrip.MarkdownCanonical{}.Normalize([]byte(s))
		require.NoError(t, err)
		return string(out)
	}
	padded := "| Option | Description |\n| ------ | ----------- |\n| data   | path to data files. |\n| ext    | extension. |\n"
	okapi := "| Option | Description |\n| ------ | ----------- |\n| data | path to data files. |\n| ext | extension. |\n"
	compact := "|Option|Description|\n|------|-----------|\n|data|path to data files.|\n|ext|extension.|\n"
	assert.Equal(t, norm(okapi), norm(padded), "source padding and Okapi's rewrite are the same table")
	assert.Equal(t, norm(okapi), norm(compact), "compact rows are the same table")
	assert.NotEqual(t, norm(okapi), norm("| Option | Description |\n| ------ | ----------- |\n| data |\n| ext | extension. |\n"),
		"a lost cell is still a difference")

	// A row inside a list item keeps its indent atom.
	assert.Equal(t, "\t| a | b |", norm("  |  a  |  b  |"))
}

// TestMarkdownCanonicalFoldsATXClosingSequence pins the fold for #2430: Okapi
// writes an ATX heading's closing sequence on a line of its own, native keeps
// it on the heading line, and both spell the same heading.
func TestMarkdownCanonicalFoldsATXClosingSequence(t *testing.T) {
	norm := func(s string) string {
		out, err := roundtrip.MarkdownCanonical{}.Normalize([]byte(s))
		require.NoError(t, err)
		return string(out)
	}
	source := "### An h3 header ###\n\nBody text.\n"
	okapi := "### An h3 header\n###\n\nBody text.\n"
	assert.Equal(t, norm(okapi), norm(source), "the relocated closing sequence is the same heading")
	assert.Equal(t, norm("# a\n\nb\n"), norm("# a #   \n\nb\n"), "trailing whitespace after the sequence")
	assert.Equal(t, norm("# a # b\n"), norm("# a # b #\n"), "only the last run of hashes is a closing sequence")
	assert.Equal(t, "\t# a", norm("   # a #"), "an indented heading keeps its indent atom")

	// Hashes that are content stay, and an empty heading stays a difference.
	assert.Equal(t, "# a \\#", norm("# a \\#"))
	assert.Equal(t, "# a#", norm("# a#"))
	assert.Equal(t, "#", norm("# #"))
	assert.NotEqual(t, norm("# a\n"), norm("#\n"))
	assert.NotEqual(t, norm("# a\n\nb\n"), norm("# a\n\n#\n"), "a bare marker away from a heading line is not dropped")
}
