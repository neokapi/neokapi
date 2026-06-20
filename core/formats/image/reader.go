// Package image reads raster image files as documents: it emits the image as a
// Media part and, when a vision engine is registered (the kapi-vision plugin),
// recognizes text with OCR and emits positioned text Blocks, recovering tier-2
// structure (headings/paragraphs/tables) from the OCR line geometry the same
// way the PDF geometry path does. With no vision engine installed the reader
// still opens the file and emits its Media (no text) rather than failing, so an
// image is always a valid, inspectable document.
//
// Accepted formats: PNG and JPEG decode natively for OCR; GIF, BMP, TIFF and
// WebP decode in-core (pure Go) and are re-encoded to PNG before OCR; HEIC/HEIF
// and AVIF have no Go decoder and are transcoded with ffmpeg (the kapi-av
// bundle) when available, degrading to a Media-only asset when it is not. See
// convert.go (normalizeForOCR) and detect.go (classify/Sniff).
//
// Alt-text / caption localization: when an "<image>.alt.txt" sidecar sits beside
// the source, its text is attached to the Media (AltText) and emitted as a
// caption Block linked to the image (RoleCaption + RelCaptionOf). That block
// translates through the normal block path — no special tool support — and the
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
	"strconv"
	"strings"

	// Register PNG and JPEG decoders for image.DecodeConfig.
	_ "image/jpeg"
	_ "image/png"

	"github.com/neokapi/neokapi/core/docmeta"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/structure"
	"github.com/neokapi/neokapi/core/vision"
)

// The image-format configuration (Config) and its ApplyMap live in config.go,
// alongside every other format's config — so the maturity audit's L1 `config`
// signal (gateEngine: has('config')) fires on the conventional filename.

// Reader implements format.DataFormatReader for raster images.
type Reader struct {
	format.BaseFormatReader
}

// NewReader constructs an image reader.
func NewReader() *Reader {
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "image",
			FormatDisplayName: "Image",
			FormatMimeType:    "image/png",
			FormatExtensions: []string{
				".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff",
				".webp", ".heic", ".heif", ".avif",
			},
			Cfg: defaultConfig(),
		},
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
			{0xff, 0xd8, 0xff},                            // JPEG
			[]byte("GIF87a"), []byte("GIF89a"),            // GIF
			[]byte("BM"),             // BMP
			{0x49, 0x49, 0x2a, 0x00}, // TIFF little-endian
			{0x4d, 0x4d, 0x00, 0x2a}, // TIFF big-endian
		},
		Sniff: Sniff,
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
// image as a Media part (by reference — never inline bytes), then (if a vision
// engine is registered) OCR text Blocks with tier-2 structure, then the
// LayerEnds.
//
// The image bytes are never loaded into the kapi process: the source is resolved
// to a local file path (the original file when it is one, else a bounded
// streaming spill to a temp file), the Media part references it by URI, and the
// OCR engine (the plugin) opens and decodes that path itself.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)

		imgPath, cleanup, err := r.materialize()
		if err != nil {
			ch <- model.PartResult{Error: err}
			return
		}
		defer cleanup()

		// DecodeConfig reads only the image header, not the whole file. A decode
		// failure is fatal only when the bytes are not a recognized raster at all;
		// formats with no in-core Go decoder (HEIC/AVIF) are still valid Media
		// assets, so we fall back to content-classified MIME and unknown
		// dimensions rather than failing the read.
		cfg, fmtName, derr := decodeConfigFile(imgPath)
		mime := mimeForFormat(fmtName)
		if derr != nil {
			kind, _ := classifyFile(imgPath)
			if kind == kindUnknown {
				ch <- model.PartResult{Error: fmt.Errorf("image: decode: %w", derr)}
				return
			}
			mime = mimeForKind(kind)
		}

		uri := r.Doc.URI
		if uri == "" {
			uri = imgPath
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
		metaBlocks := docmeta.Apply(root, readImageMetadata(imgPath, mime), "meta")
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
		// image's alt text / caption — the localizable accessible text.
		altText := readAltSidecar(r.Doc.URI)

		// The image as a Media part — by URI reference, never inline bytes, so the
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
		// flows through the normal Block translation path (TM, AI, brand voice,
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
		// still valid. With OCR off, the image is a Media asset only — the
		// whole-image-localization mode.
		fcfg := defaultConfig()
		if c, ok := r.Cfg.(*Config); ok && c != nil {
			fcfg = c
		}
		if fcfg.OCR && vision.Available("") {
			if parts := r.ocrParts(ctx, imgPath, locale, fcfg.Layout); parts != nil {
				for _, p := range parts {
					ch <- model.PartResult{Part: p}
				}
			}
		}

		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerEnd, Resource: pageLayer}}
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerEnd, Resource: root}}
	}()
	return ch
}

// materialize resolves the document to a readable local file path without ever
// holding the whole image in memory. If the source URI is already a local file,
// it is used directly (no copy). Otherwise the reader streams doc.Reader to a
// temp file with a bounded buffer and returns that path; cleanup removes it.
func (r *Reader) materialize() (string, func(), error) {
	noop := func() {}
	if r.Doc.URI != "" {
		if info, err := os.Stat(r.Doc.URI); err == nil && !info.IsDir() {
			return r.Doc.URI, noop, nil
		}
	}
	if r.Doc.Reader == nil {
		return "", noop, errors.New("image: no readable source")
	}
	tmp, err := os.CreateTemp("", "kapi-image-*")
	if err != nil {
		return "", noop, fmt.Errorf("image: temp: %w", err)
	}
	// io.Copy streams in ~32 KiB chunks — the full image is never buffered.
	if _, err := io.Copy(tmp, r.Doc.Reader); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", noop, fmt.Errorf("image: spill: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", noop, err
	}
	name := tmp.Name()
	return name, func() { _ = os.Remove(name) }, nil
}

// altSidecarPath returns the convention path for an image's alt-text/caption
// sidecar: the image path with ".alt.txt" appended (hero.png → hero.png.alt.txt).
// The source sidecar holds the source alt text; localized output is written to
// the same-named sidecar beside the translated image.
func altSidecarPath(imagePath string) string { return imagePath + ".alt.txt" }

// readAltSidecar returns the trimmed contents of the image's alt-text sidecar, or
// "" when the source has no local file path or no sidecar exists. The sidecar is
// small accessible text, so reading it fully is fine (unlike the image bytes).
func readAltSidecar(uri string) string {
	if uri == "" {
		return ""
	}
	b, err := os.ReadFile(altSidecarPath(uri))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// decodeConfigFile reads just the header of the image at path.
func decodeConfigFile(path string) (image.Config, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return image.Config{}, "", err
	}
	defer func() { _ = f.Close() }()
	return image.DecodeConfig(f)
}

// ocrParts runs the registered vision engine over the image at imgPath and
// returns the structured Part stream (tier-2 structure over the OCR line
// geometry), or nil on any failure (best-effort: the Media is already emitted).
func (r *Reader) ocrParts(ctx context.Context, imgPath string, locale model.LocaleID, useLayout bool) []*model.Part {
	// The vision engine decodes PNG/JPEG only; normalize other rasters
	// (GIF/BMP/TIFF/WebP in-core, HEIC/AVIF via ffmpeg) to a temp PNG first. A
	// normalization failure means OCR is skipped — the Media is already emitted.
	ocrPath, cleanup, err := normalizeForOCR(ctx, imgPath)
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
