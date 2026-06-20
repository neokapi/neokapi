package doxygen_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/doxygen"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func readDoxygen(t *testing.T, input string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := doxygen.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

func readDoxygenConfig(t *testing.T, input string, configure func(*doxygen.Config)) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := doxygen.NewReader()
	if configure != nil {
		configure(reader.Config().(*doxygen.Config))
	}
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

func collectBlocks(parts []*model.Part) []*model.Block {
	return testutil.FilterBlocks(parts)
}

func blockTexts(blocks []*model.Block) []string {
	var texts []string
	for _, b := range blocks {
		texts = append(texts, b.SourceText())
	}
	return texts
}

func blockTextsContain(texts []string, substr string) bool {
	for _, txt := range texts {
		if strings.Contains(txt, substr) {
			return true
		}
	}
	return false
}

// translatableBlocks returns only the blocks an MT step would touch
// (Translatable=true), filtering out the non-translatable RoleCode content
// blocks the reader surfaces for \code/\verbatim/… region bodies.
func translatableBlocks(blocks []*model.Block) []*model.Block {
	var out []*model.Block
	for _, b := range blocks {
		if b.Translatable {
			out = append(out, b)
		}
	}
	return out
}

// nonTranslatableBlocks returns only the surfaced content blocks
// (Translatable=false).
func nonTranslatableBlocks(blocks []*model.Block) []*model.Block {
	var out []*model.Block
	for _, b := range blocks {
		if !b.Translatable {
			out = append(out, b)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// DoxygenFilterTest — extraction tests
// ---------------------------------------------------------------------------

// okapi: DoxygenFilterTest#testDefaultInfo
func TestExtract_DefaultInfo(t *testing.T) {
	parts := readDoxygen(t, "/// A comment\n")
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "text/x-doxygen-txt", layer.MimeType)
}

// okapi: DoxygenFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	parts := readDoxygen(t, "/// Hello\n")
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "text/x-doxygen-txt", layer.MimeType)
	assert.Equal(t, model.LocaleEnglish, layer.Locale)
}

// okapi: DoxygenFilterTest#testSimpleLine
func TestExtract_SimpleLine(t *testing.T) {
	parts := readDoxygen(t, "/// A simple line comment\n")
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from /// comment")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "A simple line comment"),
		"should extract 'A simple line comment', got %v", texts)
}

// okapi: DoxygenFilterTest#testMultipleLines
func TestExtract_MultipleLines(t *testing.T) {
	input := "/// First line\n/// Second line\n/// Third line\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from multi-line /// comments")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "First line"),
		"should contain 'First line', got %v", texts)
	assert.True(t, blockTextsContain(texts, "Second line"),
		"should contain 'Second line', got %v", texts)
}

// okapi: DoxygenFilterTest#testOneLiner
func TestExtract_OneLiner(t *testing.T) {
	input := "int x; ///< A one-liner comment\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text from ///< one-liner")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "A one-liner comment"),
		"should contain 'A one-liner comment', got %v", texts)
}

// okapi: DoxygenFilterTest#testBlankOneLiner
func TestExtract_BlankOneLiner(t *testing.T) {
	input := "int x; ///<\n"
	parts := readDoxygen(t, input)
	require.NotEmpty(t, parts, "should produce parts without error")
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: DoxygenFilterTest#testJavadocLine
func TestExtract_JavadocLine(t *testing.T) {
	input := "/** A Javadoc comment */\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text from /** */ comment")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "A Javadoc comment"),
		"should contain 'A Javadoc comment', got %v", texts)
}

// okapi: DoxygenFilterTest#testJavadocMultiline
func TestExtract_JavadocMultiline(t *testing.T) {
	input := "/**\n * First line\n * Second line\n */\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text from multi-line /** */ comment")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "First line"),
		"should contain 'First line', got %v", texts)
}

// okapi: DoxygenFilterTest#testDoxygenClassCommand1
func TestExtract_ClassCommand1(t *testing.T) {
	input := "/// \\class MyClass\n/// Brief description.\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text after \\class command")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "Brief description"),
		"should contain 'Brief description', got %v", texts)
}

// okapi: DoxygenFilterTest#testDoxygenClassCommand2
func TestExtract_ClassCommand2(t *testing.T) {
	input := "/*! \\class Test class.h \"inc/class.h\"\n *  \\brief This is a test class.\n *\n * Some details about the Test class\n */\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text from \\class command variant 2")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "This is a test class"),
		"should contain 'This is a test class', got %v", texts)
}

// okapi: DoxygenFilterTest#testDoxygenCodeCommand
func TestExtract_CodeCommand(t *testing.T) {
	input := "/// Before code\n/// \\code\n///     some_code();\n/// \\endcode\n/// After code\n"
	parts := readDoxygen(t, input)
	texts := blockTexts(translatableBlocks(collectBlocks(parts)))

	assert.True(t, blockTextsContain(texts, "Before code"),
		"should contain 'Before code', got %v", texts)
	assert.True(t, blockTextsContain(texts, "After code"),
		"should contain 'After code', got %v", texts)
	// Code between \code … \endcode is never translatable.
	for _, text := range texts {
		assert.False(t, strings.Contains(text, "some_code()"),
			"code block content should not be in a translatable block, got %q", text)
	}
}

// With non-translatable-content surfacing on (the default), the \code body is
// surfaced as a RoleCode content block — visible to ingestion, skipped by MT.
func TestExtract_CodeCommand_SurfacedAsContent(t *testing.T) {
	input := "/// Before code\n/// \\code\n///     some_code();\n/// \\endcode\n/// After code\n"
	parts := readDoxygen(t, input)
	content := nonTranslatableBlocks(collectBlocks(parts))
	require.Len(t, content, 1, "the \\code body surfaces as one non-translatable content block")
	code := content[0]
	assert.False(t, code.Translatable)
	assert.Equal(t, model.RoleCode, code.SemanticRole())
	assert.True(t, code.PreserveWhitespace)
	assert.Contains(t, code.SourceText(), "some_code();")
	// Single verbatim run — no inline parse.
	require.Len(t, code.SourceRuns(), 1)
	// Round-trip stays byte-exact with the flag on.
	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input))
}

// With surfacing disabled (the Okapi-faithful parity config) the \code body
// stays in skeleton and no content block is emitted.
func TestExtract_CodeCommand_FlagOff(t *testing.T) {
	input := "/// Before code\n/// \\code\n///     some_code();\n/// \\endcode\n/// After code\n"
	parts := readDoxygenConfig(t, input, func(c *doxygen.Config) {
		c.SetExtractNonTranslatableContent(false)
	})
	assert.Empty(t, nonTranslatableBlocks(collectBlocks(parts)),
		"no content block when surfacing is disabled")
	// Round-trip still byte-exact.
	assert.Equal(t, input, snippetRoundtripWithSkeletonConfig(t, input, func(c *doxygen.Config) {
		c.SetExtractNonTranslatableContent(false)
	}))
}

// okapi: DoxygenFilterTest#testDoxygenItalicCommand
func TestExtract_ItalicCommand(t *testing.T) {
	input := "/// This has \\e italic and \\a arg text\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text with inline commands")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "italic"),
		"should contain 'italic', got %v", texts)
	assert.True(t, blockTextsContain(texts, "arg"),
		"should contain 'arg', got %v", texts)
}

// okapi: DoxygenFilterTest#testDoxygenImageCommand
func TestExtract_ImageCommand(t *testing.T) {
	input := "/// Here is a snapshot:\n/// \\image html application.jpg\n/// End of description.\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text around \\image command")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "Here is a snapshot"),
		"should contain 'Here is a snapshot', got %v", texts)
}

// okapi: DoxygenFilterTest#testHtmlBoldCommand
func TestExtract_HtmlBoldCommand(t *testing.T) {
	input := "/// This has <b>bold</b> text\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text with HTML bold")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "bold"),
		"should contain 'bold', got %v", texts)
}

// okapi: DoxygenFilterTest#testOrphanedEndCommand
func TestExtract_OrphanedEndCommand(t *testing.T) {
	input := "/// Some text </summary> more text\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text despite orphaned end command")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "Some text"),
		"should contain 'Some text', got %v", texts)
}

// okapi: DoxygenFilterTest#testPositiveFloatListFalsePositive
func TestExtract_PositiveFloatListFalsePositive(t *testing.T) {
	input := "/// The value is 1.0 or 2.5 here.\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text with float numbers")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "1.0") || blockTextsContain(texts, "value"),
		"should contain the text with float, got %v", texts)
}

// okapi: DoxygenFilterTest#testOpenTwiceWithString
func TestExtract_OpenTwiceWithString(t *testing.T) {
	input := "/// Hello from Doxygen\n"
	parts1 := readDoxygen(t, input)
	blocks1 := collectBlocks(parts1)
	parts2 := readDoxygen(t, input)
	blocks2 := collectBlocks(parts2)

	require.Len(t, blocks2, len(blocks1),
		"double extraction should produce same number of blocks")
	if len(blocks1) > 0 && len(blocks2) > 0 {
		assert.Equal(t, blocks1[0].SourceText(), blocks2[0].SourceText(),
			"double extraction should produce same text")
	}
}

// okapi: DoxygenFilterTest#testDelimiterTokenizer
func TestExtract_DelimiterTokenizer(t *testing.T) {
	input := "/// First comment\nint x;\n/// Second comment\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from delimited comments")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "First comment"),
		"should contain 'First comment', got %v", texts)
	assert.True(t, blockTextsContain(texts, "Second comment"),
		"should contain 'Second comment', got %v", texts)
}

// okapi: DoxygenFilterTest#testPrefixSuffixTokenizer
func TestExtract_PrefixSuffixTokenizer(t *testing.T) {
	input := "/*! Block comment 1 */\nint x;\n/*! Block comment 2 */\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from /*! */ comments")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "Block comment 1"),
		"should contain 'Block comment 1', got %v", texts)
	assert.True(t, blockTextsContain(texts, "Block comment 2"),
		"should contain 'Block comment 2', got %v", texts)
}

// ---------------------------------------------------------------------------
// DoxygenFilterTest — output tests (roundtrip of snippets)
// ---------------------------------------------------------------------------

func roundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := doxygen.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := doxygen.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	return buf.String()
}

// okapi: DoxygenFilterTest#testOutputSimpleLine
func TestOutput_SimpleLine(t *testing.T) {
	input := "/// A simple line comment"
	output := roundtrip(t, input)
	assert.Contains(t, output, "A simple line comment",
		"roundtrip should preserve simple line comment text")
}

// okapi: DoxygenFilterTest#testOutputOneLiner
func TestOutput_OneLiner(t *testing.T) {
	input := "int x; ///< A one-liner comment"
	output := roundtrip(t, input)
	assert.Contains(t, output, "A one-liner comment",
		"roundtrip should preserve one-liner comment text")
	assert.Contains(t, output, "int x;",
		"roundtrip should preserve code prefix")
}

// okapi: DoxygenFilterTest#testOutputMultipleLines
func TestOutput_MultipleLines(t *testing.T) {
	input := "/// First line\n/// Second line\n/// Third line"
	output := roundtrip(t, input)
	assert.Contains(t, output, "First line",
		"roundtrip should preserve first line text")
	assert.Contains(t, output, "Second line",
		"roundtrip should preserve second line text")
}

// okapi-unmapped: DoxygenFilterTest#testOutputMultipleLineList — skipped in Java surefire (Issue #403)
func TestOutput_MultipleLineList(t *testing.T) {
	t.Skip("Issue #403 — skipped in Java surefire as well")
}

// okapi: DoxygenFilterTest#testOutputJavadocMultipleLines
func TestOutput_JavadocMultipleLines(t *testing.T) {
	input := "/**\n * First line\n * Second line\n * Third line\n */"
	output := roundtrip(t, input)
	assert.Contains(t, output, "First line",
		"roundtrip should preserve Javadoc first line")
	assert.Contains(t, output, "Second line",
		"roundtrip should preserve Javadoc second line")
}

// TestOutput_TrailingJavadocComment verifies the /**< text */ trailing
// Javadoc form survives round-trip with the same delimiter, mirroring
// the existing /*!< */ handling.
func TestOutput_TrailingJavadocComment(t *testing.T) {
	input := "TVal1, /**< enum value TVal1. */"
	output := roundtrip(t, input)
	assert.Contains(t, output, "/**<",
		"roundtrip should preserve /**< delimiter, got %q", output)
	assert.Contains(t, output, "enum value TVal1.",
		"roundtrip should preserve trailing comment text, got %q", output)
}

// TestOutput_MultiSectionCommentGroup verifies that a single /*! ... */
// comment containing multiple translatable sections (\param a … \param b
// … \return …) round-trips with all sections preserved. Before the
// group-aware writer landed, the writer wrote only the first section
// per skeleton ref and silently dropped the rest.
func TestOutput_MultiSectionCommentGroup(t *testing.T) {
	input := "/*!\n  \\param a first arg description.\n  \\param b second arg description.\n  \\return the computed result\n*/\n"
	output := roundtrip(t, input)
	assert.Contains(t, output, "first arg description.",
		"roundtrip should preserve first \\param description, got %q", output)
	assert.Contains(t, output, "second arg description.",
		"roundtrip should preserve second \\param description, got %q", output)
	assert.Contains(t, output, "the computed result",
		"roundtrip should preserve \\return description, got %q", output)
}

// ---------------------------------------------------------------------------
// DoxygenFilterTest — double extraction tests (full files)
// ---------------------------------------------------------------------------

// okapi: DoxygenFilterTest#testDoubleExtractionSample
func TestDoubleExtraction_Sample(t *testing.T) {
	content, err := os.ReadFile("testdata/sample.h")
	require.NoError(t, err)
	assertDoubleExtraction(t, string(content))
}

// okapi: DoxygenFilterTest#testDoubleExtractionQtStyle
func TestDoubleExtraction_QtStyle(t *testing.T) {
	content, err := os.ReadFile("testdata/qt-style.h")
	require.NoError(t, err)
	assertDoubleExtraction(t, string(content))
}

// okapi: DoxygenFilterTest#testDoubleExtractionJavadocStyle
func TestDoubleExtraction_JavadocStyle(t *testing.T) {
	content, err := os.ReadFile("testdata/javadoc-style.h")
	require.NoError(t, err)
	assertDoubleExtraction(t, string(content))
}

// okapi: DoxygenFilterTest#testDoubleExtractionSpecialCommands
func TestDoubleExtraction_SpecialCommands(t *testing.T) {
	content, err := os.ReadFile("testdata/special_commands.h")
	require.NoError(t, err)
	assertDoubleExtraction(t, string(content))
}

// assertDoubleExtraction verifies that reading the same content twice produces consistent results.
func assertDoubleExtraction(t *testing.T, content string) {
	t.Helper()
	parts1 := readDoxygen(t, content)
	blocks1 := collectBlocks(parts1)
	parts2 := readDoxygen(t, content)
	blocks2 := collectBlocks(parts2)

	require.Len(t, blocks2, len(blocks1),
		"double extraction should produce same number of blocks")
	for i := range blocks1 {
		assert.Equal(t, blocks1[i].SourceText(), blocks2[i].SourceText(),
			"block %d text should match on double extraction", i)
	}
}

// ---------------------------------------------------------------------------
// Full-file extraction tests
// ---------------------------------------------------------------------------

// okapi: DoxygenFilterTest#testDoubleExtractionSample (extraction aspect)
func TestExtract_SampleFile(t *testing.T) {
	content, err := os.ReadFile("testdata/sample.h")
	require.NoError(t, err)
	parts := readDoxygen(t, string(content))
	blocks := translatableBlocks(collectBlocks(parts))
	require.NotEmpty(t, blocks, "sample.h should produce translatable blocks")
	texts := blockTexts(blocks)

	assert.True(t, blockTextsContain(texts, "Brief description"),
		"should extract \\brief text from sample.h, got %v", texts)
	assert.True(t, blockTextsContain(texts, "detailed class description"),
		"should extract detailed description, got %v", texts)

	// Code between \code and \endcode should NOT be translatable.
	for _, text := range texts {
		assert.False(t, strings.Contains(text, "jimmy.crack"),
			"code block content should not be in a translatable block")
	}

	// Regular C++ comments should NOT be extracted.
	for _, text := range texts {
		assert.False(t, strings.Contains(text, "Not a Doxygen comment"),
			"regular comments should not be extracted")
	}
}

// okapi: DoxygenFilterTest#testDoubleExtractionQtStyle (extraction aspect)
func TestExtract_QtStyleFile(t *testing.T) {
	content, err := os.ReadFile("testdata/qt-style.h")
	require.NoError(t, err)
	parts := readDoxygen(t, string(content))
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "qt-style.h should produce translatable blocks")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "A test class"),
		"should extract //! comment, got %v", texts)
	assert.True(t, blockTextsContain(texts, "elaborate class description"),
		"should extract /*! */ comment, got %v", texts)
}

// okapi: DoxygenFilterTest#testDoubleExtractionJavadocStyle (extraction aspect)
func TestExtract_JavadocStyleFile(t *testing.T) {
	content, err := os.ReadFile("testdata/javadoc-style.h")
	require.NoError(t, err)
	parts := readDoxygen(t, string(content))
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "javadoc-style.h should produce translatable blocks")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "A test class"),
		"should extract /** */ comment, got %v", texts)
	assert.True(t, blockTextsContain(texts, "A constructor"),
		"should extract constructor doc, got %v", texts)
}

// okapi: DoxygenFilterTest#testDoubleExtractionSpecialCommands (extraction aspect)
func TestExtract_SpecialCommandsFile(t *testing.T) {
	content, err := os.ReadFile("testdata/special_commands.h")
	require.NoError(t, err)
	parts := readDoxygen(t, string(content))
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "special_commands.h should produce translatable blocks")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "Additional documentation"),
		"should extract \\addtogroup text, got %v", texts)
}

// ---------------------------------------------------------------------------
// Additional comment style tests
// ---------------------------------------------------------------------------

// okapi: DoxygenFilterTest#testSimpleLine (//! variant)
func TestExtract_ExclamationLineComment(t *testing.T) {
	input := "//! An exclamation line comment\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text from //! comments")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "An exclamation line comment"),
		"should contain 'An exclamation line comment', got %v", texts)
}

// okapi: DoxygenFilterTest#testJavadocMultiline (/*! */ variant)
func TestExtract_QtBlockComment(t *testing.T) {
	input := "/*!\n  A Qt-style block comment.\n*/\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text from /*! */ comments")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "Qt-style block comment"),
		"should contain 'Qt-style block comment', got %v", texts)
}

// okapi: DoxygenFilterTest#testSimpleLine (non-doxygen exclusion)
func TestExtract_RegularCommentExcluded(t *testing.T) {
	input := "// This is a regular comment, not Doxygen\nint x = 0;\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	texts := blockTexts(blocks)
	for _, text := range texts {
		assert.False(t, strings.Contains(text, "regular comment"),
			"regular C++ comments should not be extracted, got %q", text)
	}
}

// okapi: DoxygenFilterTest (layer structure validation)
func TestExtract_LayerStructure(t *testing.T) {
	parts := readDoxygen(t, "/// Hello\n")
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type,
		"last part should be LayerEnd")
}

// okapi: DoxygenFilterTest (block ID uniqueness)
func TestExtract_BlockIDsUnique(t *testing.T) {
	input := "/// First block\nint x;\n/// Second block\nint y;\n/// Third block\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks)
	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// ---------------------------------------------------------------------------
// Metadata and signature tests
// ---------------------------------------------------------------------------

func TestReaderSignature(t *testing.T) {
	reader := doxygen.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/x-doxygen-txt")
	assert.Contains(t, sig.Extensions, ".h")
	assert.Contains(t, sig.Extensions, ".cpp")
}

func TestReaderMetadata(t *testing.T) {
	reader := doxygen.NewReader()
	assert.Equal(t, "doxygen", reader.Name())
	assert.Equal(t, "Doxygen Comments", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := doxygen.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	parts := readDoxygen(t, "")
	blocks := collectBlocks(parts)
	assert.Empty(t, blocks)
}

// ---------------------------------------------------------------------------
// Roundtrip with target locale
// ---------------------------------------------------------------------------

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()
	input := "/// Hello world\nint x;\n/// Goodbye world\n"

	reader := doxygen.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if strings.Contains(block.SourceText(), "Hello") {
				block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
			} else if strings.Contains(block.SourceText(), "Goodbye") {
				block.SetTargetText(model.LocaleFrench, "Au revoir le monde")
			}
		}
	}

	var buf bytes.Buffer
	writer := doxygen.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)
	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour le monde")
	assert.Contains(t, output, "Au revoir le monde")
	assert.NotContains(t, output, "Hello world")
	assert.NotContains(t, output, "Goodbye world")
}

// ---------------------------------------------------------------------------
// Verbatim exclusion test
// ---------------------------------------------------------------------------

func TestExtract_VerbatimExcluded(t *testing.T) {
	input := "/// Before verbatim\n/// \\verbatim\n///   not translated\n/// \\endverbatim\n/// After verbatim\n"
	parts := readDoxygen(t, input)
	texts := blockTexts(translatableBlocks(collectBlocks(parts)))

	assert.True(t, blockTextsContain(texts, "Before verbatim"),
		"should contain 'Before verbatim', got %v", texts)
	assert.True(t, blockTextsContain(texts, "After verbatim"),
		"should contain 'After verbatim', got %v", texts)
	// Verbatim body is never translatable.
	for _, text := range texts {
		assert.False(t, strings.Contains(text, "not translated"),
			"verbatim content should not be in a translatable block, got %q", text)
	}

	// With surfacing on (default), the verbatim body surfaces as a RoleCode
	// content block, and the document still round-trips byte-exact.
	content := nonTranslatableBlocks(collectBlocks(parts))
	require.Len(t, content, 1, "the \\verbatim body surfaces as one content block")
	assert.False(t, content[0].Translatable)
	assert.Equal(t, model.RoleCode, content[0].SemanticRole())
	assert.True(t, content[0].PreserveWhitespace)
	assert.Contains(t, content[0].SourceText(), "not translated")
	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input))
}

// ---------------------------------------------------------------------------
// Param/return description extraction
// ---------------------------------------------------------------------------

func TestExtract_ParamDescription(t *testing.T) {
	input := "/// \\param name The name of the person\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract param description")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "The name of the person"),
		"should contain param description, got %v", texts)
	// The param name itself should not be in the translatable text alone
	for _, text := range texts {
		assert.NotEqual(t, "name", strings.TrimSpace(text),
			"should not extract just the param name")
	}
}

func TestExtract_ReturnDescription(t *testing.T) {
	input := "/// \\return The computed result\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract return description")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "The computed result"),
		"should contain return description, got %v", texts)
}

// ---------------------------------------------------------------------------
// Trailing Qt-style comment
// ---------------------------------------------------------------------------

func TestExtract_TrailingQtComment(t *testing.T) {
	input := "int x; /*!< Trailing Qt comment */\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract trailing Qt comment")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "Trailing Qt comment"),
		"should contain 'Trailing Qt comment', got %v", texts)
}

// TestExtract_TrailingJavadocComment covers the /**< text */ form, which
// Doxygen treats the same as /*!< text */ — a documentation comment
// attached to the preceding declaration. The reader was previously
// emitting these lines as untranslatable Data, so the comment text
// remained in the source language on round-trip.
func TestExtract_TrailingJavadocComment(t *testing.T) {
	input := "TVal1, /**< enum value TVal1. */\n"
	parts := readDoxygen(t, input)
	blocks := collectBlocks(parts)
	require.NotEmpty(t, blocks, "should extract trailing /**<...*/ comment")
	texts := blockTexts(blocks)
	assert.True(t, blockTextsContain(texts, "enum value TVal1."),
		"should contain 'enum value TVal1.', got %v", texts)
}

// ---------------------------------------------------------------------------
// Non-translatable content surfacing (#928)
// ---------------------------------------------------------------------------

// A comment whose ONLY body is a \code…\endcode region (no translatable prose)
// previously emitted one opaque Data. With surfacing on (default) it now emits
// a non-translatable RoleCode content block carrying the code body; both
// round-trips stay byte-exact.
func TestExtract_CodeOnlyComment_Surfaced(t *testing.T) {
	input := "/// \\code\n///     int x = 5;\n/// \\endcode\nint main() {}\n"
	parts := readDoxygen(t, input)

	require.Empty(t, blockTexts(translatableBlocks(collectBlocks(parts))),
		"a code-only comment yields no translatable blocks")
	content := nonTranslatableBlocks(collectBlocks(parts))
	require.Len(t, content, 1, "the code body surfaces as one content block")
	assert.False(t, content[0].Translatable)
	assert.Equal(t, model.RoleCode, content[0].SemanticRole())
	assert.True(t, content[0].PreserveWhitespace)
	assert.Contains(t, content[0].SourceText(), "int x = 5;")

	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input),
		"code-only comment round-trips byte-exact (skeleton)")
}

// With surfacing off, the code-only comment stays opaque Data (no content
// block), exactly as before this feature.
func TestExtract_CodeOnlyComment_FlagOff(t *testing.T) {
	input := "/// \\code\n///     int x = 5;\n/// \\endcode\nint main() {}\n"
	parts := readDoxygenConfig(t, input, func(c *doxygen.Config) {
		c.SetExtractNonTranslatableContent(false)
	})
	assert.Empty(t, collectBlocks(parts), "no blocks at all when surfacing is disabled")

	hasCommentData := false
	for _, p := range parts {
		if p.Type == model.PartData {
			if d := p.Resource.(*model.Data); strings.HasPrefix(d.Name, "comment.") {
				hasCommentData = true
				assert.Contains(t, d.Properties["raw"], "int x = 5;")
			}
		}
	}
	assert.True(t, hasCommentData, "code-only comment stays opaque Data with the flag off")

	assert.Equal(t, input, snippetRoundtripWithSkeletonConfig(t, input, func(c *doxygen.Config) {
		c.SetExtractNonTranslatableContent(false)
	}), "round-trip stays byte-exact with the flag off")
}

// Every Doxygen exclude family (\verbatim, \dot, \msc, \htmlonly, \latexonly,
// \xmlonly, \manonly, \rtfonly, \docbookonly) surfaces its body as a RoleCode
// content block and round-trips byte-exact.
func TestExtract_ExcludeFamilies_Surfaced(t *testing.T) {
	cases := []struct {
		open, close, body string
	}{
		{"\\dot", "\\enddot", "digraph G { a -> b; }"},
		{"\\msc", "\\endmsc", "a => b [ label = hi ];"},
		{"\\htmlonly", "\\endhtmlonly", "<b>raw html</b>"},
		{"\\latexonly", "\\endlatexonly", "\\frac{1}{2}"},
		{"\\xmlonly", "\\endxmlonly", "<note/>"},
		{"\\manonly", "\\endmanonly", ".SH NAME"},
		{"\\rtfonly", "\\endrtfonly", "{\\rtf raw}"},
		{"\\docbookonly", "\\enddocbookonly", "<para/>"},
	}
	for _, tc := range cases {
		t.Run(strings.TrimPrefix(tc.open, "\\"), func(t *testing.T) {
			input := "/// Before\n/// " + tc.open + "\n/// " + tc.body + "\n/// " + tc.close + "\n/// After\n"
			parts := readDoxygen(t, input)

			content := nonTranslatableBlocks(collectBlocks(parts))
			require.Len(t, content, 1, "%s body surfaces as one content block", tc.open)
			assert.Equal(t, model.RoleCode, content[0].SemanticRole())
			assert.Contains(t, content[0].SourceText(), tc.body)

			texts := blockTexts(translatableBlocks(collectBlocks(parts)))
			for _, txt := range texts {
				assert.NotContains(t, txt, tc.body, "%s body must not be translatable", tc.open)
			}
			assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input),
				"%s round-trips byte-exact", tc.open)
		})
	}
}

// On the no-skeleton writer path the surfaced content blocks carry no
// round-trip responsibility — the embedded code body is reproduced once by the
// comment group's lineLayout (view block skipped), and a code-only comment is
// reproduced once by the raw carrier block. Neither must be duplicated.
func TestRoundTrip_NoSkeleton_SurfacedNotDuplicated(t *testing.T) {
	// Embedded code in a translatable comment → view block, skipped.
	embedded := roundtrip(t, "/// Before code\n/// \\code\n///     some_code();\n/// \\endcode\n/// After code\n")
	assert.Equal(t, 1, strings.Count(embedded, "some_code();"),
		"embedded code body appears exactly once, got: %q", embedded)
	assert.Contains(t, embedded, "Before code")
	assert.Contains(t, embedded, "After code")

	// Code-only comment → raw carrier, reproduced verbatim once.
	codeOnly := roundtrip(t, "/// \\code\n///     int x = 5;\n/// \\endcode\nint main() {}\n")
	assert.Equal(t, 1, strings.Count(codeOnly, "int x = 5;"),
		"code-only body appears exactly once, got: %q", codeOnly)
	assert.Contains(t, codeOnly, "int main() {}")
}

// ---------------------------------------------------------------------------
// Config tests
// ---------------------------------------------------------------------------

func TestConfig(t *testing.T) {
	cfg := &doxygen.Config{}
	assert.Equal(t, "doxygen", cfg.FormatName())
	require.NoError(t, cfg.Validate())
	cfg.Reset()
	require.NoError(t, cfg.ApplyMap(nil))
	require.Error(t, cfg.ApplyMap(map[string]any{"unknown": true}))

	s := cfg.Schema()
	assert.Equal(t, "Doxygen Comments", s.Title)
	assert.Equal(t, "doxygen", s.FormatMeta.ID)
}

func TestConfig_ExtractNonTranslatableContent(t *testing.T) {
	cfg := &doxygen.Config{}
	// Default ON regardless of how the Config is constructed (zero value).
	assert.True(t, cfg.ExtractNonTranslatableContent(), "default is ON")

	cfg.SetExtractNonTranslatableContent(false)
	assert.False(t, cfg.ExtractNonTranslatableContent())
	cfg.SetExtractNonTranslatableContent(true)
	assert.True(t, cfg.ExtractNonTranslatableContent())

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	assert.False(t, cfg.ExtractNonTranslatableContent())
	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": true}))
	assert.True(t, cfg.ExtractNonTranslatableContent())
	require.Error(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": "nope"}))

	// Schema declares the property (default true) in the parser group.
	prop, ok := cfg.Schema().Properties["extractNonTranslatableContent"]
	require.True(t, ok, "schema declares extractNonTranslatableContent")
	assert.Equal(t, "boolean", prop.Type)
	assert.Equal(t, true, prop.Default)
}

// ---------------------------------------------------------------------------
// RoundTrip integration contract
// ---------------------------------------------------------------------------

// Native equivalent of Okapi's RoundTripDoxygenIT: it runs RoundTripComparison
// over a corpus of real .h files, asserting the extracted text units are stable
// across an extract→merge→re-extract roundtrip. This test reproduces the same
// observable contract over the representative fixtures that roundtrip cleanly
// (sample.h, qt-style.h, javadoc-style.h — lists.h is excluded for the reasons
// documented in skeleton_test.go): read each file, write it back through the
// skeleton store with no translation, re-extract, and assert the same number of
// text units with identical prose.
//
// Trailing whitespace is trimmed before comparison: the native reader collapses
// a multi-line `/** */` / `///` comment into one text unit whose SourceText
// carries a trailing newline, which the merge round folds away on re-extraction
// (the WhitespaceAdjustingEventBuilder reflow okapi performs). The prose content
// is byte-stable; only that trailing newline differs, so it is the one
// normalization applied — the body of every unit must still match exactly.
//
// okapi: RoundTripDoxygenIT#doxygenFiles
// okapi: DoxygenXliffCompareIT#doxygenXliffCompareFiles
// okapi-skip: RoundTripDoxygenIT#doxygenFilesSerialized — Okapi serialized-skeleton roundtrip variant; native uses its own skeleton store (no serialized-skeleton mode)
func TestRoundTrip_DoxygenIT(t *testing.T) {
	trimTrailing := func(texts []string) []string {
		out := make([]string, len(texts))
		for i, s := range texts {
			out[i] = strings.TrimRight(s, "\n")
		}
		return out
	}
	for _, name := range []string{"testdata/sample.h", "testdata/qt-style.h", "testdata/javadoc-style.h"} {
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile(name)
			require.NoError(t, err)

			// Extract → write (no translation) via the skeleton store.
			merged := snippetRoundtripWithSkeleton(t, string(content))

			// Re-extract the merged output and compare source text units.
			first := blockTexts(collectBlocks(readDoxygen(t, string(content))))
			second := blockTexts(collectBlocks(readDoxygen(t, merged)))
			require.NotEmpty(t, first, "%s should produce translatable blocks", name)
			assert.Equal(t, trimTrailing(first), trimTrailing(second),
				"%s text units must be stable across an extract→write→re-extract roundtrip", name)
		})
	}
}
