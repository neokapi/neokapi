package json_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: JSONFilterTest#testValue
func TestReadSimpleJSON(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	input := `{"title": "Hello World", "description": "A simple test"}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 2)
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
	assert.Contains(t, texts, "A simple test")
}

// okapi: JSONFilterTest#testObject
func TestReadNestedJSON(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	input := `{"nested": {"key": "Nested value", "deep": {"inner": "Deep value"}}}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Nested value")
	assert.Contains(t, texts, "Deep value")

	// Verify key paths
	names := make(map[string]string)
	for _, b := range blocks {
		names[b.Name] = b.SourceText()
	}
	assert.Equal(t, "Nested value", names["nested.key"])
	assert.Equal(t, "Deep value", names["nested.deep.inner"])
}

// okapi: JSONFilterTest#testList
func TestReadArrayStrings(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"extractIsolatedStrings": true,
	}))
	input := `{"items": ["First", "Second", "Third"]}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "First")
	assert.Contains(t, texts, "Second")
	assert.Contains(t, texts, "Third")

	// Verify key paths include array indices
	names := make(map[string]string)
	for _, b := range blocks {
		names[b.Name] = b.SourceText()
	}
	assert.Equal(t, "First", names["items[0]"])
	assert.Equal(t, "Second", names["items[1]"])
	assert.Equal(t, "Third", names["items[2]"])
}

// okapi: JSONFilterTest#testDecimalNumber
func TestReadNonStringValues(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	input := `{"name": "Test", "count": 42, "active": true, "value": null}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	// Only "name" should be a block (string value)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Test", blocks[0].SourceText())
	assert.Equal(t, "name", blocks[0].Name)

	// Count data parts (non-string values)
	dataCount := 0
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
		}
	}
	assert.Equal(t, 3, dataCount, "should have 3 Data parts for number, boolean, and null")
}

func TestReadEmptyJSON(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`{}`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestReadLayerStartEnd(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(`{"key": "value"}`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "json", layer.Format)
	assert.Equal(t, "application/json", layer.MimeType)
}

func TestReaderSignature(t *testing.T) {
	t.Parallel()
	reader := jsonfmt.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/json")
	assert.Contains(t, sig.Extensions, ".json")
}

func TestReaderMetadata(t *testing.T) {
	t.Parallel()
	reader := jsonfmt.NewReader()
	assert.Equal(t, "json", reader.Name())
	assert.Equal(t, "JSON", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadInvalidJSON(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	// Unterminated string — invalid even under JSON5 lenient mode
	// (single quotes / bare keys, which `{invalid json` would otherwise
	// be scanned as).
	err := reader.Open(ctx, testutil.RawDocFromString(`{"key": "unterminated`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	// Read channel directly; expect an error result
	ch := reader.Read(ctx)
	var foundError bool
	for result := range ch {
		if result.Error != nil {
			foundError = true
		}
	}
	assert.True(t, foundError, "expected an error for invalid JSON input")
}

func TestReadInvalidJSONError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	// Unterminated string — invalid even under JSON5 lenient mode
	// (single quotes / bare keys, which `{invalid json` would otherwise
	// be scanned as).
	err := reader.Open(ctx, testutil.RawDocFromString(`{"key": "unterminated`, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)
	var foundError bool
	for result := range ch {
		if result.Error != nil {
			foundError = true
			break
		}
	}
	assert.True(t, foundError, "expected an error for invalid JSON")
}

func TestReadFromFile(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	f, err := os.Open("testdata/simple.json")
	require.NoError(t, err)

	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"extractIsolatedStrings": true,
	}))
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.json", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	// simple.json has: title, description, nested.key, items[0], items[1], items[2]
	require.Len(t, blocks, 6)

	names := make(map[string]string)
	for _, b := range blocks {
		names[b.Name] = b.SourceText()
	}

	assert.Equal(t, "Hello World", names["title"])
	assert.Equal(t, "A simple test", names["description"])
	assert.Equal(t, "Nested value", names["nested.key"])
	assert.Equal(t, "First", names["items[0]"])
	assert.Equal(t, "Second", names["items[1]"])
	assert.Equal(t, "Third", names["items[2]"])
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	input := `{"title": "Hello World", "description": "A simple test"}`

	// Read
	reader := jsonfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write (source language roundtrip)
	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Parse both to compare structure (key order may differ)
	var expected, actual map[string]any
	require.NoError(t, json.Unmarshal([]byte(input), &expected))
	require.NoError(t, json.Unmarshal(buf.Bytes(), &actual))

	assert.Equal(t, expected["title"], actual["title"])
	assert.Equal(t, expected["description"], actual["description"])
}

func TestRoundTripWithTranslation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	input := `{"greeting": "Hello", "farewell": "Goodbye"}`

	// Read
	reader := jsonfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set French targets
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			switch block.SourceText() {
			case "Hello":
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			case "Goodbye":
				block.SetTargetText(model.LocaleFrench, "Au revoir")
			}
		}
	}

	// Write with French locale
	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.Equal(t, "Bonjour", result["greeting"])
	assert.Equal(t, "Au revoir", result["farewell"])
}

func TestRoundTripNested(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	input := `{"parent": {"child": "Original"}}`

	// Read
	reader := jsonfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set target
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			block.SetTargetText(model.LocaleGerman, "Uebersetzt")
		}
	}

	// Write with German locale
	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleGerman)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	parent, ok := result["parent"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Uebersetzt", parent["child"])
}

func TestRoundTripArray(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	input := `{"items": ["Apple", "Banana", "Cherry"]}`

	// Read
	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"extractIsolatedStrings": true,
	}))
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set Spanish targets
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			switch block.SourceText() {
			case "Apple":
				block.SetTargetText(model.LocaleSpanish, "Manzana")
			case "Banana":
				block.SetTargetText(model.LocaleSpanish, "Platano")
			case "Cherry":
				block.SetTargetText(model.LocaleSpanish, "Cereza")
			}
		}
	}

	// Write with Spanish locale
	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleSpanish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	items, ok := result["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 3)
	assert.Equal(t, "Manzana", items[0])
	assert.Equal(t, "Platano", items[1])
	assert.Equal(t, "Cereza", items[2])
}

func TestRoundTripFileSimple(t *testing.T) {
	t.Parallel()
	original, err := os.ReadFile("testdata/simple.json")
	require.NoError(t, err)

	ctx := t.Context()

	// Read
	f, err := os.Open("testdata/simple.json")
	require.NoError(t, err)
	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"extractIsolatedStrings": true,
	}))
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.json", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write (source roundtrip - no translation)
	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Compare JSON structure (not exact string, since key order may vary)
	var expected, actual any
	require.NoError(t, json.Unmarshal(original, &expected))
	require.NoError(t, json.Unmarshal(buf.Bytes(), &actual))
	assert.Equal(t, expected, actual)
}

// okapi: JSONFilterTest#testEscape
func TestReadUnicodeJSON(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	input := `{"greeting": "Hello \u4e16\u754c", "japanese": "\u3053\u3093\u306b\u3061\u306f"}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello \u4e16\u754c")
	assert.Contains(t, texts, "\u3053\u3093\u306b\u3061\u306f")
}

// okapi: JSONFilterTest#testEmptyValue
func TestReadEmptyStringValue(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	input := `{"empty": "", "notempty": "value"}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	// Empty strings are still strings; they should be extracted as blocks
	require.Len(t, blocks, 2)
	names := make(map[string]string)
	for _, b := range blocks {
		names[b.Name] = b.SourceText()
	}
	assert.Empty(t, names["empty"])
	assert.Equal(t, "value", names["notempty"])
}

func TestReadMixedArray(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"extractIsolatedStrings": true,
	}))
	input := `{"mixed": ["text", 42, true, "more text"]}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	// Only string elements should be blocks
	require.Len(t, blocks, 2)
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "text")
	assert.Contains(t, texts, "more text")
}

func TestConfigDefaults(t *testing.T) {
	t.Parallel()
	cfg := &jsonfmt.Config{}
	cfg.Reset()
	assert.True(t, cfg.ExtractAllPairs)
	assert.Equal(t, "json", cfg.FormatName())
	require.NoError(t, cfg.Validate())
}

func TestRoundTripSourceOnly(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	input := `{"msg": "Hello"}`

	// Read
	reader := jsonfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write without setting locale (should use source text)
	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, "Hello", result["msg"])
}

func TestBlocksAreTranslatable(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	input := `{"key": "value"}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.True(t, blocks[0].Translatable)
}

// okapi: JSONFilterTest#testPath
func TestReadDeeplyNested(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	input := `{"a": {"b": {"c": {"d": "deep"}}}}`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "deep", blocks[0].SourceText())
	assert.Equal(t, "a.b.c.d", blocks[0].Name)
}

// partsByKind splits emitted parts into translatable blocks, non-translatable
// content blocks, and Data parts — the three buckets the
// extractNonTranslatableContent flag moves content between.
func partsByKind(parts []*model.Part) (translatable, content []*model.Block, data []*model.Data) {
	for _, p := range parts {
		switch p.Type {
		case model.PartBlock:
			if b, ok := p.Resource.(*model.Block); ok {
				if b.Translatable {
					translatable = append(translatable, b)
				} else {
					content = append(content, b)
				}
			}
		case model.PartData:
			if d, ok := p.Resource.(*model.Data); ok {
				data = append(data, d)
			}
		}
	}
	return translatable, content, data
}

// By default (extractNonTranslatableContent on), isolated string values in an
// array — not extracted for translation — surface as non-translatable content
// blocks (visible to ingestion/LLM consumers, skipped by MT) instead of opaque
// textless Data parts.
func TestReadIsolatedStringsAsContent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	input := `{"tags": ["alpha", "beta"]}`
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	translatable, content, data := partsByKind(parts)

	assert.Empty(t, translatable, "isolated strings are never translatable by default")
	require.Len(t, content, 2, "both isolated strings surface as non-translatable content")
	assert.Empty(t, data, "isolated strings no longer fall through to textless Data")

	byName := map[string]*model.Block{}
	for _, b := range content {
		byName[b.Name] = b
	}
	for _, name := range []string{"tags[0]", "tags[1]"} {
		b := byName[name]
		require.NotNil(t, b, "expected content block for %s", name)
		assert.False(t, b.Translatable)
		assert.Empty(t, b.SemanticRole(), "plain string value carries no special role")
	}
	assert.Equal(t, "alpha", byName["tags[0]"].SourceText())
	assert.Equal(t, "beta", byName["tags[1]"].SourceText())
}

// With extractNonTranslatableContent off (the Okapi-faithful config), isolated
// array strings stay opaque textless Data — byte-identical to the prior default.
func TestReadIsolatedStringsAsDataWhenDisabled(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"extractNonTranslatableContent": false,
	}))
	input := `{"tags": ["alpha", "beta"]}`
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	translatable, content, data := partsByKind(parts)

	assert.Empty(t, translatable)
	assert.Empty(t, content, "no content blocks surface when surfacing is disabled")
	require.Len(t, data, 2, "isolated strings stay as textless Data")
	names := []string{data[0].Name, data[1].Name}
	assert.Contains(t, names, "tags[0]")
	assert.Contains(t, names, "tags[1]")
}

// By default, a value excluded from extraction (here via the exceptions regex)
// surfaces as a non-translatable content block while the extracted sibling stays
// translatable.
func TestReadExcludedValueAsContent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"exceptions": "^_internal$",
	}))
	input := `{"keep": "Hi", "_internal": "secret"}`
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	translatable, content, data := partsByKind(parts)

	require.Len(t, translatable, 1)
	assert.Equal(t, "Hi", translatable[0].SourceText())
	assert.Equal(t, "keep", translatable[0].Name)

	require.Len(t, content, 1, "the excluded value surfaces as one non-translatable content block")
	assert.False(t, content[0].Translatable)
	assert.Equal(t, "secret", content[0].SourceText())
	assert.Equal(t, "_internal", content[0].Name)
	assert.Empty(t, content[0].SemanticRole())

	assert.Empty(t, data, "excluded value no longer falls through to textless Data")
}

// A configured note whose object scope closes with no following translatable
// block to attach it to (a "dangling note") would otherwise be silently
// dropped. By default (extractNonTranslatableContent on) the reader flushes the
// note text as a non-translatable Data part at object-scope close so
// ingestion/LLM consumers can still see it. The note key + delimiters stay in
// the skeleton, so the document still round-trips byte-exact.
func TestReadDanglingNoteFlushedAsData(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"noteRules": "^_note$",
	}))
	input := `{"text": "Hello", "_note": "trailing hint"}`
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	translatable, content, data := partsByKind(parts)

	require.Len(t, translatable, 1)
	assert.Equal(t, "Hello", translatable[0].SourceText())
	assert.Empty(t, translatable[0].Notes(), "the note trails the block, so it never attaches as an annotation")
	assert.Empty(t, content, "a dangling note is not surfaced as a translatable/content block")

	require.Len(t, data, 1, "the dangling note text is flushed as one Data part")
	assert.Equal(t, "trailing hint", data[0].Properties["text"])
	assert.Equal(t, "_note", data[0].Name)
}

// The dangling-note Data part rides on the same flag as the rest of the
// non-translatable content surfacing: with extractNonTranslatableContent off
// (the Okapi-faithful / parity config) the part stream is byte-identical to the
// prior behavior — the note text stays skeleton-only and no Data is emitted.
func TestReadDanglingNoteSuppressedWhenDisabled(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"noteRules":                     "^_note$",
		"extractNonTranslatableContent": false,
	}))
	input := `{"text": "Hello", "_note": "trailing hint"}`
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	translatable, content, data := partsByKind(parts)

	require.Len(t, translatable, 1)
	assert.Equal(t, "Hello", translatable[0].SourceText())
	assert.Empty(t, content)
	assert.Empty(t, data, "no Data is emitted for the dangling note when surfacing is disabled")
}

// A note that DOES have a following translatable block still attaches as a
// NoteAnnotation (the established behavior) and is therefore not also flushed as
// a dangling Data part.
func TestReadConsumedNoteIsAnnotationNotData(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"noteRules": "^_note$",
	}))
	input := `{"_note": "hint", "text": "Hello"}`
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	translatable, content, data := partsByKind(parts)

	require.Len(t, translatable, 1)
	notes := translatable[0].Notes()
	require.Len(t, notes, 1, "the note attaches to the following block")
	assert.Equal(t, "hint", notes[0].Text)
	assert.Empty(t, content)
	assert.Empty(t, data, "a consumed note is not also flushed as a dangling Data part")
}

// Only dangling NOTES are flushed; a dangling ID stays skeleton-only. An
// identifier with no following block to name leaves no Data part behind.
func TestReadDanglingIDNotFlushed(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"idRules": "^_id$",
	}))
	input := `{"text": "Hi", "_id": "abc"}`
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	translatable, content, data := partsByKind(parts)

	require.Len(t, translatable, 1)
	assert.Equal(t, "text", translatable[0].Name, "the id trails the block, so it is not used as a name")
	assert.Empty(t, content)
	assert.Empty(t, data, "a dangling id is never flushed as Data")
}

// A note dangling inside a NESTED object scope is flushed at that inner scope's
// close (mid-document), before the parent's later translatable block, and the
// document still round-trips byte-exact through the skeleton store.
func TestReadDanglingNoteInNestedScope(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"noteRules": "_note$",
	}))
	input := `{"meta": {"_note": "section note"}, "text": "Hello"}`
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	translatable, _, data := partsByKind(parts)

	require.Len(t, translatable, 1)
	assert.Equal(t, "Hello", translatable[0].SourceText())
	require.Len(t, data, 1)
	assert.Equal(t, "section note", data[0].Properties["text"])
	assert.Equal(t, "meta._note", data[0].Name)

	// The note key + delimiters stay in the skeleton, so the round-trip is
	// byte-exact even though the note text is now surfaced as a Data part.
	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input, map[string]any{
		"noteRules": "_note$",
	}))
}

// TestWriterTerminatesOnUnexpectedObjectToken guards against a writer hang:
// when leniently-parsed malformed input leaves a non-string, non-comma,
// non-close token at the head of an object, writeTokenObject must still advance
// (emit it verbatim) instead of spinning forever. Reconstruction runs on the
// stored original (no skeleton store).
func TestWriterTerminatesOnUnexpectedObjectToken(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// `]` inside the object (after a complete pair) is the unexpected token.
	input := `{"a": "b", ] }`

	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	writer := jsonfmt.NewWriter()
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	done := make(chan error, 1)
	go func() { done <- writer.Write(ctx, testutil.PartsToChannel(parts)) }()

	select {
	case err := <-done:
		require.NoError(t, err)
		writer.Close()
		// The extracted value round-trips and the stray token is preserved.
		assert.Contains(t, buf.String(), `"b"`)
		assert.Contains(t, buf.String(), `]`)
	case <-time.After(10 * time.Second):
		t.Fatal("writer.Write did not terminate on malformed object input")
	}
}

// With surfacing disabled, an excluded value stays opaque textless Data —
// byte-identical to the prior default.
func TestReadExcludedValueAsDataWhenDisabled(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"exceptions":                    "^_internal$",
		"extractNonTranslatableContent": false,
	}))
	input := `{"keep": "Hi", "_internal": "secret"}`
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	translatable, content, data := partsByKind(parts)

	require.Len(t, translatable, 1)
	assert.Equal(t, "Hi", translatable[0].SourceText())
	assert.Empty(t, content)
	require.Len(t, data, 1, "excluded value stays as textless Data")
	assert.Equal(t, "_internal", data[0].Name)
}
