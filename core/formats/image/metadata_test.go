package image

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/docmeta"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/vision"
)

func pngChunk(ctype string, data []byte) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(data)))
	buf.WriteString(ctype)
	buf.Write(data)
	h := crc32.NewIEEE()
	h.Write([]byte(ctype))
	h.Write(data)
	_ = binary.Write(&buf, binary.BigEndian, h.Sum32())
	return buf.Bytes()
}

// insertAfterIHDR splices chunks in right after the PNG IHDR (sig 8 + IHDR 25),
// before IDAT — where text metadata conventionally lives.
func insertAfterIHDR(png []byte, chunks ...[]byte) []byte {
	const ihdrEnd = 8 + 4 + 4 + 13 + 4
	out := append([]byte(nil), png[:ihdrEnd]...)
	for _, c := range chunks {
		out = append(out, c...)
	}
	return append(out, png[ihdrEnd:]...)
}

func tEXt(keyword, text string) []byte { return pngChunk("tEXt", []byte(keyword+"\x00"+text)) }

func iTXtXMP(xml string) []byte {
	data := []byte("XML:com.adobe.xmp\x00") // keyword + null
	data = append(data, 0, 0)               // compression flag (uncompressed) + method
	data = append(data, 0)                  // empty language tag + null
	data = append(data, 0)                  // empty translated keyword + null
	return pngChunk("iTXt", append(data, []byte(xml)...))
}

// collect reads the image and returns the document-layer properties plus the
// metadata blocks (those on the metadata plane).
func collectMeta(t *testing.T, in string, src []byte) (map[string]string, []*model.Block) {
	t.Helper()
	vision.ResetForTest()
	defer vision.ResetForTest()
	var props map[string]string
	var meta []*model.Block
	for _, p := range readParts(t, in, src) {
		switch res := p.Resource.(type) {
		case *model.Layer:
			if res.ID == "doc1" && props == nil {
				props = res.Properties
			}
		case *model.Block:
			if st, ok := res.Structure(); ok && st.Layer == model.LayerMetadata {
				meta = append(meta, res)
			}
		}
	}
	return props, meta
}

func TestPNGTextMetadata(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "shot.png")
	src := insertAfterIHDR(makePNG(t, 32, 24),
		tEXt("Title", "Login screen"),
		tEXt("Description", "The sign-in form"),
		tEXt("Author", "Acme Design"),
		tEXt("Software", "Figma"),
	)
	if err := os.WriteFile(in, src, 0o644); err != nil {
		t.Fatal(err)
	}

	props, meta := collectMeta(t, in, src)

	// Title + Description are translatable metadata blocks.
	got := map[string]string{}
	for _, b := range meta {
		got[b.Properties[docmeta.MetadataFieldProperty]] = b.SourceText()
		if !b.Translatable {
			t.Errorf("metadata block %s should be translatable", b.ID)
		}
	}
	if got["png:title"] != "Login screen" || got["png:description"] != "The sign-in form" {
		t.Errorf("translatable metadata = %v", got)
	}
	// Author/Software are non-translatable layer properties.
	if props["png:author"] != "Acme Design" {
		t.Errorf("png:author property = %q", props["png:author"])
	}
	if props["png:software"] != "Figma" {
		t.Errorf("png:software property = %q", props["png:software"])
	}
	if _, ok := props["png:title"]; ok {
		t.Error("translatable title must be a block, not a property")
	}
}

func TestPNGXMPMetadata(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "xmp.png")
	xmp := `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:dc="http://purl.org/dc/elements/1.1/">` +
		`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">Pricing chart</rdf:li></rdf:Alt></dc:title>` +
		`<dc:description><rdf:Alt><rdf:li>Tiers &amp; prices</rdf:li></rdf:Alt></dc:description>` +
		`<dc:creator><rdf:Seq><rdf:li>Acme</rdf:li></rdf:Seq></dc:creator>` +
		`</rdf:RDF></x:xmpmeta>`
	src := insertAfterIHDR(makePNG(t, 16, 16), iTXtXMP(xmp))
	if err := os.WriteFile(in, src, 0o644); err != nil {
		t.Fatal(err)
	}

	props, meta := collectMeta(t, in, src)

	got := map[string]string{}
	for _, b := range meta {
		got[b.Properties[docmeta.MetadataFieldProperty]] = b.SourceText()
	}
	if got["xmp:dc:title"] != "Pricing chart" {
		t.Errorf("xmp:dc:title = %q", got["xmp:dc:title"])
	}
	if got["xmp:dc:description"] != "Tiers & prices" { // entity unescaped
		t.Errorf("xmp:dc:description = %q", got["xmp:dc:description"])
	}
	if props["xmp:dc:creator"] != "Acme" {
		t.Errorf("xmp:dc:creator property = %q", props["xmp:dc:creator"])
	}
}

func TestNoMetadata(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "plain.png")
	src := makePNG(t, 16, 16)
	if err := os.WriteFile(in, src, 0o644); err != nil {
		t.Fatal(err)
	}
	_, meta := collectMeta(t, in, src)
	if len(meta) != 0 {
		t.Errorf("plain image should yield no metadata blocks, got %d", len(meta))
	}
}
