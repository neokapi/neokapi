package editor_test

import (
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/editor"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseItem(t *testing.T) {
	t.Parallel()
	// Create an HTML reader
	reader := htmlfmt.NewReader()

	htmlContent := `<html><body><p>Hello world</p><p>Second paragraph</p></body></html>`
	doc := &model.RawDocument{
		URI:          "test.html",
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(strings.NewReader(htmlContent)),
	}

	result, err := editor.ParseItem(t.Context(), reader, doc, "en", "html", "test.html")
	require.NoError(t, err)

	// Verify result has all expected fields populated
	assert.NotEmpty(t, result.Parts)
	assert.NotEmpty(t, result.Blocks)
	assert.NotNil(t, result.BlockIndex)
	assert.NotEmpty(t, result.BlockIndexJSON)
	preview := result.PreviewHTML()
	assert.NotEmpty(t, preview)

	// The preview carries kat-block markers.
	assert.Contains(t, preview, "kat-block")

	// BlockIndex should have the correct metadata
	assert.Equal(t, "en", result.BlockIndex.SourceLanguage)
	assert.Equal(t, "html", result.BlockIndex.OriginalFormat)
	assert.Equal(t, "test.html", result.BlockIndex.OriginalItem)
}

// The preview marks each block by its durable key (convergence.BlockKey), the
// key a venue stores as the block's source id, and reads it when it is built:
// a key resolved after the parse is the one the markers carry. The parsed
// blocks keep the reader's ids.
func TestParseItem_PreviewMarksBlocksByTheirKey(t *testing.T) {
	t.Parallel()
	doc := &model.RawDocument{
		URI:          "test.html",
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(strings.NewReader(`<html><body><p>Hello world</p><p>Second paragraph</p></body></html>`)),
	}
	result, err := editor.ParseItem(t.Context(), htmlfmt.NewReader(), doc, "en", "html", "test.html")
	require.NoError(t, err)
	require.Len(t, result.Blocks, 2)

	first, second := result.Blocks[0], result.Blocks[1]
	readerIDs := []string{first.ID, second.ID}
	first.Key = "u-0123456789abcdef"
	second.Key, second.Name = "", "body/p#2"

	preview := result.PreviewHTML()
	assert.Contains(t, preview, `<kat-block id="u-0123456789abcdef">`, "a resolved key marks its block")
	assert.Contains(t, preview, `<kat-block id="body/p#2">`, "a block with no key is marked by its name")
	for _, id := range readerIDs {
		assert.NotContains(t, preview, `<kat-block id="`+id+`">`, "no marker carries the reader's id")
	}
	assert.Equal(t, readerIDs[0], first.ID, "the parsed blocks keep the reader's ids")
	assert.Equal(t, readerIDs[1], second.ID, "the parsed blocks keep the reader's ids")
}
