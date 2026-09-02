//go:build js && wasm

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall/js"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/terms"
)

// labInspectAnnotated reads a file through the kapi format reader exactly like
// labInspect, then runs a small pipeline of read-only annotators over the parsed
// blocks so they gain stand-off overlays — terminology, voice vocabulary and
// rule-based checks — before serializing the content tree. Where plain
// labInspect only parses, this surfaces the engine's interpretations so the
// docs "Anatomy" explorer can highlight vocabulary terms and check findings on
// a rendered document.
//
// The annotators are deterministic and offline: term overlays come from the
// seeded in-memory terms (LookupAll over the source text), brand overlays
// from profile.MatchVocabulary against the seeded voice profile (wasm_backends.go),
// and check overlays from the shared source-only shape rules (double spaces,
// doubled words — check.HygieneOverlay).
// Each is a source-anchored overlay (Variant nil) carrying its matched span text
// and type-specific props, picked up by the existing OverlayView serializer.
//
// It returns the same {ok, format, json, bytes} shape as labInspect (a Promise,
// since os.ReadFile and the reader are async under js/wasm), but with the
// blocks' `overlays` populated. An optional second argument is a JSON options
// object: {term:bool, brand:bool, qa:bool} to toggle individual annotators (all
// default to true).
func labInspectAnnotated(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errorResult("labInspectAnnotated expects a path")
	}
	path := args[0].String()
	opts := annotateOptions{Term: true, Brand: true, QA: true}
	if len(args) >= 2 && args[1].Type() == js.TypeString {
		var parsed annotateOptions
		if err := json.Unmarshal([]byte(args[1].String()), &parsed); err == nil {
			opts = parsed
		}
	}
	executor := js.FuncOf(func(_ js.Value, p []js.Value) any {
		resolve := p[0]
		go func() { resolve.Invoke(doInspectAnnotated(path, opts)) }()
		return js.Undefined()
	})
	return js.Global().Get("Promise").New(executor)
}

// annotateOptions toggles which annotators run. The zero value disables all, so
// callers pass an explicit object; labInspectAnnotated defaults all to true when
// no options argument is given.
type annotateOptions struct {
	Term  bool `json:"term"`
	Brand bool `json:"brand"`
	QA    bool `json:"qa"`
	// Segment, when set, runs the segmentation engine over each block and writes
	// the primary sentence segmentation overlay, so the preview shows sentence
	// boundaries. SegmentEngine names the engine ("" = default srx; "uax29"
	// bridges to ICU4X in the browser). SegmentLocale (BCP-47, "" = "en") is the
	// content language; locale-sensitive engines (Intl.Segmenter, ICU) tailor
	// their rules to it.
	Segment       bool   `json:"segment"`
	SegmentEngine string `json:"segmentEngine"`
	SegmentLocale string `json:"segmentLocale"`
}

func doInspectAnnotated(path string, opts annotateOptions) (result any) {
	defer func() {
		if r := recover(); r != nil {
			result = errorResult("internal error inspecting file")
		}
	}()

	data, err := os.ReadFile(path)
	if err != nil {
		return errorResult(err.Error())
	}

	// Content-aware detection so an extension shared by several formats
	// (e.g. .xlf/.xliff → XLIFF 1.2 and 2.x) resolves to the reader that
	// actually matches the bytes, not the alphabetically-first claimant.
	fmtName, err := app.FormatReg.Detect(path, registry.DetectOptions{})
	if err != nil {
		return errorResult("unsupported format for " + filepath.Base(path))
	}
	reader, err := app.FormatReg.NewReader(fmtName)
	if err != nil {
		return errorResult(err.Error())
	}

	ctx := context.Background()
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		return errorResult(err.Error())
	}

	var parts []*model.Part
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			reader.Close()
			return errorResult(res.Error.Error())
		}
		if res.Part != nil {
			parts = append(parts, res.Part)
		}
	}
	reader.Close()

	annotateParts(ctx, parts, opts)

	tree := editor.BuildContentTree(parts, string(fmtName))
	treeJSON, err := json.Marshal(tree)
	if err != nil {
		return errorResult(err.Error())
	}

	return map[string]any{
		"ok":     true,
		"format": string(fmtName),
		"json":   string(treeJSON),
		"bytes":  len(data),
	}
}

// annotateParts walks the part stream and writes source-anchored overlays onto
// every translatable Block, in place. It mirrors what a flow of read-only
// Annotate tools would produce, but emits the overlays directly so the content
// tree's `overlays` view is populated (the streaming check tools today write
// findings as annotations/properties rather than overlays).
func annotateParts(ctx context.Context, parts []*model.Part, opts annotateOptions) {
	for _, p := range parts {
		if p == nil || p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok || !b.Translatable {
			continue
		}
		source := b.SourceText()
		if strings.TrimSpace(source) == "" {
			continue
		}
		runs := b.SourceRuns()

		if opts.Term {
			if ov := termOverlay(ctx, runs, source); ov != nil {
				b.Overlays = append(b.Overlays, *ov)
			}
		}
		if opts.Brand {
			if ov := voiceOverlay(runs, source); ov != nil {
				b.Overlays = append(b.Overlays, *ov)
			}
		}
		if opts.QA {
			// The shape checks (double spaces, doubled words) come from the shared
			// check.HygieneOverlay, which judges the run-aware flattening and
			// maps its ranges back onto the runs — the same rules, and the same
			// verdicts, as the `hygiene.*` findings `kapi check` reports.
			if ov := check.HygieneOverlay(runs); ov != nil {
				b.Overlays = append(b.Overlays, *ov)
			}
		}
		if opts.Segment {
			// Write the primary sentence segmentation overlay; BuildContentTree
			// surfaces it as the block's `segments`, which the preview renders as
			// sentence boundaries. Only attach when it actually splits.
			if spans := segmentSpans(ctx, runs, opts.SegmentEngine, opts.SegmentLocale); len(spans) > 1 {
				b.SetSegmentation(nil, spans)
			}
		}
	}
}

// segmentSpans runs the named segmentation engine ("" = default srx) over the
// source runs in the given locale ("" = "en") and returns its run-anchored
// spans, or nil on any error / when no engine is registered.
func segmentSpans(ctx context.Context, runs []model.Run, engineName, locale string) []model.Span {
	// Build the lab engine for this option (pure-Go srx / raw uax29 / okapi
	// hybrid); all trim, so spans are clean sentences regardless of engine.
	eng, err := demoSegEngine(engineName)
	if err != nil {
		return nil
	}
	if locale == "" {
		locale = "en"
	}
	spans, err := eng.Segment(ctx, runs, model.LocaleID(locale))
	if err != nil {
		return nil
	}
	return spans
}

// termOverlay builds an OverlayTerm over the source runs from the seeded
// terms. Each matched term becomes a span carrying the matched
// surface form (text), its required translation and domain. Returns nil when
// the terms store is unseeded or nothing matches.
func termOverlay(ctx context.Context, runs []model.Run, source string) *model.Overlay {
	tb := app.TermsBackend
	if tb == nil {
		return nil
	}
	matches, err := tb.LookupAll(ctx, source, terms.LookupOptions{
		SourceLocale: model.LocaleID("en"),
		TargetLocale: model.LocaleID("fr"),
	})
	if err != nil || len(matches) == 0 {
		return nil
	}
	spans := make([]model.Span, 0, len(matches))
	for _, m := range matches {
		props := map[string]string{"term": m.Term.Text}
		if tgt := m.Concept.PreferredTerm(model.LocaleID("fr")); tgt != nil {
			props["target"] = tgt.Text
		}
		if m.Concept.Domain != "" {
			props["domain"] = m.Concept.Domain
		}
		spans = append(spans, model.Span{
			Range: model.RangeAnchorForBytes(runs, m.Position.Start, m.Position.End),
			Props: props,
		})
	}
	return &model.Overlay{Type: model.OverlayTerm, Spans: spans}
}

// voiceOverlay builds an OverlayQA over the source runs from the seeded voice
// profile — both halves of its deterministic gate, vocabulary
// (profile.MatchVocabulary) and prohibited style patterns
// (profile.MatchPatterns). Findings ride on the `OverlayQA` type (the model's
// fixed overlay enum has no dedicated voice type) and are tagged with
// category="voice-vocabulary" or category="voice-pattern" plus the matched term
// or rule, severity and any preferred replacement. Returns nil when nothing
// matches.
func voiceOverlay(runs []model.Run, source string) *model.Overlay {
	hits := profile.MatchVocabulary(voiceProfile, source)
	patterns := profile.MatchPatterns(voiceProfile, source)
	if len(hits) == 0 && len(patterns) == 0 {
		return nil
	}
	spans := make([]model.Span, 0, len(hits)+len(patterns))
	for _, h := range hits {
		props := map[string]string{
			"category": "voice-vocabulary",
			"severity": string(h.Severity),
			"term":     h.Term,
		}
		switch h.Kind {
		case profile.VocabCompetitor:
			props["kind"] = "competitor"
			props["message"] = fmt.Sprintf("Competitor term %q found", h.Term)
		default:
			props["kind"] = "forbidden"
			props["message"] = fmt.Sprintf("Forbidden term %q found", h.Term)
		}
		if h.Replacement != "" {
			props["replacement"] = h.Replacement
		}
		spans = append(spans, model.Span{
			Range: model.RangeAnchorForBytes(runs, h.Start, h.End),
			Props: props,
		})
	}
	for _, p := range patterns {
		message := p.Description
		if strings.TrimSpace(message) == "" {
			message = fmt.Sprintf("Prohibited pattern %q matched", p.Regex)
		}
		spans = append(spans, model.Span{
			Range: model.RangeAnchorForBytes(runs, p.Start, p.End),
			Props: map[string]string{
				"category": "voice-pattern",
				"severity": string(p.Severity),
				"kind":     "pattern",
				"pattern":  p.Regex,
				"message":  message,
			},
		})
	}
	return &model.Overlay{Type: model.OverlayQA, Spans: spans}
}
