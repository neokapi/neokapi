//go:build js

// Package icu4xjs provides a browser (GOOS=js) segmentation engine that bridges
// to ICU4X running as a companion WebAssembly module on the host page. Go's wasm
// target has no cgo, so the cgo ICU `uax29` engine is absent in the browser and
// the only in-binary segmenter is the pure-Go SRX engine. This package fills the
// gap by calling out — via syscall/js — to ICU4X (the Unicode Consortium's Rust
// reimplementation of ICU, shipped to JS/WASM through its official `icu` npm
// package). It registers under the name "uax29" so the same `engine: uax29`
// selection works in the browser as on native, letting the segmentation lab
// switch between SRX (pure-Go, in-binary) and UAX-29 (ICU4X, host-bridged).
//
// Host contract: the page must define a global function
//
//	globalThis.kapiICU4XSentenceBreaks(text: string, locale: string) => number[]
//
// returning the INTERIOR sentence-break offsets as Unicode code-point (rune)
// indices into text — excluding 0 and text length, ascending. The JS glue that
// wraps ICU4X's SentenceSegmenter is responsible for converting ICU4X's offsets
// to code-point indices (JS strings are UTF-16; Go spans are rune-indexed). When
// the function is absent (ICU4X not loaded) Segment returns a clear error rather
// than a wrong result, so a build without the bridge degrades visibly.
//
// Blank-import this package into a wasm entrypoint to make the engine available.
package icu4xjs

import (
	"context"
	"errors"
	"syscall/js"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
)

// jsFuncName is the global the host page must define (see package doc).
const jsFuncName = "kapiICU4XSentenceBreaks"

func init() {
	segment.RegisterEngine("uax29", newEngine)
}

type engine struct {
	lang string
	mask segment.MaskOptions
}

func newEngine(cfg segment.Config) (segment.Segmenter, error) {
	return &engine{lang: cfg.Language, mask: cfg.Mask}, nil
}

// Layer reports that this engine produces primary sentence segmentation.
func (e *engine) Layer() string { return segment.LayerSentence }

// Segment flattens the runs, asks the host ICU4X bridge for the interior
// sentence breaks over the masked text, and projects them to run-anchored spans
// — mirroring the native uax29/srx engines, which also operate over the same
// flattened rune text and call [segment.Flattened.Spans].
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
		return nil, errors.New("icu4xjs: host did not define " + jsFuncName + " — ICU4X segmenter not loaded")
	}
	res := fn.Invoke(string(text), locale)
	if res.Type() != js.TypeObject {
		return nil, errors.New("icu4xjs: " + jsFuncName + " did not return an array")
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
