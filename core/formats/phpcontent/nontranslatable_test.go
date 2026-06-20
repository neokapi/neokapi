package phpcontent_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/phpcontent"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nonTranslatableBlocks returns every non-translatable Block in the slice.
func nonTranslatableBlocks(blocks []*model.Block) []*model.Block {
	var out []*model.Block
	for _, b := range blocks {
		if !b.Translatable {
			out = append(out, b)
		}
	}
	return out
}

// translatableTexts returns the source text of every translatable Block — the
// view machine translation sees. It must stay identical regardless of the
// extractNonTranslatableContent flag.
func translatableTexts(blocks []*model.Block) []string {
	out := []string{}
	for _, b := range blocks {
		if b.Translatable {
			out = append(out, b.SourceText())
		}
	}
	return out
}

// --- Config / schema surface ---

func TestExtractNonTranslatableContent_DefaultOn(t *testing.T) {
	t.Parallel()
	// Zero-value config (however constructed) has surfacing ON.
	var zero phpcontent.Config
	assert.True(t, zero.ExtractNonTranslatableContent(), "zero-value Config defaults to extraction on")

	cfg := &phpcontent.Config{}
	cfg.Reset()
	assert.True(t, cfg.ExtractNonTranslatableContent(), "Reset() keeps extraction on")
}

func TestExtractNonTranslatableContent_Setter(t *testing.T) {
	t.Parallel()
	cfg := &phpcontent.Config{}
	cfg.Reset()
	cfg.SetExtractNonTranslatableContent(false)
	assert.False(t, cfg.ExtractNonTranslatableContent())
	cfg.SetExtractNonTranslatableContent(true)
	assert.True(t, cfg.ExtractNonTranslatableContent())
}

func TestExtractNonTranslatableContent_ApplyMap(t *testing.T) {
	t.Parallel()
	cfg := &phpcontent.Config{}
	cfg.Reset()

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	assert.False(t, cfg.ExtractNonTranslatableContent())

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": true}))
	assert.True(t, cfg.ExtractNonTranslatableContent())

	require.Error(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": "yes"}))
}

func TestExtractNonTranslatableContent_Schema(t *testing.T) {
	t.Parallel()
	cfg := &phpcontent.Config{}
	s := cfg.Schema()
	require.NotNil(t, s)
	prop, ok := s.Properties["extractNonTranslatableContent"]
	require.True(t, ok, "schema declares extractNonTranslatableContent")
	assert.Equal(t, "boolean", prop.Type)
	assert.Equal(t, true, prop.Default)
}

// --- _skip_ directive (finding 2) ---

func TestExtractNonTranslatable_SkipDirective_DefaultOn(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n//_skip_\n$text = 'Skip this';\n$other = 'Keep this';"
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)

	// The skipped literal surfaces as a non-translatable RoleCode content block.
	skipped := blocks[0]
	assert.False(t, skipped.Translatable, "skipped literal must not be translatable")
	assert.Equal(t, "Skip this", skipped.SourceText())
	assert.Equal(t, model.RoleCode, skipped.SemanticRole())
	assert.True(t, skipped.PreserveWhitespace)

	// The next literal stays translatable. The MT-visible payload is unchanged.
	assert.True(t, blocks[1].Translatable)
	assert.Equal(t, "Keep this", blocks[1].SourceText())
	assert.Equal(t, []string{"Keep this"}, translatableTexts(blocks))
}

func TestExtractNonTranslatable_SkipDirectiveConcat_DefaultOn(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := phpcontent.NewReader()
	// Each part of the skipped concat chain surfaces as its own verbatim block;
	// the concat operator / quotes stay in skeleton (byte-exact, see roundtrip).
	input := "<?php\n//_skip_\n$text = 'Skip' . ' this';\n$other = 'Keep';"
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	nt := nonTranslatableBlocks(blocks)
	require.Len(t, nt, 2)
	assert.Equal(t, "Skip", nt[0].SourceText())
	assert.Equal(t, " this", nt[1].SourceText())
	for _, b := range nt {
		assert.Equal(t, model.RoleCode, b.SemanticRole())
		assert.True(t, b.PreserveWhitespace)
	}
	// MT-visible payload unchanged.
	assert.Equal(t, []string{"Keep"}, translatableTexts(blocks))
}

func TestExtractNonTranslatable_SkipDirective_FlagOff_StaysData(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := phpcontent.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	input := "<?php\n//_skip_\n$text = 'Skip this';\n$other = 'Keep this';"
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1, "flag off: only the kept literal is a Block")
	assert.Equal(t, "Keep this", blocks[0].SourceText())

	// Prior behavior for the _skip_ directive: the skipped literal is pure
	// skeleton text — it surfaces neither as a Block nor as a Data part.
	assert.Empty(t, nonTranslatableBlocks(blocks), "flag off: no non-translatable block")
	for _, p := range parts {
		if p.Type != model.PartData {
			continue
		}
		_, ok := p.Resource.(*model.Data).Properties["skipped"]
		assert.False(t, ok, "flag off: _skip_ literal must not surface as Data{skipped}")
	}

	// And the merge output stays byte-exact (the flag-off contract).
	assert.Equal(t, input, phpSkeletonRoundtripFlagOff(t, input))
}

// phpSkeletonRoundtripFlagOff is phpSkeletonRoundtrip with surfacing disabled.
func phpSkeletonRoundtripFlagOff(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := phpcontent.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	writer := phpcontent.NewWriter()

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

// --- _bskip_ / _eskip_ region (finding 1) ---

func TestExtractNonTranslatable_BSkipRegion_DefaultOn(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := phpcontent.NewReader()
	input := "<?php\n$a = 'Before';\n//_bskip_\n$b = 'Skip';\n//_eskip_\n$c = 'After';"
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 3)

	assert.True(t, blocks[0].Translatable)
	assert.Equal(t, "Before", blocks[0].SourceText())

	assert.False(t, blocks[1].Translatable, "literal inside _bskip_ must not be translatable")
	assert.Equal(t, "Skip", blocks[1].SourceText())
	assert.Equal(t, model.RoleCode, blocks[1].SemanticRole())
	assert.True(t, blocks[1].PreserveWhitespace)

	assert.True(t, blocks[2].Translatable)
	assert.Equal(t, "After", blocks[2].SourceText())

	assert.Equal(t, []string{"Before", "After"}, translatableTexts(blocks))
}

// --- _btext_ inclusion mode (finding 1, extractOutsideDirectives=false) ---

func TestExtractNonTranslatable_BTextOnly_DefaultOn(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := phpcontent.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{"extractOutsideDirectives": false}))
	input := "<?php\n$a = 'Outside1';\n//_btext_\n$b = 'Inside';\n//_etext_\n$c = 'Outside2';"
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 3)

	assert.False(t, blocks[0].Translatable)
	assert.Equal(t, "Outside1", blocks[0].SourceText())
	assert.Equal(t, model.RoleCode, blocks[0].SemanticRole())

	assert.True(t, blocks[1].Translatable)
	assert.Equal(t, "Inside", blocks[1].SourceText())

	assert.False(t, blocks[2].Translatable)
	assert.Equal(t, "Outside2", blocks[2].SourceText())
	assert.Equal(t, model.RoleCode, blocks[2].SemanticRole())

	// Only the _btext_ region is MT-visible.
	assert.Equal(t, []string{"Inside"}, translatableTexts(blocks))
}

func TestExtractNonTranslatable_BTextOnly_FlagOff_StaysData(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := phpcontent.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"extractOutsideDirectives":      false,
		"extractNonTranslatableContent": false,
	}))
	input := "<?php\n$a = 'Outside1';\n//_btext_\n$b = 'Inside';\n//_etext_\n$c = 'Outside2';"
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 1, "flag off: only the _btext_ literal is a Block")
	assert.Equal(t, "Inside", blocks[0].SourceText())
}

// --- Byte-exact round trip with the flag ON (skeleton path) ---

func TestExtractNonTranslatable_ByteExact_BSkipDoubleQuoted(t *testing.T) {
	t.Parallel()
	input := "<?php\n//_bskip_\n$b = \"Skip \\\"me\\\" now\";\n//_eskip_\n$c = 'Keep';"
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "double-quoted skipped literal must round-trip byte-exact")
}

func TestExtractNonTranslatable_ByteExact_SkipEscapedSingleQuote(t *testing.T) {
	t.Parallel()
	// Single-quoted parser unescapes \' and \\ in token.value; riding the RAW
	// body (not token.value) is what keeps this byte-exact.
	input := "<?php\n//_skip_\n$a = 'It\\'s skipped';\n$b = 'Keep';"
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "escaped single-quoted skipped literal must round-trip byte-exact")
}

func TestExtractNonTranslatable_ByteExact_SkipHeredoc(t *testing.T) {
	t.Parallel()
	input := "<?php\n//_bskip_\n$a = <<<EOT\nSkipped heredoc\nEOT;\n//_eskip_\n$b = 'Keep';\n"
	output := phpSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "heredoc skipped literal must round-trip byte-exact")
}

func TestExtractNonTranslatable_ByteExact_BTextOnly(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	input := "<?php\n$a = 'Outside1';\n//_btext_\n$b = 'Inside';\n//_etext_\n$c = 'Outside2';"

	reader := phpcontent.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{"extractOutsideDirectives": false}))
	writer := phpcontent.NewWriter()

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

	assert.Equal(t, input, buf.String(), "btext-only round trip must be byte-exact with surfacing on")
}
