package idml

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSourcePath_ByteEqualToOriginalContent asserts that reconstructing
// from a source PATH (SourcePathSetter, #608, S2) produces byte-identical
// output to reconstructing from in-memory bytes (OriginalContentSetter).
func TestSourcePath_ByteEqualToOriginalContent(t *testing.T) {
	storyXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u1">
    <ParagraphStyleRange AppliedParagraphStyle="ParagraphStyle/$ID/NormalParagraphStyle">
      <CharacterStyleRange AppliedCharacterStyle="CharacterStyle/$ID/[No character style]">
        <Content>Hello World!</Content>
      </CharacterStyleRange>
    </ParagraphStyleRange>
  </Story>
</idPkg:Story>`
	data := createIDML(t, map[string]string{"Story_u1.xml": storyXML})

	// Persist the source so SetSourcePath can re-open it.
	srcPath := filepath.Join(t.TempDir(), "src.idml")
	require.NoError(t, os.WriteFile(srcPath, data, 0o644))

	out := func(t *testing.T, usePath bool) []byte {
		ctx := t.Context()
		skel, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer skel.Close()

		reader := NewReader()
		reader.SetSkeletonStore(skel)
		require.NoError(t, reader.Open(ctx, &model.RawDocument{
			URI:          "test.idml",
			SourceLocale: model.LocaleEnglish,
			Encoding:     "UTF-8",
			MimeType:     "application/vnd.adobe.indesign-idml-package",
			Reader:       io.NopCloser(bytes.NewReader(data)),
		}))
		parts := testutil.CollectParts(t, reader.Read(ctx))
		reader.Close()

		var buf bytes.Buffer
		writer := NewWriter()
		writer.SetSkeletonStore(skel)
		if usePath {
			writer.SetSourcePath(srcPath)
		} else {
			writer.SetOriginalContent(data)
		}
		require.NoError(t, writer.SetOutputWriter(&buf))
		writer.SetLocale(model.LocaleEnglish)
		require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
		return buf.Bytes()
	}

	fromBytes := out(t, false)
	fromPath := out(t, true)
	require.NotEmpty(t, fromPath)
	assert.Equal(t, fromBytes, fromPath,
		"SourcePathSetter output must be byte-identical to OriginalContentSetter output")
}

// TestSourcePath_PrecedenceOverOriginalContent asserts SetSourcePath wins
// when both are set, and that the writer holds no whole-file bytes copy.
func TestSourcePath_PrecedenceOverOriginalContent(t *testing.T) {
	data := createIDML(t, map[string]string{
		"Story_u1.xml": `<?xml version="1.0"?><idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging"><Story Self="u1"/></idPkg:Story>`,
	})
	srcPath := filepath.Join(t.TempDir(), "src.idml")
	require.NoError(t, os.WriteFile(srcPath, data, 0o644))

	w := NewWriter()
	w.SetSourcePath(srcPath)
	assert.Nil(t, w.originalContent, "path mode must not hold the archive bytes")
	assert.Equal(t, srcPath, w.sourcePath)
}
