package transtable_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/transtable"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimalHeader = "TransTableV1\ten\tfr\n"

// okapi: TransTableFilterTest#testStartDocument
// Verifies LayerStart/LayerEnd wraps transtable content and the
// layer's source locale is taken from the header.
func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := minimalHeader + "\"okpCtx:tu=1\"\t\"hello\"\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "transtable", layer.Format)
	assert.Equal(t, model.LocaleID("en"), layer.Locale)
}

// okapi: TransTableFilterTest#testMinimalInput
// Single source-only data row → one block, no target.
func TestSourceOnlyEntry(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := minimalHeader + "\"okpCtx:tu=1\"\t\"source\"\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "source", blocks[0].SourceText())
	assert.Equal(t, "1", blocks[0].Properties["tu_id"])
	assert.False(t, blocks[0].HasTarget(model.LocaleFrench), "source-only row should leave target empty")
}

// okapi: TransTableFilterTest#testMinimalSourceTarget
// Three-cell row → bilingual block.
func TestBilingualPair(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := minimalHeader + "\"okpCtx:tu=1\"\t\"source\"\t\"target\"\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "source", blocks[0].SourceText())
	assert.Equal(t, "target", blocks[0].TargetText(model.LocaleFrench))
}

// okapi: TransTableFilterTest#testQuotesInput
// Quotes around cells are optional.
func TestUnquotedCells(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "\"TransTableV1\"\t\"en\"\t\"fr\"\nokpCtx:tu=1\tsource\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "source", blocks[0].SourceText())
}

// okapi: TransTableFilterTest#testUnSegmented
// Distinct tu=ids each get their own text unit.
func TestMultipleEntries(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "\"TransTableV1\"\t\"en\"\t\"fr\"\n" +
		"okpCtx:tu=1:s=0\tsource1\n" +
		"okpCtx:tu=2\tsource2\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "source1", blocks[0].SourceText())
	assert.Equal(t, "source2", blocks[1].SourceText())
	assert.Equal(t, "1", blocks[0].Properties["tu_id"])
	assert.Equal(t, "2", blocks[1].Properties["tu_id"])
}

// okapi: TransTableFilterTest#testSegmented
// Rows sharing tu=<id> + :s=<seg-id> merge into one segmented text unit.
func TestSegmentedTextUnit(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "\"TransTableV1\"\t\"en\"\t\"fr\"\n" +
		"okpCtx:tu=1:s=0\tsource1\n" +
		"okpCtx:tu=2:s=0\tsrc2-seg0\n" +
		"okpCtx:tu=2:s=1\tsrc2-seg1\n" +
		"okpCtx:tu=2:s=2\tsrc2-seg2\n" +
		"okpCtx:tu=3:s=ZZZ\tsrc3-segZZZ\n" +
		"okpCtx:tu=4\tsrc4-seg0\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 4)
	assert.Equal(t, "source1", blocks[0].SourceText())
	assert.Equal(t, "src2-seg0src2-seg1src2-seg2", blocks[1].SourceText())
	assert.Equal(t, "src3-segZZZ", blocks[2].SourceText())
	assert.Equal(t, "src4-seg0", blocks[3].SourceText())

	// tu=2 should have three segments after grouping. In the Run model
	// the segmentation rides as a stand-off overlay over the single
	// source run sequence; assert the three spans carry the seg ids and
	// split the source back into the original three cells.
	seg := blocks[1].SourceSegmentation()
	require.NotNil(t, seg)
	require.Len(t, seg.Spans, 3)
	assert.Equal(t, "0", seg.Spans[0].ID)
	assert.Equal(t, "1", seg.Spans[1].ID)
	assert.Equal(t, "2", seg.Spans[2].ID)
	assert.Equal(t, "src2-seg0", model.RunsText(blocks[1].SourceSegmentRuns(0)))
	assert.Equal(t, "src2-seg1", model.RunsText(blocks[1].SourceSegmentRuns(1)))
	assert.Equal(t, "src2-seg2", model.RunsText(blocks[1].SourceSegmentRuns(2)))
}

// okapi: TransTableFilterTest#testSegmentedWithTarget
// Whitespace lines do not break segment grouping.
func TestWhitespaceLinesSkipped(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := "\"TransTableV1\"\t\"en\"\t\"fr\"\n" +
		"okpCtx:tu=1:s=0\tsource1\ttarget1\n" +
		"okpCtx:tu=2:s=0\tsrc2-seg0\n" +
		"\n" +
		"  \n" +
		"\t\n" +
		"okpCtx:tu=2:s=1\tsrc2-seg1\ttrg2-seg1\n" +
		"okpCtx:tu=2:s=2\tsrc2-seg2\n" +
		"okpCtx:tu=3:s=ZZZ\tsrc3-segZZZ\n" +
		"okpCtx:tu=4\tsrc4-seg0\ttrg4-seg0\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 4)
	assert.Equal(t, "source1", blocks[0].SourceText())
	assert.Equal(t, "src2-seg0src2-seg1src2-seg2", blocks[1].SourceText())
	assert.Equal(t, "src3-segZZZ", blocks[2].SourceText())
	assert.Equal(t, "src4-seg0", blocks[3].SourceText())

	// Whitespace lines should not surface as Data parts.
	for _, p := range parts {
		assert.NotEqual(t, model.PartData, p.Type, "whitespace lines must be absorbed silently")
	}
}

// okapi: TransTableFilterTest#testQuotesInput
// Embedded \t and \n escapes are unescaped during parse.
func TestEscapedTabAndNewlineInValue(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	input := minimalHeader + `"okpCtx:tu=1"	"line1\nline2"	"col\tA"` + "\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "line1\nline2", blocks[0].SourceText())
	assert.Equal(t, "col\tA", blocks[0].TargetText(model.LocaleFrench))
}

// allowSegments=false collapses :s=<id> back into per-row text units.
func TestAllowSegmentsFalse(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{"allowSegments": false}))
	input := minimalHeader +
		"okpCtx:tu=1:s=0\tA\n" +
		"okpCtx:tu=1:s=1\tB\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2, "with allowSegments=false each row is its own text unit")
	assert.Equal(t, "A", blocks[0].SourceText())
	assert.Equal(t, "B", blocks[1].SourceText())
}

// Empty input: just LayerStart/End.
func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

// Invalid signature surfaces as an error PartResult.
func TestInvalidSignature(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("NotATransTable\ten\tfr\n", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	var sawErr bool
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			sawErr = true
			break
		}
	}
	assert.True(t, sawErr, "malformed header should yield an error PartResult")
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := transtable.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReaderSignature(t *testing.T) {
	reader := transtable.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/x-transtable")
}

func TestReaderMetadata(t *testing.T) {
	reader := transtable.NewReader()
	assert.Equal(t, "transtable", reader.Name())
	assert.Equal(t, "Translation Table", reader.DisplayName())
}

func TestConfigFormatName(t *testing.T) {
	cfg := &transtable.Config{}
	assert.Equal(t, "transtable", cfg.FormatName())
}

func TestConfigValidate(t *testing.T) {
	cfg := &transtable.Config{}
	require.NoError(t, cfg.Validate())
}

func TestConfigReset(t *testing.T) {
	cfg := &transtable.Config{}
	cfg.Reset()
	assert.True(t, cfg.AllowSegments, "default allowSegments should be true")
}

func TestConfigApplyMapUnknown(t *testing.T) {
	cfg := &transtable.Config{}
	err := cfg.ApplyMap(map[string]any{"unknown": true})
	require.Error(t, err)
}

func TestConfigApplyMapAllowSegments(t *testing.T) {
	cfg := &transtable.Config{}
	cfg.Reset()
	require.NoError(t, cfg.ApplyMap(map[string]any{"allowSegments": false}))
	assert.False(t, cfg.AllowSegments)
}

func TestConfigApplyMapAllowSegmentsBadType(t *testing.T) {
	cfg := &transtable.Config{}
	err := cfg.ApplyMap(map[string]any{"allowSegments": "yes"})
	require.Error(t, err)
}

// Round-trip: read TransTable v1 input, then write it back targeting
// the same locale and verify the rendered rows preserve the source +
// target cells (not necessarily byte-exact — that's covered by the
// skeleton-store tests).
//
// okapi: RoundTripTranstableIT#transtableFiles — native extract→write over a real TransTable document, asserting source+target cells survive; Okapi's transtableFiles does extract→merge→compare over a corpus.
// okapi-skip: RoundTripTranstableIT#transtableSerializedFiles — Okapi serialized-skeleton variant; native uses its own skeleton store, not Okapi's serialized event/skeleton format.
func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	input := "TransTableV1\ten\tfr\n" +
		"\"okpCtx:tu=1\"\t\"Hello\"\t\"\"\n" +
		"\"okpCtx:tu=2\"\t\"Goodbye\"\t\"\"\n"
	reader := transtable.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

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

	var buf bytes.Buffer
	writer := transtable.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "TransTableV1\ten\tfr")
	assert.Contains(t, output, `"okpCtx:tu=1"`+"\t"+`"Hello"`+"\t"+`"Bonjour"`)
	assert.Contains(t, output, `"okpCtx:tu=2"`+"\t"+`"Goodbye"`+"\t"+`"Au revoir"`)
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately

	reader := transtable.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(minimalHeader+"okpCtx:tu=1\tHello\nokpCtx:tu=2\tGoodbye\n", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)
	var count int
	for range ch {
		count++
	}
	assert.LessOrEqual(t, count, 5)
}

func TestWriterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	var buf bytes.Buffer
	writer := transtable.NewWriter()
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := make(chan *model.Part)
	cancel()

	err = writer.Write(ctx, ch)
	assert.ErrorIs(t, err, context.Canceled)
}

// Regression: read the upstream Okapi `test01.xml.txt` fixture from
// okapi-testdata and verify the six text units come through with the
// expected sources and no targets. Skips cleanly when the corpus
// hasn't been fetched.
//
// okapi: TranstableXliffCompareIT#transtableXliffCompareFiles — extracts the actual upstream Okapi transtable fixture and asserts the text units match the expected sources; Okapi's transtableXliffCompareFiles extracts to XLIFF and compares against a gold XLIFF corpus. Corpus-gated: t.Skip()s (→ pending) when okapi-testdata is unfetched.
func TestReadOkapiTest01Fixture(t *testing.T) {
	ctx := t.Context()
	root, err := spec.FindOkapiTestdataRoot()
	if err != nil {
		t.Skipf("okapi-testdata not available: %v", err)
		return
	}
	path := filepath.Join(root, "okapi", "filters", "transtable", "src", "test", "resources", "test01.xml.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Skipf("upstream fixture not present: %v", err)
			return
		}
		t.Fatalf("read fixture: %v", err)
	}

	reader := transtable.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromReader(bytes.NewReader(data), path, model.LocaleEnglish)))
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 6)
	wantSources := []string{
		"Text of the first record",
		"Description of the first record",
		"  Path:",
		"Path of the file to process",
		"Text of the third record",
		"Description of the first record",
	}
	for i, want := range wantSources {
		assert.Equal(t, want, blocks[i].SourceText(), "block %d source", i)
	}
}
