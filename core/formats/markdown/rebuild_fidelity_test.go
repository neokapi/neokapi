package markdown_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertRebuildKeepsItsBlocks drives input through the writer's no-skeleton
// rebuild path and asserts the contract FuzzRoundTripMarkdown holds it to: the
// rebuilt document still reads as blocks, it carries the same number of them,
// and from the first pass on the content identity is a fixed point.
func assertRebuildKeepsItsBlocks(t *testing.T, input string) (out1 string) {
	t.Helper()
	ctx := context.Background()

	first, keys1, ok := tripMarkdown(ctx, []byte(input))
	require.True(t, ok, "the reader declined %q", input)

	second, keys2, ok := tripMarkdown(ctx, first)
	require.True(t, ok, "re-reading the rebuilt Markdown lost every block:\nin:   %q\nout1: %q", input, first)
	require.Len(t, keys2, len(keys1), "round-trip changed the block count:\nin:   %q\nout1: %q", input, first)

	_, keys3, ok := tripMarkdown(ctx, second)
	require.True(t, ok, "re-reading the writer's own output lost every block: %q", second)
	assert.Equal(t, keys2, keys3, "block content identity drifted after the first pass:\nin:   %q\nout1: %q", input, first)
	assert.Equal(t, string(first), string(second), "the rebuild path is not idempotent for %q", input)
	return string(first)
}

// TestRebuildEscapesAMarkerACarriageReturnExposes is the #2528 reproducer. A
// document whose lines end in a bare carriage return, the classic Mac ending,
// splits on the rebuild path: goldmark ends a line on "\n" alone, so a "\r"
// stays inside the line as whitespace, the leading one is dropped from the
// block's text, and the "#" it sat in front of opens a heading on the way back
// in.
func TestRebuildEscapesAMarkerACarriageReturnExposes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"heading marker and a second line", "\r#\r0\n0"},
		{"heading marker twice", "\r#\r#"},
		{"no marker to expose", "a\rb\rc"},
		{"bullet marker", "\r-\r0"},
		{"thematic break", "\r---\r0"},
		{"quote marker", "\r>\r0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRebuildKeepsItsBlocks(t, tc.input)
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input),
				"the skeleton path replays these bytes exactly")
		})
	}
}

// TestRebuildEscapesALiteralEmphasisDelimiter is the #2527 reproducer. A "*" or
// "_" the reader hands over as text is one goldmark left literal where the
// source stood; the rebuild path can put it beside an emphasis it re-spelled,
// where the two pair and the block's content moves. #2499 fixed the case of two
// pairs that touch; a run of three or more delimiters still drifted.
func TestRebuildEscapesALiteralEmphasisDelimiter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"pair after a literal run", "_*__0_"},
		{"code span between the pairs", "_*_`*_"},
		{"underscore run before a pair", "__*__0_"},
		{"asterisk run after a pair", "_0_**"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}

// TestRebuildLeavesALoneDelimiterAlone pins the other half of #2527: a
// delimiter that cannot pair in the output is left as the source spelled it,
// because a backslash in front of it would spell a character into text that
// reads back the same without one.
func TestRebuildLeavesALoneDelimiterAlone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"spaced asterisks flank nothing", "2 * 3 * 4\n", "2 * 3 * 4\n"},
		{"intraword underscores", "foo_bar_baz\n", "foo_bar_baz\n"},
		{"a single asterisk", "5*3 in a sentence\n", "5*3 in a sentence\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, assertRebuildKeepsItsBlocks(t, tc.input))
		})
	}
}

// TestRebuildEscapesALinkReferenceDefinition is the #2526 reproducer. The
// rebuild path escapes a literal "<" so it cannot open inline HTML, and "\<" IS
// a link destination where a bare "<" is not: the paragraph came back as a link
// reference definition, which holds nothing to translate, so the block was
// gone. A link with no text at all costs the block the same way.
func TestRebuildEscapesALinkReferenceDefinition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"label colon and a bare angle", "[a]:<"},
		{"empty link text", "[](<)"},
		{"empty image text", "![](<)"},
		{"empty reference link", "[][<]"},
		{"label colon and a destination", "[a]:< 'title'"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRebuildKeepsItsBlocks(t, tc.input)
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input),
				"the skeleton path replays these bytes exactly")
		})
	}
}

// TestRebuildLeavesALoneBracketAlone pins the narrowness of the #2526 escapes:
// a bracket the writer emits as a link's own markup, and a paragraph that only
// looks like a definition, are left as they are.
func TestRebuildLeavesALoneBracketAlone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"a link that opens the block", "[x](/y) and more text\n", "[x](/y) and more text\n"},
		{"a label with no destination", "[a]: and more text\n", "[a]: and more text\n"},
		{"a literal empty bracket pair", "An empty [] pair here\n", "An empty [] pair here\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, assertRebuildKeepsItsBlocks(t, tc.input))
		})
	}
}

// TestRebuildKeepsTheSpacesBeforeABackslashBreak is the rebuild half of #2529.
// The reader keeps the whitespace a backslash break does not spell, which is
// content; writing that content followed by a bare newline hands it straight
// back to the break, because two or more spaces before a line ending ARE a hard
// break. The rebuild spells the break with a backslash instead.
func TestRebuildKeepsTheSpacesBeforeABackslashBreak(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"two spaces before the backslash", "a  \\\nb", "a  \\\nb\n"},
		{"one space before the backslash", "a \\\nb", "a \nb\n"},
		{"a literal backslash and two spaces", "000000\\  \\\n00", "000000\\  \\\n00\n"},
		{"no whitespace before the backslash", "a\\\nb", "a\nb\n"},
		{"a spaces break stays a soft one", "a  \nb", "a\nb\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, assertRebuildKeepsItsBlocks(t, tc.input))
		})
	}
}
