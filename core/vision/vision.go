// Package vision is the framework-side seam for document vision: OCR, and (in
// later phases) ML layout and table-structure analysis over page images. It
// defines the Engine interface and a name-keyed engine registry, mirroring
// core/segment: the heavy ONNX models live in an out-of-process plugin
// (kapi-vision) that a host registers as an engine, so the framework stays
// pure-Go and the capability is simply absent when the plugin is not installed.
//
// Phase 1 exposes OCR. Layout and Table methods are added in later phases; the
// interface is intentionally small so backends (the Go+ONNX plugin now, an
// optional docling sidecar later) implement only what they support.
package vision

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/neokapi/neokapi/core/model"
)

// OCRLine is one recognized line of text with its position on the page image in
// top-left pixel coordinates and the model's confidence in [0,1].
type OCRLine struct {
	Text       string
	BBox       model.Rect
	Confidence float64
}

// OCRResult is the recognized text of one page image plus the image's pixel
// dimensions (so callers can normalize or scale boxes).
type OCRResult struct {
	Lines  []OCRLine
	Width  int
	Height int
}

// OCROptions tunes recognition. All fields are advisory.
type OCROptions struct {
	// Lang is an advisory language hint (e.g. "en", "ch"); empty lets the engine
	// use its default model.
	Lang string
}

// Engine runs vision models over page images. Implementations are typically
// backed by the out-of-process kapi-vision plugin and load models lazily. An
// Engine is used sequentially by one caller; callers Close it when done.
//
// OCR takes a filesystem PATH, not bytes, by design: the host (kapi) must never
// load a large image into memory. The plugin opens and decodes the file itself,
// so the image bytes live only in the plugin process.
type Engine interface {
	// OCR recognizes text lines in the image file at imagePath (PNG/JPEG). The
	// path must be readable by the engine's process (the local filesystem).
	OCR(ctx context.Context, imagePath string, opts OCROptions) (*OCRResult, error)
	// Close releases the engine (e.g. terminates the plugin subprocess).
	Close() error
}

// Factory opens an Engine, performing whatever discovery/spawn the backend needs
// (e.g. locating and launching the kapi-vision plugin).
type Factory func() (Engine, error)

// ErrNoEngine is returned by Open when no vision engine is registered — the
// kapi-vision plugin is not installed, or no host wired one up.
var ErrNoEngine = errors.New("vision: no engine registered (install the kapi-vision plugin)")

var (
	mu          sync.RWMutex
	factories   = map[string]Factory{}
	defaultName string
)

// RegisterEngine registers a named engine factory. The first engine registered
// becomes the default. Registering a duplicate name overwrites it. A host
// (e.g. the kapi CLI) registers the "vision" engine that discovers and drives
// the plugin; framework-only builds register none, so vision is absent.
func RegisterEngine(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if f == nil {
		return
	}
	factories[name] = f
	if defaultName == "" {
		defaultName = name
	}
}

// Available reports whether the named engine ("" = default) is registered.
func Available(name string) bool {
	mu.RLock()
	defer mu.RUnlock()
	if name == "" {
		name = defaultName
	}
	if name == "" {
		return false
	}
	_, ok := factories[name]
	return ok
}

// Open opens the named engine ("" = default), returning ErrNoEngine if none is
// registered. The caller owns the returned Engine and must Close it.
func Open(name string) (Engine, error) {
	mu.RLock()
	if name == "" {
		name = defaultName
	}
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		if name == "" {
			return nil, ErrNoEngine
		}
		return nil, fmt.Errorf("vision: engine %q not registered: %w", name, ErrNoEngine)
	}
	return f()
}

// ResetForTest clears the registry. It exists for tests that register a fake
// engine and must not leak it across cases.
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	factories = map[string]Factory{}
	defaultName = ""
}

// OCRResultFromBlocks builds an OCRResult from blocks that already carry text and
// top-left geometry — e.g. a PDF page's positioned text runs. It is the inverse
// of BlocksFromOCR, letting the layout pipeline (Layout + PartsFromLayout)
// structure ANY source of positioned text, not just OCR: a layout model runs over
// the rendered page raster while the text comes from the document itself (more
// accurate than re-OCRing a vector PDF).
//
// width/height are the rendered raster's pixel dimensions — and the block
// geometry must live in that same pixel space (render the page at 72 DPI so PDF
// points map 1:1 to pixels, with a top-left origin). Blocks lacking text or
// geometry are skipped.
func OCRResultFromBlocks(blocks []*model.Block, width, height int) *OCRResult {
	res := &OCRResult{Width: width, Height: height}
	for _, b := range blocks {
		if b == nil {
			continue
		}
		text := b.SourceText()
		if text == "" {
			continue
		}
		g, ok := b.Geometry()
		if !ok || g == nil {
			continue
		}
		conf := 1.0
		if o, ok := b.SourceOrigin(); ok && o.Confidence > 0 {
			conf = o.Confidence
		}
		res.Lines = append(res.Lines, OCRLine{Text: text, BBox: g.BBox, Confidence: conf})
	}
	return res
}

// setOCRProvenance records on a block that its source text was produced by OCR,
// carrying the line's confidence — the gate a media-refine tier reads (AD-030).
// The provenance is uniform across lines; a 0 confidence still marks Kind ocr.
func setOCRProvenance(b *model.Block, ln OCRLine) {
	b.SetSourceOrigin(&model.Origin{Kind: model.OriginOCR, Confidence: ln.Confidence})
}

// BlocksFromOCR converts recognized lines into positioned content Blocks: one
// Block per line, carrying a top-left GeometryAnnotation, with IDs allocated
// from counter (advanced in place) so they stay unique across pages. Empty lines
// are skipped. The blocks can be fed to core/structure.Analyze for tier-2
// structure exactly like the PDF geometry path.
func BlocksFromOCR(res *OCRResult, page int, counter *int) []*model.Block {
	if res == nil {
		return nil
	}
	var out []*model.Block
	for _, ln := range res.Lines {
		if ln.Text == "" {
			continue
		}
		*counter++
		b := model.NewBlock(fmt.Sprintf("tu%d", *counter), ln.Text)
		if ln.BBox.W > 0 || ln.BBox.H > 0 {
			b.SetGeometry(&model.GeometryAnnotation{Page: page, BBox: ln.BBox, Origin: "top-left"})
		}
		setOCRProvenance(b, ln)
		out = append(out, b)
	}
	return out
}
