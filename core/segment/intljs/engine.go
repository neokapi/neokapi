//go:build js

// Package intljs provides a browser (GOOS=js) segmentation engine that bridges
// to the platform's built-in Intl.Segmenter. It registers under the name "intl"
// so the same `engine: intl` selection works in the WASM CLI, the flow editor,
// and the segmentation lab — a zero-download Unicode sentence baseline that is
// always available in a modern browser (no companion wasm, unlike uax29/ICU4X).
//
// Host contract: the page must define a global function
//
//	globalThis.kapiIntlSentenceBreaks(text: string, locale: string) => number[]
//
// returning the INTERIOR sentence-break offsets as Unicode code-point (rune)
// indices into text — excluding 0 and text length, ascending. The JS glue wraps
// Intl.Segmenter({granularity:"sentence"}) and converts its UTF-16 indices to
// code-point indices (Go spans are rune-indexed). When the function is absent the
// engine returns a clear error rather than a wrong result.
//
// Intl.Segmenter is a browser/JS API with no native Go equivalent, so this engine
// is WASM-only (//go:build js); on native targets the "intl" engine is simply not
// registered (segment.HasEngine("intl") is false), the same way the cgo uax29
// engine is absent in the browser.
//
// Blank-import this package into a wasm entrypoint to make the engine available.
package intljs

import (
	"context"
	"errors"
	"syscall/js"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
)

// jsFuncName is the global the host page must define (see package doc).
const jsFuncName = "kapiIntlSentenceBreaks"

func init() {
	segment.Register(segment.EngineDescriptor{
		Name:        "intl",
		Label:       "Unicode baseline (Intl.Segmenter)",
		Description: "Browser-native Unicode sentence boundaries via Intl.Segmenter — zero-download, no companion wasm.",
		Order:       15,
		New: func(base segment.BaseConfig, _ map[string]any) (segment.Segmenter, error) {
			return &engine{lang: base.Language, mask: base.Mask}, nil
		},
	})
}

type engine struct {
	lang string
	mask segment.MaskOptions
}

// Layer reports that this engine produces primary sentence segmentation.
func (e *engine) Layer() string { return segment.LayerSentence }

// Segment flattens the runs, asks the host Intl.Segmenter bridge for the interior
// sentence breaks over the masked text, and projects them to run-anchored spans —
// mirroring the uax29/srx engines, which operate over the same flattened rune
// text and call [segment.Flattened.Spans].
func (e *engine) Segment(ctx context.Context, runs []model.Run, loc model.LocaleID) ([]model.Span, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fl := segment.Flatten(runs, e.mask)
	text := fl.Runes()
	if len(text) == 0 {
		return nil, nil
	}
	locale := e.lang
	if locale == "" {
		locale = string(loc)
	}

	fn := js.Global().Get(jsFuncName)
	if !fn.Truthy() {
		return nil, errors.New("intljs: host did not define " + jsFuncName + " — Intl.Segmenter bridge not installed")
	}
	res := fn.Invoke(string(text), locale)
	if res.Type() != js.TypeObject {
		return nil, errors.New("intljs: " + jsFuncName + " did not return an array")
	}

	n := res.Length()
	breaks := make([]int, 0, n)
	for i := 0; i < n; i++ {
		off := res.Index(i).Int()
		if off > 0 && off < len(text) {
			breaks = append(breaks, off)
		}
	}
	return fl.Spans(breaks), nil
}
