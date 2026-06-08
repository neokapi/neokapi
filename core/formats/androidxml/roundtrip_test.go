package androidxml_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/androidxml"
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

	r := androidxml.NewReader()
	require.NoError(t, r.Open(t.Context(), newDoc(path, data)))
	defer r.Close()

	var parts []*model.Part
	for res := range r.Read(t.Context()) {
		require.NoError(t, res.Error)
		parts = append(parts, res.Part)
	}
	return parts, data
}

// writeParts feeds parts through the writer and returns the produced bytes. An
// empty locale means "preserve existing values".
func writeParts(t *testing.T, parts []*model.Part, locale model.LocaleID) []byte {
	t.Helper()
	w := androidxml.NewWriter()
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

func blockByName(parts []*model.Part) map[string]*model.Block {
	m := make(map[string]*model.Block)
	for _, b := range blocks(parts) {
		m[b.Name] = b
	}
	return m
}

// runsHaveInlineCodes reports whether the run sequence contains any non-text
// (inline-code) run — a placeholder, paired code, or subblock reference.
func runsHaveInlineCodes(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}

// TestInterfaces ensures the reader/writer satisfy the format interfaces.
func TestInterfaces(t *testing.T) {
	var _ format.DataFormatReader = androidxml.NewReader()
	var _ format.DataFormatWriter = androidxml.NewWriter()
}

// TestByteFaithfulRoundTrip verifies that reading then writing an unchanged
// document reproduces the original bytes exactly — including the prolog,
// comments, whitespace, entities, escapes, CDATA, and xliff:g markup.
func TestByteFaithfulRoundTrip(t *testing.T) {
	parts, original := readParts(t, filepath.Join("testdata", "strings.xml"))
	out := writeParts(t, parts, "")
	assert.Equal(t, string(original), string(out),
		"round-trip must reproduce the original bytes")
}

// TestSniff verifies Android resource detection and that generic XML is not
// stolen.
func TestSniff(t *testing.T) {
	android, err := os.ReadFile(filepath.Join("testdata", "strings.xml"))
	require.NoError(t, err)
	notAndroid, err := os.ReadFile(filepath.Join("testdata", "not_android.xml"))
	require.NoError(t, err)

	tests := []struct {
		name string
		data string
		want bool
	}{
		{"android resources file", string(android), true},
		{"generic catalog xml", string(notAndroid), false},
		{"string element", `<resources><string name="x">hi</string></resources>`, true},
		{"string-array element", `<resources><string-array name="x"><item>a</item></string-array></resources>`, true},
		{"plurals element", `<resources><plurals name="x"><item quantity="one">a</item></plurals></resources>`, true},
		{"empty string element", `<resources><string>hi</string></resources>`, true},
		{"resources without translatable elements", `<resources><color name="x">#fff</color></resources>`, false},
		{"plain xml no resources", `<?xml version="1.0"?><root><a>b</a></root>`, false},
		{"html-ish", `<html><body><p>hello</p></body></html>`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, androidxml.Sniff([]byte(tt.data)))
		})
	}
}

// TestExtraction checks which entries are extracted and how names/sources/notes
// are modeled.
func TestExtraction(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "strings.xml"))
	by := blockByName(parts)

	// Plain strings, array items, and plurals items are extracted; the
	// translatable="false" string and array are not, nor is the resource alias.
	wantNames := []string{
		"app_name", "greeting", "welcome_user", "items_in_cart", "percent_complete",
		"escaped_text", "entities", "file_size", "rich_html",
		"cart_items[one]", "cart_items[other]",
		"weekdays[0]", "weekdays[1]", "weekdays[2]",
	}
	for _, n := range wantNames {
		assert.Contains(t, by, n, "expected block %q", n)
	}

	dontWant := []string{
		"do_not_translate", "alias_ref",
		"config_values[0]", "config_values[1]",
	}
	for _, n := range dontWant {
		assert.NotContains(t, by, n, "block %q must not be extracted", n)
	}

	require.Len(t, blocks(parts), len(wantNames),
		"exactly the translatable entries are extracted")

	// Comment immediately preceding an entry becomes a developer note.
	app := by["app_name"]
	require.NotNil(t, app)
	notes := app.Notes()
	require.Len(t, notes, 1, "app_name should carry a note")
	note := notes[0]
	assert.Equal(t, "Shown on the welcome screen.", note.Text)
	assert.Equal(t, "developer", note.From)

	// No preceding comment → no note.
	assert.Empty(t, by["greeting"].Notes(), "greeting has no preceding comment")

	// Entities are decoded into the source text.
	assert.Equal(t, "Fish & Chips <tasty>", by["entities"].SourceText())

	// Android backslash escapes are preserved verbatim (translator edits the
	// stored form, not the runtime form).
	assert.Equal(t, `It\'s a \"quoted\" word with a tab\tand newline\n`,
		by["escaped_text"].SourceText())
}

// TestPluralAndArrayProps verifies plural/array structural properties.
func TestPluralAndArrayProps(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "strings.xml"))
	by := blockByName(parts)

	one := by["cart_items[one]"]
	require.NotNil(t, one)
	assert.Equal(t, "plurals", one.Properties["androidxml.kind"])
	assert.Equal(t, "cart_items", one.Properties["androidxml.pluralsName"])
	assert.Equal(t, "one", one.Properties["androidxml.quantity"])

	w0 := by["weekdays[0]"]
	require.NotNil(t, w0)
	assert.Equal(t, "string-array", w0.Properties["androidxml.kind"])
	assert.Equal(t, "weekdays", w0.Properties["androidxml.arrayName"])
	assert.Equal(t, "0", w0.Properties["androidxml.index"])
	assert.Equal(t, "Monday", w0.SourceText())

	// The array's preceding comment attaches only to the first item.
	notes := w0.Notes()
	require.Len(t, notes, 1)
	assert.Equal(t, "A list of weekday names.", notes[0].Text)
	assert.Empty(t, by["weekdays[1]"].Notes())
}

// TestPlaceholderProtection verifies that printf args, xliff:g spans, and CDATA
// are protected as inline codes rather than flattened/translatable text.
func TestPlaceholderProtection(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "strings.xml"))
	by := blockByName(parts)

	// %1$s positional printf arg → Ph; text-only flatten omits it, markup render
	// keeps it.
	welcome := by["welcome_user"]
	require.NotNil(t, welcome)
	assert.True(t, runsHaveInlineCodes(welcome.SourceRuns()), "%1$s should be a code")
	assert.Equal(t, "Welcome back, !", welcome.SourceText())
	assert.Equal(t, "Welcome back, %1$s!", model.RenderRunsWithData(welcome.SourceRuns()))

	// %% literal escape is one code, and the trailing %1$d is another.
	pct := by["percent_complete"]
	require.NotNil(t, pct)
	assert.Equal(t, "%1$d%% complete", model.RenderRunsWithData(pct.SourceRuns()))

	// xliff:g spans are paired codes; the text between them stays translatable.
	fileSize := by["file_size"]
	require.NotNil(t, fileSize)
	assert.True(t, runsHaveInlineCodes(fileSize.SourceRuns()))
	assert.Equal(t,
		`Used <xliff:g id="size" example="1.2 GB">%1$s</xliff:g> of <xliff:g id="total" example="5 GB">%2$s</xliff:g>`,
		model.RenderRunsWithData(fileSize.SourceRuns()))
	// The literal connective text survives text-only flattening.
	assert.Equal(t, "Used  of ", fileSize.SourceText())

	// CDATA is preserved verbatim as one opaque code.
	rich := by["rich_html"]
	require.NotNil(t, rich)
	assert.True(t, runsHaveInlineCodes(rich.SourceRuns()))
	assert.Equal(t,
		`<![CDATA[Read our <a href="https://example.com">terms &amp; conditions</a> first.]]>`,
		model.RenderRunsWithData(rich.SourceRuns()))
}

// TestTranslationUpdate verifies that supplying target translations rewrites the
// matching values in place while leaving every other byte untouched.
func TestTranslationUpdate(t *testing.T) {
	path := filepath.Join("testdata", "strings.xml")
	parts, original := readParts(t, path)

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok {
			continue
		}
		switch b.Name {
		case "greeting":
			b.SetTargetText("de", "Hallo, Welt!")
		case "weekdays[0]":
			b.SetTargetText("de", "Montag")
		}
	}

	out := writeParts(t, parts, "de")
	outStr := string(out)

	assert.Contains(t, outStr, "<string name=\"greeting\">Hallo, Welt!</string>")
	assert.NotContains(t, outStr, "Hello, world!")
	assert.Contains(t, outStr, "<item>Montag</item>")

	// Reverting just the two changed values reproduces the original byte-for-byte.
	rev := bytes.Replace(out, []byte("Hallo, Welt!"), []byte("Hello, world!"), 1)
	rev = bytes.Replace(rev, []byte("<item>Montag</item>"), []byte("<item>Monday</item>"), 1)
	assert.Equal(t, string(original), string(rev),
		"only the two translated values should differ from the original")
}

// TestTranslationWithPlaceholders verifies a translation that reorders xliff:g
// and printf codes re-emits their markup verbatim.
func TestTranslationWithPlaceholders(t *testing.T) {
	path := filepath.Join("testdata", "strings.xml")
	parts, _ := readParts(t, path)

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok || b.Name != "welcome_user" {
			continue
		}
		// Build a French target reusing the original Ph run for %1$s.
		src := b.SourceRuns()
		var ph model.Run
		for _, r := range src {
			if r.Ph != nil {
				ph = r
			}
		}
		require.NotNil(t, ph.Ph)
		b.SetTargetRuns("fr", []model.Run{
			{Text: &model.TextRun{Text: "Bon retour, "}},
			ph,
			{Text: &model.TextRun{Text: " !"}},
		})
	}

	out := string(writeParts(t, parts, "fr"))
	assert.Contains(t, out, `<string name="welcome_user">Bon retour, %1$s !</string>`)
}

// TestTranslationEscaping verifies XML-significant characters in a new
// translation are entity-encoded while inline markup passed through verbatim is
// not double-encoded.
func TestTranslationEscaping(t *testing.T) {
	path := filepath.Join("testdata", "strings.xml")
	parts, _ := readParts(t, path)

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		if b, ok := p.Resource.(*model.Block); ok && b.Name == "greeting" {
			b.SetTargetText("fr", "Fish & <chips>")
		}
	}

	out := string(writeParts(t, parts, "fr"))
	// '&' and '<' are entity-encoded; '>' is left bare, which is well-formed XML
	// (XML 1.0 §2.4 requires escaping '>' only inside the "]]>" sequence). Keeping
	// '>' bare is what makes genuine Android values like "Do NOT check ->"
	// round-trip byte-faithfully.
	assert.Contains(t, out, "<string name=\"greeting\">Fish &amp; &lt;chips></string>")
}

// TestWriteFromScratch verifies that, with no original, the writer produces a
// valid canonical resources document the reader can read back.
func TestWriteFromScratch(t *testing.T) {
	src := model.LocaleID("en")
	mkString := func(id, name, text string) *model.Block {
		b := model.NewBlock(id, text)
		b.Name = name
		b.SourceLocale = src
		b.Properties["androidxml.kind"] = "string"
		return b
	}
	mkPlural := func(id, name, quantity, text string) *model.Block {
		b := model.NewBlock(id, text)
		b.Name = name + "[" + quantity + "]"
		b.SourceLocale = src
		b.Properties["androidxml.kind"] = "plurals"
		b.Properties["androidxml.pluralsName"] = name
		b.Properties["androidxml.quantity"] = quantity
		return b
	}
	mkArrayItem := func(id, name string, idx int, text string) *model.Block {
		b := model.NewBlock(id, text)
		b.Name = name + "[" + string(rune('0'+idx)) + "]"
		b.SourceLocale = src
		b.Properties["androidxml.kind"] = "string-array"
		b.Properties["androidxml.arrayName"] = name
		b.Properties["androidxml.index"] = string(rune('0' + idx))
		return b
	}

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1", Format: "androidxml"}},
		{Type: model.PartBlock, Resource: mkString("tu1", "title", "Welcome")},
		{Type: model.PartBlock, Resource: mkString("tu2", "cancel", "Cancel")},
		{Type: model.PartBlock, Resource: mkPlural("tu3", "n_items", "one", "%1$d item")},
		{Type: model.PartBlock, Resource: mkPlural("tu4", "n_items", "other", "%1$d items")},
		{Type: model.PartBlock, Resource: mkArrayItem("tu5", "colors", 0, "Red")},
		{Type: model.PartBlock, Resource: mkArrayItem("tu6", "colors", 1, "Green")},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	out := writeParts(t, parts, "")
	outStr := string(out)
	assert.Contains(t, outStr, "<resources>")
	assert.Contains(t, outStr, `<string name="title">Welcome</string>`)
	assert.Contains(t, outStr, `<plurals name="n_items">`)
	assert.Contains(t, outStr, `<item quantity="one">%1$d item</item>`)
	assert.Contains(t, outStr, `<string-array name="colors">`)
	assert.Contains(t, outStr, `<item>Red</item>`)

	// Read it back and confirm the entries survive.
	r := androidxml.NewReader()
	require.NoError(t, r.Open(t.Context(), newDoc("scratch.xml", out)))
	defer r.Close()
	var readBack []*model.Part
	for res := range r.Read(t.Context()) {
		require.NoError(t, res.Error)
		readBack = append(readBack, res.Part)
	}
	// 2 strings + 2 plural items + 2 array items.
	require.Len(t, blocks(readBack), 6)
}

// TestArrayItemMarkupRoundTrip exercises the item-rewrite path with inline
// xliff:g markup inside a <string-array> item: an untouched item must round-trip
// byte-for-byte, and a translated item must splice in place while preserving the
// markup verbatim.
func TestArrayItemMarkupRoundTrip(t *testing.T) {
	doc := `<?xml version="1.0" encoding="utf-8"?>
<resources xmlns:xliff="urn:oasis:names:tc:xliff:document:1.2">
    <string-array name="hints">
        <item>Tap <xliff:g id="btn">%1$s</xliff:g> to start</item>
        <item>Plain hint</item>
    </string-array>
</resources>
`
	r := androidxml.NewReader()
	require.NoError(t, r.Open(t.Context(), newDoc("strings.xml", []byte(doc))))
	var parts []*model.Part
	for res := range r.Read(t.Context()) {
		require.NoError(t, res.Error)
		parts = append(parts, res.Part)
	}
	require.NoError(t, r.Close())

	by := blockByName(parts)
	require.Contains(t, by, "hints[0]")
	require.Contains(t, by, "hints[1]")
	// The xliff:g markup is protected and re-renders verbatim.
	assert.Equal(t, `Tap <xliff:g id="btn">%1$s</xliff:g> to start`,
		model.RenderRunsWithData(by["hints[0]"].SourceRuns()))

	// Unchanged round-trip is byte-faithful.
	out := writeParts(t, parts, "")
	assert.Equal(t, doc, string(out))

	// Translating the second (plain) item splices in place; the markup item is
	// untouched.
	by["hints[1]"].SetTargetText("de", "Einfacher Hinweis")
	out2 := string(writeParts(t, parts, "de"))
	assert.Contains(t, out2, "<item>Einfacher Hinweis</item>")
	assert.Contains(t, out2, `<item>Tap <xliff:g id="btn">%1$s</xliff:g> to start</item>`)
}

// TestConfigToggles verifies the extraction toggles.
func TestConfigToggles(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "strings.xml"))
	require.NoError(t, err)

	read := func(apply func(*androidxml.Config)) []*model.Block {
		r := androidxml.NewReader()
		cfg := &androidxml.Config{}
		cfg.Reset()
		apply(cfg)
		require.NoError(t, r.SetConfig(cfg))
		require.NoError(t, r.Open(t.Context(), newDoc("strings.xml", data)))
		defer r.Close()
		var parts []*model.Part
		for res := range r.Read(t.Context()) {
			require.NoError(t, res.Error)
			parts = append(parts, res.Part)
		}
		return blocks(parts)
	}

	// With SkipNonTranslatable=false, the translatable="false" string and array
	// items are extracted too.
	bs := read(func(c *androidxml.Config) { c.SkipNonTranslatable = false })
	names := map[string]bool{}
	for _, b := range bs {
		names[b.Name] = true
	}
	assert.True(t, names["do_not_translate"])
	assert.True(t, names["config_values[0]"])

	// With SkipResourceReferences=false, the @string/app_name alias is extracted.
	bs = read(func(c *androidxml.Config) { c.SkipResourceReferences = false })
	names = map[string]bool{}
	for _, b := range bs {
		names[b.Name] = true
	}
	assert.True(t, names["alias_ref"])

	// With ExtractComments=false, no note annotations are produced.
	bs = read(func(c *androidxml.Config) { c.ExtractComments = false })
	for _, b := range bs {
		assert.Empty(t, b.Notes(), "no notes when ExtractComments=false (%s)", b.Name)
	}
}
