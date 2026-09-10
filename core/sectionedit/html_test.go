package sectionedit

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTMLSectionPatchPreservesSourceOutsideBody(t *testing.T) {
	source := []byte("<!DOCTYPE html>\n<html><head><title>Untouched</title></head><body><main class='docs'>\n<h1 id='keep'>Parent</h1>\n<p>Old <em>text</em> &amp; <a href='/guide'>link</a>.</p>\n<h2>Child</h2><p>Nested.</p>\n<h1 data-x=\"y\">Next</h1><p>Tail.</p></main></body></html>")
	original := bytes.Clone(source)
	sections, err := inspectHTML(source)
	require.NoError(t, err)
	require.Len(t, sections, 3)
	assert.Contains(t, sections[0].Content, "Old *text* & [link](</guide>)\\.")
	assert.Contains(t, sections[0].Content, "## Child")
	assert.Equal(t, []string{"Parent", "Child"}, sections[1].Path)
	fragment := "New **bold** and [link](https://example.test).\n\n- First\n- Second\n\n## New child\n\n```go\nrun()\n```"
	patches, err := planHTML(source, 0, fragment)
	require.NoError(t, err)
	assert.Equal(t, original, source)
	result := applyAdapterPatch(t, source, patches)
	assert.True(t, bytes.HasPrefix(result, []byte("<!DOCTYPE html>\n<html><head><title>Untouched</title></head><body><main class='docs'>\n<h1 id='keep'>Parent</h1>")))
	assert.True(t, bytes.HasSuffix(result, []byte("<h1 data-x=\"y\">Next</h1><p>Tail.</p></main></body></html>")))
	assert.Contains(t, string(result), "<strong>bold</strong>")
	assert.Contains(t, string(result), "<code class=\"language-go\">run()")
	after, err := inspectHTML(result)
	require.NoError(t, err)
	require.Len(t, after, 3)
	assert.Equal(t, "New child", after[1].Title)
}

func TestHTMLNestedSectionAndSeparateContainers(t *testing.T) {
	source := []byte("<div id='one'><h1>Same</h1><p>Keep</p><h2>Same</h2><p>Replace</p></div>\n<div id='two'><h2>Same</h2><p>Untouched</p></div>")
	sections, err := inspectHTML(source)
	require.NoError(t, err)
	require.Len(t, sections, 3)
	assert.Equal(t, []string{"Same"}, sections[2].Path)
	patches, err := planHTML(source, 1, "Replacement.")
	require.NoError(t, err)
	result := applyAdapterPatch(t, source, patches)
	assert.Equal(t, "<div id='one'><h1>Same</h1><p>Keep</p><h2>Same</h2>\n<p>Replacement.</p>\n</div>\n<div id='two'><h2>Same</h2><p>Untouched</p></div>", string(result))
}

func TestHTMLReadableProjectionRetainsListsCodeAndInlineSpacing(t *testing.T) {
	source := []byte("<h1>Title</h1><p><em>First</em> next <strong>bold</strong> tail <code>a`b</code>.</p><ol start='3'><li>One<ul><li>Child</li></ul></li><li><p>Two</p><p>More</p></li></ol><pre><code class='language-go'>a &lt; b\n```\n</code></pre>")
	sections, err := inspectHTML(source)
	require.NoError(t, err)
	require.Len(t, sections, 1)
	content := sections[0].Content
	assert.Contains(t, content, "*First* next **bold** tail `` a`b ``\\.")
	assert.Contains(t, content, "3. One\n   \n   - Child")
	assert.Contains(t, content, "4. Two")
	assert.Contains(t, content, "````go\na < b\n```\n````")
}

func TestHTMLUnsupportedAndMalformedShapesFailExplicitly(t *testing.T) {
	cases := []string{
		"<h1>Title</h1><p>Unclosed",
		"<h1>Title</h1><p>Wrong</div>",
		"<h1>Title</h1><p>Implicit<p>Paragraph</p>",
		"<h1>Title</h1><table><tr><td>Cell</td></tr></table>",
		"<h1>Title</h1><script>alert(1)</script>",
		"<h1>Title</h1><p><img src='x'></p>",
		"<h1>Title</h1><div><h2>Wrapped</h2></div>",
		"<h1>Title</h1>Unwrapped text",
		"<h1>Title</h1><p />",
		"<p><h1>Nested invalid heading</h1></p>",
		"<h1>Title</h1><!-- body annotation -->",
	}
	for _, source := range cases {
		t.Run(source, func(t *testing.T) { _, err := inspectHTML([]byte(source)); require.Error(t, err) })
	}
	sections, err := inspectHTML([]byte("<p>No heading.</p>"))
	require.NoError(t, err)
	assert.Empty(t, sections)
	_, err = planHTML([]byte("<p>No heading.</p>"), 0, "New")
	require.Error(t, err)
	_, err = inspectHTML([]byte{0xff})
	require.Error(t, err)
}

func TestHTMLRendererDoesNotEnableRawHTML(t *testing.T) {
	source := []byte("<h1>Title</h1><p>Old</p>")
	patches, err := planHTML(source, 0, "<script>alert(1)</script>\n\nSafe **text**.")
	require.NoError(t, err)
	assert.NotContains(t, patches[0].Replacement, "<script>")
	assert.Contains(t, patches[0].Replacement, "<strong>text</strong>")
}

func TestHTMLNonstandardListNumberingIsRejected(t *testing.T) {
	for _, list := range []string{
		"<ol reversed><li>Third</li><li>Second</li></ol>",
		"<ol><li value='5'>Fifth</li></ol>",
		"<ol type='I'><li>Roman</li></ol>",
	} {
		_, err := inspectHTML([]byte("<h1>Title</h1>" + list))
		require.ErrorContains(t, err, "cannot be projected faithfully")
	}
}
