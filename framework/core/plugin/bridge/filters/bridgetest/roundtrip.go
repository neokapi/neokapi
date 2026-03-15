package bridgetest

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RoundTripResult holds the output of a roundtrip test.
type RoundTripResult struct {
	Parts  []*model.Part
	Output []byte
}

// RoundTrip performs a full read → write cycle via BridgeProcessor's single-pass
// pipeline. Java reads each event, sends the part to Go, receives it back
// unmodified, applies it, and writes — all in one filter iteration.
func RoundTrip(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any) RoundTripResult {
	t.Helper()
	return roundTrip(t, registry, cfg, filterClass, content, uri, mimeType, filterParams, "en", "fr")
}

// RoundTripWithLocales is like RoundTrip but allows specifying source and target locales.
func RoundTripWithLocales(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any, srcLocale, tgtLocale model.LocaleID) RoundTripResult {
	t.Helper()
	return roundTrip(t, registry, cfg, filterClass, content, uri, mimeType, filterParams, string(srcLocale), string(tgtLocale))
}

func roundTrip(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any, srcLocale, tgtLocale string) RoundTripResult {
	t.Helper()

	// Single-pass roundtrip via BridgeProcessor. Java reads each event,
	// sends part to Go, Go returns it unmodified, Java writes.
	processor := bridge.NewBridgeProcessor(registry, cfg, filterClass)
	if filterParams != nil {
		processor.SetFilterParams(filterParams)
	}

	// Determine input source.
	var inputPath string
	var inputContent []byte
	if filepath.IsAbs(uri) {
		if _, err := os.Stat(uri); err == nil {
			inputPath = uri
		}
	}
	if inputPath == "" {
		inputContent = content
	}

	// Capture parts as they flow through the identity processFn.
	var parts []*model.Part
	var output bytes.Buffer

	err := processor.ExecuteWithWriter(context.Background(), bridge.ProcessExecuteParams{
		InputPath:    inputPath,
		Content:      inputContent,
		SourceLocale: srcLocale,
		TargetLocale: tgtLocale,
		OutputLocale: tgtLocale,
		Encoding:     "UTF-8",
		MimeType:     mimeType,
	}, func(in <-chan *model.Part) <-chan *model.Part {
		out := make(chan *model.Part, 64)
		go func() {
			defer close(out)
			for p := range in {
				parts = append(parts, p)
				out <- p
			}
		}()
		return out
	}, &output)
	require.NoError(t, err, "roundtrip via BridgeProcessor")

	return RoundTripResult{
		Parts:  parts,
		Output: output.Bytes(),
	}
}

// AssertRoundTrip performs a roundtrip and asserts the output matches the input byte-for-byte.
func AssertRoundTrip(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any) RoundTripResult {
	t.Helper()

	result := RoundTrip(t, registry, cfg, filterClass, content, uri, mimeType, filterParams)
	assert.Equal(t, string(content), string(result.Output),
		"roundtrip output should match original input")
	return result
}

// RoundTripTestFiles runs roundtrip tests over all files matching a glob pattern.
func RoundTripTestFiles(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass, globPattern, mimeType string, filterParams map[string]any, knownFailing ...string) {
	t.Helper()
	RoundTripTestFilesWithLocales(t, registry, cfg, filterClass, globPattern, mimeType, filterParams, "en", "fr", knownFailing...)
}

// RoundTripTestFilesWithLocales is like RoundTripTestFiles but with explicit locales.
func RoundTripTestFilesWithLocales(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass, globPattern, mimeType string, filterParams map[string]any, srcLocale, tgtLocale string, knownFailing ...string) {
	t.Helper()

	t.Cleanup(func() {
		stats := registry.Stats()
		t.Logf("[registry-stats] filter=%s max_total=%d bridge_count=%d global_in_use=%d",
			filterClass, stats.MaxTotal, stats.BridgeCount, stats.GlobalInUse)
	})

	failing := make(map[string]bool, len(knownFailing))
	for _, f := range knownFailing {
		failing[f] = true
	}

	files, err := filepath.Glob(globPattern)
	require.NoError(t, err, "globbing test files")

	if len(files) == 0 {
		t.Fatalf("no test files matching %s (check glob pattern and testdata directory)", globPattern)
	}

	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if failing[name] {
				t.Skipf("known failing file: %s", name)
			}
			content, err := os.ReadFile(f)
			require.NoError(t, err)
			AssertRoundTripEventsWithLocales(t, registry, cfg, filterClass, content, f, mimeType, filterParams, model.LocaleID(srcLocale), model.LocaleID(tgtLocale))
		})
	}
}

// AssertRoundTripEvents performs a roundtrip and validates using event-level comparison.
func AssertRoundTripEvents(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any) RoundTripResult {
	t.Helper()

	result := RoundTrip(t, registry, cfg, filterClass, content, uri, mimeType, filterParams)
	rereadParts := ReadBytes(t, registry, cfg, filterClass, result.Output, uri, mimeType, filterParams)
	compareParts(t, result.Parts, rereadParts)

	return result
}

// AssertRoundTripEventsWithLocales is like AssertRoundTripEvents but with explicit locales.
func AssertRoundTripEventsWithLocales(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any, srcLocale, tgtLocale model.LocaleID) RoundTripResult {
	t.Helper()

	result := RoundTripWithLocales(t, registry, cfg, filterClass, content, uri, mimeType, filterParams, srcLocale, tgtLocale)
	rereadParts := ReadBytesWithLocales(t, registry, cfg, filterClass, result.Output, uri, mimeType, filterParams, string(srcLocale), string(tgtLocale))
	compareParts(t, result.Parts, rereadParts)

	return result
}

// compareParts performs event-level comparison of two part lists.
func compareParts(t *testing.T, expected, actual []*model.Part) {
	t.Helper()

	if !assert.Equal(t, len(expected), len(actual), "part count mismatch") {
		t.Logf("expected %d parts:", len(expected))
		for i, p := range expected {
			t.Logf("  [%d] %s %s", i, p.Type, partSummary(p))
		}
		t.Logf("actual %d parts:", len(actual))
		for i, p := range actual {
			t.Logf("  [%d] %s %s", i, p.Type, partSummary(p))
		}
		return
	}

	for i := range expected {
		ep, ap := expected[i], actual[i]
		prefix := fmt.Sprintf("part[%d]", i)

		assert.Equal(t, ep.Type, ap.Type, "%s: type mismatch", prefix)

		switch ep.Type {
		case model.PartBlock:
			compareBlocks(t, prefix, ep, ap)
		case model.PartLayerStart, model.PartLayerEnd:
			compareLayers(t, prefix, ep, ap)
		case model.PartData:
			compareData(t, prefix, ep, ap)
		case model.PartGroupStart:
			compareGroupStart(t, prefix, ep, ap)
		case model.PartGroupEnd:
			compareGroupEnd(t, prefix, ep, ap)
		}
	}
}

func compareBlocks(t *testing.T, prefix string, ep, ap *model.Part) {
	t.Helper()
	eb, _ := ep.Resource.(*model.Block)
	ab, _ := ap.Resource.(*model.Block)
	if eb == nil || ab == nil {
		return
	}
	assert.Equal(t, eb.ID, ab.ID, "%s: block ID", prefix)
	assert.Equal(t, eb.SourceText(), ab.SourceText(), "%s: source text", prefix)
	assert.Equal(t, eb.Translatable, ab.Translatable, "%s: translatable", prefix)
	assert.Equal(t, eb.Name, ab.Name, "%s: block name", prefix)
	assert.Equal(t, eb.Type, ab.Type, "%s: block type", prefix)
	assert.Equal(t, eb.PreserveWhitespace, ab.PreserveWhitespace, "%s: preserve whitespace", prefix)

	if len(eb.Annotations) > 0 || len(ab.Annotations) > 0 {
		assert.Equal(t, len(eb.Annotations), len(ab.Annotations), "%s: annotation count", prefix)
		for key := range eb.Annotations {
			_, ok := ab.Annotations[key]
			assert.True(t, ok, "%s: missing annotation key %q", prefix, key)
		}
	}

	if assert.Equal(t, len(eb.Source), len(ab.Source), "%s: source segment count", prefix) {
		for j := range eb.Source {
			sp := fmt.Sprintf("%s.source[%d]", prefix, j)
			assert.Equal(t, eb.Source[j].ID, ab.Source[j].ID, "%s: segment ID", sp)
			compareFragments(t, sp, eb.Source[j].Content, ab.Source[j].Content)
		}
	}
}

func compareFragments(t *testing.T, prefix string, ef, af *model.Fragment) {
	t.Helper()
	if ef == nil && af == nil {
		return
	}
	if ef == nil || af == nil {
		t.Errorf("%s: fragment nil mismatch (expected=%v actual=%v)", prefix, ef == nil, af == nil)
		return
	}
	assert.Equal(t, ef.Text(), af.Text(), "%s: fragment text", prefix)
	assert.Equal(t, len(ef.Spans), len(af.Spans), "%s: span count", prefix)

	n := len(ef.Spans)
	if len(af.Spans) < n {
		n = len(af.Spans)
	}
	for k := range n {
		sp := fmt.Sprintf("%s.span[%d]", prefix, k)
		es, as := ef.Spans[k], af.Spans[k]
		assert.Equal(t, es.SpanType, as.SpanType, "%s: span type", sp)
		assert.Equal(t, es.ID, as.ID, "%s: span ID", sp)
		assert.Equal(t, es.Data, as.Data, "%s: span data", sp)
		assert.Equal(t, es.Type, as.Type, "%s: span semantic type", sp)
		assert.Equal(t, es.OuterData, as.OuterData, "%s: span outer data", sp)
		assert.Equal(t, es.DisplayText, as.DisplayText, "%s: span display text", sp)
		assert.Equal(t, es.OriginalID, as.OriginalID, "%s: span original ID", sp)
		assert.Equal(t, es.Flags, as.Flags, "%s: span flags", sp)
		assert.Equal(t, es.Deletable, as.Deletable, "%s: span deletable", sp)
		assert.Equal(t, es.Cloneable, as.Cloneable, "%s: span cloneable", sp)
	}
}

func compareLayers(t *testing.T, prefix string, ep, ap *model.Part) {
	t.Helper()
	el, _ := ep.Resource.(*model.Layer)
	al, _ := ap.Resource.(*model.Layer)
	if el == nil || al == nil {
		return
	}
	assert.Equal(t, el.MimeType, al.MimeType, "%s: layer mime type", prefix)
	assert.True(t, strings.EqualFold(el.Encoding, al.Encoding),
		"%s: layer encoding (case-insensitive): expected %q, got %q", prefix, el.Encoding, al.Encoding)
	assert.Equal(t, el.Locale, al.Locale, "%s: layer locale", prefix)
}

func compareData(t *testing.T, prefix string, ep, ap *model.Part) {
	t.Helper()
	ed, _ := ep.Resource.(*model.Data)
	ad, _ := ap.Resource.(*model.Data)
	if ed == nil || ad == nil {
		return
	}
	assert.Equal(t, ed.ID, ad.ID, "%s: data ID", prefix)
	assert.Equal(t, ed.Name, ad.Name, "%s: data name", prefix)
}

func compareGroupStart(t *testing.T, prefix string, ep, ap *model.Part) {
	t.Helper()
	eg, _ := ep.Resource.(*model.GroupStart)
	ag, _ := ap.Resource.(*model.GroupStart)
	if eg == nil || ag == nil {
		return
	}
	assert.Equal(t, eg.ID, ag.ID, "%s: group ID", prefix)
	assert.Equal(t, eg.Name, ag.Name, "%s: group name", prefix)
	assert.Equal(t, eg.Type, ag.Type, "%s: group type", prefix)
	assert.Equal(t, eg.Properties, ag.Properties, "%s: group properties", prefix)
}

func compareGroupEnd(t *testing.T, prefix string, ep, ap *model.Part) {
	t.Helper()
	eg, _ := ep.Resource.(*model.GroupEnd)
	ag, _ := ap.Resource.(*model.GroupEnd)
	if eg == nil || ag == nil {
		return
	}
	assert.Equal(t, eg.ID, ag.ID, "%s: group end ID", prefix)
}

func partSummary(p *model.Part) string {
	switch p.Type {
	case model.PartBlock:
		if b, ok := p.Resource.(*model.Block); ok {
			text := b.SourceText()
			if len(text) > 60 {
				text = text[:60] + "..."
			}
			return fmt.Sprintf("id=%s text=%q", b.ID, text)
		}
	case model.PartLayerStart:
		if l, ok := p.Resource.(*model.Layer); ok {
			return fmt.Sprintf("id=%s", l.ID)
		}
	}
	return ""
}
