package image

import (
	"os"
	"path/filepath"
	"testing"
)

// ftypBox builds a minimal ISOBMFF header: a 32-byte ftyp box with the given
// major brand at offset 8 — enough for classify, which only reads the brand.
func ftypBox(brand string) []byte {
	b := make([]byte, 32)
	b[3] = 0x20 // box size = 32
	copy(b[4:8], "ftyp")
	copy(b[8:12], brand)
	return b
}

// riffWebP builds the 12-byte RIFF/WEBP container prefix classify keys on.
func riffWebP() []byte {
	b := make([]byte, 16)
	copy(b[0:4], "RIFF")
	copy(b[8:12], "WEBP")
	return b
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want rasterKind
	}{
		{"png", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, kindPNG},
		{"jpeg", []byte{0xff, 0xd8, 0xff, 0xe0}, kindJPEG},
		{"gif87", []byte("GIF87a...."), kindGIF},
		{"gif89", []byte("GIF89a...."), kindGIF},
		{"bmp", []byte("BM\x00\x00"), kindBMP},
		{"tiff-le", []byte{0x49, 0x49, 0x2a, 0x00}, kindTIFF},
		{"tiff-be", []byte{0x4d, 0x4d, 0x00, 0x2a}, kindTIFF},
		{"webp", riffWebP(), kindWebP},
		{"heic", ftypBox("heic"), kindHEIF},
		{"heif-mif1", ftypBox("mif1"), kindHEIF},
		{"avif", ftypBox("avif"), kindAVIF},
		{"riff-wav-not-webp", []byte("RIFF\x00\x00\x00\x00WAVEfmt "), kindUnknown},
		{"mp4-not-image", ftypBox("isom"), kindUnknown},
		{"garbage", []byte("not an image at all"), kindUnknown},
		{"empty", nil, kindUnknown},
		{"short-ftyp", []byte("\x00\x00\x00\x20ftyp"), kindUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.in); got != tc.want {
				t.Errorf("classify(%s) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

func TestSniff(t *testing.T) {
	// Sniff returns true only for the offset-based formats the magic-byte prefix
	// path can't catch; the prefix-detectable rasters return false by design.
	sniffTrue := [][]byte{riffWebP(), ftypBox("heic"), ftypBox("mif1"), ftypBox("avif")}
	for i, b := range sniffTrue {
		if !Sniff(b) {
			t.Errorf("Sniff(case %d) = false, want true", i)
		}
	}
	sniffFalse := [][]byte{
		{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'},
		{0xff, 0xd8, 0xff},
		[]byte("GIF89a"),
		[]byte("BM\x00\x00"),
		{0x49, 0x49, 0x2a, 0x00},
		ftypBox("isom"), // mp4 video, not a still image
		[]byte("RIFF\x00\x00\x00\x00WAVE"),
		[]byte("garbage"),
	}
	for i, b := range sniffFalse {
		if Sniff(b) {
			t.Errorf("Sniff(false-case %d) = true, want false", i)
		}
	}
}

func TestMimeForKind(t *testing.T) {
	cases := map[rasterKind]string{
		kindPNG:     "image/png",
		kindJPEG:    "image/jpeg",
		kindGIF:     "image/gif",
		kindBMP:     "image/bmp",
		kindTIFF:    "image/tiff",
		kindWebP:    "image/webp",
		kindHEIF:    "image/heic",
		kindAVIF:    "image/avif",
		kindUnknown: "image/png",
	}
	for k, want := range cases {
		if got := mimeForKind(k); got != want {
			t.Errorf("mimeForKind(%d) = %q, want %q", k, got, want)
		}
	}
}

func TestGoDecodable(t *testing.T) {
	for _, k := range []rasterKind{kindPNG, kindJPEG, kindGIF, kindBMP, kindTIFF, kindWebP} {
		if !k.goDecodable() {
			t.Errorf("kind %d should be goDecodable", k)
		}
	}
	for _, k := range []rasterKind{kindHEIF, kindAVIF, kindUnknown} {
		if k.goDecodable() {
			t.Errorf("kind %d should not be goDecodable", k)
		}
	}
}

func TestClassifyFile(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "x.png")
	if err := os.WriteFile(png, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	if k, err := classifyFile(png); err != nil || k != kindPNG {
		t.Errorf("classifyFile(png) = %d, %v; want kindPNG", k, err)
	}

	avif := filepath.Join(dir, "x.avif")
	if err := os.WriteFile(avif, ftypBox("avif"), 0o644); err != nil {
		t.Fatal(err)
	}
	if k, err := classifyFile(avif); err != nil || k != kindAVIF {
		t.Errorf("classifyFile(avif) = %d, %v; want kindAVIF", k, err)
	}

	if _, err := classifyFile(filepath.Join(dir, "missing")); err == nil {
		t.Error("classifyFile(missing) should error")
	}
}
