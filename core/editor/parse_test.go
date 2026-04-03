package editor_test

import (
	"context"
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

	result, err := editor.ParseItem(context.Background(), reader, doc, "en", "html", "test.html")
	require.NoError(t, err)

	// Verify result has all expected fields populated
	assert.NotEmpty(t, result.Parts)
	assert.NotEmpty(t, result.Blocks)
	assert.NotNil(t, result.BlockIndex)
	assert.NotEmpty(t, result.BlockIndexJSON)
	assert.NotEmpty(t, result.PreviewHTML)

	// PreviewHTML should contain kat-block markers
	assert.Contains(t, result.PreviewHTML, "kat-block")

	// BlockIndex should have the correct metadata
	assert.Equal(t, "en", result.BlockIndex.SourceLanguage)
	assert.Equal(t, "html", result.BlockIndex.OriginalFormat)
	assert.Equal(t, "test.html", result.BlockIndex.OriginalItem)
}
