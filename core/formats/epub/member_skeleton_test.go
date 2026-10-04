package epub_test

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/epub"
	"github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// htmlResolver resolves the spine items' sub-format to the real HTML reader
// and writer, as the format registry does.
type htmlResolver struct{}

func (htmlResolver) ResolveReader(string) (format.DataFormatReader, error) {
	return html.NewReader(), nil
}

func (htmlResolver) ResolveWriter(string) (format.DataFormatWriter, error) {
	return html.NewWriter(), nil
}

const inlineChapter = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter One</title></head>
<body>
  <h1>Why we utilize fresh produce</h1>
  <p>We utilize the <b>finest</b> <a href="https://example.com/utilize">ingredients</a> daily.</p>
  <p><img src="utilize.png" alt="Photos we utilize"/> Read the guide before you <em>order</em>.</p>
</body>
</html>
`

func makeInlineEPUB(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	require.NoError(t, err)
	_, err = io.WriteString(w, "application/epub+zip")
	require.NoError(t, err)
	for _, e := range []struct{ name, body string }{
		{"META-INF/container.xml", `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`},
		{"OEBPS/content.opf", `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <manifest><item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`},
		{"OEBPS/chapter1.xhtml", inlineChapter},
	} {
		fw, err := zw.Create(e.name)
		require.NoError(t, err)
		_, err = io.WriteString(fw, e.body)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// editChapter reads the EPUB through the skeleton store with the HTML
// sub-format resolved, hands every block to edit, writes it back, and returns
// the written chapter.
func editChapter(t *testing.T, data []byte, edit func(*model.Block)) string {
	t.Helper()
	ctx := t.Context()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()

	reader := epub.NewReader()
	reader.SetSubfilterResolver(htmlResolver{})
	reader.SetSkeletonStore(store)
	require.NoError(t, reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && edit != nil {
			edit(b)
		}
	}

	var buf bytes.Buffer
	writer := epub.NewWriter()
	writer.SetSubfilterResolver(htmlResolver{})
	writer.SetOriginalContent(data)
	writer.SetSkeletonStore(store)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	for _, f := range zr.File {
		if f.Name != "OEBPS/chapter1.xhtml" {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		out, err := io.ReadAll(rc)
		require.NoError(t, rc.Close())
		require.NoError(t, err)
		return string(out)
	}
	t.Fatal("OEBPS/chapter1.xhtml is missing from the written EPUB")
	return ""
}

// TestSpineItemKeepsItsInlineMarkup is the regression for spine items read
// through the HTML sub-format: the writer spliced each block's flattened text
// into the first text span of its element and dropped the rest, so every write
// moved a paragraph's inline elements, emptied, to its end, whether or not
// anything was edited, and an edit to an image's alt text was dropped.
func TestSpineItemKeepsItsInlineMarkup(t *testing.T) {
	sub := func(from, to string) func(*model.Block) {
		return func(b *model.Block) {
			text := model.RunsEditText(b.SourceRuns())
			if edited := strings.ReplaceAll(text, from, to); edited != text {
				b.EditSourceRuns(model.ParseRunsEditText(edited, b.SourceRuns()))
			}
		}
	}
	tests := []struct {
		name string
		edit func(*model.Block)
		want string
	}{
		{name: "untouched", want: inlineChapter},
		{
			name: "a word edited in every block",
			edit: sub("utilize", "use"),
			want: strings.NewReplacer(
				"Why we utilize", "Why we use",
				"We utilize the", "We use the",
				`alt="Photos we utilize"`, `alt="Photos we use"`,
			).Replace(inlineChapter),
		},
	}
	data := makeInlineEPUB(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, editChapter(t, data, tc.edit))
		})
	}
}

// TestSpineItemSkeletonRidesOnTheClosingLayer pins where the reader records a
// spine item's skeleton: on the layer that closes the item. The layer that
// opened it has already gone downstream when the item's skeleton is complete,
// and a consumer may be reading its annotations, so the reader leaves it as
// emitted.
func TestSpineItemSkeletonRidesOnTheClosingLayer(t *testing.T) {
	ctx := t.Context()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader := epub.NewReader()
	reader.SetSubfilterResolver(htmlResolver{})
	reader.SetSkeletonStore(store)
	require.NoError(t, reader.Open(ctx, rawDocFromBytes(makeInlineEPUB(t), model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())

	opened := map[string]*model.Layer{}
	closed := 0
	for _, p := range parts {
		layer, ok := p.Resource.(*model.Layer)
		if !ok || layer.Properties["subfilter.source"] == "" {
			continue
		}
		switch p.Type {
		case model.PartLayerStart:
			opened[layer.ID] = layer
		case model.PartLayerEnd:
			closed++
			start := opened[layer.ID]
			require.NotNil(t, start, "layer %s closes without opening", layer.ID)
			_, onEnd := layer.Anno("epub:member-skeleton")
			assert.True(t, onEnd, "the closing layer carries the item's skeleton")
			_, onStart := start.Anno("epub:member-skeleton")
			assert.False(t, onStart, "the opening layer is left as it was emitted")
		}
	}
	assert.Equal(t, 1, closed, "the book has one spine item read through the HTML reader")
}

// TestUntouchedSpineItemKeepsItsBytesWithoutASkeleton covers the package
// rewrite a write takes when no skeleton store is wired: a block nothing
// changed is left as the item holds it, inline markup included.
func TestUntouchedSpineItemKeepsItsBytesWithoutASkeleton(t *testing.T) {
	ctx := t.Context()
	data := makeInlineEPUB(t)

	reader := epub.NewReader()
	reader.SetSubfilterResolver(htmlResolver{})
	require.NoError(t, reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())

	var buf bytes.Buffer
	writer := epub.NewWriter()
	writer.SetSubfilterResolver(htmlResolver{})
	writer.SetOriginalContent(data)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	for _, f := range zr.File {
		if f.Name != "OEBPS/chapter1.xhtml" {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		out, err := io.ReadAll(rc)
		require.NoError(t, rc.Close())
		require.NoError(t, err)
		assert.Equal(t, inlineChapter, string(out))
		return
	}
	t.Fatal("OEBPS/chapter1.xhtml is missing from the written EPUB")
}
