package odf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/formats/odf"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSourcePath_ByteEqualToOriginalContent asserts that reconstructing an
// ODF from a source PATH (SourcePathSetter, #608, S2) produces
// byte-identical output to reconstructing from in-memory bytes
// (OriginalContentSetter), through the reparse writer path.
func TestSourcePath_ByteEqualToOriginalContent(t *testing.T) {
	data := makeODFZip(mimeODT, simpleODTContent("Hello, World!", "Second paragraph"))

	srcPath := filepath.Join(t.TempDir(), "src.odt")
	require.NoError(t, os.WriteFile(srcPath, data, 0o644))

	out := func(usePath bool) []byte {
		ctx := t.Context()
		reader := odf.NewReader()
		require.NoError(t, reader.Open(ctx, testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish)))
		parts := testutil.CollectParts(t, reader.Read(ctx))
		reader.Close()

		var buf bytes.Buffer
		writer := odf.NewWriter()
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
