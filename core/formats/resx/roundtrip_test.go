package resx_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/resx"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

func newDoc(path string, data []byte) *model.RawDocument {
	return &model.RawDocument{
		URI:          path,
		SourceLocale: "",
		Encoding:     "UTF-8",
		Reader:       nopCloser{bytes.NewReader(data)},
	}
}

// readParts reads a fixture file and returns its parts and raw bytes.
func readParts(t *testing.T, path string) ([]*model.Part, []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	r := resx.NewReader()
	require.NoError(t, r.Open(t.Context(), newDoc(path, data)))
	defer r.Close()

	var parts []*model.Part
	for res := range r.Read(t.Context()) {
		require.NoError(t, res.Error)
		parts = append(parts, res.Part)
	}
	return parts, data
}

// writeParts feeds parts through the writer and returns the produced bytes.
// An empty locale means "preserve existing values".
func writeParts(t *testing.T, parts []*model.Part, locale model.LocaleID) []byte {
	t.Helper()
	w := resx.NewWriter()
	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	if locale != "" {
		w.SetLocale(locale)
	}

	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	require.NoError(t, w.Write(t.Context(), ch))
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func blocks(parts []*model.Part) []*model.Block {
	var out []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok {
				out = append(out, b)
			}
		}
	}
	return out
}

func fixtures() []string {
	return []string{
		"Resources.resx",
		"Typed.resx",
		"Resources.resw",
	}
}

// TestInterfaces ensures the reader/writer satisfy the format interfaces.
func TestInterfaces(t *testing.T) {
	var _ format.DataFormatReader = resx.NewReader()
	var _ format.DataFormatWriter = resx.NewWriter()
}

// TestByteFaithfulRoundTrip verifies that reading then writing an unchanged
// document reproduces the original bytes exactly — including the resheader
// boilerplate, the embedded schema, typed/binary <data>, comments, entities,
// and whitespace.
func TestByteFaithfulRoundTrip(t *testing.T) {
	for _, name := range fixtures() {
		t.Run(name, func(t *testing.T) {
			parts, original := readParts(t, filepath.Join("testdata", name))
			out := writeParts(t, parts, "")
			assert.Equal(t, string(original), string(out),
				"round-trip must reproduce original bytes for %s", name)
		})
	}
}

// TestStringExtraction checks which entries are treated as translatable, plus
// name/source/note modeling for the normal resource set.
func TestStringExtraction(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "Resources.resx"))
	bs := blocks(parts)

	byName := make(map[string]*model.Block)
	for _, b := range bs {
		byName[b.Name] = b
	}

	// The five <data> string entries are extracted; resheaders and the schema
	// are not.
	require.Len(t, bs, 5)
	for _, want := range []string{"GreetingText", "SaveButton", "ItemCount", "MultiLine", "WithEntities"} {
		assert.Contains(t, byName, want, "expected block for %s", want)
	}

	// <comment> → developer note.
	greeting := byName["GreetingText"]
	require.NotNil(t, greeting)
	assert.Equal(t, "Hello, world!", greeting.SourceText())
	note, ok := model.AnnoAs[*model.NoteAnnotation](greeting, "note")
	require.True(t, ok, "GreetingText should carry a note")
	assert.Equal(t, "Shown on the welcome screen.", note.Text)
	assert.Equal(t, "developer", note.From)
	assert.True(t, greeting.PreserveWhitespace, "xml:space=preserve should set PreserveWhitespace")

	// No comment → no note.
	save := byName["SaveButton"]
	require.NotNil(t, save)
	assert.Equal(t, "Save", save.SourceText())
	_, hasNote := save.Anno("note")
	assert.False(t, hasNote, "SaveButton has no <comment>")

	// Entities are decoded into the source text.
	ent := byName["WithEntities"]
	require.NotNil(t, ent)
	assert.Equal(t, "Fish & Chips <tasty>", ent.SourceText())
}

// TestPlaceholderProtection verifies .NET composite-format placeholders become
// inline codes rather than flattened text.
func TestPlaceholderProtection(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "Resources.resx"))
	var item *model.Block
	for _, b := range blocks(parts) {
		if b.Name == "ItemCount" {
			item = b
		}
	}
	require.NotNil(t, item)

	// The {0} placeholder is a Ph run; SourceText (text-only) omits it, while
	// the markup-preserving render keeps it.
	assert.Equal(t, "You have  items in your cart.", item.SourceText())
	assert.Equal(t, "You have {0} items in your cart.",
		model.RenderRunsWithData(item.SourceRuns()))

	hasInlineCode := false
	for _, r := range item.SourceRuns() {
		if r.Kind() != model.RunKindText {
			hasInlineCode = true
			break
		}
	}
	require.True(t, hasInlineCode, "expected an inline code for {0}")
}

// TestTypedDataPassthrough verifies typed/binary <data>, <metadata>,
// <assembly>, and name-reference entries are never extracted, while plain
// string entries (including dotted control-property names) are.
func TestTypedDataPassthrough(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "Typed.resx"))
	bs := blocks(parts)

	names := make(map[string]bool)
	for _, b := range bs {
		names[b.Name] = true
	}

	// Translatable: plain string data and the dotted control-text entry.
	assert.True(t, names["WindowTitle"], "WindowTitle is a plain string")
	assert.True(t, names["CloseButton.Text"], "CloseButton.Text is a plain string")

	// Not translatable: typed Bitmap, typed Size, name-reference entry,
	// metadata, assembly.
	assert.False(t, names["AppIcon"], "typed Bitmap must not be extracted")
	assert.False(t, names["FormSize"], "typed Size must not be extracted")
	assert.False(t, names[">>AppIcon.Name"], "name-reference entry must not be extracted")
	assert.False(t, names["$this.Localizable"], "metadata must not be extracted")

	require.Len(t, bs, 2, "only the two plain string entries are translatable")
}

// TestTranslationUpdate verifies that supplying a target translation rewrites
// the matching <value> in place while leaving every other byte untouched.
func TestTranslationUpdate(t *testing.T) {
	path := filepath.Join("testdata", "Resources.resx")
	parts, original := readParts(t, path)

	// Translate GreetingText into German.
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if ok && b.Name == "GreetingText" {
			b.SetTargetText("de", "Hallo, Welt!")
		}
	}

	out := writeParts(t, parts, "de")
	outStr := string(out)

	// The translated value replaced the source.
	assert.Contains(t, outStr, "<value>Hallo, Welt!</value>")
	assert.NotContains(t, outStr, "<value>Hello, world!</value>")

	// Everything else is preserved: the comment, the resheaders, and the other
	// untranslated entries keep their source text.
	assert.Contains(t, outStr, "<comment>Shown on the welcome screen.</comment>")
	assert.Contains(t, outStr, "<value>text/microsoft-resx</value>")
	assert.Contains(t, outStr, "<value>Save</value>")

	// The only delta from the original is the one value.
	withRevert := bytes.Replace(out, []byte("Hallo, Welt!"), []byte("Hello, world!"), 1)
	assert.Equal(t, string(original), string(withRevert),
		"only the translated value should differ from the original")
}

// TestTranslationEscaping verifies that special characters in a new translation
// are XML-escaped on write.
func TestTranslationEscaping(t *testing.T) {
	path := filepath.Join("testdata", "Resources.resx")
	parts, _ := readParts(t, path)

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		if b, ok := p.Resource.(*model.Block); ok && b.Name == "SaveButton" {
			b.SetTargetText("fr", "Enregistrer & <quitter>")
		}
	}

	out := string(writeParts(t, parts, "fr"))
	assert.Contains(t, out, "<value>Enregistrer &amp; &lt;quitter&gt;</value>")
}

// TestWriteFromScratch verifies that, with no original document, the writer
// produces a valid canonical ResX that the reader can read back.
func TestWriteFromScratch(t *testing.T) {
	src := model.LocaleID("en")
	mkBlock := func(id, name, text string) *model.Block {
		b := model.NewBlock(id, text)
		b.Name = name
		b.SourceLocale = src
		return b
	}
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1", Format: "resx"}},
		{Type: model.PartBlock, Resource: mkBlock("tu1", "Title", "Welcome")},
		{Type: model.PartBlock, Resource: mkBlock("tu2", "Cancel", "Cancel")},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	out := writeParts(t, parts, "")
	assert.Contains(t, string(out), "text/microsoft-resx")
	assert.Contains(t, string(out), `<data name="Title" xml:space="preserve">`)
	assert.Contains(t, string(out), "<value>Welcome</value>")

	// Read it back and confirm the two entries survive.
	r := resx.NewReader()
	require.NoError(t, r.Open(t.Context(), newDoc("scratch.resx", out)))
	defer r.Close()
	var readBack []*model.Part
	for res := range r.Read(t.Context()) {
		require.NoError(t, res.Error)
		readBack = append(readBack, res.Part)
	}
	require.Len(t, blocks(readBack), 2)
}
