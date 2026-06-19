package openxml

import (
	"archive/zip"
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

func TestWriterBasic(t *testing.T) {
	// Read original
	original, err := os.ReadFile("testdata/simple.docx")
	require.NoError(t, err)

	f, err := os.Open("testdata/simple.docx")
	require.NoError(t, err)

	reader := NewReader()
	doc := testutil.RawDocFromReader(f, "testdata/simple.docx", model.LocaleEnglish)
	err = reader.Open(t.Context(), doc)
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(t.Context(), ch)
	require.NoError(t, err)
	writer.Close()

	// Output should be a valid ZIP
	assert.Greater(t, buf.Len(), 0, "output should not be empty")
	assert.Equal(t, byte(0x50), buf.Bytes()[0], "should start with PK")
	assert.Equal(t, byte(0x4B), buf.Bytes()[1])
}

// skeletonWriteOnce reads original with a fresh skeleton store and returns
// the recompiled .docx bytes. Native is faithful (Word Style Optimisation
// removed), so this exercises the faithful flush path: skeleton
// reconstruction + lang strip/retarget + postNonWSOForName + the O3
// per-part strip/decompress caches.
func skeletonWriteOnce(t *testing.T, original []byte, uri string) []byte {
	t.Helper()

	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(original),
	}
	require.NoError(t, reader.Open(t.Context(), doc))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel(parts)))
	writer.Close()

	return append([]byte(nil), buf.Bytes()...)
}

// TestWriterPartCachingDeterministic guards the O3 per-part strip and
// decompress caches (#608) on the faithful flush path: word/*.xml parts
// are stripped (stripWMLSkippableElementsCached) and decompressed
// (readZipFileCached) at most once, and the cached slices are handed out
// read-only to both the strip and emit steps. A stray in-place mutation
// would corrupt a later read of the same part, so two independent writes
// of the same input must produce byte-identical output.
func TestWriterPartCachingDeterministic(t *testing.T) {
	for _, name := range []string{"simple.docx", "formatted.docx"} {
		t.Run(name, func(t *testing.T) {
			original, err := os.ReadFile("testdata/" + name)
			require.NoError(t, err)

			out1 := skeletonWriteOnce(t, original, name)
			out2 := skeletonWriteOnce(t, original, name)
			require.NotEmpty(t, out1)
			assert.Equal(t, out1, out2,
				"repeated skeleton writes must be byte-identical (cache must not leak mutated part bytes)")
		})
	}
}

// onlyWriter wraps an io.Writer to hide any bytes.Buffer-specific
// interfaces (e.g. io.WriterTo / Bytes), proving the writer streams to a
// forward-only io.Writer without needing seek/buffer access (#608, S3).
type onlyWriter struct{ w io.Writer }

func (o onlyWriter) Write(p []byte) (int, error) { return o.w.Write(p) }

// TestWriterStreamsToForwardOnlyWriter guards the S3-openxml change that
// passes w.Output straight to zip.NewWriter instead of buffering the whole
// archive. The output written to a plain forward-only io.Writer must be a
// valid, non-empty .docx (PK header) and byte-identical to a write into a
// bytes.Buffer.
func TestWriterStreamsToForwardOnlyWriter(t *testing.T) {
	original, err := os.ReadFile("testdata/simple.docx")
	require.NoError(t, err)

	writeInto := func(out io.Writer) {
		skelStore, err := format.NewSkeletonStore()
		require.NoError(t, err)
		defer skelStore.Close()

		reader := NewReader()
		reader.SetSkeletonStore(skelStore)
		doc := &model.RawDocument{
			URI:          "simple.docx",
			SourceLocale: model.LocaleEnglish,
			Encoding:     "UTF-8",
			Reader:       readCloserFromBytes(original),
		}
		require.NoError(t, reader.Open(t.Context(), doc))
		parts := testutil.CollectParts(t, reader.Read(t.Context()))
		reader.Close()

		writer := NewWriter()
		writer.SetOriginalContent(original)
		writer.SetSkeletonStore(skelStore)
		require.NoError(t, writer.SetOutputWriter(out))
		require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel(parts)))
		writer.Close()
	}

	var streamed, buffered bytes.Buffer
	writeInto(onlyWriter{w: &streamed}) // forward-only writer
	writeInto(&buffered)                // bytes.Buffer

	require.NotEmpty(t, streamed.Bytes())
	require.Equal(t, byte(0x50), streamed.Bytes()[0], "should start with PK")
	require.Equal(t, byte(0x4B), streamed.Bytes()[1])
	assert.Equal(t, buffered.Bytes(), streamed.Bytes(),
		"streaming to a forward-only writer must match buffered output")
}

// TestPullLeadingFldCharEndMigration locks the byte output of the fld-end
// migration splice after the O(n²) String()+concat+Reset pattern was
// replaced with slices.Insert on a []byte buffer (#608, O5). The migration
// must be byte-identical to the prior strings.Builder implementation.
func TestPullLeadingFldCharEndMigration(t *testing.T) {
	t.Run("migrates leading fld-end into previous paragraph", func(t *testing.T) {
		// Para1 holds an unmatched fld-begin (balance +1, eligible);
		// para2 leads with a bare fld-end run that must migrate up.
		in := `<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r></w:p>` +
			`<w:p><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>`
		want := `<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r>` +
			`<w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>` +
			`<w:p></w:p>`
		got := pullLeadingFldCharEndIntoPrevParagraph([]byte(in))
		assert.Equal(t, want, string(got))
	})

	t.Run("no migration when balance is zero", func(t *testing.T) {
		// Self-contained field in para1, plain text in para2 — nothing
		// to migrate; output must be byte-identical to input.
		in := `<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r>` +
			`<w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>` +
			`<w:p><w:r><w:t>hello</w:t></w:r></w:p>`
		got := pullLeadingFldCharEndIntoPrevParagraph([]byte(in))
		assert.Equal(t, in, string(got))
	})

	t.Run("fast-path passthrough without fld-end", func(t *testing.T) {
		in := `<w:p><w:r><w:t>plain</w:t></w:r></w:p>`
		got := pullLeadingFldCharEndIntoPrevParagraph([]byte(in))
		assert.Equal(t, in, string(got))
	})

	t.Run("migrates inside txbxContent", func(t *testing.T) {
		inner := `<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r></w:p>` +
			`<w:p><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>`
		in := `<w:txbxContent>` + inner + `</w:txbxContent>`
		wantInner := `<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r>` +
			`<w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>` +
			`<w:p></w:p>`
		want := `<w:txbxContent>` + wantInner + `</w:txbxContent>`
		got := pullLeadingFldCharEndIntoPrevParagraphInTxbxContents([]byte(in))
		assert.Equal(t, want, string(got))
	})
}

func TestWriterNilOriginal(t *testing.T) {
	writer := NewWriter()
	var buf bytes.Buffer
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := make(chan *model.Part)
	close(ch)

	err = writer.Write(t.Context(), ch)
	require.Error(t, err, "should error without original content")
}

func TestWriterMediaReplacement(t *testing.T) {
	replacementPNG := []byte("REPLACED-IMAGE-DATA")

	// The variant asset travels as a *model.Media reference: inline Data for a
	// small asset, or a file path (URI) the writer streams from. Both land
	// byte-identical in the output ZIP.
	cases := map[string]func(t *testing.T) *model.Media{
		"inline-data": func(_ *testing.T) *model.Media {
			return &model.Media{Data: replacementPNG}
		},
		"file-uri": func(t *testing.T) *model.Media {
			p := filepath.Join(t.TempDir(), "variant.png")
			require.NoError(t, os.WriteFile(p, replacementPNG, 0o644))
			return &model.Media{URI: p}
		},
	}

	for name, mkMedia := range cases {
		t.Run(name, func(t *testing.T) {
			// Build a DOCX with an embedded PNG.
			docxFile := buildDocxWithMedia(t)
			original, err := os.ReadFile(docxFile.Name())
			require.NoError(t, err)

			// Read the original.
			f, err := os.Open(docxFile.Name())
			require.NoError(t, err)

			reader := NewReader()
			doc := testutil.RawDocFromReader(f, "test.docx", model.LocaleEnglish)
			require.NoError(t, reader.Open(t.Context(), doc))
			parts := testutil.CollectParts(t, reader.Read(t.Context()))
			reader.Close()

			// Write with a media replacement.
			var buf bytes.Buffer
			writer := NewWriter()
			writer.SetOriginalContent(original)
			writer.SetMediaReplacement("word/media/test.png", mkMedia(t))
			require.NoError(t, writer.SetOutputWriter(&buf))

			ch := testutil.PartsToChannel(parts)
			require.NoError(t, writer.Write(t.Context(), ch))
			writer.Close()

			// Verify the output ZIP contains the replacement.
			zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			require.NoError(t, err)

			for _, f := range zr.File {
				if f.Name == "word/media/test.png" {
					data, err := readZipFile(f)
					require.NoError(t, err)
					assert.Equal(t, replacementPNG, data, "media should be replaced with locale variant")
					return
				}
			}
			t.Fatal("word/media/test.png not found in output ZIP")
		})
	}
}
