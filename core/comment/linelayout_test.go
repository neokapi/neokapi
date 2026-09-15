package comment

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var cLine = Markers{Line: []string{"//", "#!"}, Block: []BlockMarker{{Open: "/*", Close: "*/"}}}

// lineComment locates the comment that opens at the first occurrence of marker
// in src and runs to the end of the line holding the last occurrence of last.
func lineComment(t *testing.T, src, marker, last string) Comment {
	t.Helper()
	start := strings.Index(src, marker)
	require.GreaterOrEqual(t, start, 0)
	end := strings.LastIndex(src, last) + len(last)
	return Comment{Start: start, End: end, Style: StyleLine}
}

// Each layout of line comments reads its text without markers or what sits
// between a marker and its text, renders its own text to its own bytes, and
// lays new text out the way it laid out the old.
func TestLineLayout(t *testing.T) {
	for _, tc := range []struct {
		name, src, first, last, text string
		newText, want                string
	}{
		{name: "lines with a space after the marker",
			src: "function f() {\n  // Parses the input.\n  //\n  // Stops at the end.\n  return 1;\n}\n", first: "// Parses", last: "end.",
			text:    "Parses the input.\n\nStops at the end.",
			newText: "Reads the input.\nStops.",
			want:    "// Reads the input.\n  // Stops."},
		{name: "lines with no space after the marker",
			src: "//Parses.\n//More.\nexport const x = 1;\n", first: "//Parses", last: "More.",
			text:    "Parses.\nMore.",
			newText: "Reads.",
			want:    "//Reads."},
		{name: "a marker written longer",
			src: "/// Parses.\n/// More.\nexport const x = 1;\n", first: "/// Parses", last: "More.",
			text:    "Parses.\nMore.",
			newText: "Reads.\n\nMore.",
			want:    "/// Reads.\n///\n/// More."},
		{name: "text lines that all open with a space keep it",
			src: "//  indented\n//  twice\nexport const x = 1;\n", first: "//  indented", last: "twice",
			text:    "indented\ntwice",
			newText: "once",
			want:    "//  once"},
		{name: "a comment after code",
			src: "export const x = 1; // the answer\n", first: "// the", last: "answer",
			text:    "the answer",
			newText: "a better answer",
			want:    "// a better answer"},
		{name: "CRLF line endings",
			src: "// Parses.\r\n// More.\r\nexport const x = 1;\r\n", first: "// Parses", last: "More.",
			text:    "Parses.\nMore.",
			newText: "Reads.\nMore.\nAgain.",
			want:    "// Reads.\r\n// More.\r\n// Again."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			c := lineComment(t, tc.src, tc.first, tc.last)
			l, err := ParseLineLayout(src, c, cLine)
			require.NoError(t, err)
			assert.Equal(t, tc.text, l.Text())
			same, err := l.Render(strings.Split(l.Text(), "\n"))
			require.NoError(t, err)
			assert.Equal(t, tc.src[c.Start:c.End], string(same), "the comment's own text renders to its own bytes")
			got, err := l.Render(strings.Split(tc.newText, "\n"))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))

			after := tc.src[:c.Start] + string(got) + tc.src[c.End:]
			again, err := ParseLineLayout([]byte(after), Comment{Start: c.Start, End: c.Start + len(got), Style: StyleLine}, cLine)
			require.NoError(t, err)
			assert.Equal(t, tc.newText, again.Text(), "the rendered comment reads back as the text")
		})
	}
}

func TestLineLayoutRefuses(t *testing.T) {
	t.Run("a comment with no one layout", func(t *testing.T) {
		for name, tc := range map[string]struct{ src, first, last string }{
			"lines indented two ways":         {"  // a\n    // b\nx\n", "// a", "b"},
			"lines opening with two markers":  {"// a\n#! b\nx\n", "// a", "b"},
			"empty lines written two ways":    {"// a\n//\n// b\n// \n// c\nx\n", "// a", "c"},
			"line endings written two ways":   {"// a\r\n// b\n// c\nx\n", "// a", "c"},
			"a comment with no marker":        {"x = 1\n", "x", "1"},
			"a comment after code over lines": {"x = 1 // a\n// b\n", "// a", "b"},
		} {
			_, err := ParseLineLayout([]byte(tc.src), lineComment(t, tc.src, tc.first, tc.last), cLine)
			refusal, ok := AsRefusal(err)
			require.True(t, ok, "%s: %v", name, err)
			assert.Equal(t, RefusedLayout, refusal.Reason, "%s: %s", name, refusal.Detail)
		}
	})

	t.Run("a comment after code holds one line of text", func(t *testing.T) {
		src := "x = 1 // a\n"
		l, err := ParseLineLayout([]byte(src), lineComment(t, src, "// a", "a"), cLine)
		require.NoError(t, err)
		_, err = l.Render([]string{"a", "b"})
		refusal, ok := AsRefusal(err)
		require.True(t, ok, "%v", err)
		assert.Equal(t, RefusedText, refusal.Reason)
	})

	// With no space between the marker and the text, a text line made of the
	// marker's last character reads back as more marker.
	t.Run("must fail: a text line the layout would read as an empty line is refused", func(t *testing.T) {
		src := "//a\n//b\nx\n"
		l, err := ParseLineLayout([]byte(src), lineComment(t, src, "//a", "b"), cLine)
		require.NoError(t, err)
		for _, text := range []string{"/", "//", "/ "} {
			got, err := l.Render([]string{"a", text})
			refusal, ok := AsRefusal(err)
			require.True(t, ok, "%q: rendered %q: %v", text, got, err)
			assert.Equal(t, RefusedText, refusal.Reason, "%q: %s", text, refusal.Detail)
		}
	})

	t.Run("must fail: a text line ending in the splice is refused", func(t *testing.T) {
		src := "// a\nint x;\n"
		l, err := ParseLineLayout([]byte(src), lineComment(t, src, "// a", "a"), Markers{Line: []string{"//"}, Splice: `\`})
		require.NoError(t, err)
		_, err = l.Render([]string{`runs on \`})
		refusal, ok := AsRefusal(err)
		require.True(t, ok, "%v", err)
		assert.Equal(t, RefusedText, refusal.Reason)
	})
}
