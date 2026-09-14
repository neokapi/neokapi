package golang

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rewritten returns src with the comment id rewritten to text, failing the
// test on a refusal.
func rewritten(t *testing.T, name, src, id, text string, opts comment.RenderOptions) string {
	t.Helper()
	r, err := comment.Rewrite(Provider{}, name, []byte(src), nil, comment.Target{ID: id}, text, opts)
	require.NoError(t, err)
	return string(r.Source)
}

// commentLines returns the lines of src that hold a comment marker.
func commentLinesOf(src string) []string {
	var out []string
	for line := range strings.SplitSeq(src, "\n") {
		if strings.Contains(line, "//") {
			out = append(out, line)
		}
	}
	return out
}

func TestRender(t *testing.T) {
	long := "Parse reads the whole input, one line at a time, and stops at the first line that holds nothing but a marker, which it reports as the end of the section it was reading."

	t.Run("a paragraph wraps at the default width with the gofmt prefix", func(t *testing.T) {
		got := rewritten(t, "p.go", "package p\n\n// Parse parses.\nfunc Parse() {}\n", "func/Parse", long, comment.RenderOptions{})
		lines := commentLinesOf(got)
		require.Greater(t, len(lines), 1)
		for _, line := range lines {
			assert.True(t, strings.HasPrefix(line, "// "), "%q", line)
			assert.LessOrEqual(t, columns(line), comment.DefaultWidth, "%q", line)
		}
		assert.Equal(t, long, strings.Join(trimMarkers(lines), " "), "wrapping keeps every word in order")
	})

	t.Run("a comment in a body keeps its indentation on every line", func(t *testing.T) {
		got := rewritten(t, "p.go", "package p\n\nfunc F() {\n\t// Old.\n\t_ = 1\n}\n", "func/F/comment", "First paragraph.\n\nSecond paragraph.", comment.RenderOptions{})
		assert.Equal(t, "package p\n\nfunc F() {\n\t// First paragraph.\n\t//\n\t// Second paragraph.\n\t_ = 1\n}\n", got)
	})

	t.Run("a wider comment sets its own width, and a configured width wins", func(t *testing.T) {
		wide := "// " + strings.Repeat("word ", 20) + "end.\n"
		src := "package p\n\n" + wide + "func Parse() {}\n"
		for _, line := range commentLinesOf(rewritten(t, "p.go", src, "func/Parse", long, comment.RenderOptions{})) {
			assert.LessOrEqual(t, columns(line), columns(strings.TrimSuffix(wide, "\n")), "%q", line)
		}
		lines := commentLinesOf(rewritten(t, "p.go", src, "func/Parse", long, comment.RenderOptions{Width: 40}))
		assert.Greater(t, len(lines), 4)
		for _, line := range lines {
			assert.LessOrEqual(t, columns(line), 40, "%q", line)
		}
	})

	t.Run("a code block stays as the text holds it, however long", func(t *testing.T) {
		code := "\tresult := Parse(strings.NewReader(input), WithMarker(\"--\"), WithLimit(1024), WithStrict(true))"
		src := "package p\n\n// Parse parses.\n//\n//" + code + "\nfunc Parse() {}\n"
		got := rewritten(t, "p.go", src, "func/Parse", long+"\n\n"+code, comment.RenderOptions{})
		assert.Contains(t, got, "\n//"+code+"\n")
	})

	t.Run("a list in a doc comment is written as gofmt writes it", func(t *testing.T) {
		src := "package p\n\n// Parse parses:\n//   - one\n//   - two\nfunc Parse() {}\n"
		got := rewritten(t, "p.go", src, "func/Parse", "Parse reads:\n - one\n - two", comment.RenderOptions{})
		assert.Equal(t, "package p\n\n// Parse reads:\n//   - one\n//   - two\nfunc Parse() {}\n", got)
	})

	t.Run("a long list item wraps under its text", func(t *testing.T) {
		src := "package p\n\n// Parse parses:\n//   - one\n//   - two\nfunc Parse() {}\n"
		got := rewritten(t, "p.go", src, "func/Parse", "Parse parses:\n  - "+long+"\n  - two", comment.RenderOptions{})
		lines := commentLinesOf(got)
		assert.True(t, strings.HasPrefix(lines[1], "//   - Parse reads"), "%q", lines[1])
		assert.True(t, strings.HasPrefix(lines[2], "//     "), "a continuation sits under the item's text: %q", lines[2])
		for _, line := range lines {
			assert.LessOrEqual(t, columns(line), comment.DefaultWidth, "%q", line)
		}
	})

	t.Run("a deprecation paragraph opens with its marker after wrapping", func(t *testing.T) {
		src := "package p\n\n// Old parses.\n//\n// Deprecated: use New.\nfunc Old() {}\n"
		text := "Old parses the old way.\n\nDeprecated: " + long
		got := rewritten(t, "p.go", src, "func/Old", text, comment.RenderOptions{})
		assert.Contains(t, got, "//\n// Deprecated: Parse reads")
		f, err := Provider{}.Locate("p.go", []byte(got))
		require.NoError(t, err)
		assert.True(t, f.Comments[0].Deprecated)
	})

	t.Run("a file with CRLF line endings keeps them", func(t *testing.T) {
		src := "package p\r\n\r\n// Parse parses.\r\nfunc Parse() {}\r\n"
		got := rewritten(t, "p.go", src, "func/Parse", "Parse reads.\n\nIt stops.", comment.RenderOptions{})
		assert.Equal(t, "package p\r\n\r\n// Parse reads.\r\n//\r\n// It stops.\r\nfunc Parse() {}\r\n", got)
	})

	t.Run("a trailing comment holds one line and is not wrapped", func(t *testing.T) {
		src := "package p\n\nvar x = 1 // old\n"
		got := rewritten(t, "p.go", src, "var/x", long, comment.RenderOptions{})
		assert.Equal(t, "package p\n\nvar x = 1 // "+long+"\n", got)

		_, err := comment.Rewrite(Provider{}, "p.go", []byte(src), nil, comment.Target{ID: "var/x"}, "one\ntwo", comment.RenderOptions{})
		refusal, ok := comment.AsRefusal(err)
		require.True(t, ok, "%v", err)
		assert.Equal(t, comment.RefusedText, refusal.Reason)
	})

	t.Run("a word wider than the line sits on its own line", func(t *testing.T) {
		word := strings.Repeat("x", 120)
		got := rewritten(t, "p.go", "package p\n\n// Parse parses.\nfunc Parse() {}\n", "func/Parse", "Before "+word+" after.", comment.RenderOptions{})
		assert.Equal(t, "package p\n\n// Before\n// "+word+"\n// after.\nfunc Parse() {}\n", got)
	})

	t.Run("a list marker never starts a wrapped line", func(t *testing.T) {
		text := strings.Repeat("a ", 39) + "- b c"
		got := rewritten(t, "p.go", "package p\n\n// Parse parses.\nfunc Parse() {}\n", "func/Parse", text, comment.RenderOptions{})
		lines := commentLinesOf(got)
		require.Len(t, lines, 2)
		assert.True(t, strings.HasSuffix(lines[0], " a -"), "the marker stays on the line before: %q", lines[0])
		assert.Equal(t, "// b c", lines[1])
	})

	t.Run("text a Go comment cannot hold is refused", func(t *testing.T) {
		for name, text := range map[string]string{
			"empty":                   " \n\t\n",
			"a lone carriage return":  "Parse\rreads.",
			"a bidirectional control": "Parse reads \u202Eevil.",
			"a NUL":                   "Parse\x00reads.",
			"a line separator":        "Parse\u2028reads.",
			"invalid UTF-8":           "Parse \xffreads.",
		} {
			_, err := comment.Rewrite(Provider{}, "p.go", []byte("package p\n\n// Parse parses.\nfunc Parse() {}\n"), nil, comment.Target{ID: "func/Parse"}, text, comment.RenderOptions{})
			refusal, ok := comment.AsRefusal(err)
			require.True(t, ok, "%s: %v", name, err)
			assert.Equal(t, comment.RefusedText, refusal.Reason, name)
		}
	})

	t.Run("comment markers, right-to-left text and combining marks are prose", func(t *testing.T) {
		text := "Parse ends at */ or // and reads \u05E9\u05DC\u05D5\u05DD \u05E2\u05D5\u05DC\u05DD and e\u0301."
		got := rewritten(t, "p.go", "package p\n\n// Parse parses.\nfunc Parse() {}\n", "func/Parse", text, comment.RenderOptions{})
		assert.Contains(t, got, "// "+text+"\n")
	})
}

func trimMarkers(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = strings.TrimPrefix(strings.TrimSpace(line), "// ")
	}
	return out
}
