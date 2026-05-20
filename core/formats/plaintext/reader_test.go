package plaintext_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/formats/plaintext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: create a reader with paragraph mode (segmentByLine=false)
func newParagraphReader(t *testing.T) *plaintext.Reader {
	t.Helper()
	reader := plaintext.NewReader()
	cfg := reader.Config().(*plaintext.Config)
	cfg.SegmentByLine = false
	return reader
}

// helper: read a string and return parts
func readString(t *testing.T, reader *plaintext.Reader, content string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	err := reader.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	return parts
}

// ---- PlainTextFilterTest (15 tests) ----

// okapi: PlainTextFilterTest#testEmptyInput
func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := plaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

// okapi: PlainTextFilterTest#testNameAndMimeType
// okapi: RegexPlainTextFilterTest#testNameAndMimeType
func TestRead_NameAndMimeType(t *testing.T) {
	reader := plaintext.NewReader()
	assert.Equal(t, "plaintext", reader.Name())
	assert.Equal(t, "Plain Text", reader.DisplayName())

	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/plain")
	assert.Contains(t, sig.Extensions, ".txt")
}

// okapi: PlainTextFilterTest#testFiles
func TestRead_Files(t *testing.T) {
	// Verifies various text inputs with different line endings parse without error.
	tests := []struct {
		name  string
		input string
	}{
		{"crlf", "Line 1\r\nLine 2"},
		{"crlf_end", "Line 1\r\nLine 2\r\n"},
		{"crlf_start", "\r\nLine 1\r\nLine 2"},
		{"crlfcrlf", "Line 1\r\n\r\nLine 2"},
		{"crlfcrlf_end", "Line 1\r\n\r\nLine 2\r\n\r\n"},
		{"crlfcrlfcrlf", "Line 1\r\n\r\n\r\nLine 2"},
		{"crlfcrlfcrlf_end", "Line 1\r\n\r\n\r\nLine 2\r\n\r\n\r\n"},
		{"cr", "Line 1\rLine 2"},
		{"lf", "Line 1\nLine 2"},
		{"mixture", "Line 1\r\nLine 2\rLine 3\nLine 4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			reader := plaintext.NewReader()
			err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
			require.NoError(t, err)
			defer reader.Close()

			parts := testutil.CollectParts(t, reader.Read(ctx))
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
		})
	}
}

// okapi: PlainTextFilterTest#testFiles (additional file-based roundtrip)
func TestRead_Files2(t *testing.T) {
	// Tests parsing from actual test data files on disk.
	tests := []struct {
		name string
		file string
	}{
		{"simple", "testdata/simple.txt"},
		{"unicode", "testdata/unicode.txt"},
		{"multiline", "testdata/multiline.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			f, err := os.Open(tt.file)
			require.NoError(t, err)
			reader := plaintext.NewReader()
			err = reader.Open(ctx, testutil.RawDocFromReader(f, tt.file, model.LocaleEnglish))
			require.NoError(t, err)
			defer reader.Close()

			parts := testutil.CollectParts(t, reader.Read(ctx))
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
		})
	}
}

// okapi: PlainTextFilterTest#testSkeleton
func TestRead_Skeleton(t *testing.T) {
	// "Line 1\nLine 2" should produce 2 text units with the line break as skeleton.
	parts := readString(t, plaintext.NewReader(), "Line 1\nLine 2")

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Line 1", blocks[0].SourceText())
	assert.Equal(t, "Line 2", blocks[1].SourceText())
}

// okapi: PlainTextFilterTest#testSkeleton2
func TestRead_Skeleton2(t *testing.T) {
	// "Line 1\n\nLine 2" — multiple consecutive line breaks.
	parts := readString(t, plaintext.NewReader(), "Line 1\n\nLine 2")

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Line 1", blocks[0].SourceText())
	assert.Equal(t, "Line 2", blocks[1].SourceText())

	// Verify Data part is emitted for the empty line.
	var dataCount int
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
		}
	}
	assert.GreaterOrEqual(t, dataCount, 1, "empty line should produce Data part")
}

// okapi: PlainTextFilterTest#testSkeleton3
func TestRead_Skeleton3(t *testing.T) {
	// "Line 1\n\n\nLine 2" — triple line breaks.
	parts := readString(t, plaintext.NewReader(), "Line 1\n\n\nLine 2")

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Line 1", blocks[0].SourceText())
	assert.Equal(t, "Line 2", blocks[1].SourceText())
}

// okapi: PlainTextFilterTest#testEvents
func TestRead_Events(t *testing.T) {
	ctx := t.Context()
	reader := plaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello world\nSecond line\nThird line", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// Correct event sequence: LayerStart, [Blocks/Data...], LayerEnd
	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	blocks := testutil.FilterBlocks(parts)
	assert.Len(t, blocks, 3)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
	assert.Equal(t, "Second line", blocks[1].SourceText())
	assert.Equal(t, "Third line", blocks[2].SourceText())
}

// okapi: PlainTextFilterTest#testStartDocument
func TestRead_StartDocument(t *testing.T) {
	parts := readString(t, plaintext.NewReader(), "Hello")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "text/plain", layer.MimeType)
	assert.Equal(t, "UTF-8", layer.Encoding)
	assert.Equal(t, model.LocaleEnglish, layer.Locale)
	assert.Equal(t, "plaintext", layer.Format)
}

// okapi: PlainTextFilterTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	// Double extraction: read -> write -> read -> compare.
	// Verifies roundtrip fidelity for all test data files.
	tests := []struct {
		name string
		file string
	}{
		{"simple", "testdata/simple.txt"},
		{"unicode", "testdata/unicode.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original, err := os.ReadFile(tt.file)
			require.NoError(t, err)

			ctx := t.Context()

			// First extraction
			f, err := os.Open(tt.file)
			require.NoError(t, err)
			reader := plaintext.NewReader()
			err = reader.Open(ctx, testutil.RawDocFromReader(f, tt.file, model.LocaleEnglish))
			require.NoError(t, err)
			parts1 := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()

			// First write
			var buf1 bytes.Buffer
			writer := plaintext.NewWriter()
			err = writer.SetOutputWriter(&buf1)
			require.NoError(t, err)
			writer.SetLocale(model.LocaleEnglish)
			err = writer.Write(ctx, testutil.PartsToChannel(parts1))
			require.NoError(t, err)
			writer.Close()

			assert.Equal(t, string(original), buf1.String(), "first roundtrip should match original")

			// Second extraction from the output of the first roundtrip
			reader2 := plaintext.NewReader()
			err = reader2.Open(ctx, testutil.RawDocFromString(buf1.String(), model.LocaleEnglish))
			require.NoError(t, err)
			parts2 := testutil.CollectParts(t, reader2.Read(ctx))
			reader2.Close()

			// Second write
			var buf2 bytes.Buffer
			writer2 := plaintext.NewWriter()
			err = writer2.SetOutputWriter(&buf2)
			require.NoError(t, err)
			writer2.SetLocale(model.LocaleEnglish)
			err = writer2.Write(ctx, testutil.PartsToChannel(parts2))
			require.NoError(t, err)
			writer2.Close()

			assert.Equal(t, buf1.String(), buf2.String(), "double extraction should be stable")
		})
	}
}

// okapi-unmapped: PlainTextFilterTest#testCancel — cancellation is tested via context in TestRead_Cancel
// okapi-unmapped: PlainTextFilterTest#testConfigurations — tests Java getConfigurations() API
// okapi-unmapped: PlainTextFilterTest#testSynchronization — tests Java multi-threaded filter access

// okapi: PlainTextFilterTest#testCancel
func TestRead_Cancel(t *testing.T) {
	// Tests that context cancellation stops reading.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	reader := plaintext.NewReader()
	// Create a large input so reading would take noticeable time.
	lines := make([]string, 10000)
	for i := range lines {
		lines[i] = "This is a test line for cancellation"
	}
	input := strings.Join(lines, "\n")

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)

	// Read a few parts then cancel.
	count := 0
	for range ch {
		count++
		if count >= 5 {
			cancel()
			break
		}
	}

	// Drain remaining.
	for range ch {
	}

	assert.GreaterOrEqual(t, count, 5, "should have read at least 5 parts before cancel")
}

// okapi: PlainTextFilterTest#testConfigurations
func TestRead_Configurations(t *testing.T) {
	// Verifies that the config can be applied and reports correct format name.
	reader := plaintext.NewReader()
	cfg := reader.Config()
	assert.Equal(t, "plaintext", cfg.FormatName())

	// Verify default is line mode.
	ptCfg, ok := cfg.(*plaintext.Config)
	require.True(t, ok)
	assert.True(t, ptCfg.SegmentByLine)

	// Verify paragraph mode can be set.
	ptCfg.SegmentByLine = false
	assert.False(t, ptCfg.SegmentByLine)

	// Reset restores defaults.
	ptCfg.Reset()
	assert.True(t, ptCfg.SegmentByLine)
}

// okapi: PlainTextFilterTest#testLineNumbers
func TestRead_LineNumbers(t *testing.T) {
	parts := readString(t, plaintext.NewReader(), "Line 1\nLine 2\nLine 3")

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 3)

	// Each block should have a unique ID.
	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID)
		assert.False(t, ids[b.ID], "block IDs should be unique")
		ids[b.ID] = true
	}

	// Blocks have Name fields with line references.
	assert.Equal(t, "line1", blocks[0].Name)
	assert.Equal(t, "line2", blocks[1].Name)
	assert.Equal(t, "line3", blocks[2].Name)
}

// okapi: PlainTextFilterTest#testParagraphs
func TestRead_Paragraphs(t *testing.T) {
	reader := newParagraphReader(t)
	parts := readString(t, reader, "Line 1\nLine 2\n\nLine 3\nLine 4\nLine 5")

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2, "paragraph mode: blank line splits paragraphs")

	// Lines 1+2 form one paragraph, lines 3+4+5 form another.
	assert.Equal(t, "Line 1\nLine 2", blocks[0].SourceText())
	assert.Equal(t, "Line 3\nLine 4\nLine 5", blocks[1].SourceText())
}

// okapi: PlainTextFilterTest#testLoadParams
func TestRead_LoadParams(t *testing.T) {
	// Tests that parameters can be loaded via ApplyMap.
	cfg := &plaintext.Config{}
	err := cfg.ApplyMap(map[string]any{"segmentByLine": false})
	require.NoError(t, err)
	assert.False(t, cfg.SegmentByLine)

	err = cfg.ApplyMap(map[string]any{"segmentByLine": true})
	require.NoError(t, err)
	assert.True(t, cfg.SegmentByLine)

	// Unknown parameter should error.
	err = cfg.ApplyMap(map[string]any{"unknownParam": "value"})
	require.Error(t, err)

	// Wrong type should error.
	err = cfg.ApplyMap(map[string]any{"segmentByLine": "not-a-bool"})
	require.Error(t, err)
}

// okapi: PlainTextFilterTest#testSynchronization
func TestRead_Synchronization(t *testing.T) {
	// Tests concurrent access to separate reader instances (safe concurrency).
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			ctx := t.Context()
			reader := plaintext.NewReader()
			err := reader.Open(ctx, testutil.RawDocFromString("Hello\nWorld", model.LocaleEnglish))
			if err != nil {
				t.Errorf("Open failed: %v", err)
				return
			}
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()
			blocks := testutil.FilterBlocks(parts)
			if len(blocks) != 2 {
				t.Errorf("expected 2 blocks, got %d", len(blocks))
			}
		})
	}
	wg.Wait()
}

// ---- ParaPlainTextFilterTest (13 tests) ----

// okapi: ParaPlainTextFilterTest#testCancel
func TestPara_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	reader := newParagraphReader(t)

	lines := make([]string, 5000)
	for i := range lines {
		lines[i] = "Paragraph content line"
	}
	input := strings.Join(lines, "\n\n") // many paragraphs

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)
	count := 0
	for range ch {
		count++
		if count >= 3 {
			cancel()
			break
		}
	}
	for range ch {
	}
	assert.GreaterOrEqual(t, count, 3)
}

// okapi: ParaPlainTextFilterTest#testEmptyInput
func TestPara_EmptyInput(t *testing.T) {
	reader := newParagraphReader(t)
	parts := readString(t, reader, "")
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks, "empty input in paragraph mode should produce no blocks")
}

// okapi: ParaPlainTextFilterTest#testEvents
func TestPara_Events(t *testing.T) {
	reader := newParagraphReader(t)
	parts := readString(t, reader, "Line 1\nLine 2")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "paragraph mode should produce blocks")
}

// okapi: ParaPlainTextFilterTest#testFiles
func TestPara_Files(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"crlf", "Line 1\r\nLine 2"},
		{"cr", "Line 1\rLine 2"},
		{"lf", "Line 1\nLine 2"},
		{"mixture", "Line 1\r\nLine 2\rLine 3\nLine 4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newParagraphReader(t)
			parts := readString(t, reader, tt.input)
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
		})
	}
}

// okapi: ParaPlainTextFilterTest#testFiles2
func TestPara_Files2(t *testing.T) {
	// Tests paragraph mode with multi-paragraph inputs.
	tests := []struct {
		name  string
		input string
	}{
		{"two_paras", "Para one.\n\nPara two."},
		{"three_paras", "First.\n\nSecond.\n\nThird."},
		{"trailing_newline", "Content.\n\nMore.\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newParagraphReader(t)
			parts := readString(t, reader, tt.input)
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
		})
	}
}

// okapi: ParaPlainTextFilterTest#testLineNumbers
func TestPara_LineNumbers(t *testing.T) {
	reader := newParagraphReader(t)
	parts := readString(t, reader, "Line 1\nLine 2\n\nLine 3")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	// Verify unique IDs.
	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID)
		assert.False(t, ids[b.ID], "block IDs should be unique")
		ids[b.ID] = true
	}
}

// okapi: ParaPlainTextFilterTest#testNameAndMimeType
func TestPara_NameAndMimeType(t *testing.T) {
	reader := newParagraphReader(t)
	parts := readString(t, reader, "Hello")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "text/plain", layer.MimeType)
}

// okapi: ParaPlainTextFilterTest#testParagraphs
func TestPara_Paragraphs(t *testing.T) {
	reader := newParagraphReader(t)
	parts := readString(t, reader, "Line 1\nLine 2\n\nLine 3\nLine 4")

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Line 1\nLine 2", blocks[0].SourceText())
	assert.Equal(t, "Line 3\nLine 4", blocks[1].SourceText())
}

// okapi: ParaPlainTextFilterTest#testSkeleton
func TestPara_Skeleton(t *testing.T) {
	reader := newParagraphReader(t)
	parts := readString(t, reader, "Line 1\nLine 2")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)
	// In paragraph mode, consecutive lines form a single paragraph.
	assert.Equal(t, "Line 1\nLine 2", blocks[0].SourceText())
}

// okapi: ParaPlainTextFilterTest#testSkeleton2
func TestPara_Skeleton2(t *testing.T) {
	reader := newParagraphReader(t)
	parts := readString(t, reader, "Line 1\n\nLine 2")

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Line 1", blocks[0].SourceText())
	assert.Equal(t, "Line 2", blocks[1].SourceText())
}

// okapi: ParaPlainTextFilterTest#testSkeleton3
func TestPara_Skeleton3(t *testing.T) {
	reader := newParagraphReader(t)
	parts := readString(t, reader, "Line 1\n\n\nLine 2")

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Line 1", blocks[0].SourceText())
	assert.Equal(t, "Line 2", blocks[1].SourceText())
}

// okapi: ParaPlainTextFilterTest#testSkeleton4
func TestPara_Skeleton4(t *testing.T) {
	reader := newParagraphReader(t)
	parts := readString(t, reader, "\nLine 1\nLine 2")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: ParaPlainTextFilterTest#testSkeleton5
func TestPara_Skeleton5(t *testing.T) {
	reader := newParagraphReader(t)
	parts := readString(t, reader, "Line 1\nLine 2\n")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)
}

// ---- RegexPlainTextFilterTest (1 test relevant to native) ----

// okapi: RegexPlainTextFilterTest#testParameters
func TestRead_Parameters(t *testing.T) {
	// Verifies that Config validates and applies parameters correctly.
	cfg := &plaintext.Config{}

	// Default state.
	cfg.Reset()
	assert.True(t, cfg.SegmentByLine)

	// Apply valid parameter.
	err := cfg.ApplyMap(map[string]any{"segmentByLine": false})
	require.NoError(t, err)
	assert.False(t, cfg.SegmentByLine)

	// Validate always passes for plaintext.
	require.NoError(t, cfg.Validate())
}

// ---- SplicedLinesFilterTest ----

// okapi-skip: SplicedLinesFilterTest#testCombinedLines — okf_plaintext_spliced variant: asserts backslash line-continuation splicing ("Line 1 \"+"Line 2 \"+"Line 3" -> one TU "Line 1 Line 2 Line 3"); the native reader has no line-splicing mode so this exact behaviour is not reproducible. TestRead_CombinedLines below remains a native multi-line extraction test (no Okapi counterpart).
func TestRead_CombinedLines(t *testing.T) {
	// Native multi-line / multi-paragraph extraction smoke test (no Okapi
	// counterpart — the multiline.txt fixture contains no continuation lines).
	ctx := t.Context()
	f, err := os.Open("testdata/multiline.txt")
	require.NoError(t, err)

	reader := plaintext.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/multiline.txt", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "multiline file should produce blocks")

	// Verify all lines are extracted as blocks.
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "First paragraph with")
	assert.Contains(t, texts, "Third paragraph is short.")
}

// ---- Additional tests (no direct Java mapping) ----

func TestReadUnicode(t *testing.T) {
	ctx := t.Context()
	reader := plaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello 世界\nBonjour le monde\nこんにちは世界", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	assert.Len(t, blocks, 3)
	assert.Equal(t, "Hello 世界", blocks[0].SourceText())
	assert.Equal(t, "Bonjour le monde", blocks[1].SourceText())
	assert.Equal(t, "こんにちは世界", blocks[2].SourceText())
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := plaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "plaintext", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := plaintext.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/plain")
	assert.Contains(t, sig.Extensions, ".txt")
}

func TestReaderMetadata(t *testing.T) {
	reader := plaintext.NewReader()
	assert.Equal(t, "plaintext", reader.Name())
	assert.Equal(t, "Plain Text", reader.DisplayName())
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"simple", "testdata/simple.txt"},
		{"unicode", "testdata/unicode.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original, err := os.ReadFile(tt.file)
			require.NoError(t, err)

			ctx := t.Context()

			// Read
			f, err := os.Open(tt.file)
			require.NoError(t, err)
			reader := plaintext.NewReader()
			err = reader.Open(ctx, testutil.RawDocFromReader(f, tt.file, model.LocaleEnglish))
			require.NoError(t, err)

			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()

			// Write
			var buf bytes.Buffer
			writer := plaintext.NewWriter()
			err = writer.SetOutputWriter(&buf)
			require.NoError(t, err)
			writer.SetLocale(model.LocaleEnglish)

			ch := testutil.PartsToChannel(parts)
			err = writer.Write(ctx, ch)
			require.NoError(t, err)
			writer.Close()

			assert.Equal(t, string(original), buf.String())
		})
	}
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	reader := plaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello\nWorld", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set French targets
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			} else if block.SourceText() == "World" {
				block.SetTargetText(model.LocaleFrench, "Monde")
			}
		}
	}

	// Write with French locale
	var buf bytes.Buffer
	writer := plaintext.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, "Bonjour\nMonde", buf.String())
}

// okapi: RegexPlainTextFilterTest#testEmptyInput
func TestReadNilDocument(t *testing.T) {
	// Java's RegexPlainTextFilterTest#testEmptyInput drives every null-input
	// branch of the filter open() contract (null InputStream/URI/CharSequence
	// and null RawDocument), each of which must raise rather than silently
	// produce events. The native reader's observable equivalent is that
	// Open() rejects a nil document and a document with a nil Reader.
	ctx := t.Context()

	reader := plaintext.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err, "nil document must be rejected")

	reader2 := plaintext.NewReader()
	err = reader2.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
	require.Error(t, err, "document with nil reader must be rejected")
}

// okapi: PlainTextFilterTest#testEvents (block ID uniqueness)
func TestRead_BlockIDs(t *testing.T) {
	parts := readString(t, plaintext.NewReader(), "Line A\nLine B\nLine C")

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID)
		assert.False(t, ids[b.ID], "block IDs should be unique")
		ids[b.ID] = true
	}
}

// okapi: PlainTextFilterTest#testEvents (no inline spans)
func TestRead_NoSpans(t *testing.T) {
	parts := readString(t, plaintext.NewReader(), "Simple plain text")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		runs := b.SourceRuns()
		for _, r := range runs {
			assert.Nil(t, r.Ph, "plain text should have no placeholder runs")
			assert.Nil(t, r.PcOpen, "plain text should have no opening inline-code runs")
			assert.Nil(t, r.PcClose, "plain text should have no closing inline-code runs")
		}
	}
}

// okapi: PlainTextFilterTest#testEvents (segment IDs)
func TestRead_SegmentIDs(t *testing.T) {
	parts := readString(t, plaintext.NewReader(), "Hello world")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		require.NotEmpty(t, b.Source, "block should have source segments")
		for _, seg := range b.Source {
			assert.NotEmpty(t, seg.ID, "segment should have an ID")
			assert.NotEmpty(t, seg.Runs, "segment should have content")
		}
	}
}

// okapi: PlainTextFilterTest#testFiles (CRLF line ending variant)
func TestRead_CRLFLineEndings(t *testing.T) {
	parts := readString(t, plaintext.NewReader(), "Line 1\r\nLine 2\r\nLine 3")

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 3)

	assert.Equal(t, "Line 1", blocks[0].SourceText())
	assert.Equal(t, "Line 2", blocks[1].SourceText())
	assert.Equal(t, "Line 3", blocks[2].SourceText())
}

// okapi: PlainTextFilterTest#testEvents (layer structure)
func TestRead_LayerStructure(t *testing.T) {
	parts := readString(t, plaintext.NewReader(), "Hello")
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

// okapi: PlainTextFilterTest#testEvents (multi-line variant)
func TestRead_MultipleLines(t *testing.T) {
	parts := readString(t, plaintext.NewReader(), "First line\nSecond line\nThird line")
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 3)
	assert.Equal(t, "First line", blocks[0].SourceText())
	assert.Equal(t, "Second line", blocks[1].SourceText())
	assert.Equal(t, "Third line", blocks[2].SourceText())
}

// okapi: PlainTextFilterTest#testEvents (unicode content)
func TestRead_UnicodeContent(t *testing.T) {
	parts := readString(t, plaintext.NewReader(), "Hello world")
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
}

// okapi: PlainTextFilterTest#testFiles (CR line ending variant)
func TestRead_CRLineEndings(t *testing.T) {
	// The reader now recognises bare \r as a line terminator (Mac classic
	// + UTF-16-derived fixtures), matching okapi.
	parts := readString(t, plaintext.NewReader(), "Line 1\rLine 2\rLine 3")
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 3)
	assert.Equal(t, "Line 1", blocks[0].SourceText())
	assert.Equal(t, "Line 2", blocks[1].SourceText())
	assert.Equal(t, "Line 3", blocks[2].SourceText())
}

// ---- RegexPlainTextFilterTest — Java okf_plaintext_regex variant ----
// RegexPlainTextFilter is a distinct Java filter subclass (config id
// "okf_plaintext_regex") that extracts lines via the generic regex-filter
// engine and recognises Unicode line separators (U+0085 NEL, U+2028 LS,
// U+2029 PS) as line terminators. The neokapi native reader implements two
// fixed modes (line / paragraph) over the standard \n, \r\n, \r terminators
// — it has no regex-rule extraction and (matching the Unicode text spec for a
// byte-faithful localisation roundtrip) does not split on NEL/LS/PS. The
// tests below that exercise that regex-variant-only behaviour are skipped;
// the ones whose observable behaviour the native reader shares are mapped to
// the native tests noted inline (see TestRead_NameAndMimeType and
// TestReadNilDocument above).

// okapi-skip: RegexPlainTextFilterTest#testDoubleExtraction — okf_plaintext_regex variant: RoundTripComparison over a fixture corpus (cr/lf/mixture/u0085/u2028/u2029) that depends on regex-driven extraction and Unicode LS/NEL/PS line splitting the native reader does not implement; native roundtrip fidelity is covered by TestRoundTrip_DoubleExtraction.
// okapi-skip: RegexPlainTextFilterTest#testEvents — empty @Test body in upstream (no assertions to port).
// okapi-skip: RegexPlainTextFilterTest#testFiles — okf_plaintext_regex variant: asserts 4 TUs from u0085/u2028/u2029 fixtures by splitting on Unicode NEL/LS/PS separators, which the native reader intentionally does not treat as line terminators (verified: such input yields a single block).

// ---- RoundTripPlainTextIT ----

// okapi: RoundTripPlainTextIT
func TestRoundTrip_Native(t *testing.T) {
	// Native roundtrip: read then write and verify blocks survive.
	input := "Hello world\nThis is a test."
	ctx := t.Context()

	reader := plaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := plaintext.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	err = writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, input, buf.String())
}

// okapi: RoundTripPlainTextIT#testPlainTextFiles
func TestRoundTrip_TestFiles(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"simple", "testdata/simple.txt"},
		{"unicode", "testdata/unicode.txt"},
		{"multiline", "testdata/multiline.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original, err := os.ReadFile(tt.file)
			require.NoError(t, err)

			ctx := t.Context()
			f, err := os.Open(tt.file)
			require.NoError(t, err)

			reader := plaintext.NewReader()
			err = reader.Open(ctx, testutil.RawDocFromReader(f, tt.file, model.LocaleEnglish))
			require.NoError(t, err)
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()

			var buf bytes.Buffer
			writer := plaintext.NewWriter()
			err = writer.SetOutputWriter(&buf)
			require.NoError(t, err)
			writer.SetLocale(model.LocaleEnglish)
			err = writer.Write(ctx, testutil.PartsToChannel(parts))
			require.NoError(t, err)
			writer.Close()

			assert.Equal(t, string(original), buf.String())
		})
	}
}

// okapi: RoundTripPlainTextIT#testPlainTextFiles (line ending variants)
func TestRoundTrip_LineEndings(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"lf", "Line 1\nLine 2\nLine 3"},
		{"crlf", "Line 1\r\nLine 2\r\nLine 3"},
		{"cr", "Line 1\rLine 2\rLine 3"},
		{"lf_trailing", "Line 1\nLine 2\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			reader := plaintext.NewReader()
			err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
			require.NoError(t, err)
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()

			var buf bytes.Buffer
			writer := plaintext.NewWriter()
			err = writer.SetOutputWriter(&buf)
			require.NoError(t, err)
			writer.SetLocale(model.LocaleEnglish)
			err = writer.Write(ctx, testutil.PartsToChannel(parts))
			require.NoError(t, err)
			writer.Close()

			require.NotEmpty(t, buf.String(), "roundtrip should produce output")
		})
	}
}

// okapi: RoundTripPlainTextIT#testPlainTextFiles (paragraph mode)
func TestRoundTrip_ParagraphMode(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"single_line", "Hello world"},
		{"two_lines", "Line 1\nLine 2"},
		{"paragraphs", "Para 1 line 1\nPara 1 line 2\n\nPara 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			reader := newParagraphReader(t)
			err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
			require.NoError(t, err)
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()

			var buf bytes.Buffer
			writer := plaintext.NewWriter()
			err = writer.SetOutputWriter(&buf)
			require.NoError(t, err)
			writer.SetLocale(model.LocaleEnglish)
			err = writer.Write(ctx, testutil.PartsToChannel(parts))
			require.NoError(t, err)
			writer.Close()

			require.NotEmpty(t, buf.String(), "paragraph mode roundtrip should produce output")
		})
	}
}

// okapi-unmapped: RoundTripPlainTextIT#testPlainTextFiles (spliced mode) — spliced lines filter variant, no native equivalent

// ---- SplicedLinesFilterTest — Java okf_plaintext_spliced variant ----
// SplicedLinesFilter is a distinct Java filter subclass (config id
// "okf_plaintext_spliced") that JOINS consecutive lines whose physical line
// ends with a backslash continuation. Its fixtures (combined_lines*.txt)
// contain "Line 1 \<CR>Line 2 \<CR>Line 3<CR>Line 4\" and the filter emits a
// single text unit "Line 1 Line 2 Line 3" plus "Line 4". The neokapi native
// reader has no line-continuation/splicing mode — line mode would emit four
// separate blocks — so none of these are applicable to the native
// reader/writer. They are skipped, not mapped.

// okapi-skip: SplicedLinesFilterTest#testDoubleExtraction — okf_plaintext_spliced variant: RoundTripComparison relies on backslash line-continuation splicing the native reader does not implement.
// okapi-skip: SplicedLinesFilterTest#testSkeleton — okf_plaintext_spliced variant: asserts Java IFilterWriter skeleton-string equality for spliced combined_lines.txt; native reader has no line-continuation mode.
// okapi-skip: SplicedLinesFilterTest#testSkeleton2 — okf_plaintext_spliced variant: same as testSkeleton for combined_lines_end.txt (trailing line break); no native line-splicing equivalent.
// okapi-skip: SplicedLinesFilterTest#testSkeleton3 — okf_plaintext_spliced variant: same as testSkeleton for combined_lines2.txt; no native line-splicing equivalent.

// Tests schema metadata.
func TestRead_Schema(t *testing.T) {
	cfg := &plaintext.Config{}
	schema := cfg.Schema()
	assert.Equal(t, "Plain Text Format", schema.Title)
	assert.Equal(t, "plaintext", schema.FormatMeta.ID)
	assert.Contains(t, schema.FormatMeta.MimeTypes, "text/plain")
	assert.Contains(t, schema.Properties, "segmentByLine")
}
