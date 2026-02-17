package po_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/formats/po"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSimple(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	doc := testutil.RawDocFromString(
		"msgid \"Hello\"\nmsgstr \"Bonjour\"\n",
		model.LocaleEnglish,
	)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "Bonjour", blocks[0].TargetText(model.LocaleFrench))
}

func TestReadHeader(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	input := "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// Find header data
	var headerData *model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			d := p.Resource.(*model.Data)
			if d.Name == "header" {
				headerData = d
				break
			}
		}
	}
	require.NotNil(t, headerData)
	assert.Contains(t, headerData.Properties["content"], "Content-Type")

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
}

func TestReadMultiline(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	input := "msgid \"\"\n\"Hello \"\n\"World\"\nmsgstr \"\"\n\"Bonjour \"\n\"le monde\"\n"
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "Bonjour le monde", blocks[0].TargetText(model.LocaleFrench))
}

func TestReadMsgctxt(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	input := "msgctxt \"menu\"\nmsgid \"File\"\nmsgstr \"Fichier\"\n"
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "File", blocks[0].SourceText())
	assert.Equal(t, "Fichier", blocks[0].TargetText(model.LocaleFrench))
	assert.Equal(t, "menu", blocks[0].Properties["context"])
}

func TestReadPluralForms(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	input := "msgid \"One item\"\nmsgid_plural \"Many items\"\nmsgstr[0] \"Un objet\"\nmsgstr[1] \"Plusieurs objets\"\n"
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 2)
	assert.Equal(t, "One item", blocks[0].SourceText())
	assert.Equal(t, "Un objet", blocks[0].TargetText(model.LocaleFrench))
	assert.Equal(t, "singular", blocks[0].Properties["plural-form"])

	assert.Equal(t, "Many items", blocks[1].SourceText())
	assert.Equal(t, "Plusieurs objets", blocks[1].TargetText(model.LocaleFrench))
	assert.Equal(t, "plural", blocks[1].Properties["plural-form"])
}

func TestReadTranslatorComments(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	input := "# This is a translator comment\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
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
	assert.Equal(t, "This is a translator comment", commentData.Properties["comment"])
}

func TestReadReferences(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	input := "#: src/main.c:42\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// Find reference data
	var refData *model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			d := p.Resource.(*model.Data)
			if d.Name == "reference" {
				refData = d
				break
			}
		}
	}
	require.NotNil(t, refData)
	assert.Equal(t, "src/main.c:42", refData.Properties["reference"])
}

func TestReadUntranslated(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	input := "msgid \"Hello\"\nmsgstr \"\"\n"
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.False(t, blocks[0].HasTarget(model.LocaleFrench))
}

func TestReadEscapeSequences(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	input := "msgid \"Hello\\nWorld\"\nmsgstr \"Bonjour\\nMonde\"\n"
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello\nWorld", blocks[0].SourceText())
	assert.Equal(t, "Bonjour\nMonde", blocks[0].TargetText(model.LocaleFrench))
}

func TestReadEmpty(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("msgid \"Hello\"\nmsgstr \"\"\n", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "po", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := po.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/x-gettext-translation")
	assert.Contains(t, sig.Extensions, ".po")
	assert.Contains(t, sig.Extensions, ".pot")
}

func TestReaderMetadata(t *testing.T) {
	reader := po.NewReader()
	assert.Equal(t, "po", reader.Name())
	assert.Equal(t, "PO (Gettext)", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	err := reader.Open(ctx, nil)
	assert.Error(t, err)
}

func TestReadMultipleEntries(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	input := "msgid \"Hello\"\nmsgstr \"Bonjour\"\n\nmsgid \"Goodbye\"\nmsgstr \"Au revoir\"\n\nmsgid \"Thanks\"\nmsgstr \"Merci\"\n"
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 3)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "Bonjour", blocks[0].TargetText(model.LocaleFrench))
	assert.Equal(t, "Goodbye", blocks[1].SourceText())
	assert.Equal(t, "Au revoir", blocks[1].TargetText(model.LocaleFrench))
	assert.Equal(t, "Thanks", blocks[2].SourceText())
	assert.Equal(t, "Merci", blocks[2].TargetText(model.LocaleFrench))
}

func TestReadSimpleFile(t *testing.T) {
	ctx := context.Background()

	f, err := os.Open("testdata/simple.po")
	require.NoError(t, err)

	reader := po.NewReader()
	doc := testutil.RawDocFromReader(f, "testdata/simple.po", model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err = reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	// The simple.po file has 3 msgid entries (excluding header)
	require.Len(t, blocks, 3)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "Bonjour le monde", blocks[0].TargetText(model.LocaleFrench))
	assert.Equal(t, "Goodbye", blocks[1].SourceText())
	assert.Equal(t, "Au revoir", blocks[1].TargetText(model.LocaleFrench))
	assert.Equal(t, "untranslated", blocks[2].SourceText())
	assert.False(t, blocks[2].HasTarget(model.LocaleFrench))
}

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()

	input := `msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"

#: src/greeting.js:10
msgid "Hello World"
msgstr "Bonjour le monde"

#: src/farewell.js:5
msgid "Goodbye"
msgstr "Au revoir"

msgid "untranslated"
msgstr ""
`

	// Read
	reader := po.NewReader()
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := po.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, input, buf.String())
}

func TestRoundTripWithContext(t *testing.T) {
	ctx := context.Background()

	input := `msgctxt "menu"
msgid "File"
msgstr "Fichier"
`

	// Read
	reader := po.NewReader()
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := po.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, input, buf.String())
}

func TestRoundTripWithPlurals(t *testing.T) {
	ctx := context.Background()

	input := `msgid "One item"
msgid_plural "Many items"
msgstr[0] "Un objet"
msgstr[1] "Plusieurs objets"
`

	// Read
	reader := po.NewReader()
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := po.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, input, buf.String())
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := context.Background()

	// Read
	reader := po.NewReader()
	input := "msgid \"Hello\"\nmsgstr \"Bonjour\"\n\nmsgid \"World\"\nmsgstr \"Monde\"\n"
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Modify targets to German
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				block.SetTargetText(model.LocaleGerman, "Hallo")
			} else if block.SourceText() == "World" {
				block.SetTargetText(model.LocaleGerman, "Welt")
			}
		}
	}

	// Write with German locale
	var buf bytes.Buffer
	writer := po.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleGerman)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	expected := "msgid \"Hello\"\nmsgstr \"Hallo\"\n\nmsgid \"World\"\nmsgstr \"Welt\"\n"
	assert.Equal(t, expected, buf.String())
}

func TestReadFlags(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	input := "#, fuzzy\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	var flagsData *model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			d := p.Resource.(*model.Data)
			if d.Name == "flags" {
				flagsData = d
				break
			}
		}
	}
	require.NotNil(t, flagsData)
	assert.Equal(t, "fuzzy", flagsData.Properties["flags"])
}

func TestReadEscapedQuotes(t *testing.T) {
	ctx := context.Background()
	reader := po.NewReader()
	input := "msgid \"She said \\\"hello\\\"\"\nmsgstr \"Elle a dit \\\"bonjour\\\"\"\n"
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "She said \"hello\"", blocks[0].SourceText())
	assert.Equal(t, "Elle a dit \"bonjour\"", blocks[0].TargetText(model.LocaleFrench))
}

func TestWriteEscapedQuotes(t *testing.T) {
	ctx := context.Background()

	input := "msgid \"She said \\\"hello\\\"\"\nmsgstr \"Elle a dit \\\"bonjour\\\"\"\n"

	// Read
	reader := po.NewReader()
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := po.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, input, buf.String())
}
