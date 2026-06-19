package image

import (
	"bytes"
	"image/gif"
	"testing"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/vision"
)

func makeGIFBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gif.Encode(&buf, solid(w, h), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeBMPBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := bmp.Encode(&buf, solid(w, h)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeTIFFBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := tiff.Encode(&buf, solid(w, h), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The in-core (pure-Go) raster set reads as a Media asset with the correct MIME
// and dimensions, just like PNG/JPEG — no vision engine required.
func TestRead_GoDecodableFormats(t *testing.T) {
	vision.ResetForTest()
	defer vision.ResetForTest()

	cases := []struct {
		name string
		data []byte
		uri  string
		mime string
	}{
		{"gif", makeGIFBytes(t, 48, 24), "pic.gif", "image/gif"},
		{"bmp", makeBMPBytes(t, 48, 24), "pic.bmp", "image/bmp"},
		{"tiff", makeTIFFBytes(t, 48, 24), "pic.tiff", "image/tiff"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parts := readAll(t, tc.data, tc.uri)
			requireBalanced(t, parts)
			m := findMedia(parts)
			if m == nil {
				t.Fatalf("%s: expected a Media part", tc.name)
			}
			if m.MimeType != tc.mime {
				t.Errorf("%s: mime = %q, want %q", tc.name, m.MimeType, tc.mime)
			}
			if m.Properties["width"] != "48" || m.Properties["height"] != "24" {
				t.Errorf("%s: dims wrong: %v", tc.name, m.Properties)
			}
			if n := countBlocks(parts); n != 0 {
				t.Errorf("%s: no-engine read should have 0 blocks, got %d", tc.name, n)
			}
		})
	}
}

// HEIC/AVIF have no in-core Go decoder, so the header decode fails — but they
// are recognized rasters, so the read must NOT fail: the image is still emitted
// as a Media asset with the right MIME (the whole-image-localization mode).
func TestRead_ISOBMFF_MediaOnly_NonFatal(t *testing.T) {
	vision.ResetForTest()
	defer vision.ResetForTest()

	cases := []struct {
		name string
		data []byte
		uri  string
		mime string
	}{
		{"heic", ftypBox("heic"), "pic.heic", "image/heic"},
		{"avif", ftypBox("avif"), "pic.avif", "image/avif"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parts := readAll(t, tc.data, tc.uri)
			requireBalanced(t, parts)
			m := findMedia(parts)
			if m == nil {
				t.Fatalf("%s: expected a Media part even without a decoder", tc.name)
			}
			if m.MimeType != tc.mime {
				t.Errorf("%s: mime = %q, want %q", tc.name, m.MimeType, tc.mime)
			}
			if n := countBlocks(parts); n != 0 {
				t.Errorf("%s: expected 0 blocks, got %d", tc.name, n)
			}
		})
	}
}

// A non-PNG/JPEG raster still produces OCR blocks: the reader normalizes it to
// PNG (here a GIF, via the in-core transcode) before handing it to the engine.
func TestRead_OCRThroughTranscode_GIF(t *testing.T) {
	vision.ResetForTest()
	defer vision.ResetForTest()
	vision.RegisterEngine("fake", func() (vision.Engine, error) { return ocrFake{}, nil })

	parts := readAll(t, makeGIFBytes(t, 200, 100), "scan.gif")
	requireBalanced(t, parts)

	if findMedia(parts) == nil {
		t.Error("Media part should still be emitted alongside OCR")
	}
	var texts []string
	for _, p := range parts {
		if p.Type == model.PartBlock {
			texts = append(texts, p.Resource.(*model.Block).SourceText())
		}
	}
	if len(texts) != 2 || texts[0] != "Heading" || texts[1] != "First body line." {
		t.Fatalf("OCR-through-transcode blocks = %v, want [Heading, First body line.]", texts)
	}
}
