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
// or a Promise of that array, returning the INTERIOR sentence-break offsets as Unicode code-point (rune)
// indices into text — excluding 0 and text length, ascending. The JS glue that
// wraps ICU4X's SentenceSegmenter is responsible for converting ICU4X's offsets
// to code-point indices (JS strings are UTF-16; Go spans are rune-indexed). When
// the function is absent (ICU4X not loaded) Segment returns a clear error rather
// than a wrong result, so a build without the bridge degrades visibly.
//
// The Promise form serves an engine running in a Worker whose page holds
// ICU4X: the call crosses to the page and back. Waiting for it parks the
// goroutine, so a caller that answers a Promise of its own (a command, or the
// lab's asynchronous entry point) may use it, and a synchronous JS callback
// may not.
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
	segment.Register(segment.EngineDescriptor{
		Name:        "uax29",
		Label:       "Unicode baseline (UAX-29)",
		Description: "Unicode default sentence boundaries (ICU4X). A language-agnostic baseline with no exceptions.",
		Order:       10,
		New: func(base segment.BaseConfig, _ map[string]any) (segment.Segmenter, error) {
			return &engine{lang: base.Language, mask: base.Mask}, nil
		},
	})
	// Also expose ICU4X as the base breaker, so the SRX engine can run Okapi's
	// useIcu4jBreakRules hybrid (ICU base + SRX exceptions) in the browser — the
	// same composition that runs natively over cgo ICU. When ICU4X isn't loaded
	// the breaker errors and the SRX engine falls back to pure-rule.
	segment.RegisterBaseBreaker(icu4xBaseBreaker{})
}

type icu4xBaseBreaker struct{}

// BaseBreaks returns ICU4X's interior sentence-break offsets (code-point indices)
// for use as the hybrid base. Shares the host bridge with the engine.
func (icu4xBaseBreaker) BaseBreaks(ctx context.Context, text []rune, locale string) ([]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(text) == 0 {
		return nil, nil
	}
	fn := js.Global().Get(jsFuncName)
	if !fn.Truthy() {
		return nil, errors.New("icu4xjs: host did not define " + jsFuncName + " (ICU4X not loaded)")
	}
	return interiorBreaks(ctx, fn.Invoke(string(text), locale), len(text))
}

// interiorBreaks reads the bridge's answer, waiting for it when it is a
// Promise, and keeps the offsets strictly inside the text.
func interiorBreaks(ctx context.Context, res js.Value, n int) ([]int, error) {
	res, err := settle(ctx, res)
	if err != nil {
		return nil, err
	}
	if res.Type() != js.TypeObject {
		return nil, errors.New("icu4xjs: " + jsFuncName + " did not return an array")
	}
	count := res.Length()
	breaks := make([]int, 0, count)
	for i := 0; i < count; i++ {
		off := res.Index(i).Int()
		if off > 0 && off < n {
			breaks = append(breaks, off)
		}
	}
	return breaks, nil
}

// settle waits for v when it is a Promise and answers it as it is otherwise.
func settle(ctx context.Context, v js.Value) (js.Value, error) {
	if v.Type() != js.TypeObject || v.Get("then").Type() != js.TypeFunction {
		return v, nil
	}
	type answer struct {
		v   js.Value
		err error
	}
	ch := make(chan answer, 1)
	onValue := js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- answer{v: args[0]}
		return nil
	})
	onError := js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "icu4xjs: " + jsFuncName + " failed"
		if len(args) > 0 && args[0].Truthy() {
			msg += ": " + args[0].Call("toString").String()
		}
		ch <- answer{err: errors.New(msg)}
		return nil
	})
	v.Call("then", onValue, onError)
	select {
	case <-ctx.Done():
		// The callbacks stay: a late answer must not call a released func.
		return js.Undefined(), ctx.Err()
	case a := <-ch:
		onValue.Release()
		onError.Release()
		return a.v, a.err
	}
}

type engine struct {
	lang string
	mask segment.MaskOptions
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
		return nil, errors.New("icu4xjs: host did not define " + jsFuncName + " (ICU4X segmenter not loaded)")
	}
	breaks, err := interiorBreaks(ctx, fn.Invoke(string(text), locale), len(text))
	if err != nil {
		return nil, err
	}
	return fl.Spans(breaks), nil
}
