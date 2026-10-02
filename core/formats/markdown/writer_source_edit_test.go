package markdown_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	gmhtml "github.com/yuin/goldmark/renderer/html"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/model"
)

// editMarkdownSource reads doc through the skeleton round-trip `kapi apply`,
// `ksed` and MCP apply_edits use, rewrites the source of the block whose plain
// text is from to the placeholder text to, and writes it back with no locale.
func editMarkdownSource(t *testing.T, doc, from, to string) string {
	t.Helper()
	reader, writer := markdown.NewReader(), markdown.NewWriter()
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	require.NotNil(t, store)
	t.Cleanup(func() { _ = store.Close() })

	parts, err := spec.ReadParts(reader, []byte(doc))
	require.NoError(t, err)
	edited := 0
	for _, p := range parts {
		b, ok := p.Resource.(*model.Block)
		if !ok || !b.Translatable || model.RunsText(b.Source) != from {
			continue
		}
		b.SetSourceRuns(model.ParseRunsPlaceholderText(to, b.Source))
		edited++
	}
	require.Equal(t, 1, edited, "exactly one block reads %q", from)

	out, err := spec.WriteParts(writer, parts, []byte(doc))
	require.NoError(t, err)
	return string(out)
}

// renderPage renders Markdown the way a documentation site does: CommonMark
// with raw HTML passed through to the page.
func renderPage(t *testing.T, md string) string {
	t.Helper()
	var buf bytes.Buffer
	gm := goldmark.New(goldmark.WithRendererOptions(gmhtml.WithUnsafe()))
	require.NoError(t, gm.Convert([]byte(md), &buf))
	return buf.String()
}

// An edit's text is text. Markdown passes raw HTML through to the page it
// renders, so a tag, an autolink, a character reference or an inline link
// typed into an edit must reach the page as the characters typed.
func TestWriter_EditedSourceTextIsText(t *testing.T) {
	tests := []struct {
		name     string
		doc      string
		from     string
		to       string
		wantOut  string
		wantPage string // the rendered page fragment the edit must produce
	}{
		{
			name:     "a script element",
			doc:      "Hello world\n",
			from:     "Hello world",
			to:       "Hello <script>alert(1)</script> & goodbye",
			wantOut:  "Hello \\<script>alert(1)\\</script> & goodbye\n",
			wantPage: "<p>Hello &lt;script&gt;alert(1)&lt;/script&gt; &amp; goodbye</p>",
		},
		{
			name:     "an element with an event handler",
			doc:      "# Hello world\n",
			from:     "Hello world",
			to:       `Hi <img src=x onerror="alert(1)">`,
			wantOut:  "# Hi \\<img src=x onerror=\"alert(1)\">\n",
			wantPage: "<h1>Hi &lt;img src=x onerror=&quot;alert(1)&quot;&gt;</h1>",
		},
		{
			name:     "a character reference spelled out",
			doc:      "- Hello world\n",
			from:     "Hello world",
			to:       "Fish &amp; chips",
			wantOut:  "- Fish \\&amp; chips\n",
			wantPage: "<li>Fish &amp;amp; chips</li>",
		},
		{
			name:     "an autolink",
			doc:      "Hello world\n",
			from:     "Hello world",
			to:       "See <https://evil.example/>",
			wantOut:  "See \\<https://evil.example/>\n",
			wantPage: "<p>See &lt;https://evil.example/&gt;</p>",
		},
		{
			name:     "an inline link and an image",
			doc:      "Hello world\n",
			from:     "Hello world",
			to:       "[click](javascript:alert(1)) ![x](https://t.example/p.png)",
			wantOut:  "\\[click](javascript:alert(1)) !\\[x](https://t.example/p.png)\n",
			wantPage: "<p>[click](javascript:alert(1)) ![x](https://t.example/p.png)</p>",
		},
		{
			name:     "markup inside a kept code span stays literal",
			doc:      "Run `kapi init` now\n",
			from:     "Run kapi init now",
			to:       `Run <x id="1"/><b>x</b> & <y><x id="/1"/> now`,
			wantOut:  "Run `<b>x</b> & <y>` now\n",
			wantPage: "<p>Run <code>&lt;b&gt;x&lt;/b&gt; &amp; &lt;y&gt;</code> now</p>",
		},
		{
			name:     "an escape the edit already spelled is kept",
			doc:      "Hello world\n",
			from:     "Hello world",
			to:       `Use \<b> for bold`,
			wantOut:  "Use \\<b> for bold\n",
			wantPage: "<p>Use &lt;b&gt; for bold</p>",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := editMarkdownSource(t, tc.doc, tc.from, tc.to)
			assert.Equal(t, tc.wantOut, out)
			assert.Contains(t, renderPage(t, out), tc.wantPage)
		})
	}
}

// The escape only touches what the reader would have read as markup, and the
// reader turns every such construct into an inline code, so text it left as
// text keeps its bytes: a '<' that opens no tag, an unterminated tag, an
// ampersand that is no reference, brackets that are no link, and emphasis
// characters.
func TestWriter_EditKeepsUntouchedMarkdownByteExact(t *testing.T) {
	const doc = "a < b and I <3 you\n\n" +
		"x <y and z\n\n" +
		"AT&T and Q&A\n\n" +
		"2 * 3 and [1] and a_b_c\n\n" +
		"Edit me\n"

	out := editMarkdownSource(t, doc, "Edit me", "Edited")

	assert.Equal(t, strings.Replace(doc, "Edit me", "Edited", 1), out)
}
