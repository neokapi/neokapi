package epub_test

import (
	"archive/zip"
	"bytes"
	"context"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/formats/epub"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// fuzzReadEPUB drives the EPUB reader over input without ever calling t.Fatal,
// so it is safe on arbitrary fuzz input.
//
// Close() is deferred on every path that got past Open. The reader spools the
// container to a temp file in Open, and Close is what removes it; without the
// defer a fuzz run would leave one file per iteration behind. A failed Open
// cleans up after itself, so the deferred Close covers the whole surface.
func fuzzReadEPUB(ctx context.Context, input []byte) (parts []*model.Part, hadErr bool) {
	reader := epub.NewReader()
	doc := testutil.RawDocFromReader(bytes.NewReader(input), "test://input.epub", model.LocaleEnglish)
	if err := reader.Open(ctx, doc); err != nil {
		return nil, true
	}
	defer reader.Close()
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			hadErr = true
			continue
		}
		if result.Part != nil {
			parts = append(parts, result.Part)
		}
	}
	return parts, hadErr
}

func fuzzEPUBTexts(parts []*model.Part) []string {
	var texts []string
	for _, b := range testutil.FilterBlocks(parts) {
		texts = append(texts, b.SourceText())
	}
	sort.Strings(texts)
	return texts
}

func fuzzEPUBZip(entries map[string][]byte) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// mimetype must come first and be stored, per the EPUB OCF spec.
	if body, ok := entries["mimetype"]; ok {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
		if err != nil {
			return nil
		}
		if _, err := w.Write(body); err != nil {
			return nil
		}
	}
	for name, body := range entries {
		if name == "mimetype" {
			continue
		}
		w, err := zw.Create(name)
		if err != nil {
			return nil
		}
		if _, err := w.Write(body); err != nil {
			return nil
		}
	}
	if err := zw.Close(); err != nil {
		return nil
	}
	return buf.Bytes()
}

const (
	fuzzEPUBContainer = `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	fuzzEPUBOPF       = `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">x</dc:identifier><dc:title>T</dc:title><dc:language>en</dc:language></metadata><manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c1"/></spine></package>`
	fuzzEPUBChapter   = `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><body><p>Hello</p></body></html>`
)

// seedDamagedEPUB adds containers that are structurally damaged but still
// PARSEABLE — the region where a container reader silently reads the wrong
// thing rather than refusing. Corrupt zip framing is already covered by
// malformed_test.go; what is not covered is a container whose parts disagree
// with each other.
func seedDamagedEPUB(f *testing.F) {
	f.Helper()
	valid := map[string][]byte{
		"mimetype":               []byte("application/epub+zip"),
		"META-INF/container.xml": []byte(fuzzEPUBContainer),
		"OEBPS/content.opf":      []byte(fuzzEPUBOPF),
		"OEBPS/c1.xhtml":         []byte(fuzzEPUBChapter),
	}
	variants := []map[string][]byte{
		valid,
		// container.xml points at an OPF that is not in the archive.
		withEntry(valid, "META-INF/container.xml", []byte(`<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/missing.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)),
		// The rootfile path climbs out of the container.
		withEntry(valid, "META-INF/container.xml", []byte(`<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="../../etc/passwd" media-type="application/oebps-package+xml"/></rootfiles></container>`)),
		// The manifest names a chapter the archive does not contain.
		withEntry(valid, "OEBPS/content.opf", []byte(`<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">x</dc:identifier></metadata><manifest><item id="c1" href="missing.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c1"/></spine></package>`)),
		// The spine references an idref with no matching manifest item.
		withEntry(valid, "OEBPS/content.opf", []byte(`<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">x</dc:identifier></metadata><manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="nope"/></spine></package>`)),
		// A manifest item href that escapes the OEBPS directory.
		withEntry(valid, "OEBPS/content.opf", []byte(`<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">x</dc:identifier></metadata><manifest><item id="c1" href="../../../escape.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c1"/></spine></package>`)),
		// A chapter whose XHTML has an unterminated element — the #1588 shape
		// pushed down into the container's child document.
		withEntry(valid, "OEBPS/c1.xhtml", []byte(`<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><body><p>one<p>two</p><p>three</p></body></html>`)),
		// mimetype declaring something else entirely.
		withEntry(valid, "mimetype", []byte("text/plain")),
		// No mimetype at all.
		withoutEntry(valid, "mimetype"),
	}
	for _, v := range variants {
		if b := fuzzEPUBZip(v); b != nil {
			f.Add(b)
		}
	}
}

func withEntry(base map[string][]byte, name string, body []byte) map[string][]byte {
	out := make(map[string][]byte, len(base))
	maps.Copy(out, base)
	out[name] = body
	return out
}

func withoutEntry(base map[string][]byte, name string) map[string][]byte {
	out := make(map[string][]byte, len(base))
	for k, v := range base {
		if k != name {
			out[k] = v
		}
	}
	return out
}

func fuzzEPUBSeedFile(f *testing.F, name string) {
	f.Helper()
	if data, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
		f.Add(data)
	}
}

// FuzzReadEpub asserts the EPUB reader never panics and always terminates on
// arbitrary input. An EPUB is a zip of XHTML, so this covers container framing,
// the OCF/OPF parse, and the per-chapter subfilter in one target.
func FuzzReadEpub(f *testing.F) {
	fuzzEPUBSeedFile(f, "minimal.epub")
	seedDamagedEPUB(f)
	f.Add([]byte("not a zip"))
	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = fuzzReadEPUB(t.Context(), data)
	})
}

// FuzzRoundTripEpub asserts read → write → read is stable in content. The EPUB
// writer reconstructs the container from the original bytes, so the same input
// is handed to SetOriginalContent — the round-trip stays fully in memory.
func FuzzRoundTripEpub(f *testing.F) {
	fuzzEPUBSeedFile(f, "minimal.epub")
	seedDamagedEPUB(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := t.Context()
		parts1, hadErr := fuzzReadEPUB(ctx, data)
		if hadErr || len(testutil.FilterBlocks(parts1)) == 0 {
			return
		}
		texts1 := fuzzEPUBTexts(parts1)

		var buf bytes.Buffer
		writer := epub.NewWriter()
		if err := writer.SetOutputWriter(&buf); err != nil {
			return
		}
		writer.SetOriginalContent(data)
		writer.SetLocale(model.LocaleEnglish)
		if err := writer.Write(ctx, testutil.PartsToChannel(parts1)); err != nil {
			return
		}
		writer.Close()

		parts2, hadErr2 := fuzzReadEPUB(ctx, buf.Bytes())
		if hadErr2 {
			t.Fatalf("re-reading written EPUB failed; round-trip is not stable\ninput len: %d\noutput len: %d", len(data), buf.Len())
		}
		texts2 := fuzzEPUBTexts(parts2)
		if len(texts1) != len(texts2) {
			t.Fatalf("round-trip changed block count: %d -> %d\npass1: %q\npass2: %q", len(texts1), len(texts2), texts1, texts2)
		}
		for i := range texts1 {
			if texts1[i] != texts2[i] {
				t.Fatalf("round-trip drift in source text\npass1: %q\npass2: %q", texts1, texts2)
			}
		}
	})
}
