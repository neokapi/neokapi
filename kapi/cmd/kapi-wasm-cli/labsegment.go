//go:build js && wasm

package main

import (
	"context"
	"syscall/js"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
	"github.com/neokapi/neokapi/core/segment/srx"

	// Browser UAX-29: the cgo ICU uax29 engine is absent in wasm, so bridge to
	// ICU4X (a companion wasm module loaded by the host page) which registers both
	// the "uax29" engine and the base breaker the Okapi hybrid uses, via
	// syscall/js. Lets the segmentation lab offer all three engines in the browser.
	_ "github.com/neokapi/neokapi/core/segment/icu4xjs"
)

// demoEngines caches the lab's three segmentation engines. They are stateless
// across calls (rule selection is cached per-locale inside each), so a single
// instance per option is reused — avoiding a re-parse of Okapi's 313 KB ruleset
// on every segmentation. wasm is single-threaded, so a plain map is safe.
var demoEngines = map[string]segment.Segmenter{}

// demoSegEngine builds (once) the engine for a lab option, all trimming so the
// segments are clean sentences regardless of engine:
//
//	"srx"    → reduced pure-Go ruleset (default.srx), pure rule-based, no ICU.
//	"uax29"  → raw ICU4X Unicode baseline (no SRX exceptions).
//	"hybrid" → Okapi's full ruleset over the ICU4X base (useIcu4jBreakRules).
func demoSegEngine(name string) (segment.Segmenter, error) {
	if e, ok := demoEngines[name]; ok {
		return e, nil
	}
	trim := segment.MaskOptions{TrimLeadingWS: true, TrimTrailingWS: true}
	engineName := "srx"
	cfg := segment.Config{Mask: trim}
	switch name {
	case "uax29":
		engineName = "uax29"
	case "hybrid":
		cfg.SrxRules = string(srx.OkapiRuleset())
	default: // "srx" / ""
		cfg.SrxRules = string(srx.DefaultRuleset())
	}
	eng, err := segment.NewEngine(engineName, cfg)
	if err != nil {
		return nil, err
	}
	demoEngines[name] = eng
	return eng, nil
}

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

	eng, err := demoSegEngine(engineName)
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

// labSegmentEngines lists the three runnable lab segmentation options (see
// demoSegEngine). "uax29" and "hybrid" both need the ICU4X bridge loaded; "srx"
// is pure-Go. (SaT is a native plugin, surfaced in the UI but not runnable here.)
func labSegmentEngines(_ js.Value, _ []js.Value) any {
	return []any{"srx", "uax29", "hybrid"}
}
