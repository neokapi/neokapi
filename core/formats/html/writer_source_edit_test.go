package html_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/model"
)

// editSource reads doc through the skeleton round-trip `kapi apply`, `ksed`
// and MCP apply_edits use, edits the block whose plain text or edit text is
// from to the edit text to (inline codes as <x id="…"/> tokens, the shape
// `kapi inspect` shows), and writes the document back with no locale.
func editSource(t *testing.T, doc, from, to string) string {
	t.Helper()
	reader, writer := htmlfmt.NewReader(), htmlfmt.NewWriter()
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	require.NotNil(t, store)
	t.Cleanup(func() { _ = store.Close() })

	parts, err := spec.ReadParts(reader, []byte(doc))
	require.NoError(t, err)
	edited := 0
	for _, p := range parts {
		b, ok := p.Resource.(*model.Block)
		if !ok || !b.Translatable || (model.RunsText(b.SourceRuns()) != from && model.RunsEditText(b.SourceRuns()) != from) {
			continue
		}
		b.EditSourceRuns(model.ParseRunsEditText(to, b.SourceRuns()))
		edited++
	}
	require.Equal(t, 1, edited, "exactly one block reads %q", from)

	out, err := spec.WriteParts(writer, parts, []byte(doc))
	require.NoError(t, err)
	return string(out)
}

// readBackTexts reads html as a browser would see it: the DOM reader decodes
// character references, so a block's plain text is the text a reader sees.
func readBackTexts(t *testing.T, html string) []string {
	t.Helper()
	parts, err := spec.ReadParts(htmlfmt.NewReader(), []byte(html))
	require.NoError(t, err)
	var texts []string
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
			texts = append(texts, model.RunsText(b.SourceRuns()))
		}
	}
	return texts
}

// An edit's text is text. Inline codes travel as <x id="…"/> tokens, so a
// literal '<', '&' or '"' in the edited wording must come back out of the
// document as that character, never as markup, a character reference, or the
// end of an attribute value.
func TestWriter_EditedSourceTextIsText(t *testing.T) {
	tests := []struct {
		name     string
		doc      string
		from     string
		to       string
		wantOut  string // a substring the written document must contain
		readBack string // the block's text as a browser reads it back
	}{
		{
			name:     "a script element in paragraph text",
			doc:      `<html><body><p>Hello world</p></body></html>`,
			from:     "Hello world",
			to:       "Hello <script>alert(1)</script> & goodbye",
			wantOut:  `<p>Hello &lt;script>alert(1)&lt;/script> &amp; goodbye</p>`,
			readBack: "Hello <script>alert(1)</script> & goodbye",
		},
		{
			name:     "a character reference spelled out in text",
			doc:      `<html><body><p>Hello world</p></body></html>`,
			from:     "Hello world",
			to:       "Fish &amp; chips &lt;3",
			wantOut:  `<p>Fish &amp;amp; chips &amp;lt;3</p>`,
			readBack: "Fish &amp; chips &lt;3",
		},
		{
			name:     "markup typed beside a kept inline code",
			doc:      `<html><body><p>Read the <a href="t.html">terms</a>.</p></body></html>`,
			from:     "Read the terms.",
			to:       `Read <b>all</b> the <x id="1"/>terms<x id="/1"/>.`,
			wantOut:  `<p>Read &lt;b>all&lt;/b> the <a href="t.html">terms</a>.</p>`,
			readBack: "Read <b>all</b> the terms.",
		},
		{
			name:     "an end tag in the document title",
			doc:      `<html><head><meta charset="utf-8"><title>Pricing</title></head><body><p>x</p></body></html>`,
			from:     "Pricing",
			to:       "x</title><script>alert(1)</script>",
			wantOut:  `<title>x&lt;/title>&lt;script>alert(1)&lt;/script></title>`,
			readBack: "x</title><script>alert(1)</script>",
		},
		{
			name:     "a quote after an angle bracket in a block element's attribute",
			doc:      `<html><body><p title="Note">Body</p></body></html>`,
			from:     "Note",
			to:       `a<" onclick="alert(1)`,
			wantOut:  `<p title="a<&quot; onclick=&quot;alert(1)">`,
			readBack: `a<" onclick="alert(1)`,
		},
		{
			name:     "a single quote in a single-quoted attribute",
			doc:      `<html><body><p title='Note'>Body</p></body></html>`,
			from:     "Note",
			to:       `x' onclick='alert(1)`,
			wantOut:  `<p title='x&#39; onclick=&#39;alert(1)'>`,
			readBack: `x' onclick='alert(1)`,
		},
		{
			name:     "spaces in an unquoted attribute",
			doc:      `<html><body><p><img src=c.png alt=chart> Prices.</p></body></html>`,
			from:     "chart",
			to:       `a b onerror=alert(1)`,
			wantOut:  `alt="a b onerror=alert(1)">`,
			readBack: `a b onerror=alert(1)`,
		},
		{
			name:     "a reference and a quote in an inline element's attribute",
			doc:      `<html><body><p><img src="c.png" alt="chart"> Prices.</p></body></html>`,
			from:     "chart",
			to:       `a &amp; b" onerror="x`,
			wantOut:  `alt="a &amp;amp; b&#34; onerror=&#34;x"`,
			readBack: `a &amp; b" onerror="x`,
		},
		{
			name:     "references the edit kept keep their spelling",
			doc:      `<html><body><p>Don&rsquo;t pay&nbsp;more &mdash; it&#39;s &copy; 2026</p></body></html>`,
			from:     "Don\u2019t pay\u00a0more \u2014 it's \u00a9 2026",
			to:       "Don\u2019t pay\u00a0less \u2014 it's \u00a9 2027 & <b>",
			wantOut:  `<p>Don&rsquo;t pay&nbsp;less &mdash; it&#39;s &copy; 2027 &amp; &lt;b></p>`,
			readBack: "Don\u2019t pay\u00a0less \u2014 it's \u00a9 2027 & <b>",
		},
		{
			name:     "a rewrite that drops every reference",
			doc:      `<html><body><p>Fish &amp; chips &lt;3</p></body></html>`,
			from:     "Fish & chips <3",
			to:       "Pay less today.",
			wantOut:  `<p>Pay less today.</p>`,
			readBack: "Pay less today.",
		},
		{
			name:     "a textarea's text",
			doc:      `<html><body><form><textarea name="t">Type here</textarea></form></body></html>`,
			from:     "Type here",
			to:       "x</textarea><script>alert(1)</script>",
			wantOut:  `<textarea name="t">x&lt;/textarea>&lt;script>alert(1)&lt;/script></textarea>`,
			readBack: "x</textarea><script>alert(1)</script>",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := editSource(t, tc.doc, tc.from, tc.to)
			assert.Contains(t, out, tc.wantOut)
			assert.Contains(t, readBackTexts(t, out), tc.readBack, "output: %s", out)
		})
	}
}

// Encoding an edit must leave every block nobody edited byte-for-byte as the
// source spelled it, including text HTML tolerates unescaped: a bare
// ampersand, a '<' that opens no tag, a reference without its semicolon,
// tag-like text inside a title, a textarea or an xmp element, which the
// parser reads as text, and an attribute value whose legacy reference the
// attribute rules leave undecoded.
func TestWriter_EditKeepsUntouchedBlocksByteExact(t *testing.T) {
	const doc = `<html><head><meta charset="utf-8"><title>a <b> c & d &copy 2020</title></head><body>` +
		`<form><textarea name="t">Type <b>here</b> & go</textarea></form>` +
		`<div><xmp>raw <b>x</b> &amp; y</xmp></div>` +
		`<p title="a&copy=2 and R&D">Text with a <a href="/x" title="q&notit; done">link</a> here.</p>` +
		`<p>Q&A and AT&T</p>` +
		`<p>a < b and I <3 you</p>` +
		`<p>&copy 2020 &amp; more &#60x</p>` +
		`<pre>x & y < z</pre>` +
		`<div>bare & text <3</div>` +
		`<p><img src=a.png alt=chart> x</p>` +
		`<p title='it is noted'>y</p>` +
		`<p title="t & u">Edit me</p>` +
		`</body></html>`

	out := editSource(t, doc, "Edit me", "Edited")

	assert.Equal(t, strings.Replace(doc, "Edit me", "Edited", 1), out)
}
