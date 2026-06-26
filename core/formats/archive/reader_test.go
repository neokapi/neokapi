package archive_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildRegistry(t *testing.T) *registry.FormatRegistry {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	return reg
}

func makeZip(t *testing.T, entries map[string][]byte, order []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range order {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(entries[name])
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func readArchive(t *testing.T, reg *registry.FormatRegistry, uri string, data []byte) (childFormats, dataNames, blockTexts []string) {
	t.Helper()
	reader, err := reg.NewReader("archive")
	require.NoError(t, err)
	doc := &model.RawDocument{URI: uri, Reader: io.NopCloser(bytes.NewReader(data))}
	require.NoError(t, reader.Open(context.Background(), doc))
	for pr := range reader.Read(context.Background()) {
		require.NoError(t, pr.Error)
		switch pr.Part.Type {
		case model.PartLayerStart:
			if l, ok := pr.Part.Resource.(*model.Layer); ok && l.Properties["subfilter.source"] == "archive" {
				childFormats = append(childFormats, l.Format)
			}
		case model.PartData:
			if d, ok := pr.Part.Resource.(*model.Data); ok {
				dataNames = append(dataNames, d.Name)
			}
		case model.PartBlock:
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				blockTexts = append(blockTexts, b.SourceText())
			}
		}
	}
	require.NoError(t, reader.Close())
	return
}

func TestArchiveIsReadOnly(t *testing.T) {
	reg := buildRegistry(t)
	assert.True(t, reg.HasReader("archive"))
	assert.False(t, reg.HasWriter("archive"), "archive is inspection-only; containers are localized via the container binding")
}

func TestDetectsPlainZipAsArchive(t *testing.T) {
	reg := buildRegistry(t)
	data := makeZip(t, map[string][]byte{
		"messages.json": []byte(`{"greeting":"Hello"}`),
		"notes.txt":     []byte("a note"),
	}, []string{"messages.json", "notes.txt"})
	name, err := reg.Detector().Detect("bundle.zip", bytes.NewReader(data), "")
	require.NoError(t, err)
	assert.Equal(t, "archive", name)
}

func TestEpubSniffWinsOverArchive(t *testing.T) {
	reg := buildRegistry(t)
	data := makeZip(t, map[string][]byte{
		"mimetype":    []byte("application/epub+zip"),
		"content.opf": []byte("<package/>"),
	}, []string{"mimetype", "content.opf"})
	name, err := reg.Detector().Detect("book.epub", bytes.NewReader(data), "")
	require.NoError(t, err)
	assert.Equal(t, "epub", name)
}

func TestInspectSurfacesEntries(t *testing.T) {
	reg := buildRegistry(t)
	data := makeZip(t, map[string][]byte{
		"locales/en.json": []byte(`{"greeting":"Hello","farewell":"Bye"}`),
		"README.md":       []byte("# Title\n\nSome prose.\n"),
		"logo.png":        []byte("\x89PNG\r\n\x1a\nbinary"),
		"data.bin":        []byte("\x00\x01opaque"),
	}, []string{"locales/en.json", "README.md", "logo.png", "data.bin"})

	childFormats, dataNames, blockTexts := readArchive(t, reg, "bundle.zip", data)

	assert.ElementsMatch(t, []string{"json", "markdown"}, childFormats)
	assert.ElementsMatch(t, []string{"logo.png", "data.bin"}, dataNames)
	assert.Contains(t, blockTexts, "Hello")
	assert.Contains(t, blockTexts, "Bye")
}

func TestExcludeGlobListsEntryAsData(t *testing.T) {
	reg := buildRegistry(t)
	reader, err := reg.NewReader("archive")
	require.NoError(t, err)
	require.NoError(t, reader.Config().ApplyMap(map[string]any{"exclude": []any{"vendor/**"}}))

	data := makeZip(t, map[string][]byte{
		"app.json":        []byte(`{"greeting":"Hi"}`),
		"vendor/lib.json": []byte(`{"x":"y"}`),
	}, []string{"app.json", "vendor/lib.json"})
	doc := &model.RawDocument{URI: "bundle.zip", Reader: io.NopCloser(bytes.NewReader(data))}
	require.NoError(t, reader.Open(context.Background(), doc))

	var childNames, dataNames []string
	for pr := range reader.Read(context.Background()) {
		require.NoError(t, pr.Error)
		switch pr.Part.Type {
		case model.PartLayerStart:
			if l, ok := pr.Part.Resource.(*model.Layer); ok && l.Properties["subfilter.source"] == "archive" {
				childNames = append(childNames, l.Name)
			}
		case model.PartData:
			if d, ok := pr.Part.Resource.(*model.Data); ok {
				dataNames = append(dataNames, d.Name)
			}
		}
	}
	require.NoError(t, reader.Close())

	assert.Contains(t, childNames, "app.json")
	assert.Contains(t, dataNames, "vendor/lib.json")
	assert.NotContains(t, childNames, "vendor/lib.json")
}
