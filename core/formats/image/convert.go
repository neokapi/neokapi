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
	// reader.go). HEIC/AVIF have no Go decoder, so they transcode via ffmpeg.
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

// normalizeForOCR returns the path of a PNG/JPEG the vision engine can decode:
// a private copy of src (see imageSource.privateCopy) when the engine reads
// it natively, or a temp PNG transcoded from it otherwise. The returned
// cleanup removes any temp file it created. A non-nil error means OCR should
// be skipped for this image; the document is still valid (the Media is
// emitted regardless).
func normalizeForOCR(ctx context.Context, src *imageSource) (string, func(), error) {
	noop := func() {}
	kind, err := classifyReader(src.section())
	if err != nil {
		return "", noop, err
	}
	switch kind {
	case kindGIF, kindBMP, kindTIFF, kindWebP:
		return transcodeGoToPNG(src)
	case kindHEIF, kindAVIF:
		return transcodeFFmpegToPNG(ctx, src)
	default:
		// PNG and JPEG the engine decodes natively. For an unrecognized header
		// the engine gets the bytes as they are and may try.
		return src.privateCopy()
	}
}

// transcodeGoToPNG decodes a pure-Go-decodable raster (GIF/BMP/TIFF/WebP) and
// re-encodes it as a temp PNG for OCR.
func transcodeGoToPNG(src *imageSource) (string, func(), error) {
	noop := func() {}
	img, _, err := image.Decode(src.section())
	if err != nil {
		return "", noop, fmt.Errorf("image: decode: %w", err)
	}
	return encodePNGTemp(img)
}

// transcodeFFmpegToPNG converts a format the in-core decoders can't read
// (HEIC/AVIF) to a temp PNG via ffmpeg (the kapi-av bundle or PATH). ffmpeg
// reads a private copy of the source and writes a temp file this process
// created. When ffmpeg is unavailable it returns errUnsupportedOCR so the
// caller degrades to Media-only rather than failing.
func transcodeFFmpegToPNG(ctx context.Context, src *imageSource) (string, func(), error) {
	noop := func() {}
	if !ffmpegAvailable() {
		return "", noop, errUnsupportedOCR
	}
	in, inCleanup, err := src.privateCopy()
	if err != nil {
		return "", noop, err
	}
	defer inCleanup()
	//pathguard: creates a new file in the OS temp dir
	tmp, err := os.CreateTemp("", "kapi-image-ocr-*.png")
	if err != nil {
		return "", noop, err
	}
	name := tmp.Name()
	_ = tmp.Close()
	//pathguard: removes the temp file it created with os.CreateTemp
	cleanup := func() { _ = os.Remove(name) }
	if err := ffmpegConvert(ctx, in, name); err != nil {
		cleanup()
		return "", noop, err
	}
	return name, cleanup, nil
}

// encodePNGTemp writes img to a temp PNG and returns its path plus a cleanup.
func encodePNGTemp(img image.Image) (string, func(), error) {
	noop := func() {}
	//pathguard: creates a new file in the OS temp dir
	tmp, err := os.CreateTemp("", "kapi-image-ocr-*.png")
	if err != nil {
		return "", noop, err
	}
	name := tmp.Name()
	//pathguard: removes the temp file it created with os.CreateTemp
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
