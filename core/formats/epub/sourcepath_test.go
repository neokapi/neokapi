package epub_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/formats/epub"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSourcePath_ByteEqualToOriginalContent asserts that reconstructing an
// EPUB from a source PATH (SourcePathSetter, #608, S2) produces
// byte-identical output to reconstructing from in-memory bytes
// (OriginalContentSetter), through the no-skeleton writeEPUB path.
func TestSourcePath_ByteEqualToOriginalContent(t *testing.T) {
	data := makeEPUB(t)

	srcPath := filepath.Join(t.TempDir(), "src.epub")
	require.NoError(t, os.WriteFile(srcPath, data, 0o644))

	out := func(usePath bool) []byte {
		ctx := t.Context()
		reader := epub.NewReader()
		require.NoError(t, reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish)))
		parts := testutil.CollectParts(t, reader.Read(ctx))
		reader.Close()

		var buf bytes.Buffer
		writer := epub.NewWriter()
		require.NoError(t, writer.SetOutputWriter(&buf))
		if usePath {
			writer.SetSourcePath(srcPath)
		} else {
			writer.SetOriginalContent(data)
		}
		writer.SetLocale(model.LocaleEnglish)
		require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
		require.NoError(t, writer.Close())
		return buf.Bytes()
	}

	fromBytes := out(false)
	fromPath := out(true)
	require.NotEmpty(t, fromPath)
	assert.Equal(t, fromBytes, fromPath,
		"SourcePathSetter output must be byte-identical to OriginalContentSetter output")
}
