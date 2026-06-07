//go:build js && wasm

package main

import (
	"context"
	"syscall/js"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"

	// Browser UAX-29: the cgo ICU uax29 engine is absent in wasm, so bridge to
	// ICU4X (a companion wasm module loaded by the host page) which registers the
	// "uax29" engine via syscall/js. Lets the segmentation lab offer SRX vs
	// UAX-29 in the browser.
	_ "github.com/neokapi/neokapi/core/segment/icu4xjs"
)

// labSegment segments raw text with a named engine and locale, returning the
// resulting sentence segments — the backend of the docs "Segmentation" lab.
// Engine "" selects the default (srx). srx is pure-Go and always available in
// the browser; "uax29" is served by the ICU4X bridge and returns a clear error
// when ICU4X is not loaded on the page. Synchronous: no blocking I/O, just
// flatten + segment (the ICU4X path makes one re-entrant JS call).
func labSegment(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errorResult("labSegment expects (text, engine, locale)")
	}
	text := args[0].String()
	engine := ""
	if len(args) > 1 {
		engine = args[1].String()
	}
	locale := "en"
	if len(args) > 2 && args[2].String() != "" {
		locale = args[2].String()
	}
	return doSegment(text, engine, locale)
}

func doSegment(text, engineName, locale string) (result any) {
	defer func() {
		if r := recover(); r != nil {
			result = errorResult("internal error segmenting text")
		}
	}()

	if text == "" {
		return map[string]any{"ok": true, "engine": resolveEngineName(engineName), "segments": []any{}}
	}

	// Trim leading/trailing whitespace so SRX and UAX-29 yield identical clean
	// segments (inter-sentence whitespace stays uncovered, not attached to a side).
	eng, err := segment.NewEngine(engineName, segment.Config{
		Mask: segment.MaskOptions{TrimLeadingWS: true, TrimTrailingWS: true},
	})
	if err != nil {
		return errorResult(err.Error())
	}
	runs := []model.Run{{Text: &model.TextRun{Text: text}}}
	spans, err := eng.Segment(context.Background(), runs, model.LocaleID(locale))
	if err != nil {
		return errorResult(err.Error())
	}

	segs := make([]any, 0, len(spans))
	for i := range spans {
		segs = append(segs, map[string]any{
			"text": model.RunsText(spans[i].Range.ExtractRuns(runs)),
		})
	}
	// No interior breaks → the whole text is a single segment.
	if len(segs) == 0 {
		segs = append(segs, map[string]any{"text": text})
	}
	return map[string]any{
		"ok":       true,
		"engine":   resolveEngineName(engineName),
		"segments": segs,
	}
}

func resolveEngineName(n string) string {
	if n == "" {
		return segment.DefaultEngine
	}
	return n
}

// labSegmentEngines lists the segmentation engines registered in this build so
// the UI offers only what can run. Note "uax29" appears when the ICU4X bridge is
// linked even before ICU4X is loaded on the page; selecting it without ICU4X
// loaded returns a clear runtime error from labSegment.
func labSegmentEngines(_ js.Value, _ []js.Value) any {
	names := segment.Engines()
	out := make([]any, len(names))
	for i, n := range names {
		out[i] = n
	}
	return out
}
