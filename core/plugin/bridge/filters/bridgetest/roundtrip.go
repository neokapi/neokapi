package bridgetest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RoundTripResult holds the output of a roundtrip test.
type RoundTripResult struct {
	// Parts extracted during the read phase.
	Parts []*model.Part
	// Output is the reconstructed document bytes from the write phase.
	Output []byte
}

// RoundTrip performs a full read → write cycle through the bridge:
//  1. Read the input content using the specified filter to extract parts
//  2. Write the parts back through the same filter to reconstruct the document
//
// Returns the extracted parts and the reconstructed output bytes.
func RoundTrip(t *testing.T, pool *bridge.BridgePool, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any) RoundTripResult {
	t.Helper()

	// --- Read phase ---
	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass)
	if filterParams != nil {
		reader.SetFilterParams(filterParams)
	}

	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: "en",
		TargetLocale: "fr",
		Encoding:     "UTF-8",
		MimeType:     mimeType,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))
	t.Cleanup(func() { _ = reader.Close() })

	var parts []*model.Part
	var readErr error
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			readErr = pr.Error
			break
		}
		parts = append(parts, pr.Part)
	}
	require.NoError(t, readErr, "roundtrip read phase")

	// --- Write phase ---
	var output bytes.Buffer
	writer := bridge.NewBridgeFormatWriter(pool, cfg, filterClass)
	if filterParams != nil {
		writer.SetFilterParams(filterParams)
	}
	writer.SetOriginalContent(content)
	writer.SetEncoding("UTF-8")
	writer.SetLocale("fr")
	require.NoError(t, writer.SetOutputWriter(&output))

	partsCh := make(chan *model.Part, len(parts))
	for _, p := range parts {
		partsCh <- p
	}
	close(partsCh)

	require.NoError(t, writer.Write(ctx, partsCh), "roundtrip write phase")

	return RoundTripResult{
		Parts:  parts,
		Output: output.Bytes(),
	}
}

// AssertRoundTrip performs a roundtrip and asserts the output matches the input
// byte-for-byte. This is the strongest form of roundtrip validation.
func AssertRoundTrip(t *testing.T, pool *bridge.BridgePool, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any) RoundTripResult {
	t.Helper()

	result := RoundTrip(t, pool, cfg, filterClass, content, uri, mimeType, filterParams)
	assert.Equal(t, string(content), string(result.Output),
		"roundtrip output should match original input")
	return result
}

// RoundTripTestFiles runs roundtrip tests over all files matching a glob pattern
// within the testdata directory. Each file becomes a subtest named after the
// filename.
//
// This uses event-level comparison (like Java's EventRoundTripIT): the output
// is re-read through the same filter and the extracted parts are compared
// semantically. This tolerates cosmetic differences (whitespace normalization,
// Unicode escape forms, attribute reordering) that don't affect content.
//
// Files listed in knownFailing are expected to fail and are skipped with a log
// message rather than failing the test (matching Java's knownFailingFiles
// pattern).
func RoundTripTestFiles(t *testing.T, pool *bridge.BridgePool, cfg bridge.BridgeConfig, filterClass, globPattern, mimeType string, filterParams map[string]any, knownFailing ...string) {
	t.Helper()

	failing := make(map[string]bool, len(knownFailing))
	for _, f := range knownFailing {
		failing[f] = true
	}

	files, err := filepath.Glob(globPattern)
	require.NoError(t, err, "globbing test files")

	if len(files) == 0 {
		t.Skipf("no test files matching %s", globPattern)
	}

	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			if failing[name] {
				t.Skipf("known failing file: %s", name)
			}
			content, err := os.ReadFile(f)
			require.NoError(t, err)
			AssertRoundTripEvents(t, pool, cfg, filterClass, content, name, mimeType, filterParams)
		})
	}
}

// AssertRoundTripEvents performs a roundtrip and validates using event-level
// comparison: the reconstructed output is re-read through the same filter and
// the extracted parts are compared with the original parts. This mirrors
// Java's EventRoundTripIT approach.
func AssertRoundTripEvents(t *testing.T, pool *bridge.BridgePool, cfg bridge.BridgeConfig, filterClass string, content []byte, uri, mimeType string, filterParams map[string]any) RoundTripResult {
	t.Helper()

	result := RoundTrip(t, pool, cfg, filterClass, content, uri, mimeType, filterParams)

	// Re-read the output to get parts for comparison.
	rereadParts := ReadBytes(t, pool, cfg, filterClass, result.Output, uri, mimeType, filterParams)
	compareParts(t, result.Parts, rereadParts)

	return result
}

// compareParts performs event-level comparison of two part lists.
// It compares part types, and for each part type: IDs, key fields, and content.
func compareParts(t *testing.T, expected, actual []*model.Part) {
	t.Helper()

	if !assert.Equal(t, len(expected), len(actual), "part count mismatch") {
		// Log what we got for debugging.
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

	// Compare source segments in detail.
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
	// Layer ID and Name are derived from the temp file URI on the Java side,
	// so they differ between reads. Encoding may also change (the write phase
	// normalizes to UTF-8). Compare only the stable mime type field.
	assert.Equal(t, el.MimeType, al.MimeType, "%s: layer mime type", prefix)
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

// partSummary returns a short description of a part for debug logging.
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
