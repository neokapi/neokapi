package markdown

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// inlineSpellings parses src the way the reader does and returns, in document
// order, the source bytes each inline node's resolved offsets cover. Every
// inline node must resolve.
func inlineSpellings(t *testing.T, src string) []string {
	t.Helper()
	source := []byte(src)
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	doc := md.Parser().Parse(text.NewReader(source))
	var out []string
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n.Type() != ast.TypeInline {
			return ast.WalkContinue, nil
		}
		start, ok := inlineNodeStart(n, source)
		require.True(t, ok, "%s: start not resolved", n.Kind())
		end, ok := inlineNodeEnd(n, source)
		require.True(t, ok, "%s: end not resolved", n.Kind())
		require.LessOrEqual(t, start, end, "%s: inverted range", n.Kind())
		require.LessOrEqual(t, end, len(source), "%s: range past the source", n.Kind())
		out = append(out, string(source[start:end]))
		return ast.WalkContinue, nil
	})
	require.NoError(t, err)
	return out
}

// TestInlineNodeOffsets pins the resolver on every inline kind the reader
// meets: the range of each node covers exactly its own spelling, markup
// included, with a soft or hard break's bytes riding the text before it.
func TestInlineNodeOffsets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		src  string
		want []string
	}{
		{"*a* <https://x>", []string{"*a*", "a", " ", "<https://x>"}},
		{"**[a](b)** x", []string{"**[a](b)**", "[a](b)", "a", " x"}},
		{"`c` <https://x>", []string{"`c`", "c", " ", "<https://x>"}},
		{"`` `c` `` <https://x>", []string{"`` `c` ``", "`c`", " ", "<https://x>"}},
		{"[a](b 't') <https://x>", []string{"[a](b 't')", "a", " ", "<https://x>"}},
		{"[a](<b c> \"t\") x", []string{"[a](<b c> \"t\")", "a", " x"}},
		{"[a](b\n't') x", []string{"[a](b\n't')", "a", " x"}},
		{"[a](b(c)d) x", []string{"[a](b(c)d)", "a", " x"}},
		{"[](b)<https://x>", []string{"[](b)", "<https://x>"}},
		{"![i](s 't')x", []string{"![i](s 't')", "i", "x"}},
		{"[a][r] [a][] [a] <https://x>\n\n[r]: /u\n[a]: /v", []string{"[a][r]", "a", " ", "[a][]", "a", " ", "[a]", "a", " ", "<https://x>"}},
		{"https://x <https://x>", []string{"https://x ", "<https://x>"}}, // linkify leaves a URL followed by text as text
		{"<https://x><https://y>", []string{"<https://x>", "<https://y>"}},
		{"<foo@bar.com> www.example.com", []string{"<foo@bar.com>", " ", "www.example.com"}},
		{"*<https://x>*", []string{"*<https://x>*", "<https://x>"}},
		{"~~a~~<https://x>", []string{"~~a~~", "a", "<https://x>"}},
		{"<b>x</b> <https://x>", []string{"<b>", "x", "</b>", " ", "<https://x>"}},
		{"- [ ] <https://x>", []string{"[ ] ", "<https://x>"}},
		{"> a \n> <https://x>", []string{"a", " \n> ", "<https://x>"}},
		{"a  \n<https://x>", []string{"a  \n", "<https://x>"}},
		{"a\\\n<https://x>", []string{"a\\\n", "<https://x>"}},
		{"a\r\n<https://x>\r\n", []string{"a\r\n", "<https://x>"}},
		{"# <https://x> #", []string{"<https://x>"}},
		{"| <https://x> | y |\n| --- | --- |\n| <https://z> | w |", []string{"<https://x>", "y", "<https://z>", "w"}},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, inlineSpellings(t, tc.src))
		})
	}
}

// TestScanInlineLinkCloser pins the closer scanner on the destination and
// title spellings CommonMark 6.6 allows, and on the bytes that are not a
// closer.
func TestScanInlineLinkCloser(t *testing.T) {
	t.Parallel()
	tests := []struct {
		src        string
		dest       string
		title      string
		titleOpen  byte // 0 when there is no title
		titleClose byte
		ok         bool
	}{
		{src: "](b)", dest: "b", ok: true},
		{src: "]()", ok: true},
		{src: "](<>)", dest: "<>", ok: true},
		{src: "](<b c>)", dest: "<b c>", ok: true},
		{src: "](a(b)c)", dest: "a(b)c", ok: true},
		{src: "](a\\)b)", dest: "a\\)b", ok: true},
		{src: "]( b )", dest: "b", ok: true},
		{src: "](b \"t\")", dest: "b", title: "t", titleOpen: '"', titleClose: '"', ok: true},
		{src: "](b 't')", dest: "b", title: "t", titleOpen: '\'', titleClose: '\'', ok: true},
		{src: "](b (t))", dest: "b", title: "t", titleOpen: '(', titleClose: ')', ok: true},
		{src: "](b  't'  )", dest: "b", title: "t", titleOpen: '\'', titleClose: '\'', ok: true},
		{src: "](b\n't')", dest: "b", title: "t", titleOpen: '\'', titleClose: '\'', ok: true},
		{src: "](b 'it\\'s')", dest: "b", title: "it\\'s", titleOpen: '\'', titleClose: '\'', ok: true},
		{src: "](<b> \"a 'q' b\")", dest: "<b>", title: "a 'q' b", titleOpen: '"', titleClose: '"', ok: true},
		{src: "](b\"t\")", dest: "b\"t\"", ok: true},
		{src: "](b"},
		{src: "](b 't')x", dest: "b", title: "t", titleOpen: '\'', titleClose: '\'', ok: true},
		{src: "](b 't)"},
		{src: "](b 't' x)"},
		{src: "](<b)"},
		{src: "](a(b)"},
		{src: "[b)"},
		{src: "]"},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			t.Parallel()
			source := []byte(tc.src)
			c, ok := scanInlineLinkCloser(source, 0)
			require.Equal(t, tc.ok, ok)
			if !ok {
				return
			}
			assert.Equal(t, tc.dest, c.dest)
			assert.Equal(t, tc.title, c.title)
			if tc.titleOpen == 0 {
				assert.Equal(t, -1, c.titleOpen)
				assert.Equal(t, -1, c.titleClose)
			} else {
				assert.Equal(t, tc.titleOpen, source[c.titleOpen])
				assert.Equal(t, tc.titleClose, source[c.titleClose])
			}
			want := strings.TrimSuffix(tc.src, "x")
			assert.Equal(t, want, string(source[:c.end]), "the closer ends at its parenthesis")
		})
	}
}
