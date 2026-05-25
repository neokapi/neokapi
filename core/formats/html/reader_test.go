package html_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: HtmlSnippetsTest#minimalCompleteHtml
func TestReadSimpleHTML(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Hello world</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.GreaterOrEqual(t, len(blocks), 1)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
}

// okapi: HtmlSnippetsTest#ITextUnitsInARowWithTwoHeaders
func TestReadMultipleBlocks(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><h1>Title</h1><p>Paragraph</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(blocks), 2)
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Title")
	assert.Contains(t, texts, "Paragraph")
}

// okapi: HtmlSnippetsTest#testPWithInlines
func TestReadInlineSpans(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Click <b>here</b> for info</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 1)

	runs := blocks[0].SourceRuns()
	require.NotEmpty(t, runs)
	assert.Equal(t, "Click here for info", model.RunsText(runs))

	var inlineRuns []model.Run
	for _, r := range runs {
		if r.Text == nil {
			inlineRuns = append(inlineRuns, r)
		}
	}
	require.Len(t, inlineRuns, 2)
	require.NotNil(t, inlineRuns[0].PcOpen)
	assert.Equal(t, "fmt:bold", inlineRuns[0].PcOpen.Type)
	assert.Equal(t, "html:b", inlineRuns[0].PcOpen.SubType)
	assert.Equal(t, "1", inlineRuns[0].PcOpen.ID)
	assert.Equal(t, "<b>", inlineRuns[0].PcOpen.Data)
	assert.Equal(t, "[B]", inlineRuns[0].PcOpen.Disp)
	require.NotNil(t, inlineRuns[0].PcOpen.Constraints)
	assert.True(t, inlineRuns[0].PcOpen.Constraints.Deletable)
	require.NotNil(t, inlineRuns[1].PcClose)
	assert.Equal(t, "fmt:bold", inlineRuns[1].PcClose.Type)
	assert.Equal(t, "1", inlineRuns[1].PcClose.ID) // same ID as opening
	assert.Equal(t, "</b>", inlineRuns[1].PcClose.Data)
}

// okapi: HtmlSnippetsTest#testHref
func TestReadLinkSpan(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Visit <a href="http://example.com">our site</a></p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 1)

	runs := blocks[0].SourceRuns()
	assert.Equal(t, "Visit our site", model.RunsText(runs))

	// Should have opening and closing link:hyperlink runs
	var openingRun *model.PcOpenRun
	var closingRun *model.PcCloseRun
	for _, r := range runs {
		if r.PcOpen != nil && r.PcOpen.Type == "link:hyperlink" {
			openingRun = r.PcOpen
		}
		if r.PcClose != nil && r.PcClose.Type == "link:hyperlink" {
			closingRun = r.PcClose
		}
	}
	require.NotNil(t, openingRun)
	require.NotNil(t, closingRun)
	assert.Contains(t, openingRun.Data, "href")
	assert.Equal(t, "html:a", openingRun.SubType)
	assert.Equal(t, "1", openingRun.ID)
	assert.Equal(t, openingRun.ID, closingRun.ID)
}

// okapi: HtmlSnippetsTest#paraWithBreak
func TestReadPlaceholderSpan(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Line one<br/>Line two</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.GreaterOrEqual(t, len(blocks), 1)

	runs := blocks[0].SourceRuns()
	assert.Equal(t, "Line oneLine two", model.RunsText(runs))

	// br should be a placeholder run with semantic type
	found := false
	for _, r := range runs {
		if r.Ph != nil && r.Ph.Type == "struct:break" {
			found = true
			assert.Equal(t, "html:br", r.Ph.SubType)
			assert.Equal(t, "1", r.Ph.ID)
		}
	}
	assert.True(t, found, "expected <br/> to be a placeholder run")
}

// okapi: HtmlFullFileTest#testSkippedScriptandStyleElements
func TestReadScriptNonTranslatable(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Hello</p><script>var x = 1;</script><p>World</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	texts := testutil.BlockTexts(blocks)

	assert.Contains(t, texts, "Hello")
	assert.Contains(t, texts, "World")
	// Script content should NOT be in blocks
	for _, text := range texts {
		assert.NotContains(t, text, "var x")
	}
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Test</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "html", layer.Format)
}

// okapi: HtmlConfigurationTest#defaultConfiguration
func TestReadTitle(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><head><title>Page Title</title></head><body><p>Content</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	texts := testutil.BlockTexts(blocks)

	assert.Contains(t, texts, "Page Title")
	assert.Contains(t, texts, "Content")
}

func TestReaderSignature(t *testing.T) {
	reader := htmlfmt.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/html")
	assert.Contains(t, sig.Extensions, ".html")
	assert.Contains(t, sig.Extensions, ".htm")
}

func TestWriteBlockWithSkeleton(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`<html><body><p>Hello world</p></body></html>`, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set French targets
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
		}
	}

	// Write
	var buf bytes.Buffer
	writer := htmlfmt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour le monde")
	assert.NotContains(t, output, "Hello world")
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReaderMetadata(t *testing.T) {
	reader := htmlfmt.NewReader()
	assert.Equal(t, "html", reader.Name())
	assert.Equal(t, "HTML", reader.DisplayName())
}

// TestReadBareTextLeadingSpacePreservedAfterInline asserts that when a
// container's content gets split across the bare-text top-level path
// (e.g. an `<li>` whose content makes forwardScanForBlockChildren
// return its safe-default container classification), text following an
// inline element keeps its leading whitespace. Previously the bare-text
// path called trimLeadingHTMLWhitespace unconditionally, dropping the
// space after `</i>` in `<i>need</i> to recieve` and corrupting the
// round-trip.
//
// okapi: HtmlFilter trims a text-unit's leading whitespace once at unit
// start; subsequent text-runs inside the same unit keep their content
// verbatim. See HtmlFilter#characters in
// okapi/filters/html/src/main/java/net/sf/okapi/filters/html/HtmlFilter.java.
func TestReadBareTextLeadingSpacePreservedAfterInline(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	src := `<html><body><ul>
<li>
<a name="g3">    you don't <i>need</i> to recieve scripts; this content is intentionally long enough to force the forward-scan to treat its container as inline-only and split text across the bare-text path.
</a></li></ul></body></html>`
	err = reader.Open(ctx, testutil.RawDocFromString(src, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	// Find a block whose first run is text starting with `to recieve` —
	// that's the bare-text continuation we care about. If the block was
	// emitted via inline-collection (PcOpen first run), the trim
	// concern doesn't apply (collectInlineTokens preserves text
	// verbatim). Either way, the assertion is that whatever path emits
	// `to recieve` keeps the leading space.
	for _, b := range blocks {
		text := b.SourceText()
		if !strings.Contains(text, "to recieve") {
			continue
		}
		// Look for the actual "to recieve" position and check what
		// precedes it in the rendered text.
		idx := strings.Index(text, "to recieve")
		require.Greater(t, idx, 0, "block %s: 'to recieve' must not be at offset 0; preceded text dropped: %q", b.ID, text)
		precedingChar := text[idx-1]
		assert.Equal(t, byte(' '), precedingChar,
			"block %s: char before 'to recieve' must be a space; got %q in text %q", b.ID, string(precedingChar), text)
		return
	}
	t.Fatalf("expected a block containing 'to recieve' but none found")
}

// TestReadEntityInBareTextBlock asserts that HTML entities surviving the
// top-level (bare-text) block path are extracted as inline placeholder
// runs rather than literal text. Previously, the bare-text path created
// blocks via `model.NewBlock(blockID, text)` which preserved `&amp;` as
// text — pseudo-translation then substituted `amp` letter-by-letter
// (`&amp;` → `&àmƥ;`). This regression-tests the buildBlockWithEntities
// fix that mirrors addTextWithEntities for the bare-text path.
//
// okapi: HtmlFilter peels entity references into opaque inline `<code>`
// pairs regardless of whether the surrounding container is a leaf block
// or not. See HtmlFilter#startEntity in
// okapi/filters/html/src/main/java/net/sf/okapi/filters/html/HtmlFilter.java.
func TestReadEntityInBareTextBlock(t *testing.T) {
	ctx := t.Context()
	reader := htmlfmt.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	// `<td>` with content that exceeds the tokenizer buffer triggers
	// forwardScanForBlockChildren's safe-default container classification,
	// so text inside the td emits via the bare-text top-level path.
	// Synthesise that condition with a deeply-nested inline structure
	// containing `&amp;` entity in source.
	src := `<html><body><table><tr><td><a href="x"><b><font size="-1"><span style="font-size: 12px;">BUFO &amp; Paranormal</span></font></b></a></td></tr></table></body></html>`
	err = reader.Open(ctx, testutil.RawDocFromString(src, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.NotEmpty(t, blocks)

	// Locate the block with the entity.
	var target *model.Block
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "BUFO") {
			target = b
			break
		}
	}
	require.NotNil(t, target, "block containing BUFO should exist")
	require.NotEmpty(t, target.Source)

	var sawEntity bool
	for _, r := range target.Source {
		if r.Ph != nil && r.Ph.Type == "code:entity" && r.Ph.Data == "&amp;" {
			sawEntity = true
			break
		}
	}
	assert.True(t, sawEntity, "expected `&amp;` entity to be extracted as a code:entity placeholder run; got runs: %+v", target.Source)
}
