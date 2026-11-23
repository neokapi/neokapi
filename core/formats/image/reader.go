// Package image reads raster image files as documents: it emits the image as a
// Media part. When a vision engine is registered (the kapi-vision plugin), it
// also recognizes text with OCR and emits positioned text Blocks. It recovers
// tier-2 structure (headings/paragraphs/tables) from the OCR line geometry the
// same way the PDF geometry path does. With no vision engine installed the reader
// still opens the file and emits its Media (no text) rather than failing, so an
// image is always a valid, inspectable document.
//
// Accepted formats: PNG and JPEG decode natively for OCR. GIF, BMP, TIFF and
// WebP decode in-core (pure Go) and are re-encoded to PNG before OCR. HEIC/HEIF
// and AVIF have no Go decoder and are transcoded with ffmpeg (the kapi-av
// bundle) when available, degrading to a Media-only asset when it is not. See
// convert.go (normalizeForOCR) and detect.go (classify/Sniff).
//
// Alt-text / caption localization: when an "<image>.alt.txt" sidecar sits beside
// the source, its text is attached to the Media (AltText) and emitted as a
// caption Block linked to the image (RoleCaption + RelCaptionOf). That block
// translates through the normal block path, no special tool support, and the
// writer folds the localized text back into a per-locale sidecar.
//
// Embedded metadata (PNG text chunks, XMP) is mapped onto the document layer via
// core/docmeta: translatable fields (title/description/keywords) become
// metadata-plane Blocks; the rest become namespaced Layer properties. Metadata is
// read without loading the pixel data (see metadata.go).
package image

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	// Register PNG and JPEG decoders for image.DecodeConfig.
	_ "image/jpeg"
	_ "image/png"

	"github.com/neokapi/neokapi/core/docmeta"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/rootpath"
	"github.com/neokapi/neokapi/core/structure"
	"github.com/neokapi/neokapi/core/vision"
)

// The image-format configuration (Config) and its ApplyMap live in config.go,
// alongside every other format's config, so the maturity audit's L1 `config`
// signal (gateEngine: has('config')) fires on the conventional filename.

// Reader implements format.DataFormatReader for raster images.
type Reader struct {
	format.BaseFormatReader
}

// NewReader constructs an image reader.
func NewReader() *Reader {
	return &Reader{
		FormatName:        "image",
		FormatDisplayName: "Image",
		FormatMimeType:    "image/png",
		FormatExtensions: []string{
			".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff",
			".webp", ".heic", ".heif", ".avif",
		},
		Cfg: defaultConfig(),
	}
}

// Signature is the detection metadata for raster images. PNG/JPEG/GIF/BMP/TIFF
// are matched by unambiguous magic-byte prefixes; WebP and the ISOBMFF still
// images HEIC/HEIF and AVIF need the Sniff hook (their markers sit past offset
// 0 and share a container prefix with audio/video).
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes: []string{
			"image/png", "image/jpeg", "image/gif", "image/bmp", "image/tiff",
			"image/webp", "image/heic", "image/heif", "image/avif",
		},
		Extensions: []string{
			".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff",
			".webp", ".heic", ".heif", ".avif",
		},
		MagicBytes: [][]byte{
			{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, // PNG
			{0xff, 0xd8, 0xff},                 // JPEG
			[]byte("GIF87a"), []byte("GIF89a"), // GIF
			[]byte("BM"),             // BMP
			{0x49, 0x49, 0x2a, 0x00}, // TIFF little-endian
			{0x4d, 0x4d, 0x00, 0x2a}, // TIFF big-endian
		},
		Sniff:  Sniff,
		Binary: true,
	}
}

// Open stores the document for reading.
func (r *Reader) Open(_ context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("image: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Close is a no-op (no retained resources).
func (r *Reader) Close() error { return nil }

// mimeForFormat maps image.DecodeConfig's format name to a MIME type.
func mimeForFormat(f string) string {
	switch f {
	case "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "bmp":
		return "image/bmp"
	case "tiff":
		return "image/tiff"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

// Read streams the document: a root Layer, a single page Layer carrying the
// image as a Media part (by reference, never inline bytes), then (if a vision
// engine is registered) OCR text Blocks with tier-2 structure, then the
// LayerEnds.
//
// The image bytes are never loaded whole into the kapi process. The reader
// works from the descriptor the caller opened (doc.ReaderAt), or from a
// bounded spill of doc.Reader to a temp file, and never reopens a file by
// name. The Media part references the image by URI, and the OCR engine (the
// plugin) reads a private copy (see imageSource.privateCopy).
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	return format.StreamParts(ctx, func(ctx context.Context, ch chan<- model.PartResult) error {
		src, err := openImageSource(r.Doc)
		if err != nil {
			return err
		}
		defer src.close()

		// DecodeConfig reads only the image header, not the whole file. A decode
		// failure is fatal only when the bytes are not a recognized raster at all;
		// formats with no in-core Go decoder (HEIC/AVIF) are still valid Media
		// assets, so we fall back to content-classified MIME and unknown
		// dimensions rather than failing the read.
		cfg, fmtName, derr := image.DecodeConfig(src.section())
		mime := mimeForFormat(fmtName)
		if derr != nil {
			kind, _ := classifyReader(src.section())
			if kind == kindUnknown {
				return fmt.Errorf("image: decode: %w", derr)
			}
			mime = mimeForKind(kind)
		}

		uri := r.Doc.URI
		if uri == "" {
			uri, _ = format.SourceFile(r.Doc)
		}
		locale := r.Doc.SourceLocale
		if locale.IsEmpty() {
			locale = model.LocaleEnglish
		}

		root := &model.Layer{
			ID: "doc1", Name: uri, Format: "image", Locale: locale,
			Encoding: "binary", MimeType: mime,
		}
		// Image metadata (PNG text chunks, XMP): translatable fields become
		// metadata-plane Blocks on the document layer; the rest are namespaced
		// Properties. Populate the layer before emitting its LayerStart.
		metaBlocks := docmeta.Apply(root, readImageMetadata(src.section(), mime), "meta")
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerStart, Resource: root}}
		for _, b := range metaBlocks {
			ch <- model.PartResult{Part: &model.Part{Type: model.PartBlock, Resource: b}}
		}

		pageLayer := &model.Layer{
			ID: "page1", Name: "Page 1", Format: "image", Locale: locale,
			Properties: map[string]string{"page-number": "1"},
		}
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerStart, Resource: pageLayer}}

		// An "<image>.alt.txt" sidecar (beside the original file) carries the
		// image's alt text / caption, the localizable accessible text.
		altText := ""
		if source, ok := format.SourceFile(r.Doc); ok {
			altText = readAltSidecar(source)
		}

		// The image as a Media part, by URI reference, never inline bytes, so the
		// page's binary never travels through the kapi Part stream.
		ch <- model.PartResult{Part: &model.Part{Type: model.PartMedia, Resource: &model.Media{
			ID:       "img1",
			MimeType: mime,
			URI:      uri,
			Filename: path.Base(uri),
			AltText:  altText,
			Properties: map[string]string{
				"width":  strconv.Itoa(cfg.Width),
				"height": strconv.Itoa(cfg.Height),
			},
		}}}

		// The alt text as a translatable caption Block linked to the image. It
		// flows through the normal Block translation path (TM, AI, voice profile,
		// sessions) like any other content; the writer folds the localized target
		// back into a per-locale sidecar.
		if altText != "" {
			capBlock := model.NewBlock("alt1", altText)
			capBlock.SetSemanticRole(model.RoleCaption, 0)
			capBlock.AddRelation(model.RelCaptionOf, "img1")
			ch <- model.PartResult{Part: &model.Part{Type: model.PartBlock, Resource: capBlock}}
		}

		// OCR + structure, when enabled and a vision engine is installed. Failures
		// are non-fatal: the image Media is already emitted, so the document is
		// still valid. With OCR off, the image is a Media asset only, the
		// whole-image-localization mode.
		fcfg := defaultConfig()
		if c, ok := r.Cfg.(*Config); ok && c != nil {
			fcfg = c
		}
		if fcfg.OCR && vision.Available("") {
			if parts := r.ocrParts(ctx, src, locale, fcfg.Layout); parts != nil {
				for _, p := range parts {
					ch <- model.PartResult{Part: p}
				}
			}
		}

		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerEnd, Resource: pageLayer}}
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerEnd, Resource: root}}
		return nil
	})
}

// imageSource is the image the reader works from: a random-access view over
// the bytes the caller opened, or over a temp file this process spilled
// doc.Reader into when the caller gave no such view.
type imageSource struct {
	ra    io.ReaderAt
	size  int64
	spill *os.File // the temp file holding the bytes, when the reader spilled them
}

// openImageSource returns the document's bytes without holding the whole
// image in memory. It reads through doc.ReaderAt when the caller opened one.
// It never opens a file by the document's URI or by the descriptor's name,
// because that name can be replaced after the caller opened it. Otherwise it
// streams doc.Reader to a temp file with a bounded buffer; close removes it.
func openImageSource(doc *model.RawDocument) (*imageSource, error) {
	if doc.ReaderAt != nil {
		if sec, ok := format.ReopenSource(doc); ok {
			if sr, ok := sec.(*io.SectionReader); ok {
				return &imageSource{ra: sr, size: sr.Size()}, nil
			}
		}
	}
	if doc.Reader == nil {
		return nil, errors.New("image: no readable source")
	}
	tmp, err := os.CreateTemp("", "kapi-image-*")
	if err != nil {
		return nil, fmt.Errorf("image: temp: %w", err)
	}
	// io.Copy streams in ~32 KiB chunks, the full image is never buffered.
	n, err := io.Copy(tmp, doc.Reader)
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, fmt.Errorf("image: spill: %w", err)
	}
	return &imageSource{ra: tmp, size: n, spill: tmp}, nil
}

// section returns a fresh reader over the whole image.
func (s *imageSource) section() *io.SectionReader {
	return io.NewSectionReader(s.ra, 0, s.size)
}

// close removes the spill file, when there is one.
func (s *imageSource) close() {
	if s.spill != nil {
		_ = s.spill.Close()
		_ = os.Remove(s.spill.Name())
	}
}

// privateCopy returns the name of a file holding the image bytes, for a
// program that reads a path (the OCR engine, ffmpeg). The program gets a temp
// file this process created and wrote from the descriptor, never the source's
// own name. That name lies in the project, and a link put in its place after
// the caller opened it would lead the program to any file. The spill file is
// such a copy already. cleanup removes a file privateCopy wrote.
func (s *imageSource) privateCopy() (string, func(), error) {
	noop := func() {}
	if s.spill != nil {
		return s.spill.Name(), noop, nil
	}
	tmp, err := os.CreateTemp("", "kapi-image-copy-*")
	if err != nil {
		return "", noop, fmt.Errorf("image: temp: %w", err)
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	_, err = io.Copy(tmp, s.section())
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		cleanup()
		return "", noop, fmt.Errorf("image: copy: %w", err)
	}
	return name, cleanup, nil
}

// altSidecarPath returns the convention path for an image's alt-text/caption
// sidecar: the image path with ".alt.txt" appended (hero.png → hero.png.alt.txt).
// The source sidecar holds the source alt text; localized output is written to
// the same-named sidecar beside the translated image.
func altSidecarPath(imagePath string) string { return imagePath + ".alt.txt" }

// maxAltSidecarBytes bounds how much of an alt-text sidecar is read. Alt text
// is a sentence or two.
const maxAltSidecarBytes = 64 << 10

// readAltSidecar returns the trimmed contents of the image's alt-text sidecar, or
// "" when the source has no local file path or no sidecar exists.
//
// The sidecar is read only when it is a regular file that resolves inside the
// image's own directory: a symbolic link that leads out of it, a device or a
// FIFO is ignored, as a missing sidecar is.
func readAltSidecar(uri string) string {
	if uri == "" {
		return ""
	}
	f, err := rootpath.OpenRegular(filepath.Dir(uri), altSidecarPath(uri))
	if err != nil {
		return ""
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxAltSidecarBytes))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// ocrParts runs the registered vision engine over the image and returns the
// structured Part stream (tier-2 structure over the OCR line geometry), or
// nil on any failure (best-effort: the Media is already emitted).
func (r *Reader) ocrParts(ctx context.Context, src *imageSource, locale model.LocaleID, useLayout bool) []*model.Part {
	// The vision engine decodes PNG/JPEG only; normalize other rasters
	// (GIF/BMP/TIFF/WebP in-core, HEIC/AVIF via ffmpeg) to a temp PNG first. A
	// normalization failure means OCR is skipped, the Media is already emitted.
	ocrPath, cleanup, err := normalizeForOCR(ctx, src)
	if err != nil {
		return nil
	}
	defer cleanup()

	eng, err := vision.Open("")
	if err != nil {
		return nil
	}
	defer func() { _ = eng.Close() }()

	res, err := eng.OCR(ctx, ocrPath, vision.OCROptions{Lang: locale.String()})
	if err != nil || res == nil {
		return nil
	}
	counter, groupCounter := 0, 0

	// Tier-3: if layout is enabled and the engine supports it, assign the OCR
	// lines to layout regions (authoritative roles + reading order). Fall back to
	// the geometric tier-2 (structure.Analyze) when layout is off, unavailable,
	// or yields nothing.
	if le, ok := eng.(vision.LayoutEngine); ok && useLayout {
		if regions, lerr := le.Layout(ctx, ocrPath, vision.LayoutOptions{Lang: locale.String()}); lerr == nil && len(regions) > 0 {
			regions = vision.SortReadingOrder(regions)
			if parts := vision.PartsFromLayout(regions, res, &counter, &groupCounter); len(parts) > 0 {
				return parts
			}
		}
	}

	blocks := vision.BlocksFromOCR(res, 1, &counter)
	if len(blocks) == 0 {
		return nil
	}
	return structure.ToParts(structure.Analyze(blocks), &groupCounter)
}
