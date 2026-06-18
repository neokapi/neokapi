// Package image reads raster image files (PNG, JPEG) as documents: it emits the
// image as a Media part and, when a vision engine is registered (the kapi-vision
// plugin), recognizes text with OCR and emits positioned text Blocks, recovering
// tier-2 structure (headings/paragraphs/tables) from the OCR line geometry the
// same way the PDF geometry path does. With no vision engine installed the
// reader still opens the file and emits its Media (no text) rather than failing,
// so an image is always a valid, inspectable document.
//
// The reader is read-only — there is no image writer; editing tools fail cleanly
// rather than overwriting the picture with extracted text.
package image

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"path"
	"strconv"

	// Register PNG and JPEG decoders for image.DecodeConfig.
	_ "image/jpeg"
	_ "image/png"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/structure"
	"github.com/neokapi/neokapi/core/vision"
)

// Config is the (currently empty) image-format configuration.
type Config struct{}

func (c *Config) FormatName() string { return "image" }
func (c *Config) Reset()             {}
func (c *Config) Validate() error    { return nil }
func (c *Config) ApplyMap(values map[string]any) error {
	for key := range values {
		return fmt.Errorf("image: unknown parameter: %s", key)
	}
	return nil
}

// Reader implements format.DataFormatReader for raster images.
type Reader struct {
	format.BaseFormatReader
}

// NewReader constructs an image reader.
func NewReader() *Reader {
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "image",
			FormatDisplayName: "Image (OCR)",
			FormatMimeType:    "image/png",
			FormatExtensions:  []string{".png", ".jpg", ".jpeg"},
			Cfg:               &Config{},
		},
	}
}

// Signature is the detection metadata for raster images.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"image/png", "image/jpeg"},
		Extensions: []string{".png", ".jpg", ".jpeg"},
		MagicBytes: [][]byte{
			{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, // PNG
			{0xff, 0xd8, 0xff},                            // JPEG
		},
	}
}

// Open stores the document for reading.
func (r *Reader) Open(_ context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("image: nil document or reader")
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
	default:
		return "image/png"
	}
}

// Read streams the document: a root Layer, a single page Layer carrying the
// image as a Media part, then (if a vision engine is registered) OCR text Blocks
// with tier-2 structure, then the LayerEnds.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)

		data, err := io.ReadAll(r.Doc.Reader)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("image: read document: %w", err)}
			return
		}
		cfg, fmtName, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("image: decode: %w", err)}
			return
		}

		uri := r.Doc.URI
		if uri == "" {
			uri = "image"
		}
		locale := r.Doc.SourceLocale
		if locale.IsEmpty() {
			locale = model.LocaleEnglish
		}
		mime := mimeForFormat(fmtName)

		root := &model.Layer{
			ID: "doc1", Name: uri, Format: "image", Locale: locale,
			Encoding: "binary", MimeType: mime,
		}
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerStart, Resource: root}}

		pageLayer := &model.Layer{
			ID: "page1", Name: "Page 1", Format: "image", Locale: locale,
			Properties: map[string]string{"page-number": "1"},
		}
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerStart, Resource: pageLayer}}

		// The image itself, as a Media part — always emitted, OCR or not.
		ch <- model.PartResult{Part: &model.Part{Type: model.PartMedia, Resource: &model.Media{
			ID:       "img1",
			MimeType: mime,
			Data:     data,
			Filename: path.Base(uri),
			Size:     int64(len(data)),
			Properties: map[string]string{
				"width":  strconv.Itoa(cfg.Width),
				"height": strconv.Itoa(cfg.Height),
			},
		}}}

		// OCR, if a vision engine is installed. Failures are non-fatal: the image
		// Media is already emitted, so the document is still valid.
		if vision.Available("") {
			if parts := r.ocrParts(ctx, data, locale); parts != nil {
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

// ocrParts runs the registered vision engine over the image and returns the
// structured Part stream (tier-2 structure over the OCR line geometry), or nil
// on any failure (best-effort: the caller has already emitted the Media).
func (r *Reader) ocrParts(ctx context.Context, data []byte, locale model.LocaleID) []*model.Part {
	eng, err := vision.Open("")
	if err != nil {
		return nil
	}
	defer func() { _ = eng.Close() }()

	res, err := eng.OCR(ctx, data, vision.OCROptions{Lang: locale.String()})
	if err != nil || res == nil {
		return nil
	}
	counter, groupCounter := 0, 0
	blocks := vision.BlocksFromOCR(res, 1, &counter)
	if len(blocks) == 0 {
		return nil
	}
	return structure.ToParts(structure.Analyze(blocks), &groupCounter)
}
