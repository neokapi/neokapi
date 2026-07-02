package plaintext_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/plaintext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for the paragraph and splice-lines modes absorbed from the
// retired paraplaintext and splicedlines format packages (#1074): the
// same behaviors are now config options on the plaintext format.

// configureReader builds a plaintext reader with the given config map
// applied (exercising the public ApplyMap surface, like recipe configs do).
func configureReader(t *testing.T, values map[string]any) *plaintext.Reader {
	t.Helper()
	reader := plaintext.NewReader()
	cfg := reader.Config().(*plaintext.Config)
	require.NoError(t, cfg.ApplyMap(values))
	require.NoError(t, cfg.Validate())
	return reader
}

func readBlocksWithConfig(t *testing.T, input string, values map[string]any) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := configureReader(t, values)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()
	return testutil.CollectBlocks(t, reader.Read(ctx))
}

// skeletonRoundtripWithConfig runs the full read → (mutate) → write cycle
// through a skeleton store and returns the reconstructed output.
func skeletonRoundtripWithConfig(t *testing.T, input string, values map[string]any, locale model.LocaleID, mutate func([]*model.Part)) string {
	t.Helper()
	ctx := t.Context()

	reader := configureReader(t, values)
	writer := plaintext.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	if mutate != nil {
		mutate(parts)
	}

	var buf bytes.Buffer
	if !locale.IsEmpty() {
		writer.SetLocale(locale)
	}
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	return buf.String()
}

// --- Paragraph mode (absorbed paraplaintext behavior) ---

func TestParagraphs_Config_ExtractsParagraphBlocks(t *testing.T) {
	input := "Para one line 1\nPara one line 2\n\nPara two\n"
	blocks := readBlocksWithConfig(t, input, map[string]any{"paragraphs": true})
	require.Len(t, blocks, 2)
	assert.Equal(t, "Para one line 1\nPara one line 2", blocks[0].SourceText())
	assert.Equal(t, "Para two", blocks[1].SourceText())
}

func TestParagraphs_Roundtrip_BlankLinePreservation(t *testing.T) {
	inputs := []string{
		"Para one\n\nPara two",
		"Para one\n\n\n\nPara two\n",
		"\n\nLeading blanks\n\nTrailing blanks\n\n\n",
		"Line 1\r\nLine 2\r\n\r\nLine 3\r\n",
	}
	for _, input := range inputs {
		output := skeletonRoundtripWithConfig(t, input, map[string]any{"paragraphs": true}, "", nil)
		assert.Equal(t, input, output, "paragraph mode roundtrip should preserve blank lines byte-exact")
	}
}

func TestParagraphs_Roundtrip_WithTranslation(t *testing.T) {
	input := "Hello world\nsecond line\n\nGoodbye\n"
	locale := model.LocaleID("fr")
	output := skeletonRoundtripWithConfig(t, input, map[string]any{"paragraphs": true}, locale, func(parts []*model.Part) {
		for _, p := range parts {
			if p.Type != model.PartBlock {
				continue
			}
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Hello world\nsecond line":
				b.SetTargetText(locale, "Bonjour le monde\ndeuxième ligne")
			case "Goodbye":
				b.SetTargetText(locale, "Au revoir")
			}
		}
	})
	assert.Equal(t, "Bonjour le monde\ndeuxième ligne\n\nAu revoir\n", output)
}

func TestParagraphs_MutuallyExclusiveWithSpliceLines(t *testing.T) {
	cfg := plaintext.NewReader().Config().(*plaintext.Config)
	require.NoError(t, cfg.ApplyMap(map[string]any{"paragraphs": true, "spliceLines": true}))
	assert.Error(t, cfg.Validate())
}

// --- Splice mode (absorbed splicedlines behavior) ---

// TestSplice_CombinedLines mirrors the classic combined_lines fixture
// behavior of Okapi's okf_splicedlines (SplicedLinesFilterTest
// #testCombinedLines): backslash-terminated lines join with the next
// line into one text unit.
func TestSplice_CombinedLines(t *testing.T) {
	input := "First part \\\nsecond part\nStandalone line\nA \\\nB \\\nC\n"
	blocks := readBlocksWithConfig(t, input, map[string]any{"spliceLines": true})
	require.Len(t, blocks, 3)
	assert.Equal(t, "First part \nsecond part", blocks[0].SourceText())
	assert.Equal(t, "Standalone line", blocks[1].SourceText())
	assert.Equal(t, "A \nB \nC", blocks[2].SourceText())
}

func TestSplice_Roundtrip_ByteExact(t *testing.T) {
	inputs := []string{
		"First part \\\nsecond part\nStandalone line\nA \\\nB \\\nC\n",
		"Line 1 \\\r\nLine 2\r\nPlain\r\n",
		"No continuation here",
		"Blank lines\n\n\nkept \\\nintact\n",
		"Trailing splicer \\",
		"Trailing splicer with newline \\\n",
	}
	for _, input := range inputs {
		output := skeletonRoundtripWithConfig(t, input, map[string]any{"spliceLines": true}, "", nil)
		assert.Equal(t, input, output, "splice mode roundtrip should be byte-exact")
	}
}

func TestSplice_Roundtrip_WithTranslation(t *testing.T) {
	input := "Hello \\\nworld\nGoodbye\n"
	locale := model.LocaleID("de")
	output := skeletonRoundtripWithConfig(t, input, map[string]any{"spliceLines": true}, locale, func(parts []*model.Part) {
		for _, p := range parts {
			if p.Type != model.PartBlock {
				continue
			}
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Hello \nworld":
				b.SetTargetText(locale, "Hallo \nWelt")
			case "Goodbye":
				b.SetTargetText(locale, "Auf Wiedersehen")
			}
		}
	})
	// The multi-line target is re-spliced with the continuation marker.
	assert.Equal(t, "Hallo \\\nWelt\nAuf Wiedersehen\n", output)
}

func TestSplice_CustomMarker(t *testing.T) {
	input := "First part _\nsecond part\nPlain line\n"
	values := map[string]any{"spliceLines": true, "spliceMarker": "_"}

	blocks := readBlocksWithConfig(t, input, values)
	require.Len(t, blocks, 2)
	assert.Equal(t, "First part \nsecond part", blocks[0].SourceText())
	assert.Equal(t, "Plain line", blocks[1].SourceText())

	output := skeletonRoundtripWithConfig(t, input, values, "", nil)
	assert.Equal(t, input, output, "custom marker roundtrip should be byte-exact")
}

func TestSplice_NonSkeletonWrite_ReaddsMarkers(t *testing.T) {
	ctx := t.Context()
	input := "One \\\ntwo\nThree"
	reader := configureReader(t, map[string]any{"spliceLines": true})
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	writer := plaintext.NewWriter()
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	assert.Equal(t, "One \\\ntwo\nThree", buf.String())
}

func TestSplice_LegacySegmentByLineStillWorks(t *testing.T) {
	// segmentByLine=false remains the compatibility spelling for
	// paragraph mode.
	blocks := readBlocksWithConfig(t, "P1\n\nP2", map[string]any{"segmentByLine": false})
	require.Len(t, blocks, 2)
	assert.Equal(t, "P1", blocks[0].SourceText())
	assert.Equal(t, "P2", blocks[1].SourceText())
}
