package messageformat_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/messageformat"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readBlocksWithFlag extracts blocks (no skeleton store) with
// ExtractNonTranslatableContent set explicitly.
func readBlocksWithFlag(t *testing.T, input string, extract bool) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := messageformat.NewReader()
	cfg := reader.Config().(*messageformat.Config)
	cfg.SetExtractNonTranslatableContent(extract)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()
	return testutil.FilterBlocks(testutil.CollectParts(t, reader.Read(ctx)))
}

func splitByTranslatable(blocks []*model.Block) (translatable, content []*model.Block) {
	for _, b := range blocks {
		if b.Translatable {
			translatable = append(translatable, b)
		} else {
			content = append(content, b)
		}
	}
	return translatable, content
}

// TestConfigDefaultExtractsNonTranslatableContent documents the opt-out
// default: a fresh config surfaces framing prose.
func TestConfigDefaultExtractsNonTranslatableContent(t *testing.T) {
	cfg := messageformat.NewReader().Config().(*messageformat.Config)
	assert.True(t, cfg.ExtractNonTranslatableContent(),
		"ExtractNonTranslatableContent should default to true")
}

// TestFrameSiblings_SurfacedAsContent verifies that the literal prose framing a
// plural picker ("You have " / " in your cart.") surfaces as non-translatable
// content blocks when the flag is on, while the branch bodies stay translatable.
func TestFrameSiblings_SurfacedAsContent(t *testing.T) {
	input := "You have {count, plural, one {# item} other {# items}} in your cart."

	translatable, content := splitByTranslatable(readBlocksWithFlag(t, input, true))

	require.Len(t, translatable, 2, "the plural branches stay translatable")
	for _, b := range translatable {
		assert.True(t, b.Translatable)
	}

	require.Len(t, content, 2, "both framing siblings surface as content blocks")
	assert.Equal(t, []string{"You have ", " in your cart."}, blockTexts(content))
	for _, b := range content {
		assert.False(t, b.Translatable, "framing prose must not be MT payload")
		assert.True(t, b.PreserveWhitespace, "framing prose rides as a verbatim run")
		assert.Empty(t, b.SemanticRole(), "plain prose carries no semantic role")
		// Single verbatim run, no inline placeholder parse.
		runs := b.SourceRuns()
		require.Len(t, runs, 1)
		require.NotNil(t, runs[0].Text)
		assert.Nil(t, runs[0].Ph)
	}
}

// TestFrameSiblings_SuppressedWhenFlagOff verifies that with the flag off the
// part stream contains only the prior translatable branch blocks — no content
// blocks — so parity (which forces the flag off) stays byte-identical.
func TestFrameSiblings_SuppressedWhenFlagOff(t *testing.T) {
	input := "You have {count, plural, one {# item} other {# items}} in your cart."

	translatable, content := splitByTranslatable(readBlocksWithFlag(t, input, false))

	assert.Empty(t, content, "no content blocks should be emitted with the flag off")
	require.Len(t, translatable, 2, "the two plural branches are still extracted")
}

// TestFrameSiblings_FlagOffMatchesPriorStream asserts the flag-off block stream
// is identical (ids and texts) to the translatable-only stream — i.e. the flag
// only adds content parts, it never changes the existing ones.
func TestFrameSiblings_FlagOffMatchesPriorStream(t *testing.T) {
	input := "{gender, select, male {He} female {She} other {They}} bought {count, plural, one {# item} other {# items}}."

	off := readBlocksWithFlag(t, input, false)
	onTranslatable, _ := splitByTranslatable(readBlocksWithFlag(t, input, true))

	require.Len(t, onTranslatable, len(off))
	for i := range off {
		assert.Equal(t, off[i].ID, onTranslatable[i].ID, "block %d id", i)
		assert.Equal(t, off[i].SourceText(), onTranslatable[i].SourceText(), "block %d text", i)
		assert.True(t, off[i].Translatable)
	}
}

// roundtripFlag runs read→write through the skeleton store with the flag set.
func roundtripFlag(t *testing.T, input string, extract bool) string {
	t.Helper()
	ctx := t.Context()

	reader := messageformat.NewReader()
	reader.Config().(*messageformat.Config).SetExtractNonTranslatableContent(extract)
	writer := messageformat.NewWriter()

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

// TestFrameSiblings_ByteExactRoundtrip verifies the framing-prose surfacing
// keeps the write round-trip byte-exact regardless of the flag: the raw line
// still rides the skeleton, and content blocks carry no skeleton ref.
func TestFrameSiblings_ByteExactRoundtrip(t *testing.T) {
	inputs := []string{
		"You have {count, plural, one {# item} other {# items}} in your cart.",
		"{gender, select, male {He} female {She} other {They}} bought {count, plural, one {# item} other {# items}}.",
		"{gender, select, male {He has {count, plural, one {# item} other {# items}}} other {They have {count, plural, one {# item} other {# items}}}}",
	}
	for _, input := range inputs {
		assert.Equal(t, input, roundtripFlag(t, input, true), "byte-exact with flag on: %s", input)
		assert.Equal(t, input, roundtripFlag(t, input, false), "byte-exact with flag off: %s", input)
	}
}

// TestFrameSiblings_SkeletonStreamHasContentBlocks confirms the content blocks
// are emitted in the skeleton (byte-exact) read path too, alongside the branch
// blocks, when the flag is on.
func TestFrameSiblings_SkeletonStreamHasContentBlocks(t *testing.T) {
	ctx := t.Context()
	input := "You have {count, plural, one {# item} other {# items}} in your cart."

	reader := messageformat.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	_, content := splitByTranslatable(testutil.FilterBlocks(parts))
	assert.Equal(t, []string{"You have ", " in your cart."}, blockTexts(content),
		"framing prose surfaces in the skeleton read path as well")
}

// TestApplyMap_ExtractNonTranslatableContent verifies the config key round-trips
// through ApplyMap and that unknown keys are still rejected.
func TestApplyMap_ExtractNonTranslatableContent(t *testing.T) {
	cfg := &messageformat.Config{}
	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	assert.False(t, cfg.ExtractNonTranslatableContent())

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": true}))
	assert.True(t, cfg.ExtractNonTranslatableContent())

	require.Error(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": "nope"}),
		"non-bool value must be rejected")
	require.Error(t, cfg.ApplyMap(map[string]any{"bogus": true}),
		"unknown keys must still be rejected")
}
