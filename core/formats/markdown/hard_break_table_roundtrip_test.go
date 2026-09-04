package markdown_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertSkeletonByteExact reads input, writes it back untranslated through the
// skeleton path and asserts the output is byte-identical, then reads the
// output and asserts it yields the same blocks with the same source text.
func assertSkeletonByteExact(t *testing.T, input string) []*model.Block {
	t.Helper()
	before := readBlocks(t, input)
	require.NotEmpty(t, before, "input has no blocks")

	out := roundtripWithSkeleton(t, input)
	require.Equal(t, input, out, "skeleton path is not byte-exact")

	after := readBlocks(t, out)
	require.Len(t, after, len(before), "block count changed on re-read")
	for i := range before {
		assert.Equal(t, before[i].SourceText(), after[i].SourceText(), "block %d text changed on re-read", i)
	}
	return before
}

// TestHardBreakSpellingRoundTrips is the hard-break half of #1661. The bytes
// that spell a hard line break (two or more spaces, or a backslash, before
// the newline) used to be trimmed on read and never written back, so the
// skeleton path turned every hard break into a soft one. The spelling now
// rides a placeholder while the block's text keeps the bare "\n" it always
// had, so the content key of a paragraph with a hard break is unchanged.
func TestHardBreakSpellingRoundTrips(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		text  string // source text of the first block
	}{
		{"two spaces", "Some text with a hard break here.  \nAnd the line that follows it.\n",
			"Some text with a hard break here.\nAnd the line that follows it."},
		{"five spaces", "0     \n0\n", "0\n0"},
		{"backslash", "a\\\nb\n", "a\nb"},
		{"space then backslash", "a \\\nb\n", "a\nb"},
		{"crlf", "a  \r\nb\r\n", "a\nb"},
		{"inside emphasis", "*a  \nb*\n", "a\nb"},
		{"after a code span", "`c`  \nb\n", "c\nb"},
		{"after a link", "[l](u)  \nb\n", "l\nb"},
		{"in a blockquote", "> a  \n> b\n", "a\nb"},
		{"in a list item", "- a  \n  b\n", "a\nb"},
		{"mixed with a soft break", "Foo\nbar  \nbaz\n", "Foo\nbar\nbaz"},
		{"mixed in a blockquote", "> a  \n> b\n> c\n", "a\n> b\n> c"},
		{"mixed in a list item", "- a  \n  b\n  c\n", "a\n  b\n  c"},
		{"inconsistent blockquote prefix", "> a  \n>b  \n> c\n", "a\n>b\n> c"},
		{"in a setext heading", "a  \nb\n===\n", "a\nb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			blocks := assertSkeletonByteExact(t, tc.input)
			assert.Equal(t, tc.text, blocks[0].SourceText(), "the block text carries the break as a bare newline")
		})
	}
}

// TestTableBytesRoundTrip is the table half of #1661. The reader used to
// synthesise `| ` and ` ` around every cell and re-derive the rest of the
// table, so cell padding collapsed, a table inside a blockquote or list item
// lost its container marker, a header-only table doubled its delimiter row,
// a comment-only cell vanished, and an escaped pipe outside a code span came
// back doubled. Every byte that is not a cell's content now replays from
// source.
func TestTableBytesRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"padded columns", "| Version | Date       | Notes            |\n| ------- | ---------- | ---------------- |\n| 1.0     | 2026-01-01 | Initial release. |\n"},
		{"compact rows", "|a|b|\n|-|-|\n|1|2|\n"},
		{"alignment colons", "| a | b |\n|:--|--:|\n| 1 | 2 |\n"},
		{"no outer pipes", "a | b\n--- | ---\n1 | 2\n"},
		{"short row", "| a | b |\n|---|---|\n| 1 |\n"},
		{"long row", "| a | b |\n| --- | --- |\n| 1 | 2 | 3 |\n"},
		{"empty header cells", "|  |  |\n| --- | --- |\n| 1 | 2 |\n"},
		{"empty body row", "| a |\n| - |\n|  |\n"},
		{"header only", "| a | b |\n| --- | --- |\n"},
		{"trailing whitespace after the last pipe", "| a | b |\n| --- | --- |\n| 1 | 2 |  \n"},
		{"no final newline", "| a | b |\n| --- | --- |\n| 1 | 2 |"},
		{"comment-only cell", "| a | b |\n| --- | --- |\n| <!-- c --> | 2 |\n"},
		{"entity and inline html in cells", "| a | b |\n| --- | --- |\n| &amp; | <b>x</b> |\n"},
		{"escaped pipe in a body cell", "| a | b |\n| --- | --- |\n| x\\|y | 2 |\n"},
		{"escaped pipe in a header cell", "| a \\| b | c |\n| --- | --- |\n| 1 | 2 |\n"},
		{"escaped pipe in a code span", "| a | b |\n| --- | --- |\n| `x\\|y` | 2 |\n"},
		{"escaped pipe in a header code span", "| `a\\|b` | c |\n| --- | --- |\n| 1 | 2 |\n"},
		{"escaped pipe beside a code span", "| a | b |\n| --- | --- |\n| `x` y\\|z | 2 |\n"},
		{"cell that is only an escaped pipe", "|a|\n|-|\n|\\||\n"},
		{"indented", "  | a | b |\n  | --- | --- |\n  | 1 | 2 |\n"},
		{"in a blockquote", "> | a | b |\n> | --- | --- |\n> | 1 | 2 |\n"},
		{"after a blockquote paragraph", "> q\n>\n> | a | b |\n> | --- | --- |\n> | 1 | 2 |\n"},
		{"in a list item", "- | a | b |\n  | --- | --- |\n  | 1 | 2 |\n"},
		{"between ordered items", "1. x\n\n   | a | b |\n   | --- | --- |\n   | 1 | 2 |\n\n2. y\n"},
		{"followed by a bare row", "| a | b |\n| --- | --- |\n| 1 | 2 |\nafter\n"},
		{"followed by a list", "| a | b |\n| --- | --- |\n| 1 | 2 |\n- item\n"},
		{"two tables", "| a | b |\n| --- | --- |\n| 1 | 2 |\n\n| c | d |\n| --- | --- |\n| 3 | 4 |\n"},
		{"after front matter", "---\ntitle: T\n---\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n"},
		{"between a heading and a paragraph", "# H\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n\nPara.\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertSkeletonByteExact(t, tc.input)
		})
	}
}

// TestTableCellPipeEscapedOnce pins the writer's escaping rule for a table
// cell: a pipe leaving a cell is escaped unless the byte before it is already
// a backslash, which is how the GFM row splitter decides whether a pipe
// belongs to the cell. A translated cell can spell a pipe either way.
func TestTableCellPipeEscapedOnce(t *testing.T) {
	t.Parallel()
	input := "| a | b |\n| --- | --- |\n| x | 2 |\n"
	blocks := readBlocks(t, input)
	require.Len(t, blocks, 4)

	require.Equal(t, "x", blocks[2].SourceText())

	out := roundtripTranslated(t, input, model.LocaleGerman, func(blocks []*model.Block) {
		blocks[2].SetTargetText(model.LocaleGerman, "bare | and escaped \\| pipes")
	})
	assert.Equal(t, "| a | b |\n| --- | --- |\n| bare \\| and escaped \\| pipes | 2 |\n", out)
}

// roundtripTranslated reads input through the skeleton path, hands the blocks
// to translate so it can set targets on them, and writes the document back
// for locale.
func roundtripTranslated(t *testing.T, input string, locale model.LocaleID, translate func([]*model.Block)) string {
	t.Helper()
	ctx := t.Context()

	reader := markdown.NewReader()
	writer := markdown.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	translate(testutil.FilterBlocks(parts))

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(locale)
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}
