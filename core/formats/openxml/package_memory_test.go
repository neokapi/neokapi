package openxml

import (
	"archive/zip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeMediaDocx builds a .docx with a fixed body and mediaBytes of embedded
// media, stored uncompressed so it occupies the same space in the archive that a
// real embedded image (already-compressed) would. It returns the path and the
// media entries' contents by zip path.
func writeMediaDocx(t *testing.T, dir string, mediaBytes int) (string, map[string][]byte) {
	t.Helper()

	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	body.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for i := range 200 {
		body.WriteString(`<w:p><w:r><w:t xml:space="preserve">Paragraph ` + strconv.Itoa(i) + ` of the document body.</w:t></w:r></w:p>`)
	}
	body.WriteString(`<w:sectPr/></w:body></w:document>`)

	path := filepath.Join(dir, fmt.Sprintf("media-%d.docx", mediaBytes))
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	zw := zip.NewWriter(f)
	add := func(name string, data []byte, method uint16) {
		w, werr := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		require.NoError(t, werr)
		_, werr = w.Write(data)
		require.NoError(t, werr)
	}
	add("[Content_Types].xml", []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`+
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`+
		`<Default Extension="xml" ContentType="application/xml"/>`+
		`<Default Extension="png" ContentType="image/png"/>`+
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>`+
		`</Types>`), zip.Deflate)
	add("_rels/.rels", []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>`+
		`</Relationships>`), zip.Deflate)
	add("word/_rels/document.xml.rels", []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`), zip.Deflate)
	add("word/document.xml", []byte(body.String()), zip.Deflate)

	// Random bytes: incompressible, so Store keeps the archive honest about how
	// much media it carries. The reader identifies media by path and extension,
	// never by decoding it.
	media := map[string][]byte{}
	const chunk = 1 << 20
	for i := 0; i*chunk < mediaBytes; i++ {
		buf := make([]byte, chunk)
		_, rerr := rand.Read(buf)
		require.NoError(t, rerr)
		name := fmt.Sprintf("word/media/image%d.png", i+1)
		add(name, buf, zip.Store)
		media[name] = buf
	}
	require.NoError(t, zw.Close())
	return path, media
}

// readMediaParts reads doc with media extraction on and returns the Media parts.
func readMediaParts(t *testing.T, doc *model.RawDocument) []*model.Media {
	t.Helper()
	r := NewReader()
	r.cfg.ExtractMedia = true
	require.NoError(t, r.Open(t.Context(), doc))
	t.Cleanup(func() { r.Close() })

	var out []*model.Media
	for res := range r.Read(t.Context()) {
		require.NoError(t, res.Error)
		if res.Part != nil && res.Part.Type == model.PartMedia {
			m, ok := res.Part.Resource.(*model.Media)
			require.True(t, ok)
			out = append(out, m)
		}
	}
	return out
}

// TestMediaPartsAreDeferred pins the contract embedded media travels under: the
// part carries its identity — content-addressed key, size, MIME — but not its
// bytes, and the deferred accessor produces exactly the bytes in the package.
func TestMediaPartsAreDeferred(t *testing.T) {
	path, want := writeMediaDocx(t, t.TempDir(), 3<<20)
	media := readMediaParts(t, testutil.RawDocFromFile(t, path, model.LocaleEnglish))
	require.Len(t, media, len(want))

	for _, m := range media {
		zipPath := m.Properties["zipPath"]
		expect, ok := want[zipPath]
		require.True(t, ok, "unexpected media entry %q", zipPath)

		assert.Empty(t, m.Data, "media bytes must not be materialized by the reader")
		assert.True(t, m.HasContent(), "media must still be readable through its accessor")
		assert.Equal(t, int64(len(expect)), m.Size)
		assert.Equal(t, "image/png", m.MimeType)

		sum := sha256.Sum256(expect)
		assert.Equal(t, hex.EncodeToString(sum[:]), m.BlobKey,
			"the content-addressed key must be computed without materializing the asset")

		got, err := m.Bytes()
		require.NoError(t, err)
		assert.Equal(t, expect, got)

		require.NoError(t, m.Materialize())
		assert.Equal(t, expect, m.Data, "Materialize must inline the same bytes")
	}
}

// TestRandomAccessReadMatchesBufferedRead is the equivalence the whole change
// rests on: reading a package in place through a file handle must produce the
// same part stream as buffering it, since only the source of the bytes differs.
func TestRandomAccessReadMatchesBufferedRead(t *testing.T) {
	path, _ := writeMediaDocx(t, t.TempDir(), 2<<20)

	collect := func(doc *model.RawDocument) []string {
		r := NewReader()
		r.cfg.ExtractMedia = true
		require.NoError(t, r.Open(t.Context(), doc))
		defer r.Close()
		var got []string
		for res := range r.Read(t.Context()) {
			require.NoError(t, res.Error)
			if res.Part == nil {
				continue
			}
			id := ""
			if res.Part.Resource != nil {
				id = res.Part.Resource.ResourceID()
			}
			got = append(got, fmt.Sprintf("%d:%s", res.Part.Type, id))
		}
		return got
	}

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	buffered := collect(testutil.RawDocFromReader(strings.NewReader(string(data)), path, model.LocaleEnglish))
	inPlace := collect(testutil.RawDocFromFile(t, path, model.LocaleEnglish))
	assert.Equal(t, buffered, inPlace)
}

// TestPackageReadBoundedMemory is the regression guard for #1699: reading a
// package used to cost roughly 3.5x the file's size in peak heap, all of it
// copies of embedded media — the CLI's buffer, the reader's buffer, and an
// eager blob per asset. None of that is required by the format, so peak heap
// must not scale with how much media the package carries.
func TestPackageReadBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("memory test skipped in -short")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(20))

	dir := t.TempDir()

	// liveHeap forces a GC so the sample reflects the retained working set
	// rather than transient per-entry garbage the read legitimately produces.
	liveHeap := func() uint64 {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapAlloc
	}

	peakReading := func(mediaBytes int) (peak, size uint64) {
		path, _ := writeMediaDocx(t, dir, mediaBytes)
		info, err := os.Stat(path)
		require.NoError(t, err)

		r := NewReader()
		r.cfg.ExtractMedia = true
		require.NoError(t, r.Open(t.Context(), testutil.RawDocFromFile(t, path, model.LocaleEnglish)))
		defer r.Close()

		base := liveHeap()
		// Every part is sampled: the two documents differ only in media-entry
		// count, so equal density is automatic and a single retained asset is
		// large enough to show up in any sample after it.
		var parts []*model.Part
		for res := range r.Read(t.Context()) {
			require.NoError(t, res.Error)
			if res.Part == nil {
				continue
			}
			// Hold the stream the way a non-streaming run does, so a reader that
			// materialized media would keep it live rather than shed it.
			parts = append(parts, res.Part)
			if h := liveHeap(); h > base && h-base > peak {
				peak = h - base
			}
		}
		runtime.KeepAlive(parts)
		return peak, uint64(info.Size())
	}

	ps, smallSize := peakReading(4 << 20)
	pl, largeSize := peakReading(64 << 20) // 16x the media

	testutil.AssertBoundedMemory(t, "openxml package reader", ps, smallSize, pl, largeSize)
}

// TestDeferredMediaSurvivesReaderClose records the accessor's lifetime: it is
// bound to the document's bytes, not to the reader, so a consumer draining the
// part stream and closing the reader can still materialize what it kept. What
// it does not survive is the document source closing — which is why the parse
// cache and the bowrain connector materialize before letting go.
func TestDeferredMediaSurvivesReaderClose(t *testing.T) {
	path, want := writeMediaDocx(t, t.TempDir(), 2<<20)

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	info, err := f.Stat()
	require.NoError(t, err)

	r := NewReader()
	r.cfg.ExtractMedia = true
	require.NoError(t, r.Open(t.Context(), &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(f),
		ReaderAt:     f,
		Size:         info.Size(),
	}))

	var media []*model.Media
	for res := range r.Read(t.Context()) {
		require.NoError(t, res.Error)
		if res.Part != nil && res.Part.Type == model.PartMedia {
			media = append(media, res.Part.Resource.(*model.Media))
		}
	}
	require.NoError(t, r.Close())
	require.NotEmpty(t, media)

	got, err := media[0].Bytes()
	require.NoError(t, err)
	assert.Equal(t, want[media[0].Properties["zipPath"]], got)
}
