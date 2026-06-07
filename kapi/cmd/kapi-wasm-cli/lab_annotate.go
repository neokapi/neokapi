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

	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
	"github.com/neokapi/neokapi/termbase"
)

// labInspectAnnotated reads a file through the kapi format reader exactly like
// labInspect, then runs a small pipeline of read-only annotators over the parsed
// blocks so they gain stand-off overlays — terminology, brand vocabulary and
// rule-based QA — before serializing the content tree. Where plain labInspect
// only parses, this surfaces the engine's interpretations so the docs "Anatomy"
// explorer can highlight vocabulary terms and QA findings on a rendered document.
//
// The annotators are deterministic and offline: term overlays come from the
// seeded in-memory termbase (LookupAll over the source text), brand overlays
// from brand.MatchVocabulary against the seeded brand profile (wasm_backends.go),
// and QA overlays from source-only heuristics (double spaces, doubled words).
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
	// bridges to ICU4X in the browser).
	Segment       bool   `json:"segment"`
	SegmentEngine string `json:"segmentEngine"`
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

	ext := strings.ToLower(filepath.Ext(path))
	fmtName, err := app.FormatReg.DetectByExtension(ext)
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
			if ov := brandOverlay(runs, source); ov != nil {
				b.Overlays = append(b.Overlays, *ov)
			}
		}
		if opts.QA {
			if ov := qaOverlay(runs, source); ov != nil {
				b.Overlays = append(b.Overlays, *ov)
			}
		}
		if opts.Segment {
			// Write the primary sentence segmentation overlay; BuildContentTree
			// surfaces it as the block's `segments`, which the preview renders as
			// sentence boundaries. Only attach when it actually splits.
			if spans := segmentSpans(ctx, runs, opts.SegmentEngine); len(spans) > 1 {
				b.SetSegmentation(nil, spans)
			}
		}
	}
}

// segmentSpans runs the named segmentation engine ("" = default srx) over the
// source runs and returns its run-anchored spans, or nil on any error / when no
// engine is registered.
func segmentSpans(ctx context.Context, runs []model.Run, engineName string) []model.Span {
	eng, err := segment.NewEngine(engineName, segment.Config{})
	if err != nil {
		return nil
	}
	spans, err := eng.Segment(ctx, runs, model.LocaleID("en"))
	if err != nil {
		return nil
	}
	return spans
}

// termOverlay builds an OverlayTerm over the source runs from the seeded
// termbase. Each matched glossary term becomes a span carrying the matched
// surface form (text), its required translation and domain. Returns nil when
// the termbase is unseeded or nothing matches.
func termOverlay(ctx context.Context, runs []model.Run, source string) *model.Overlay {
	tb := app.TBBackend
	if tb == nil {
		return nil
	}
	matches, err := tb.LookupAll(ctx, source, termbase.LookupOptions{
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
			Range: model.RunRangeForBytes(runs, m.Position.Start, m.Position.End),
			Props: props,
		})
	}
	return &model.Overlay{Type: model.OverlayTerm, Spans: spans}
}

// brandOverlay builds an OverlayQA over the source runs from the seeded brand
// profile (brand.MatchVocabulary). Brand findings ride on the QA overlay type
// (the model's fixed overlay enum has no dedicated brand type) and are tagged
// with category="brand-vocabulary" plus the matched term, severity and any
// preferred replacement. Returns nil when nothing matches.
func brandOverlay(runs []model.Run, source string) *model.Overlay {
	hits := brand.MatchVocabulary(brandProfile, source)
	if len(hits) == 0 {
		return nil
	}
	spans := make([]model.Span, 0, len(hits))
	for _, h := range hits {
		props := map[string]string{
			"category": "brand-vocabulary",
			"severity": string(h.Severity),
			"term":     h.Term,
		}
		switch h.Kind {
		case brand.VocabCompetitor:
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
			Range: model.RunRangeForBytes(runs, h.Start, h.End),
			Props: props,
		})
	}
	return &model.Overlay{Type: model.OverlayQA, Spans: spans}
}

// qaOverlay builds an OverlayQA over the source runs from source-only QA
// heuristics that don't need a target (the streaming qa-check needs a committed
// target, which a freshly-parsed source document has none of). Today it flags
// double spaces and consecutive doubled words. Returns nil when the source is
// clean.
func qaOverlay(runs []model.Run, source string) *model.Overlay {
	var spans []model.Span

	// Double spaces: each run of >=2 spaces is one finding.
	for i := 0; i+1 < len(source); {
		if source[i] == ' ' && source[i+1] == ' ' {
			start := i
			for i < len(source) && source[i] == ' ' {
				i++
			}
			spans = append(spans, model.Span{
				Range: model.RunRangeForBytes(runs, start, i),
				Props: map[string]string{
					"category": "double-spaces",
					"severity": "minor",
					"message":  "Source contains double spaces",
				},
			})
			continue
		}
		i++
	}

	// Doubled words: a word immediately repeated (case-insensitive).
	for _, fr := range doubledWordRanges(source) {
		spans = append(spans, model.Span{
			Range: model.RunRangeForBytes(runs, fr[0], fr[1]),
			Props: map[string]string{
				"category": "doubled-word",
				"severity": "minor",
				"message":  fmt.Sprintf("Doubled word: %q", source[fr[0]:fr[1]]),
			},
		})
	}

	if len(spans) == 0 {
		return nil
	}
	return &model.Overlay{Type: model.OverlayQA, Spans: spans}
}

// doubledWordRanges returns the byte range of the *second* occurrence of each
// immediately-repeated word (case-insensitive, whitespace-separated). The range
// covers just the repeated word so the overlay highlights the redundant token.
func doubledWordRanges(s string) [][2]int {
	type word struct {
		text       string
		start, end int
	}
	isSpace := func(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
	var words []word
	i := 0
	for i < len(s) {
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		start := i
		for i < len(s) && !isSpace(s[i]) {
			i++
		}
		words = append(words, word{text: s[start:i], start: start, end: i})
	}
	var out [][2]int
	for j := 1; j < len(words); j++ {
		if strings.EqualFold(words[j].text, words[j-1].text) {
			out = append(out, [2]int{words[j].start, words[j].end})
		}
	}
	return out
}
