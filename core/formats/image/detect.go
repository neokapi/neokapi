package image

import (
	"bytes"
	"errors"
	"io"
	"os"
)

// rasterKind classifies a raster image by its header bytes.
type rasterKind int

const (
	kindUnknown rasterKind = iota
	kindPNG
	kindJPEG
	kindGIF
	kindBMP
	kindTIFF
	kindWebP
	kindHEIF // HEIC / HEIF still image
	kindAVIF
)

// headerLen is how many leading bytes classify needs to recognize every kind
// (the ISOBMFF ftyp brand ends at byte 12).
const headerLen = 16

// classify inspects a header and returns the raster kind, or kindUnknown when
// the bytes are not a raster image this format recognizes.
func classify(b []byte) rasterKind {
	switch {
	case bytes.HasPrefix(b, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return kindPNG
	case bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}):
		return kindJPEG
	case bytes.HasPrefix(b, []byte("GIF87a")), bytes.HasPrefix(b, []byte("GIF89a")):
		return kindGIF
	case bytes.HasPrefix(b, []byte("BM")):
		return kindBMP
	case bytes.HasPrefix(b, []byte{0x49, 0x49, 0x2a, 0x00}), // little-endian TIFF
		bytes.HasPrefix(b, []byte{0x4d, 0x4d, 0x00, 0x2a}): // big-endian TIFF
		return kindTIFF
	case len(b) >= 12 && bytes.Equal(b[0:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return kindWebP
	}
	// ISOBMFF still images carry an "ftyp" box at offset 4 with the major brand
	// at offset 8. HEIC/AVIF share this container with MP4/MOV video, so they are
	// disambiguated by brand (never a plain magic-byte prefix).
	if len(b) >= 12 && bytes.Equal(b[4:8], []byte("ftyp")) {
		switch string(b[8:12]) {
		case "avif", "avis":
			return kindAVIF
		case "heic", "heix", "heim", "heis", "hevc", "hevx", "hevm", "hevs", "mif1", "msf1":
			return kindHEIF
		}
	}
	return kindUnknown
}

// goDecodable reports whether the in-core (pure-Go) image decoders can read the
// kind. PNG/JPEG decode natively for the vision engine; GIF/BMP/TIFF/WebP decode
// via the registered x/image decoders and are re-encoded to PNG for OCR.
// HEIC/AVIF have no Go decoder and need an external transcode (ffmpeg).
func (k rasterKind) goDecodable() bool {
	switch k {
	case kindPNG, kindJPEG, kindGIF, kindBMP, kindTIFF, kindWebP:
		return true
	default:
		return false
	}
}

// mimeForKind maps a raster kind to its canonical MIME type. Used as the Media
// MIME when the in-core decoder can't read the file (HEIC/AVIF), so the asset
// still carries an accurate content type.
func mimeForKind(k rasterKind) string {
	switch k {
	case kindJPEG:
		return "image/jpeg"
	case kindGIF:
		return "image/gif"
	case kindBMP:
		return "image/bmp"
	case kindTIFF:
		return "image/tiff"
	case kindWebP:
		return "image/webp"
	case kindHEIF:
		return "image/heic"
	case kindAVIF:
		return "image/avif"
	default:
		return "image/png"
	}
}

// Sniff is the content-detection hook for the raster formats the magic-byte
// prefix path can't catch on its own: WebP (RIFF container, "WEBP" marker at
// offset 8) and the ISOBMFF still images HEIC/HEIF and AVIF (brand inside the
// ftyp box). PNG/JPEG/GIF/BMP/TIFF are detected by their unambiguous magic-byte
// prefixes (see register.go), so Sniff intentionally returns false for them.
func Sniff(data []byte) bool {
	switch classify(data) {
	case kindWebP, kindHEIF, kindAVIF:
		return true
	default:
		return false
	}
}

// classifyFile reads the header of the file at path and classifies it.
func classifyFile(path string) (rasterKind, error) {
	f, err := os.Open(path)
	if err != nil {
		return kindUnknown, err
	}
	defer func() { _ = f.Close() }()
	var hdr [headerLen]byte
	n, err := io.ReadFull(f, hdr[:])
	// A file shorter than headerLen is fine — classify works on what we read.
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return kindUnknown, err
	}
	return classify(hdr[:n]), nil
}
