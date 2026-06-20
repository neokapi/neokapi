package image

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"

	// Register the additional in-core raster decoders so image.Decode /
	// image.DecodeConfig recognize them (PNG and JPEG are registered in
	// reader.go). HEIC/AVIF have no Go decoder — they transcode via ffmpeg.
	_ "image/gif" // GIF

	_ "golang.org/x/image/bmp"  // BMP
	_ "golang.org/x/image/tiff" // TIFF
	_ "golang.org/x/image/webp" // WebP (decode-only)

	"github.com/neokapi/neokapi/core/av"
)

// errUnsupportedOCR is returned by normalizeForOCR when a recognized image
// can't be made OCR-ready (e.g. an AVIF with no ffmpeg available). The caller
// skips OCR but still emits the image as a Media asset.
var errUnsupportedOCR = errors.New("image: source cannot be normalized for OCR")

// ffmpegConvert is the ISOBMFF→PNG transcoder, a seam over av.ConvertImage so
// the routing can be tested without shelling out to ffmpeg.
var ffmpegConvert = av.ConvertImage

// ffmpegAvailable reports whether the ffmpeg transcoder is usable; a seam for
// the same reason as ffmpegConvert.
var ffmpegAvailable = av.FFmpegAvailable

// normalizeForOCR returns a path to a PNG/JPEG the vision engine can decode,
// transcoding srcPath when it is a raster the engine doesn't read natively. The
// returned cleanup removes any temp file it created (a no-op when srcPath is
// returned unchanged). A non-nil error means OCR should be skipped for this
// image; the document is still valid (the Media is emitted regardless).
func normalizeForOCR(ctx context.Context, srcPath string) (string, func(), error) {
	noop := func() {}
	kind, err := classifyFile(srcPath)
	if err != nil {
		return "", noop, err
	}
	switch kind {
	case kindPNG, kindJPEG:
		return srcPath, noop, nil // the engine decodes these natively
	case kindGIF, kindBMP, kindTIFF, kindWebP:
		return transcodeGoToPNG(srcPath)
	case kindHEIF, kindAVIF:
		return transcodeFFmpegToPNG(ctx, srcPath)
	default:
		// Unrecognized header: hand the original to the engine and let it try.
		return srcPath, noop, nil
	}
}

// transcodeGoToPNG decodes a pure-Go-decodable raster (GIF/BMP/TIFF/WebP) and
// re-encodes it as a temp PNG for OCR.
func transcodeGoToPNG(srcPath string) (string, func(), error) {
	noop := func() {}
	f, err := os.Open(srcPath)
	if err != nil {
		return "", noop, err
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		return "", noop, fmt.Errorf("image: decode %s: %w", srcPath, err)
	}
	return encodePNGTemp(img)
}

// transcodeFFmpegToPNG converts a format the in-core decoders can't read
// (HEIC/AVIF) to a temp PNG via ffmpeg (the kapi-av bundle or PATH). When
// ffmpeg is unavailable it returns errUnsupportedOCR so the caller degrades to
// Media-only rather than failing.
func transcodeFFmpegToPNG(ctx context.Context, srcPath string) (string, func(), error) {
	noop := func() {}
	if !ffmpegAvailable() {
		return "", noop, errUnsupportedOCR
	}
	tmp, err := os.CreateTemp("", "kapi-image-ocr-*.png")
	if err != nil {
		return "", noop, err
	}
	name := tmp.Name()
	_ = tmp.Close()
	cleanup := func() { _ = os.Remove(name) }
	if err := ffmpegConvert(ctx, srcPath, name); err != nil {
		cleanup()
		return "", noop, err
	}
	return name, cleanup, nil
}

// encodePNGTemp writes img to a temp PNG and returns its path plus a cleanup.
func encodePNGTemp(img image.Image) (string, func(), error) {
	noop := func() {}
	tmp, err := os.CreateTemp("", "kapi-image-ocr-*.png")
	if err != nil {
		return "", noop, err
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	if err := png.Encode(tmp, img); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", noop, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", noop, err
	}
	return name, cleanup, nil
}
