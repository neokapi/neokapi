package ts_test

// okapi-filter: ts
//
// This file contains native Go tests for the Qt TS format reader/writer,
// mapped to the Java Okapi TsFilterTest test methods.
//
// --- Java-internal API tests (not applicable to native Go implementation) ---
//
// okapi-unmapped: TsFilterTest#testStartDocument — Java StartDocument event; native uses PartLayerStart

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helpers ---

// readTS parses a Qt TS string and returns all parts.
func readTS(t *testing.T, input string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := ts.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

// readTSBlocks parses a Qt TS string and returns blocks.
func readTSBlocks(t *testing.T, input string) []*model.Block {
	t.Helper()
	return testutil.FilterBlocks(readTS(t, input))
}

// translatableBlocks returns only translatable blocks.
func translatableBlocks(blocks []*model.Block) []*model.Block {
	var result []*model.Block
	for _, b := range blocks {
		if b.Translatable {
			result = append(result, b)
		}
	}
	return result
}

// dataParts returns only Data parts.
func dataParts(parts []*model.Part) []*model.Part {
	var result []*model.Part
	for _, p := range parts {
		if p.Type == model.PartData {
			result = append(result, p)
		}
	}
	return result
}

// countPartsByType counts parts of a given type.
func countPartsByType(parts []*model.Part, pt model.PartType) int {
	n := 0
	for _, p := range parts {
		if p.Type == pt {
			n++
		}
	}
	return n
}

// blockTexts returns the source text of each block.
func blockTexts(blocks []*model.Block) []string {
	texts := make([]string, len(blocks))
	for i, b := range blocks {
		texts[i] = b.SourceText()
	}
	return texts
}

// hasInlineCodeRun reports whether any run is an inline code (Ph / PcOpen / PcClose / Sub).
func hasInlineCodeRun(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil && r.Plural == nil && r.Select == nil {
			return true
		}
	}
	return false
}

// snippetRoundtrip reads then writes a TS snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string) string {
	t.Helper()
	ctx := t.Context()

	// Read
	reader := ts.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := ts.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale("fr")

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	return buf.String()
}

// ---- TsFilterTest snippet-based tests ----

// okapi: TsFilterTest#StartDocument
// okapi: TsFilterTest#StartDocument_FromFile
func TestSnippet_StartDocument(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Hello</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart (equivalent to Java StartDocument)")

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part resource should be a Layer")
	assert.NotEmpty(t, layer.ID, "layer should have an ID")
}

// okapi: TsFilterTest#DocumentPartTsPart
func TestSnippet_DocumentPartTsPart(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Hello</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)
	require.NotEmpty(t, parts)

	// The TS element produces a Document Part. In the bridge, this appears
	// as part of the layer structure. Verify the layer framing is present.
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	// Should have Data parts for the TS element structure.
	dp := dataParts(parts)
	assert.NotEmpty(t, dp, "should have Data parts for TS structure")
}

// okapi: TsFilterTest#StartGroupContextPart
// okapi: TsFilterTest#StartGroupContextPart_FromFile
func TestSnippet_StartGroupContextPart(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>MyContext</name>
    <message>
        <source>Hello</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	// The <context> element produces a GroupStart event. The <name> child
	// provides the group's name/ID.
	groupStarts := countPartsByType(parts, model.PartGroupStart)
	assert.GreaterOrEqual(t, groupStarts, 1,
		"context element should produce at least one GroupStart")

	// Find the GroupStart part and verify it has the context name.
	for _, p := range parts {
		if p.Type == model.PartGroupStart {
			gs, ok := p.Resource.(*model.GroupStart)
			require.True(t, ok, "GroupStart resource should be a GroupStart")
			assert.NotEmpty(t, gs.ID, "group should have an ID")
			assert.Equal(t, "MyContext", gs.Name, "group name should be 'MyContext'")
			break
		}
	}
}

// okapi: TsFilterTest#TextUnitMessageUnfinished
// okapi: TsFilterTest#TextUnitMessageUnfinished_FromFile
// okapi: TsFilterTest#TextUnitMessageMissingTranslation_FromFile
func TestSnippet_TextUnitMessageUnfinished(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Hello World</source>
        <translation type="unfinished">Bonjour le monde</translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks, "should extract translatable blocks from unfinished message")

	b := blocks[0]
	assert.Equal(t, "Hello World", b.SourceText(),
		"source text should be 'Hello World'")

	// The message has type="unfinished" which means the translation exists
	// but is not yet approved.
	assert.True(t, b.HasTarget("fr"),
		"unfinished message should have a French target")
	assert.Equal(t, "Bonjour le monde", b.TargetText("fr"),
		"target text should be 'Bonjour le monde'")
}

// okapi: TsFilterTest#testTranslationStatus
// okapi: TsFilterTest#TextUnitMessageApproved_FromFile
// okapi: TsFilterTest#TextUnitMessageObsolete_FromFile
// okapi: TsFilterTest#TextUnitMessageMissingSourceAndTranslation_FromFile
// okapi: TsFilterTest#TextUnitMessageMissingSourceNotTranslation_FromFile
func TestSnippet_TranslationStatus(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message id="1">
        <source>Unfinished text</source>
        <translation type="unfinished">Texte non fini</translation>
    </message>
    <message id="2">
        <source>Finished text</source>
        <translation>Texte fini</translation>
    </message>
    <message id="3">
        <source>Obsolete text</source>
        <translation type="obsolete">Texte obsolète</translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.GreaterOrEqual(t, len(blocks), 2,
		"should extract at least 2 translatable blocks (unfinished + finished)")

	// Collect source texts from all blocks.
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Unfinished text",
		"should extract unfinished message")
	assert.Contains(t, texts, "Finished text",
		"should extract finished message")

	// Verify targets exist.
	for _, b := range blocks {
		switch b.SourceText() {
		case "Unfinished text":
			assert.True(t, b.HasTarget("fr"), "unfinished should have target")
			assert.Equal(t, "Texte non fini", b.TargetText("fr"))
		case "Finished text":
			assert.True(t, b.HasTarget("fr"), "finished should have target")
			assert.Equal(t, "Texte fini", b.TargetText("fr"))
		}
	}

	// Obsolete message should not be translatable
	allBlocks := testutil.FilterBlocks(parts)
	var obsoleteBlock *model.Block
	for _, b := range allBlocks {
		if b.SourceText() == "Obsolete text" {
			obsoleteBlock = b
			break
		}
	}
	require.NotNil(t, obsoleteBlock, "should have obsolete block")
	assert.False(t, obsoleteBlock.Translatable, "obsolete block should not be translatable")
	assert.Equal(t, "obsolete", obsoleteBlock.Properties["type"])
}

// okapi: TsFilterTest#testInlineCodes
func TestSnippet_InlineCodes(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>hello <byte value="4f"/>world</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "hello", "source should contain 'hello'")
	assert.Contains(t, text, "world", "source should contain 'world'")

	// The <byte> element should appear as an inline code run.
	runs := b.SourceRuns()
	require.NotEmpty(t, runs, "block should have runs")
	assert.True(t, hasInlineCodeRun(runs), "runs should contain an inline-code run for <byte>")
}

// okapi: TsFilterTest#testInlineCodesOutput
func TestSnippet_InlineCodesOutput(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>hello <byte value="4f"/>world</source>
        <translation type="unfinished">bonjour <byte value="4f"/>monde</translation>
    </message>
</context>
</TS>`

	output := snippetRoundtrip(t, snippet)

	// The <byte> element should be preserved in the roundtrip output.
	assert.Contains(t, output, "<byte",
		"byte element should be preserved in roundtrip output")
	assert.Contains(t, output, "hello", "source text should be preserved")
	assert.Contains(t, output, "world", "source text should be preserved")
}

// okapi: TsFilterTest#TestDecodeByteFalse
func TestSnippet_DecodeByteFalse(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>hello <byte value="4f"/>world</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	// With default settings, byte elements are handled as inline codes.
	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)

	// The byte element should be handled as an inline code.
	b := blocks[0]
	runs := b.SourceRuns()
	require.NotEmpty(t, runs)
	assert.True(t, hasInlineCodeRun(runs), "<byte> should produce an inline-code run")
}

// okapi: TsFilterTest#TestDecodeByteTrueDec
func TestSnippet_DecodeByteTrueDec(t *testing.T) {
	t.Parallel()
	// Decimal byte value: value="79" is 'O' in ASCII.
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>hello <byte value="79"/>world</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "hello", "should contain 'hello'")
	assert.Contains(t, text, "world", "should contain 'world'")
}

// okapi: TsFilterTest#TestDecodeByteTrueHex
func TestSnippet_DecodeByteTrueHex(t *testing.T) {
	t.Parallel()
	// Hex byte value: value="4f" is 'O' in ASCII.
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>hello <byte value="4f"/>world</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "hello", "should contain 'hello'")
	assert.Contains(t, text, "world", "should contain 'world'")
}

// okapi: TsFilterTest#TestDecodeByteTrueHex2
func TestSnippet_DecodeByteTrueHex2(t *testing.T) {
	t.Parallel()
	// Extended hex value with uppercase: value="4F".
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>hello <byte value="4F"/>world</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "hello", "should contain 'hello'")
	assert.Contains(t, text, "world", "should contain 'world'")
}

// okapi: TsFilterTest#TestEncodeIncludedChars
func TestSnippet_EncodeIncludedChars(t *testing.T) {
	t.Parallel()
	// Verify that special XML chars are correctly decoded.
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Text with &amp; and &lt; chars</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "&", "should decode &amp; to &")
	assert.Contains(t, text, "<", "should decode &lt; to <")
}

// okapi: TsFilterTest#TestEncodeExcludedChars
func TestSnippet_EncodeExcludedChars(t *testing.T) {
	t.Parallel()
	// Verify that encoded entities roundtrip correctly.
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Text with &amp; and &lt; chars</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	output := snippetRoundtrip(t, snippet)
	// XML entities should be re-encoded in the output.
	assert.Contains(t, output, "&amp;", "& should be encoded as &amp; in output")
	assert.Contains(t, output, "&lt;", "< should be encoded as &lt; in output")
}

// okapi: TsFilterTest#AllEvents
func TestSnippet_AllEvents(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>TestContext</name>
    <message id="1">
        <source>Hello</source>
        <translation type="unfinished">Bonjour</translation>
    </message>
    <message id="2">
        <source>World</source>
        <translation>Monde</translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	// Verify the full event stream has the right structure:
	// LayerStart, [Data/Group/Block parts], LayerEnd.
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type,
		"last part should be LayerEnd")

	// Should have GroupStart/GroupEnd for the context.
	groupStarts := countPartsByType(parts, model.PartGroupStart)
	groupEnds := countPartsByType(parts, model.PartGroupEnd)
	assert.GreaterOrEqual(t, groupStarts, 1, "should have at least one GroupStart")
	assert.Equal(t, groupStarts, groupEnds, "GroupStart and GroupEnd counts should match")

	// Should have at least 2 translatable blocks.
	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	assert.GreaterOrEqual(t, len(blocks), 2,
		"should extract at least 2 translatable blocks")

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Hello")
	assert.Contains(t, texts, "World")
}

// okapi: TsFilterTest#testTu
func TestSnippet_Tu(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Simple text</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Simple text", blocks[0].SourceText())
}

// okapi: TsFilterTest#testConsolidatedStream
func TestSnippet_ConsolidatedStream(t *testing.T) {
	t.Parallel()
	// The consolidated stream test verifies that extraction produces a coherent
	// event stream for bilingual content with source and target.
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message id="1">
        <source>Source one</source>
        <translation>Traduction un</translation>
    </message>
    <message id="2">
        <source>Source two</source>
        <translation type="unfinished">Traduction deux</translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.GreaterOrEqual(t, len(blocks), 2)

	// Both blocks should have targets.
	for _, b := range blocks {
		assert.True(t, b.HasTarget("fr"),
			"block '%s' should have a French target", b.SourceText())
	}
}

// okapi: TsFilterTest#testExtraComment
func TestSnippet_ExtraComment(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Commented text</source>
        <comment>This is a comment</comment>
        <extracomment>This is an extra comment</extracomment>
        <translatorcomment>This is a translator comment</translatorcomment>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "Commented text", b.SourceText())

	// Comments should be preserved as annotations on the block.
	require.NotNil(t, b.AnnoMap())
	note, ok := b.Anno("note")
	require.True(t, ok, "block should have a note annotation")
	n := note.(*model.NoteAnnotation)
	assert.NotEmpty(t, n.Text, "note text should not be empty")
	assert.Contains(t, n.Text, "This is a comment")
	assert.Contains(t, n.Text, "This is an extra comment")
	assert.Contains(t, n.Text, "This is a translator comment")

	// Also verify properties
	assert.Equal(t, "This is a comment", b.Properties["comment"])
	assert.Equal(t, "This is an extra comment", b.Properties["extracomment"])
	assert.Equal(t, "This is a translator comment", b.Properties["translatorcomment"])
}

// okapi: TsFilterTest#testGetName
func TestSnippet_GetName(t *testing.T) {
	t.Parallel()
	reader := ts.NewReader()
	assert.Equal(t, "ts", reader.Name())
	assert.Equal(t, "Qt TS", reader.DisplayName())
}

// okapi: TsFilterTest#testGetMimeType
func TestSnippet_GetMimeType(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Hello</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "application/x-ts", layer.MimeType,
		"layer MIME type should be application/x-ts")
}

// okapi: TsFilterTest#runTest
// okapi: TsFilterTest#testDoubleExtraction
func TestSnippet_RunTest(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Parameterized test</source>
        <translation type="unfinished">Test paramétré</translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)
	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Parameterized test", blocks[0].SourceText())

	// Verify roundtrip preserves content.
	output := snippetRoundtrip(t, snippet)
	assert.Contains(t, output, "Parameterized test")
	assert.Contains(t, output, "Test paramétré")
}

// okapi: TsFilterTest#testSourceLangNotSpecified
func TestSnippet_SourceLangNotSpecified(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr">
<context>
    <name>Test</name>
    <message>
        <source>No source lang</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)
	assert.Equal(t, "No source lang", blocks[0].SourceText())
}

// okapi: TsFilterTest#testTargetLangNotSpecified
func TestSnippet_TargetLangNotSpecified(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>No target lang</source>
        <translation type="unfinished">Pas de langue cible</translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)
	assert.Equal(t, "No target lang", blocks[0].SourceText())
}

// okapi: TsFilterTest#testTargetLangNotSpecified2
func TestSnippet_TargetLangNotSpecified2(t *testing.T) {
	t.Parallel()
	// Variant: TS element with no language or sourcelanguage at all.
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0">
<context>
    <name>Test</name>
    <message>
        <source>No langs at all</source>
        <translation type="unfinished">Aucune langue</translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)
	assert.Equal(t, "No langs at all", blocks[0].SourceText())
}

// okapi: TsFilterTest#testSourceLangEmpty
// okapi: TsFilterTest#TextUnitMessageEmptySource_FromFile
func TestSnippet_SourceLangEmpty(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="">
<context>
    <name>Test</name>
    <message>
        <source>Empty source lang</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Empty source lang", blocks[0].SourceText())
}

// okapi: TsFilterTest#testTargetLangEmpty
// okapi: TsFilterTest#TextUnitMessageEmptyTranslation_FromFile
func TestSnippet_TargetLangEmpty(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Empty target lang</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Empty target lang", blocks[0].SourceText())
}

// okapi: TsFilterTest#testInputStream
func TestSnippet_InputStream(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Stream input</source>
        <translation type="unfinished">Entrée flux</translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Stream input", blocks[0].SourceText())
}

// okapi: TsFilterTest#testInlineCodes (numerusform part)
// okapi: TsFilterTest#StartGroupNumerusPart_FromFile
// okapi: TsFilterTest#TextUnitNumerus_FromFile
func TestSnippet_NumerusForms(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message numerus="yes">
        <source>%n item(s)</source>
        <translation>
            <numerusform>%n article</numerusform>
            <numerusform>%n articles</numerusform>
        </translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.NotEmpty(t, blocks, "numerus message should produce translatable blocks")

	// Should extract the source text.
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "item") {
			found = true
			// Verify numerus marker + each form lands as one span of the
			// target-side SEGMENTATION OVERLAY so the pseudo /
			// TextModificationStep pipeline reaches both forms (not just
			// the first). The codeFinder pass splits `%n` out as a Ph run
			// so pseudo leaves printf placeholders intact — RunsText
			// returns only TextRun content, so the comparison is on the
			// post-Ph remainder (` article` / ` articles`). The `%n`
			// placeholder is preserved verbatim in the run's Ph Data and
			// re-emitted by the writer.
			assert.Equal(t, "yes", b.Properties["numerus"])
			key := model.Variant("fr")
			ov := b.SegmentationFor(&key)
			require.NotNil(t, ov, "numerus target should carry a segmentation overlay")
			require.Len(t, ov.Spans, 2, "expected one span per <numerusform>")
			assert.Equal(t, "0", ov.Spans[0].Props["numerus-form"])
			assert.Equal(t, "1", ov.Spans[1].Props["numerus-form"])
			targetRuns := b.TargetRuns("fr")
			assert.Equal(t, " article", model.RunsText(ov.Spans[0].Range.ExtractRuns(targetRuns)))
			assert.Equal(t, " articles", model.RunsText(ov.Spans[1].Range.ExtractRuns(targetRuns)))
			break
		}
	}
	assert.True(t, found, "should find block containing 'item'")
}

// TestSnippet_MultipleContexts verifies extraction from multiple <context> elements.
func TestSnippet_MultipleContexts(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Context1</name>
    <message>
        <source>First context text</source>
        <translation type="unfinished"></translation>
    </message>
</context>
<context>
    <name>Context2</name>
    <message>
        <source>Second context text</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)

	blocks := translatableBlocks(testutil.FilterBlocks(parts))
	require.GreaterOrEqual(t, len(blocks), 2)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "First context text")
	assert.Contains(t, texts, "Second context text")

	// Should have at least 2 GroupStart events (one per context).
	groupStarts := countPartsByType(parts, model.PartGroupStart)
	assert.GreaterOrEqual(t, groupStarts, 2,
		"should have at least 2 GroupStart events for 2 contexts")
}

// TestSnippet_Signature verifies the format signature.
func TestSnippet_Signature(t *testing.T) {
	t.Parallel()
	reader := ts.NewReader()
	sig := reader.Signature()

	assert.Contains(t, sig.MIMETypes, "application/x-ts")
	assert.Contains(t, sig.MIMETypes, "application/x-linguist")
	assert.Empty(t, sig.Extensions, "should not auto-detect .ts extension")

	// Sniff function should identify TS content
	require.NotNil(t, sig.Sniff)
	assert.True(t, sig.Sniff([]byte(`<TS version="2.0"></TS>`)))
	assert.False(t, sig.Sniff([]byte(`<html><body></body></html>`)))
}

// TestSnippet_Config verifies the config.
func TestSnippet_Config(t *testing.T) {
	t.Parallel()
	reader := ts.NewReader()
	cfg := reader.Config()
	assert.Equal(t, "ts", cfg.FormatName())
	require.NoError(t, cfg.Validate())

	err := cfg.(*ts.Config).ApplyMap(map[string]any{"bad": "value"})
	require.Error(t, err)
}

// TestSnippet_NilDocument verifies error on nil document.
func TestSnippet_NilDocument(t *testing.T) {
	t.Parallel()
	reader := ts.NewReader()
	err := reader.Open(t.Context(), nil)
	require.Error(t, err)
}

// TestSnippet_Roundtrip verifies full roundtrip.
//
// okapi: RoundTripTsIT#tsFiles — native extract→write over a real .ts snippet, asserting source/translation/context survive; Okapi's tsFiles does extract→merge→compare-events over a .ts corpus.
// okapi-skip: RoundTripTsIT#tsSerializedFiles — Okapi serialized-skeleton variant; native uses its own skeleton store, not Okapi's serialized event/skeleton format.
func TestSnippet_Roundtrip(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>MainWindow</name>
    <message id="msg1">
        <source>Hello World</source>
        <translation>Bonjour le monde</translation>
    </message>
    <message id="msg2">
        <source>Goodbye</source>
        <translation type="unfinished">Au revoir</translation>
    </message>
</context>
</TS>`

	output := snippetRoundtrip(t, snippet)

	// Verify key elements are preserved
	assert.Contains(t, output, "Hello World")
	assert.Contains(t, output, "Bonjour le monde")
	assert.Contains(t, output, "Goodbye")
	assert.Contains(t, output, "Au revoir")
	assert.Contains(t, output, "MainWindow")
	assert.Contains(t, output, `version="2.0"`)
	assert.Contains(t, output, `language="fr"`)
	assert.Contains(t, output, `sourcelanguage="en"`)
}

// TestSnippet_DoubleExtraction verifies reading twice produces same results.
//
// okapi: TsXliffCompareIT#tsXliffCompareFiles — re-extraction yields identical translatable content, verifying extraction is stable; Okapi's tsXliffCompareFiles extracts to XLIFF and compares against a gold XLIFF corpus.
func TestSnippet_DoubleExtraction(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Hello</source>
        <translation>Bonjour</translation>
    </message>
</context>
</TS>`

	blocks1 := readTSBlocks(t, snippet)
	blocks2 := readTSBlocks(t, snippet)

	require.Len(t, blocks2, len(blocks1))
	for i := range blocks1 {
		assert.Equal(t, blocks1[i].SourceText(), blocks2[i].SourceText())
	}
}

// TestSnippet_MessageWithID verifies that message id attribute is preserved.
func TestSnippet_MessageWithID(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message id="custom_id">
        <source>With ID</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	blocks := readTSBlocks(t, snippet)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "custom_id", blocks[0].ID)
}

// TestSnippet_AutoGeneratedID verifies that blocks without message id get generated IDs.
func TestSnippet_AutoGeneratedID(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>First</source>
        <translation type="unfinished"></translation>
    </message>
    <message>
        <source>Second</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	blocks := readTSBlocks(t, snippet)
	require.Len(t, blocks, 2)
	assert.Equal(t, "tu1", blocks[0].ID)
	assert.Equal(t, "tu2", blocks[1].ID)
}

// TestSnippet_ContextNameOnBlock verifies the context name is stored on blocks.
func TestSnippet_ContextNameOnBlock(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.0" language="fr" sourcelanguage="en">
<context>
    <name>MyContext</name>
    <message>
        <source>Text</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`

	blocks := readTSBlocks(t, snippet)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "MyContext", blocks[0].Properties["context"])
	assert.Equal(t, "MyContext", blocks[0].Name)
}

// TestReadTestdataFile_Simple verifies reading from testdata file.
func TestReadTestdataFile_Simple(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := ts.NewReader()

	f, err := os.Open("testdata/simple.ts")
	require.NoError(t, err)
	defer f.Close()

	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.ts", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := translatableBlocks(testutil.FilterBlocks(parts))

	require.Len(t, blocks, 2)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Hello")
	assert.Contains(t, texts, "Goodbye")
}

// TestReadTestdataFile_Bilingual verifies reading bilingual file with various states.
func TestReadTestdataFile_Bilingual(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := ts.NewReader()

	f, err := os.Open("testdata/bilingual.ts")
	require.NoError(t, err)
	defer f.Close()

	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/bilingual.ts", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	allBlocks := testutil.FilterBlocks(parts)

	// 3 messages total: 2 translatable (unfinished, finished), 1 obsolete (not translatable)
	require.Len(t, allBlocks, 3)

	transBlocks := translatableBlocks(allBlocks)
	assert.Len(t, transBlocks, 2)
}

// TestReadTestdataFile_Plurals verifies reading numerus forms from file.
func TestReadTestdataFile_Plurals(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := ts.NewReader()

	f, err := os.Open("testdata/plurals.ts")
	require.NoError(t, err)
	defer f.Close()

	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/plurals.ts", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)

	b := blocks[0]
	assert.Contains(t, b.SourceText(), "file")
	assert.Equal(t, "yes", b.Properties["numerus"])
}

// TestWriter_NewWriter verifies writer creation.
func TestWriter_NewWriter(t *testing.T) {
	t.Parallel()
	writer := ts.NewWriter()
	assert.Equal(t, "ts", writer.Name())
}

// TestWriter_EmptyOutput verifies writing with nil output does not panic.
func TestWriter_EmptyOutput(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	writer := ts.NewWriter()

	ch := make(chan *model.Part)
	close(ch)

	err := writer.Write(ctx, ch)
	require.NoError(t, err)
}

// TestSnippet_LayerProperties verifies the layer has correct format metadata.
func TestSnippet_LayerProperties(t *testing.T) {
	t.Parallel()
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="ro_RO" sourcelanguage="en">
<context>
    <name>Test</name>
    <message>
        <source>Hello</source>
        <translation>Salut</translation>
    </message>
</context>
</TS>`

	parts := readTS(t, snippet)
	require.NotEmpty(t, parts)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "ts", layer.Format)
	assert.True(t, layer.IsMultilingual)
}
