package vignette_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/vignette"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readVignetteBlocks reads input through a vignette reader configured with the
// given ExtractNonTranslatableContent setting and returns every Block.
func readVignetteBlocks(t *testing.T, input string, extract bool) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := vignette.NewReader()
	cfg := reader.Config().(*vignette.Config)
	cfg.SetExtractNonTranslatableContent(extract)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()
	return testutil.CollectBlocks(t, reader.Read(ctx))
}

// vignetteRoundtripWithFlag does an untranslated skeleton round-trip with the
// given ExtractNonTranslatableContent setting and returns the writer output.
func vignetteRoundtripWithFlag(t *testing.T, input string, extract bool) string {
	t.Helper()
	ctx := t.Context()

	reader := vignette.NewReader()
	reader.Config().(*vignette.Config).SetExtractNonTranslatableContent(extract)
	writer := vignette.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// TestNonTranslatableContentDefaultOn pins the new default: the non-source-locale
// instance (es_ES "bonjour") is surfaced as a Translatable:false content Block —
// visible to ingestion, skipped by MT — alongside the translatable source-side
// Block (en_US "hello").
func TestNonTranslatableContentDefaultOn(t *testing.T) {
	blocks := readVignetteBlocks(t, plainBilingualPair, true)
	require.Len(t, blocks, 2, "one translatable source Block + one non-translatable other-locale Block")

	var translatable, nonTranslatable *model.Block
	for _, b := range blocks {
		if b.Translatable {
			translatable = b
		} else {
			nonTranslatable = b
		}
	}

	require.NotNil(t, translatable, "the en_US source-side Block must be translatable")
	assert.Equal(t, "hello", translatable.SourceText())
	assert.Equal(t, "en_US", translatable.Properties["localeId"])

	require.NotNil(t, nonTranslatable, "the es_ES instance must surface as a non-translatable content Block")
	assert.Equal(t, "bonjour", nonTranslatable.SourceText(), "carries the verbatim payload text")
	assert.False(t, nonTranslatable.Translatable, "skipped instance content is visible but not MT-bound")
	assert.True(t, nonTranslatable.PreserveWhitespace, "verbatim content keeps significant whitespace")
	assert.Empty(t, nonTranslatable.SemanticRole(), "non-source-locale prose carries no specific role")
	assert.Equal(t, "SMCCONTENT-BODY", nonTranslatable.Properties["attribute"])
	assert.Equal(t, "es_ES", nonTranslatable.Properties["localeId"])
	assert.Equal(t, "true", nonTranslatable.Properties["nonSourceLocale"])
	assert.Equal(t, "true", nonTranslatable.Properties["rawVerbatim"])

	// The source is a single verbatim run (no inline parse).
	require.Len(t, nonTranslatable.Source, 1)
	require.NotNil(t, nonTranslatable.Source[0].Text)
	assert.Equal(t, "bonjour", nonTranslatable.Source[0].Text.Text)
}

// TestNonTranslatableContentVerbatimRawForHTML proves the non-translatable
// surfacing carries the LITERAL source bytes of the payload region (here the
// entity-escaped okf_html es_ES body), not the decoded/re-encoded form — so the
// writer round-trips it byte-for-byte.
func TestNonTranslatableContentVerbatimRawForHTML(t *testing.T) {
	blocks := readVignetteBlocks(t, simpleBilingualPair, true)

	var nonTranslatable *model.Block
	for _, b := range blocks {
		if !b.Translatable {
			nonTranslatable = b
		}
	}
	require.NotNil(t, nonTranslatable)
	// es_ES body source in simpleBilingualPair is the entity-escaped
	// "&lt;p&gt;ES&lt;/p&gt;"; the non-translatable block keeps it verbatim
	// (NOT decoded to "<p>ES</p>" or stripped to "ES").
	assert.Equal(t, "&lt;p&gt;ES&lt;/p&gt;", nonTranslatable.SourceText())
}

// TestNonTranslatableContentDefaultOffStreamUnchanged pins the parity / flag-off
// contract: with ExtractNonTranslatableContent OFF the part stream is identical
// to the pre-change behavior — only the translatable source-side Block reaches
// the stream; the skipped instance stays skeleton.
func TestNonTranslatableContentDefaultOff(t *testing.T) {
	blocks := readVignetteBlocks(t, plainBilingualPair, false)
	require.Len(t, blocks, 1, "flag off → only the translatable source-side Block, no content blocks")
	assert.True(t, blocks[0].Translatable)
	assert.Equal(t, "hello", blocks[0].SourceText())
}

// TestNonTranslatableContentMonolingualNoChange confirms the surfacing is a
// no-op in monolingual mode (every instance is already extracted as
// translatable there, so there is nothing left to surface).
func TestNonTranslatableContentMonolingualNoChange(t *testing.T) {
	ctx := t.Context()
	reader := vignette.NewReader()
	cfg := reader.Config().(*vignette.Config)
	cfg.Monolingual = true
	// Flag is ON by default; assert it does not add content blocks on top of
	// the four monolingual extractions.
	require.True(t, cfg.ExtractNonTranslatableContent())
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(complexTwoPair, model.LocaleEnglish)))
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 4)
	for _, b := range blocks {
		assert.True(t, b.Translatable, "monolingual mode extracts every instance as translatable")
	}
}

// TestExtractNonTranslatableContentConfig pins the flag idiom: default ON, the
// setter, and the ApplyMap key (inverted private field).
func TestExtractNonTranslatableContentConfig(t *testing.T) {
	cfg := &vignette.Config{}
	cfg.Reset()
	assert.True(t, cfg.ExtractNonTranslatableContent(), "default ON (opt-out)")

	cfg.SetExtractNonTranslatableContent(false)
	assert.False(t, cfg.ExtractNonTranslatableContent())
	cfg.SetExtractNonTranslatableContent(true)
	assert.True(t, cfg.ExtractNonTranslatableContent())

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	assert.False(t, cfg.ExtractNonTranslatableContent())
	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": true}))
	assert.True(t, cfg.ExtractNonTranslatableContent())

	require.Error(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": "yes"}),
		"non-bool value must error")
}

// TestSchemaExposesExtractFlag confirms the format schema advertises the flag
// as a boolean defaulting to true.
func TestSchemaExposesExtractFlag(t *testing.T) {
	cfg := &vignette.Config{}
	s := cfg.Schema()
	require.NotNil(t, s)
	prop, ok := s.Properties["extractNonTranslatableContent"]
	require.True(t, ok, "schema must declare the extractNonTranslatableContent property")
	assert.Equal(t, "boolean", prop.Type)
	assert.Equal(t, true, prop.Default)
}

// TestNonTranslatableContentRoundtripByteIdentical is the byte-exactness proof:
// an untranslated round-trip produces the SAME output bytes whether the flag is
// on or off, because the surfaced non-translatable content rides a skeleton ref
// whose literal payload bytes equal the skeleton text it replaces.
func TestNonTranslatableContentRoundtripByteIdentical(t *testing.T) {
	for _, input := range []struct {
		name string
		doc  string
	}{
		{"plain", plainBilingualPair},
		{"html", simpleBilingualPair},
		{"complex", complexTwoPair},
		{"empty", minimalEmptyDoc},
	} {
		t.Run(input.name, func(t *testing.T) {
			on := vignetteRoundtripWithFlag(t, input.doc, true)
			off := vignetteRoundtripWithFlag(t, input.doc, false)
			assert.Equal(t, off, on,
				"surfacing non-translatable content must not change round-trip bytes")
		})
	}
}
