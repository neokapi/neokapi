package comment

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var cBlock = BlockMarker{Open: "/*", Close: "*/"}

// Each layout reads its text without delimiters, padding or prefixes, renders
// its own text to its own bytes, and lays new text out the way it laid out the
// old.
func TestLayout(t *testing.T) {
	for _, tc := range []struct {
		name, indent, span, text string
		// newText rendered in the layout gives want.
		newText, want string
	}{
		{name: "one line", span: "/* Parse parses. */", text: "Parse parses.",
			newText: "Parse reads.", want: "/* Parse reads. */"},
		{name: "one line without spaces", span: "/*topLevel*/", text: "topLevel",
			newText: "nested", want: "/*nested*/"},
		{name: "one line drawn longer", span: "/**  Parse parses.  */", text: "Parse parses.",
			newText: "Parse reads.", want: "/**  Parse reads.  */"},
		{name: "a line of asterisks", indent: "\t",
			span:    "/*\n\t * Parse reads the input.\n\t *\n\t * It stops at the end.\n\t */",
			text:    "Parse reads the input.\n\nIt stops at the end.",
			newText: "Parse reads.\n\nIt stops.\nIt reports.",
			want:    "/*\n\t * Parse reads.\n\t *\n\t * It stops.\n\t * It reports.\n\t */"},
		{name: "a documentation comment with a tag",
			span:    "/**\n * Parses the input.\n *\n * @param src - the source\n */",
			text:    "Parses the input.\n\n@param src - the source",
			newText: "Reads the input.\n\n@param src - the source",
			want:    "/**\n * Reads the input.\n *\n * @param src - the source\n */"},
		{name: "bare lines",
			span:    "/*\nPackage demo is a fixture.\n\nIt has two paragraphs.\n*/",
			text:    "Package demo is a fixture.\n\nIt has two paragraphs.",
			newText: "Package demo is a fixture.\n\n\tcode := 1",
			want:    "/*\nPackage demo is a fixture.\n\n\tcode := 1\n*/"},
		{name: "bare lines indented under the opener", indent: "\t",
			span:    "/*\n\t   ID identifies the block,\n\t   and is never empty.\n\t*/",
			text:    "ID identifies the block,\nand is never empty.",
			newText: "ID names the block.",
			want:    "/*\n\t   ID names the block.\n\t*/"},
		{name: "text on the opener's and the closer's lines", indent: "\t",
			span:    "/* Split starts on the opener's line\n\t   and ends on the closer's. */",
			text:    "Split starts on the opener's line\nand ends on the closer's.",
			newText: "Split starts here,\ncontinues,\nand ends here.",
			want:    "/* Split starts here,\n\t   continues,\n\t   and ends here. */"},
		{name: "text on the opener's line alone, a line added below it", indent: "\t",
			span:    "/* Close releases the reader.\n\t*/",
			text:    "Close releases the reader.",
			newText: "Close releases\nthe reader.",
			want:    "/* Close releases\n\t   the reader.\n\t*/"},
		{name: "blank lines above and below the text, kept",
			span:    "/*\n\n * Copyright notice.\n\n */",
			text:    "Copyright notice.",
			newText: "Copyright notice.\n\nAll rights reserved.",
			want:    "/*\n\n * Copyright notice.\n *\n * All rights reserved.\n\n */"},
		{name: "a banner",
			span:    "/*****\n * Section.\n *****/",
			text:    "Section.",
			newText: "Another section.",
			want:    "/*****\n * Another section.\n *****/"},
		{name: "CRLF line endings",
			span:    "/*\r\n * Parse reads.\r\n *\r\n * It stops.\r\n */",
			text:    "Parse reads.\n\nIt stops.",
			newText: "Parse reads.\nIt stops.",
			want:    "/*\r\n * Parse reads.\r\n * It stops.\r\n */"},
		{name: "text that is only an asterisk, which an opener drawn longer would swallow",
			span:    "/** */",
			text:    "*",
			newText: "Parse parses.",
			want:    "/*Parse parses. */"},
		{name: "text that is only an asterisk, with no other line to read the prefix from",
			span:    "/*\n * *\n */",
			text:    "*",
			newText: "*\n*",
			want:    "/*\n * *\n * *\n */"},
		{name: "text lines that all open with a space keep it, beside a line that is only an asterisk",
			span:    "/*\n *  000\n *\n * *\n *  0\n */",
			text:    " 000\n\n*\n 0",
			newText: " 000\n*",
			want:    "/*\n *  000\n * *\n */"},
		{name: "a line that is only an asterisk is text beside the prefix",
			span:    "/*\n * List:\n * *\n */",
			text:    "List:\n*",
			newText: "List:\n*\n*",
			want:    "/*\n * List:\n * *\n * *\n */"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, err := ParseLayout([]byte(tc.span), tc.indent, cBlock)
			require.NoError(t, err)
			assert.Equal(t, tc.text, l.Text())
			same, err := l.Render(strings.Split(l.Text(), "\n"))
			require.NoError(t, err)
			assert.Equal(t, tc.span, string(same), "the comment's own text renders to its own bytes")
			got, err := l.Render(strings.Split(tc.newText, "\n"))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
			again, err := ParseLayout(got, tc.indent, cBlock)
			require.NoError(t, err)
			assert.Equal(t, tc.newText, again.Text(), "the rendered comment reads back as the text")
		})
	}
}

// A prefix that is given keeps the indentation every text line shares, which a
// prefix that is read would take for the prefix.
func TestLayoutWithPrefix(t *testing.T) {
	const span = "/*\n  - one\n    more\n  - two\n*/"
	read, err := ParseLayout([]byte(span), "", cBlock)
	require.NoError(t, err)
	assert.Equal(t, "- one\n  more\n- two", read.Text(), "a prefix that is read takes the shared indentation")

	given, err := ParseLayoutWithPrefix([]byte(span), cBlock, "")
	require.NoError(t, err)
	assert.Equal(t, "  - one\n    more\n  - two", given.Text())
	same, err := given.Render(strings.Split(given.Text(), "\n"))
	require.NoError(t, err)
	assert.Equal(t, span, string(same))
	got, err := given.Render([]string{"Cases:", "  - one"})
	require.NoError(t, err)
	assert.Equal(t, "/*\nCases:\n  - one\n*/", string(got))
}

func TestLayoutRefuses(t *testing.T) {
	t.Run("a span that is not one comment has no layout", func(t *testing.T) {
		for name, span := range map[string]string{
			"two comments on one line":            "/* a */ /* b */",
			"a delimited then a line comment":     "/* a */\n// b",
			"a line comment then a delimited one": "// a\n/* b */",
			"no text":                             "/*  */",
			"no text over lines":                  "/*\n *\n */",
			"a lone carriage return":              "/* a\rb */",
			"line endings written two ways":       "/*\r\n * a\n */",
			"invalid UTF-8":                       "/* \xff */",
			"empty lines written two ways":        "/*\n * a\n *\n * b\n\n * c\n */",
		} {
			_, err := ParseLayout([]byte(span), "", cBlock)
			refusal, ok := AsRefusal(err)
			require.True(t, ok, "%s: %v", name, err)
			assert.Equal(t, RefusedLayout, refusal.Reason, "%s: %s", name, refusal.Detail)
		}
	})

	t.Run("text holding the closer is refused in every layout", func(t *testing.T) {
		for _, span := range []string{"/* a */", "/*a*/", "/*\n * a\n */", "/*\na\n*/", "/* a\n   b */", "/**\n * a\n */"} {
			l, err := ParseLayout([]byte(span), "", cBlock)
			require.NoError(t, err, span)
			for _, text := range []string{"ends */ here", "*/", "ends here */", "x*/y"} {
				_, err := l.Render([]string{text})
				refusal, ok := AsRefusal(err)
				require.True(t, ok, "%q in %q: %v", text, span, err)
				assert.Equal(t, RefusedTerminator, refusal.Reason, "%q in %q", text, span)
			}
		}
	})

	t.Run("must fail: a text line the layout would read as an empty line is refused", func(t *testing.T) {
		l, err := ParseLayout([]byte("/*\nParse reads.\n*/"), "", cBlock)
		require.NoError(t, err)
		for _, text := range []string{"*", "* ", " * "} {
			got, err := l.Render([]string{"Parse reads.", text})
			refusal, ok := AsRefusal(err)
			require.True(t, ok, "%q: rendered %q: %v", text, got, err)
			assert.Equal(t, RefusedText, refusal.Reason, "%q: %s", text, refusal.Detail)
		}
	})

	t.Run("a single line holds one line of text", func(t *testing.T) {
		l, err := ParseLayout([]byte("/* a */"), "", cBlock)
		require.NoError(t, err)
		_, err = l.Render([]string{"a", "b"})
		refusal, ok := AsRefusal(err)
		require.True(t, ok, "%v", err)
		assert.Equal(t, RefusedText, refusal.Reason)
	})

	t.Run("a comment that nests takes balanced openers and refuses an unbalanced one", func(t *testing.T) {
		rust := BlockMarker{Open: "/*", Close: "*/", Nested: true}
		l, err := ParseLayout([]byte("/* outer /* inner */ outer */"), "", rust)
		require.NoError(t, err)
		assert.Equal(t, "outer /* inner */ outer", l.Text())
		got, err := l.Render([]string{"a /* b */ c"})
		require.NoError(t, err)
		assert.Equal(t, "/* a /* b */ c */", string(got))
		for _, text := range []string{"a /* b", "a */ b"} {
			_, err := l.Render([]string{text})
			refusal, ok := AsRefusal(err)
			require.True(t, ok, "%q: %v", text, err)
			assert.Equal(t, RefusedTerminator, refusal.Reason, text)
		}
	})

	// The text holds no closer in these cases, so only the check of the rendered
	// comment as a whole sees one: the layout's own characters complete it.
	t.Run("must fail: text meeting the layout in a closer is refused", func(t *testing.T) {
		for name, tc := range map[string]struct {
			span  string
			lines []string
		}{
			"a prefix ending in an asterisk meets a slash": {"/*\n *a\n *b\n */", []string{"a", "/c"}},
			"an opener drawn longer meets a slash":         {"/**a */", []string{"/b"}},
		} {
			l, err := ParseLayout([]byte(tc.span), "", cBlock)
			require.NoError(t, err, name)
			got, err := l.Render(tc.lines)
			refusal, ok := AsRefusal(err)
			require.True(t, ok, "%s: rendered %q: %v", name, got, err)
			assert.Equal(t, RefusedTerminator, refusal.Reason, name)
		}
	})

	t.Run("a text line ending in an asterisk beside the closer closes at the end", func(t *testing.T) {
		l, err := ParseLayout([]byte("/* a\n   b*/"), "", cBlock)
		require.NoError(t, err)
		got, err := l.Render([]string{"a", "b*"})
		require.NoError(t, err)
		assert.Equal(t, "/* a\n   b**/", string(got))
		again, err := ParseLayout(got, "", cBlock)
		require.NoError(t, err)
		assert.Equal(t, "a\nb*", again.Text())
	})
}
