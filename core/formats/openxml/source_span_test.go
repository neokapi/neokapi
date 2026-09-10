package openxml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDOCXParagraphSourceSpans(t *testing.T) {
	paragraphs := []string{
		`<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Résumé</w:t></w:r></w:p>`,
		`<w:p><w:r><w:t xml:space="preserve">Same body </w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>bold</w:t></w:r></w:p>`,
		`<w:p><w:r><w:t>Same body</w:t></w:r></w:p>`,
	}
	source := wmlDoc(strings.Join(paragraphs, "\n  "))
	blocks := readDocxBlocks(t, source)
	require.Len(t, blocks, len(paragraphs))
	for i, block := range blocks {
		span, ok := block.SourceSpan()
		require.True(t, ok, "paragraph %d has no physical source span", i)
		require.Equal(t, "word/document.xml", span.Part)
		require.Equal(t, strings.Index(source, paragraphs[i]), span.Start)
		require.Equal(t, paragraphs[i], source[span.Start:span.End])
	}
}

func TestDOCXRevisionBlockHasNoStandaloneSpan(t *testing.T) {
	source := wmlDoc(`<w:p><w:ins w:id="1"><w:r><w:t>Inserted</w:t></w:r></w:ins></w:p>`)
	blocks := readDocxBlocks(t, source)
	require.Len(t, blocks, 1)
	_, ok := blocks[0].SourceSpan()
	require.False(t, ok, "accepted revision must not claim an ordinary paragraph span")
}
