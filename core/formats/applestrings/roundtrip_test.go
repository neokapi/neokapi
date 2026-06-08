package applestrings_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	applestrings "github.com/neokapi/neokapi/core/formats/applestrings"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

func nopReader(data []byte) *nopCloser { return &nopCloser{Reader: bytes.NewReader(data)} }

// readParts reads a fixture file and returns its parts plus the raw bytes.
func readParts(t *testing.T, path string) ([]*model.Part, []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	r := applestrings.NewReader()
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: "",
		Encoding:     "UTF-8",
		Reader:       nopReader(data),
	}
	require.NoError(t, r.Open(t.Context(), doc))
	defer r.Close()
	parts := testutil.CollectParts(t, r.Read(t.Context()))
	return parts, data
}

// readPartsBytes reads from raw bytes with a chosen URI (controls kind detection).
func readPartsBytes(t *testing.T, uri string, data []byte) []*model.Part {
	t.Helper()
	r := applestrings.NewReader()
	doc := &model.RawDocument{URI: uri, Encoding: "UTF-8", Reader: nopReader(data)}
	require.NoError(t, r.Open(t.Context(), doc))
	defer r.Close()
	return testutil.CollectParts(t, r.Read(t.Context()))
}

// writeParts feeds parts through the writer and returns the produced bytes.
func writeParts(t *testing.T, parts []*model.Part, locale model.LocaleID) []byte {
	t.Helper()
	w := applestrings.NewWriter()
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

func blockByName(parts []*model.Part) map[string]*model.Block {
	m := make(map[string]*model.Block)
	for _, b := range testutil.FilterBlocks(parts) {
		m[b.Name] = b
	}
	return m
}

func fixtures() []string {
	return []string{
		"Localizable.strings",
		"Localizable.stringsdict",
	}
}

// TestByteFaithfulRoundTrip verifies that reading then writing an unchanged
// document reproduces the original bytes exactly for both file types.
func TestByteFaithfulRoundTrip(t *testing.T) {
	for _, name := range fixtures() {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name)
			parts, original := readParts(t, path)
			out := writeParts(t, parts, "")
			assert.Equal(t, string(original), string(out),
				"round-trip must reproduce original bytes for %s", name)
		})
	}
}

// TestReaderImplementsInterface ensures the reader/writer satisfy the format
// interfaces.
func TestReaderImplementsInterface(t *testing.T) {
	var _ format.DataFormatReader = applestrings.NewReader()
	var _ format.DataFormatWriter = applestrings.NewWriter()
}

// TestStringsModeling checks key→name, value→source, comment→note, and
// placeholder protection for the line-based .strings format.
func TestStringsModeling(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "Localizable.strings"))
	byName := blockByName(parts)
	require.Len(t, byName, 7)

	// Comment becomes a developer note.
	cancel := byName["button.cancel"]
	require.NotNil(t, cancel)
	assert.Equal(t, "Cancel", model.RenderRunsWithData(cancel.SourceRuns()))
	cancelNotes := cancel.Notes()
	require.Len(t, cancelNotes, 1)
	note := cancelNotes[0]
	assert.Equal(t, "Title of the cancel button", note.Text)
	assert.Equal(t, "developer", note.From)

	// Line comment also becomes a note.
	okBtn := byName["button.ok"]
	require.NotNil(t, okBtn)
	okNotes := okBtn.Notes()
	require.Len(t, okNotes, 1)
	noteOK := okNotes[0]
	assert.Equal(t, "A line comment preceding an entry", noteOK.Text)

	// printf %@ is a protected placeholder, not flattened text.
	greeting := byName["home.greeting"]
	require.NotNil(t, greeting)
	var phData []string
	for _, run := range greeting.SourceRuns() {
		if run.Ph != nil {
			phData = append(phData, run.Ph.Data)
		}
	}
	assert.Contains(t, phData, "%@")
	assert.Equal(t, "Hello, %@!", model.RenderRunsWithData(greeting.SourceRuns()))

	// Escapes round-trip into the decoded source (newline + tab present).
	summary := byName["inbox.summary"]
	require.NotNil(t, summary)
	assert.Contains(t, summary.SourceText(), "\n")
	assert.Contains(t, summary.SourceText(), "\t")

	// Positional + escaped quote.
	share := byName["share.text"]
	require.NotNil(t, share)
	var sharePh []string
	for _, run := range share.SourceRuns() {
		if run.Ph != nil {
			sharePh = append(sharePh, run.Ph.Data)
		}
	}
	assert.Contains(t, sharePh, "%1$@")
	assert.Contains(t, sharePh, "%2$@")
	assert.Equal(t, `%1$@ shared "%2$@" with you`, model.RenderRunsWithData(share.SourceRuns()))

	// \U escape decodes and %% is a single literal-percent placeholder.
	progress := byName["progress.label"]
	require.NotNil(t, progress)
	assert.Contains(t, progress.SourceText(), "…")
	var progPh []string
	for _, run := range progress.SourceRuns() {
		if run.Ph != nil {
			progPh = append(progPh, run.Ph.Data)
		}
	}
	assert.Contains(t, progPh, "%%")
}

// TestStringsdictPluralModeling verifies plural-category leaves become distinct
// blocks and the %#@var@ token is protected on the format key.
func TestStringsdictPluralModeling(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "Localizable.stringsdict"))
	byName := blockByName(parts)

	// Format key for the first entry.
	fmtKey := byName["%d files selected"]
	require.NotNil(t, fmtKey)
	assert.Equal(t, "format", fmtKey.Properties["applestrings.leaf"])
	var fmtPh []string
	for _, run := range fmtKey.SourceRuns() {
		if run.Ph != nil {
			fmtPh = append(fmtPh, run.Ph.Data)
		}
	}
	assert.Contains(t, fmtPh, "%#@files@")

	one := byName["%d files selected/files/one"]
	require.NotNil(t, one)
	assert.Equal(t, "plural", one.Properties["applestrings.leaf"])
	assert.Equal(t, "files", one.Properties["applestrings.var"])
	assert.Equal(t, "one", one.Properties["applestrings.category"])
	assert.Equal(t, "NSStringPluralRuleType", one.Properties["applestrings.specType"])
	assert.Equal(t, "d", one.Properties["applestrings.valueType"])
	assert.Equal(t, "%d file selected", model.RenderRunsWithData(one.SourceRuns()))

	other := byName["%d files selected/files/other"]
	require.NotNil(t, other)
	assert.Equal(t, "%d files selected", model.RenderRunsWithData(other.SourceRuns()))

	// Second entry with zero/one/other and a trailing %@ on the format key.
	zero := byName["%d people in %@/people/zero"]
	require.NotNil(t, zero)
	assert.Equal(t, "Nobody", zero.SourceText())

	fmtKey2 := byName["%d people in %@"]
	require.NotNil(t, fmtKey2)
	var fmt2Ph []string
	for _, run := range fmtKey2.SourceRuns() {
		if run.Ph != nil {
			fmt2Ph = append(fmt2Ph, run.Ph.Data)
		}
	}
	assert.Contains(t, fmt2Ph, "%#@people@")
	assert.Contains(t, fmt2Ph, "%@")
}

// TestStringsTranslationUpdate verifies that changing a value rewrites only that
// entry and leaves all other bytes intact.
func TestStringsTranslationUpdate(t *testing.T) {
	parts, original := readParts(t, filepath.Join("testdata", "Localizable.strings"))
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if b.Name == "button.cancel" {
			b.SetTargetText("fr", "Annuler")
		}
	}
	out := writeParts(t, parts, "fr")
	assert.NotEqual(t, string(original), string(out))
	assert.Contains(t, string(out), `"button.cancel" = "Annuler";`)
	// Other entries untouched (the OK button keeps its value and its comment).
	assert.Contains(t, string(out), `"button.ok" = "OK";`)
	assert.Contains(t, string(out), "// A line comment preceding an entry")
	// Output still parses.
	reparsed := readPartsBytes(t, "out.strings", out)
	assert.Equal(t, "Annuler", blockByName(reparsed)["button.cancel"].SourceText())
}

// TestStringsTranslationPreservesPlaceholders verifies a translated value with a
// placeholder re-emits the exact specifier bytes.
func TestStringsTranslationPreservesPlaceholders(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "Localizable.strings"))
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if b.Name == "home.greeting" {
			// Translate keeping the placeholder run, prepend French text.
			runs := []model.Run{{Text: &model.TextRun{Text: "Bonjour, "}}}
			for _, r := range b.SourceRuns() {
				if r.Ph != nil {
					runs = append(runs, r)
				}
			}
			runs = append(runs, model.Run{Text: &model.TextRun{Text: " !"}})
			b.SetTargetRuns("fr", runs)
		}
	}
	out := writeParts(t, parts, "fr")
	assert.Contains(t, string(out), `"home.greeting" = "Bonjour, %@ !";`)
}

// TestStringsdictTranslationUpdate verifies updating one plural value rewrites
// only that <string> and preserves the rest of the plist byte-for-byte.
func TestStringsdictTranslationUpdate(t *testing.T) {
	parts, original := readParts(t, filepath.Join("testdata", "Localizable.stringsdict"))
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if b.Name == "%d people in %@/people/zero" {
			b.SetTargetText("de", "Niemand")
		}
	}
	out := writeParts(t, parts, "de")
	assert.NotEqual(t, string(original), string(out))
	assert.Contains(t, string(out), "<string>Niemand</string>")
	// The DOCTYPE and other plural values are untouched.
	assert.Contains(t, string(out), "<!DOCTYPE plist PUBLIC")
	assert.Contains(t, string(out), "<string>%d people</string>")
	// Output still parses with the new value.
	reparsed := readPartsBytes(t, "out.stringsdict", out)
	assert.Equal(t, "Niemand", blockByName(reparsed)["%d people in %@/people/zero"].SourceText())
}

// TestKindDetectionBySniff verifies content-based detection when the URI lacks a
// recognised extension.
func TestKindDetectionBySniff(t *testing.T) {
	dict, err := os.ReadFile(filepath.Join("testdata", "Localizable.stringsdict"))
	require.NoError(t, err)
	parts := readPartsBytes(t, "noext", dict)
	// Sniffed as stringsdict → plural leaves present.
	_, ok := blockByName(parts)["%d files selected/files/one"]
	assert.True(t, ok, "stringsdict should be detected by content sniff")

	strs, err := os.ReadFile(filepath.Join("testdata", "Localizable.strings"))
	require.NoError(t, err)
	parts2 := readPartsBytes(t, "noext2", strs)
	_, ok2 := blockByName(parts2)["button.cancel"]
	assert.True(t, ok2, "strings should be detected by content sniff")
}

// TestUTF16RoundTrip verifies a UTF-16LE .strings input is decoded, modeled, and
// re-encoded to UTF-16LE with the BOM preserved.
func TestUTF16RoundTrip(t *testing.T) {
	src := "\"hello\" = \"world\";\n"
	// Build UTF-16LE with BOM.
	utf16le := []byte{0xFF, 0xFE}
	for _, r := range src {
		utf16le = append(utf16le, byte(r), 0x00)
	}
	parts := readPartsBytes(t, "u.strings", utf16le)
	byName := blockByName(parts)
	require.Contains(t, byName, "hello")
	assert.Equal(t, "world", byName["hello"].SourceText())

	out := writeParts(t, parts, "")
	assert.Equal(t, utf16le, out, "UTF-16LE input must round-trip byte-for-byte")
}

// TestWriteStringsFromScratch verifies the writer can emit a valid .strings when
// no original document was read (synthetic pipeline).
func TestWriteStringsFromScratch(t *testing.T) {
	layer := &model.Layer{
		ID:         "doc1",
		Format:     "applestrings",
		Properties: map[string]string{"applestrings.kind": "strings", "applestrings.encoding": "utf-8"},
	}
	mk := func(id, key, value string) *model.Block {
		b := &model.Block{
			ID:           id,
			Translatable: true,
			SourceLocale: "en",
			Source:       []model.Run{{Text: &model.TextRun{Text: value}}},
			Targets:      map[model.VariantKey]*model.Target{},
			Properties:   map[string]string{"applestrings.key": key, "applestrings.leaf": "value"},
		}
		return b
	}
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: layer},
		{Type: model.PartBlock, Resource: mk("tu1", "greeting", "Hello")},
		{Type: model.PartBlock, Resource: mk("tu2", "farewell", "Goodbye")},
		{Type: model.PartLayerEnd, Resource: layer},
	}
	out := writeParts(t, parts, "")
	assert.Contains(t, string(out), `"greeting" = "Hello";`)
	assert.Contains(t, string(out), `"farewell" = "Goodbye";`)
	// Re-read.
	reparsed := readPartsBytes(t, "scratch.strings", out)
	assert.Equal(t, "Hello", blockByName(reparsed)["greeting"].SourceText())
}

// TestWriteStringsdictFromScratch verifies a canonical .stringsdict can be built
// when no original is available.
func TestWriteStringsdictFromScratch(t *testing.T) {
	layer := &model.Layer{
		ID:         "doc1",
		Format:     "applestrings",
		Properties: map[string]string{"applestrings.kind": "stringsdict", "applestrings.encoding": "utf-8"},
	}
	mkPlural := func(id, key, variable, cat, value string) *model.Block {
		return &model.Block{
			ID:           id,
			Translatable: true,
			SourceLocale: "en",
			Source:       []model.Run{{Text: &model.TextRun{Text: value}}},
			Targets:      map[model.VariantKey]*model.Target{},
			Properties: map[string]string{
				"applestrings.key":      key,
				"applestrings.leaf":     "plural",
				"applestrings.var":      variable,
				"applestrings.category": cat,
			},
		}
	}
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: layer},
		{Type: model.PartBlock, Resource: mkPlural("tu1", "items", "count", "one", "%d item")},
		{Type: model.PartBlock, Resource: mkPlural("tu2", "items", "count", "other", "%d items")},
		{Type: model.PartLayerEnd, Resource: layer},
	}
	out := writeParts(t, parts, "")
	assert.Contains(t, string(out), "<!DOCTYPE plist PUBLIC")
	assert.Contains(t, string(out), "<string>%d item</string>")
	assert.Contains(t, string(out), "<string>%d items</string>")
	// Re-read produces the plural leaves.
	reparsed := readPartsBytes(t, "scratch.stringsdict", out)
	byName := blockByName(reparsed)
	assert.Contains(t, byName, "items/count/one")
	assert.Contains(t, byName, "items/count/other")
}

// TestConfigProtectPlaceholdersOff verifies disabling placeholder protection
// keeps values as plain text (no Ph runs) and still round-trips.
func TestConfigProtectPlaceholdersOff(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "Localizable.strings"))
	require.NoError(t, err)

	r := applestrings.NewReader()
	cfg := r.Config().(*applestrings.Config)
	require.NoError(t, cfg.ApplyMap(map[string]any{"protectPlaceholders": false}))
	doc := &model.RawDocument{URI: "x.strings", Encoding: "UTF-8", Reader: nopReader(data)}
	require.NoError(t, r.Open(t.Context(), doc))
	defer r.Close()
	parts := testutil.CollectParts(t, r.Read(t.Context()))

	greeting := blockByName(parts)["home.greeting"]
	require.NotNil(t, greeting)
	for _, run := range greeting.SourceRuns() {
		assert.Nil(t, run.Ph, "no placeholder runs when protection is off")
	}
	assert.Equal(t, "Hello, %@!", greeting.SourceText())
}
