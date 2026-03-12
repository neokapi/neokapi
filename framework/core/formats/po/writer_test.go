package po_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gokapi/gokapi/core/formats/po"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFromParts is a helper that writes parts to a PO string.
func writeFromParts(t *testing.T, parts []*model.Part, locale model.LocaleID) string { //nolint:unused // reserved for future writer tests
	t.Helper()
	ctx := context.Background()

	var buf bytes.Buffer
	writer := po.NewWriter()
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(locale)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()
	return buf.String()
}

// okapi: POWriterTest#testEscapes
func TestWrite_Escapes(t *testing.T) {
	// Escape sequences (\n, \t, \\) should survive roundtrip.
	// The reader unescapes \n to real newlines, so the writer outputs multiline format.
	input := "msgid \"Line one\\nLine two\"\nmsgstr \"Ligne un\\nLigne deux\"\n"
	output := roundTrip(t, input)
	assert.Contains(t, output, "\\n", "newline escape should survive roundtrip")
	assert.Contains(t, output, "Line one")
	assert.Contains(t, output, "Line two")
	assert.Contains(t, output, "Ligne un")
	assert.Contains(t, output, "Ligne deux")

	// Verify content survives by re-reading the output.
	parts := readDefault(t, output)
	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Line one\nLine two", blocks[0].SourceText())
	assert.Equal(t, "Ligne un\nLigne deux", blocks[0].TargetText(model.LocaleFrench))
}

// okapi: POWriterTest#testEscapesAmongAlreadyEscaped
func TestWrite_EscapesAmongAlreadyEscaped(t *testing.T) {
	// Double backslashes should survive roundtrip.
	input := "msgid \"Path: C:\\\\Users\\\\test\\n\"\nmsgstr \"\"\n"
	output := roundTrip(t, input)
	assert.Contains(t, output, "\\\\", "double backslashes should survive roundtrip")
	assert.Contains(t, output, "\\n", "newline escape should survive roundtrip")
}

// okapi: POWriterTest#testOutputWithFuzzy
func TestWrite_OutputWithFuzzy(t *testing.T) {
	// Fuzzy flag on a single entry should survive roundtrip.
	input := "#, fuzzy\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	output := roundTrip(t, input)
	assert.Contains(t, output, "fuzzy", "fuzzy flag should survive roundtrip")
	assert.Contains(t, output, "Hello")
	assert.Contains(t, output, "Bonjour")
}

// okapi: POWriterTest#testOutputWithFuzzyPlural
func TestWrite_OutputWithFuzzyPlural(t *testing.T) {
	// Fuzzy flag on a plural entry should survive roundtrip.
	input := "#, fuzzy\nmsgid \"One item\"\nmsgid_plural \"%d items\"\nmsgstr[0] \"Un article\"\nmsgstr[1] \"%d articles\"\n"
	output := roundTrip(t, input)
	assert.Contains(t, output, "fuzzy", "fuzzy should survive roundtrip for plural entries")
	assert.Contains(t, output, "msgid_plural")
	assert.Contains(t, output, "msgstr[0]")
	assert.Contains(t, output, "msgstr[1]")
}

// okapi: POWriterTest#testOutputWithLinesWithWrap
func TestWrite_OutputWithLinesWithWrap(t *testing.T) {
	// Multiline string output.
	input := "msgid \"\"\n\"This is a very long line that should be wrapped at some point to keep the PO file readable\"\nmsgstr \"\"\n"
	output := roundTrip(t, input)
	assert.Contains(t, output, "long line")
	assert.Contains(t, output, "readable")
}

// okapi: POWriterTest#testOutputWithPlural
func TestWrite_OutputWithPlural(t *testing.T) {
	input := "msgid \"One item\"\nmsgid_plural \"%d items\"\nmsgstr[0] \"Un article\"\nmsgstr[1] \"%d articles\"\n"
	output := roundTrip(t, input)
	assert.Contains(t, output, "msgid \"One item\"")
	assert.Contains(t, output, "msgid_plural \"%d items\"")
	assert.Contains(t, output, "msgstr[0] \"Un article\"")
	assert.Contains(t, output, "msgstr[1] \"%d articles\"")
}

// okapi: POWriterTest#testSrcSimpleOutput
func TestWrite_SrcSimpleOutput(t *testing.T) {
	// Source-only entry (no translation).
	input := "msgid \"Hello\"\nmsgstr \"\"\n"
	output := roundTrip(t, input)
	assert.Contains(t, output, "msgid \"Hello\"")
	assert.Contains(t, output, "msgstr \"\"")
}

// okapi: POWriterTest#testSrcTrgSimpleOutput
func TestWrite_SrcTrgSimpleOutput(t *testing.T) {
	// Simple source + target entry.
	input := "msgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	output := roundTrip(t, input)
	assert.Contains(t, output, "Hello")
	assert.Contains(t, output, "Bonjour")
	assert.Equal(t, input, output)
}
