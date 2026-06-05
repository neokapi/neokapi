package properties_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats/properties"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: PropertiesFilterTest#testEntry
func TestReadSimple(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("app.title=Hello World", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "app.title", blocks[0].Name)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
}

func TestReadMultipleEntries(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	input := "key1=value1\nkey2=value2\nkey3=value3"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 3)
	assert.Equal(t, "key1", blocks[0].Name)
	assert.Equal(t, "value1", blocks[0].SourceText())
	assert.Equal(t, "key2", blocks[1].Name)
	assert.Equal(t, "value2", blocks[1].SourceText())
	assert.Equal(t, "key3", blocks[2].Name)
	assert.Equal(t, "value3", blocks[2].SourceText())
}

// okapi: PropertiesFilterTest#testKeySpecial
func TestReadColonSeparator(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("key:value", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "key", blocks[0].Name)
	assert.Equal(t, "value", blocks[0].SourceText())
	assert.Equal(t, ":", blocks[0].Properties["separator"])
}

func TestReadSpaceSeparator(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("key value", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "key", blocks[0].Name)
	assert.Equal(t, "value", blocks[0].SourceText())
}

// okapi: PropertiesFilterTest#testSplicedEntry
func TestReadContinuationLine(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	input := "key=hello \\\nworld"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "key", blocks[0].Name)
	assert.Equal(t, "hello world", blocks[0].SourceText())
}

func TestReadContinuationMultipleLines(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	input := "key=line1 \\\n    line2 \\\n    line3"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "key", blocks[0].Name)
	assert.Equal(t, "line1 line2 line3", blocks[0].SourceText())
}

func TestReadComments(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	input := "# This is a comment\nkey=value"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// Find comment data
	var commentData *model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			d := p.Resource.(*model.Data)
			if d.Name == "comment" {
				commentData = d
				break
			}
		}
	}
	require.NotNil(t, commentData)
	assert.Equal(t, "# This is a comment", commentData.Properties["comment"])

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "key", blocks[0].Name)
	assert.Equal(t, "value", blocks[0].SourceText())
}

func TestReadExclamationComment(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	input := "! This is also a comment\nkey=value"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	var commentData *model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			d := p.Resource.(*model.Data)
			if d.Name == "comment" {
				commentData = d
				break
			}
		}
	}
	require.NotNil(t, commentData)
	assert.Equal(t, "! This is also a comment", commentData.Properties["comment"])
}

// okapi: PropertiesFilterTest#testEscapes
func TestReadUnicodeEscapes(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	input := "greeting=\\u0048\\u0065\\u006C\\u006C\\u006F"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
}

// okapi: PropertiesFilterTest#testSpecialChars
func TestReadJapaneseUnicodeEscapes(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	input := "greeting=\\u3053\\u3093\\u306B\\u3061\\u306F"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "\u3053\u3093\u306B\u3061\u306F", blocks[0].SourceText())
}

func TestReadBlankLines(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	input := "key1=value1\n\nkey2=value2"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 2)
	assert.Equal(t, "key1", blocks[0].Name)
	assert.Equal(t, "key2", blocks[1].Name)

	// Verify blank line data exists
	var blankCount int
	for _, p := range parts {
		if p.Type == model.PartData {
			d := p.Resource.(*model.Data)
			if d.Name == "blank" {
				blankCount++
			}
		}
	}
	assert.Equal(t, 1, blankCount)
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestReadEmptyValue(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("key=", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "key", blocks[0].Name)
	assert.Empty(t, blocks[0].SourceText())
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("key=value", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "properties", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := properties.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/x-java-properties")
	assert.Contains(t, sig.Extensions, ".properties")
}

func TestReaderMetadata(t *testing.T) {
	reader := properties.NewReader()
	assert.Equal(t, "properties", reader.Name())
	assert.Equal(t, "Java Properties", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadSimpleFile(t *testing.T) {
	ctx := t.Context()

	f, err := os.Open("testdata/simple.properties")
	require.NoError(t, err)

	reader := properties.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.properties", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 3)
	assert.Equal(t, "app.title", blocks[0].Name)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "app.description", blocks[1].Name)
	assert.Equal(t, "A simple application", blocks[1].SourceText())
	assert.Equal(t, "app.greeting", blocks[2].Name)
	assert.Equal(t, "Welcome, {0}!", blocks[2].SourceText())
}

func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	original, err := os.ReadFile("testdata/simple.properties")
	require.NoError(t, err)

	// Read
	f, err := os.Open("testdata/simple.properties")
	require.NoError(t, err)
	reader := properties.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.properties", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := properties.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, string(original), buf.String())
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	// Read
	reader := properties.NewReader()
	input := "greeting=Hello\nfarewell=Goodbye"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set French targets
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			} else if block.SourceText() == "Goodbye" {
				block.SetTargetText(model.LocaleFrench, "Au revoir")
			}
		}
	}

	// Write with French locale
	var buf bytes.Buffer
	writer := properties.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	expected := "greeting=Bonjour\nfarewell=Au revoir"
	assert.Equal(t, expected, buf.String())
}

func TestRoundTripWithComments(t *testing.T) {
	ctx := t.Context()
	input := "# A comment\nkey=value"

	// Read
	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := properties.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, input, buf.String())
}

func TestRoundTripWithBlankLines(t *testing.T) {
	ctx := t.Context()
	input := "key1=value1\n\nkey2=value2"

	// Read
	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := properties.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, input, buf.String())
}

func TestRoundTripColonSeparator(t *testing.T) {
	ctx := t.Context()
	input := "key:value"

	// Read
	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := properties.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, input, buf.String())
}

// okapi: PropertiesFilterTest#testSpecialCharsOutput
func TestWriteUnicodeEscapes(t *testing.T) {
	ctx := t.Context()

	// Read simple ASCII
	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("key=value", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set Japanese target
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			block.SetTargetText(model.LocaleJapanese, "\u3053\u3093\u306B\u3061\u306F")
		}
	}

	// Write with Japanese locale
	var buf bytes.Buffer
	writer := properties.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleJapanese)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, "key=\\u3053\\u3093\\u306b\\u3061\\u306f", buf.String())
}

func TestReadValueWithSpacesAroundEquals(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("key = value with spaces", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "key", blocks[0].Name)
	assert.Equal(t, "value with spaces", blocks[0].SourceText())
}

func TestReadKeyOnly(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("keyonly", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "keyonly", blocks[0].Name)
	assert.Empty(t, blocks[0].SourceText())
}

func TestReadMixedFormats(t *testing.T) {
	ctx := t.Context()
	reader := properties.NewReader()
	input := "key1=value1\nkey2:value2\nkey3 value3"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "value1", blocks[0].SourceText())
	assert.Equal(t, "=", blocks[0].Properties["separator"])
	assert.Equal(t, "value2", blocks[1].SourceText())
	assert.Equal(t, ":", blocks[1].Properties["separator"])
	assert.Equal(t, "value3", blocks[2].SourceText())
}
