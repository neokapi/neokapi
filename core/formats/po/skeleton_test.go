package po_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/po"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skeletonRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := po.NewReader()
	writer := po.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err = reader.Open(ctx, doc)
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

func TestSkeletonStore_ByteExact_SimpleEntry(t *testing.T) {
	t.Parallel()
	input := "msgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	output := skeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "simple entry roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_HeaderAndEntries(t *testing.T) {
	t.Parallel()
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
	output := skeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "header + entries roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultilineMsgstr(t *testing.T) {
	t.Parallel()
	input := `msgid ""
"Hello "
"World"
msgstr ""
"Bonjour "
"le monde"
`
	output := skeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multiline msgstr roundtrip should be byte-exact")
}

// #928: in skeleton mode, comments still surface as block notes while the
// byte-exact round-trip is unaffected — notes never touch the skeleton.
func TestSkeletonStore_CommentsAsNotes(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	input := "# Translator comment\n#. Extracted comment\nmsgid \"Save\"\nmsgstr \"Enregistrer\"\n"

	reader := po.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)

	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	require.NoError(t, reader.Open(ctx, doc))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)

	var dev, tr *model.NoteAnnotation
	for _, n := range blocks[0].Notes() {
		switch n.From {
		case "developer":
			dev = n
		case "translator":
			tr = n
		}
	}
	require.NotNil(t, dev)
	assert.Equal(t, "Extracted comment", dev.Text)
	require.NotNil(t, tr)
	assert.Equal(t, "Translator comment", tr.Text)

	// The byte-exact round-trip is unaffected by the added notes.
	assert.Equal(t, input, skeletonRoundtrip(t, input))
}

func TestSkeletonStore_ByteExact_Comments(t *testing.T) {
	t.Parallel()
	input := `# Translator comment
#. Extracted comment
#: src/main.c:42
#, fuzzy
msgctxt "menu"
msgid "Save"
msgstr "Enregistrer"
`
	output := skeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "comments roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_PluralForms(t *testing.T) {
	t.Parallel()
	input := `msgid "One item"
msgid_plural "Many items"
msgstr[0] "Un objet"
msgstr[1] "Plusieurs objets"
`
	output := skeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "plural forms roundtrip should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	t.Parallel()
	input := "msgid \"Hello\"\nmsgstr \"Bonjour\"\n\nmsgid \"World\"\nmsgstr \"Monde\"\n"
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := po.NewReader()
	writer := po.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err = reader.Open(ctx, doc)
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Add German translations
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Hello":
				b.SetTargetText(locale, "Hallo")
			case "World":
				b.SetTargetText(locale, "Welt")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := "msgid \"Hello\"\nmsgstr \"Hallo\"\n\nmsgid \"World\"\nmsgstr \"Welt\"\n"
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_ByteExact_MultipleCommentTypes(t *testing.T) {
	t.Parallel()
	input := `# Translator comment line 1
# Translator comment line 2
#. Extracted comment
#: src/app.js:10
#: src/app.js:20
#, fuzzy, c-format
msgid "Hello %s"
msgstr "Bonjour %s"
`
	output := skeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multiple comment types roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MsgctxtEntry(t *testing.T) {
	t.Parallel()
	input := `msgctxt "menu"
msgid "File"
msgstr "Fichier"
`
	output := skeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "msgctxt entry roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_FuzzyPluralEntry(t *testing.T) {
	t.Parallel()
	input := `#, fuzzy
msgid "One item"
msgid_plural "%d items"
msgstr[0] "Un article"
msgstr[1] "%d articles"
`
	output := skeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "fuzzy plural entry roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_UntranslatedEntry(t *testing.T) {
	t.Parallel()
	input := `msgid "Hello"
msgstr ""
`
	output := skeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "untranslated entry roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyInput(t *testing.T) {
	t.Parallel()
	input := ""
	output := skeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "empty input should produce empty output")
}

func TestSkeletonStore_TranslationWithPlurals(t *testing.T) {
	t.Parallel()
	input := `msgid "One item"
msgid_plural "Many items"
msgstr[0] "Un objet"
msgstr[1] "Plusieurs objets"
`
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := po.NewReader()
	writer := po.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err = reader.Open(ctx, doc)
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Add German translations
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.Properties["plural-form"] {
			case "singular":
				b.SetTargetText(locale, "Ein Gegenstand")
			case "plural":
				b.SetTargetText(locale, "Viele Gegenstaende")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	expected := `msgid "One item"
msgid_plural "Many items"
msgstr[0] "Ein Gegenstand"
msgstr[1] "Viele Gegenstaende"
`
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_ByteExact_EscapedContent(t *testing.T) {
	t.Parallel()
	input := "msgid \"She said \\\"hello\\\"\"\nmsgstr \"Elle a dit \\\"bonjour\\\"\"\n"
	output := skeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "escaped content roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_EmbeddedNewlines(t *testing.T) {
	t.Parallel()
	input := "msgid \"Hello\\nWorld\"\nmsgstr \"Bonjour\\nMonde\"\n"
	output := skeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "embedded newlines roundtrip should be byte-exact")
}
