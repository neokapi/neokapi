package epub_test

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats/epub"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// An EPUB block is addressed by spine item plus its position inside that item's
// markup. Naming it by the item alone — which is what this reader used to do —
// gave every paragraph of a chapter the same address, and a block's Name is
// folded into its context hash (core/reconcile), so that address is what the
// rest of the system matches a re-read against.

// makeBook packs an EPUB whose two chapters carry the given bodies.
func makeBook(t *testing.T, ch1Body, ch2Body string) []byte {
	t.Helper()

	const opf = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <manifest>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="ch2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="ch1"/><itemref idref="ch2"/></spine>
</package>`
	page := func(body string) string {
		return `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body>` + body + `</body></html>`
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	require.NoError(t, err)
	_, err = io.WriteString(w, "application/epub+zip")
	require.NoError(t, err)

	for name, content := range map[string]string{
		"META-INF/container.xml": containerXML,
		"OEBPS/content.opf":      opf,
		"OEBPS/chapter1.xhtml":   page(ch1Body),
		"OEBPS/chapter2.xhtml":   page(ch2Body),
	} {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(w, content)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func bookNamesByText(t *testing.T, data []byte) map[string]string {
	t.Helper()
	ctx := t.Context()
	reader := epub.NewReader()
	require.NoError(t, reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish)))
	defer reader.Close()

	out := map[string]string{}
	for _, b := range testutil.CollectBlocks(t, reader.Read(ctx)) {
		out[b.SourceText()] = b.Name
	}
	return out
}

func bookNames(t *testing.T, data []byte) []string {
	t.Helper()
	ctx := t.Context()
	reader := epub.NewReader()
	require.NoError(t, reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish)))
	defer reader.Close()

	var out []string
	for _, b := range testutil.CollectBlocks(t, reader.Read(ctx)) {
		out = append(out, b.Name)
	}
	return out
}

func TestStructuralName_DeletionInAnotherChapterDoesNotRename(t *testing.T) {
	v1 := makeBook(t, `<p>Alpha</p><p>Bravo</p>`, `<p>Charlie</p><p>Delta</p>`)
	v2 := makeBook(t, `<p>Alpha</p>`, `<p>Charlie</p><p>Delta</p>`)

	before, after := bookNamesByText(t, v1), bookNamesByText(t, v2)

	require.Equal(t, "OEBPS/chapter1.xhtml/p", before["Alpha"])
	require.Equal(t, "OEBPS/chapter1.xhtml/p[2]", before["Bravo"])
	require.Equal(t, "OEBPS/chapter2.xhtml/p", before["Charlie"])
	require.Equal(t, "OEBPS/chapter2.xhtml/p[2]", before["Delta"])

	assert.Equal(t, before["Charlie"], after["Charlie"],
		"deleting a paragraph from chapter 1 must not rename anything in chapter 2")
	assert.Equal(t, before["Delta"], after["Delta"])
	assert.Equal(t, before["Alpha"], after["Alpha"])
}

func TestStructuralName_IdenticalTextGetsDistinctNames(t *testing.T) {
	data := makeBook(t, `<div><p>Same</p><p>Same</p></div><p>Same</p>`, `<p>Same</p>`)

	names := bookNames(t, data)
	assert.Equal(t, []string{
		"OEBPS/chapter1.xhtml/div/p",
		"OEBPS/chapter1.xhtml/div/p[2]",
		"OEBPS/chapter1.xhtml/p",
		"OEBPS/chapter2.xhtml/p",
	}, names)

	seen := map[string]bool{}
	for _, n := range names {
		assert.False(t, seen[n], "duplicate block name %q — four identical paragraphs would share one identity", n)
		seen[n] = true
	}
}

func TestStructuralName_StableAcrossTwoReads(t *testing.T) {
	data := makeBook(t, `<h1>Title</h1><div><p>Alpha</p><p>Bravo</p></div>`, `<p>Charlie</p>`)
	assert.Equal(t, bookNames(t, data), bookNames(t, data),
		"two reads of identical bytes must produce identical names")
}
