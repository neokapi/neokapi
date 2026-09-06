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

// TestMarkdownCanonicalFoldsReferenceDefinitionTitlePadding pins the fold for
// #2462: Okapi's writer spells the separator before a definition's title as a
// single space whatever the source padded, and native replays the source's own
// bytes. CommonMark 4.7 reads both as the same definition.
func TestMarkdownCanonicalFoldsReferenceDefinitionTitlePadding(t *testing.T) {
	norm := func(s string) string {
		out, err := roundtrip.MarkdownCanonical{}.Normalize([]byte(s))
		require.NoError(t, err)
		return string(out)
	}
	okapi := "[id]: https://example.com/a.jpg \"The Title\"\n"
	assert.Equal(t, norm(okapi), norm("[id]: https://example.com/a.jpg  \"The Title\"\n"),
		"two spaces before the title spell the same definition")
	assert.Equal(t, norm(okapi), norm("[id]: https://example.com/a.jpg\t\t\"The Title\"\n"),
		"tabs before the title spell the same definition")
	assert.Equal(t, norm("[id]: /a 'T'\n"), norm("[id]: /a   'T'\n"))
	assert.Equal(t, norm("[id]: /a (T)\n"), norm("[id]: /a   (T)\n"))
	assert.NotEqual(t, norm(okapi), norm("[id]: https://example.com/b.jpg \"The Title\"\n"),
		"a different destination is still a difference")
	assert.Equal(t, "[id]:  /a \"T\"", norm("[id]:  /a \"T\""),
		"the separator after the colon is Okapi's too and stays")
}

// TestMarkdownCanonicalDropsReferenceDefinitionIndent pins the fold for #2482:
// okapi strips the 0-3 spaces CommonMark 4.7 allows before a link reference
// definition, and native replays the source's own bytes so a definition inside
// a container keeps what precedes it.
func TestMarkdownCanonicalDropsReferenceDefinitionIndent(t *testing.T) {
	norm := func(s string) string {
		out, err := roundtrip.MarkdownCanonical{}.Normalize([]byte(s))
		require.NoError(t, err)
		return string(out)
	}
	assert.Equal(t, norm("[R]: /x\n"), norm("   [R]: /x\n"))
	assert.Equal(t, norm("[R]: /x 'T'\n"), norm("  [R]: /x  'T'\n"))
}
