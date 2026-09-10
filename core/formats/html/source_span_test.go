package html_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sourceSpanHTMLBlocks(t *testing.T, source string) []*model.Block {
	t.Helper()
	reader := htmlfmt.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	reader.SetSkeletonStore(store)
	require.NoError(t, reader.Open(t.Context(), testutil.RawDocFromString(source, model.LocaleEnglish)))
	t.Cleanup(func() { _ = reader.Close() })
	return testutil.CollectBlocks(t, reader.Read(t.Context()))
}

func TestTokenReaderSourceSpansPreserveElementOffsets(t *testing.T) {
	source := "\ufeff<!doctype html><html><body>\r\n<h1 id='a'>Café</h1><p>Repeat <em>text</em>.</p><ul><li>First</li><li>Second</li></ul><p>Repeat <em>text</em>.</p><pre><code>run()</code></pre></body></html>"
	blocks := sourceSpanHTMLBlocks(t, source)
	require.Len(t, blocks, 6)
	expected := []string{"<h1 id='a'>Café</h1>", "<p>Repeat <em>text</em>.</p>", "<li>First</li>", "<li>Second</li>", "<p>Repeat <em>text</em>.</p>", "<pre><code>run()</code></pre>"}
	assert.Equal(t, 1, blocks[0].HeadingLevel())
	starts := []int{}
	for index, block := range blocks {
		span, ok := block.SourceSpan()
		require.True(t, ok, block.ID)
		require.Empty(t, span.Part)
		require.LessOrEqual(t, span.End, len(source))
		assert.Equal(t, expected[index], source[span.Start:span.End])
		starts = append(starts, span.Start)
	}
	assert.IsIncreasing(t, starts)
}

func TestTokenReaderDoesNotGuessSpanForImplicitlyClosedParagraph(t *testing.T) {
	source := "<body><h1>Title</h1><p>Implicit<p>Explicit</p></body>"
	blocks := sourceSpanHTMLBlocks(t, source)
	for _, block := range blocks {
		if strings.Contains(block.SourceText(), "Implicit") {
			_, ok := block.SourceSpan()
			assert.False(t, ok)
		}
	}
}
