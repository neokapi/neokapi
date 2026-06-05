package xcstrings_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	xcstrings "github.com/neokapi/neokapi/core/formats/xcstrings"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readParts reads a fixture file and returns its parts.
func readParts(t *testing.T, path string) ([]*model.Part, []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	r := xcstrings.NewReader()
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: "",
		Encoding:     "UTF-8",
		Reader:       ioNopCloser(data),
	}
	require.NoError(t, r.Open(t.Context(), doc))
	defer r.Close()
	parts := testutil.CollectParts(t, r.Read(t.Context()))
	return parts, data
}

// ioNopCloser wraps bytes in a ReadCloser.
func ioNopCloser(data []byte) *nopCloser {
	return &nopCloser{Reader: bytes.NewReader(data)}
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

// writeParts feeds parts through the writer (no locale set → preserve existing
// values) and returns the produced bytes.
func writeParts(t *testing.T, parts []*model.Part, locale model.LocaleID) []byte {
	t.Helper()
	w := xcstrings.NewWriter()
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

func fixtures() []string {
	return []string{
		"simple.xcstrings",
		"plural.xcstrings",
		"device.xcstrings",
		"substitutions.xcstrings",
		"untranslated.xcstrings",
	}
}

// TestByteFaithfulRoundTrip verifies that reading then writing an unchanged
// catalog reproduces the original bytes exactly.
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
	var _ format.DataFormatReader = xcstrings.NewReader()
	var _ format.DataFormatWriter = xcstrings.NewWriter()
}

// TestSimpleModeling checks notes, state, locales, and placeholder handling.
func TestSimpleModeling(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "simple.xcstrings"))
	blocks := testutil.FilterBlocks(parts)

	// Cancel/de, Cancel/fr, Hello/de, Hello/fr. Settings has no localizations.
	require.Len(t, blocks, 4)

	byName := make(map[string]*model.Block)
	for _, b := range blocks {
		byName[b.Name] = b
	}

	cancelDe := byName["Cancel/de"]
	require.NotNil(t, cancelDe)
	assert.Equal(t, "Abbrechen", model.RenderRunsWithData(cancelDe.TargetRuns("de")))
	assert.Equal(t, "Cancel", model.RenderRunsWithData(cancelDe.SourceRuns()))
	assert.Equal(t, "translated", cancelDe.Properties["state"])
	note, ok := cancelDe.Annotations["note"].(*model.NoteAnnotation)
	require.True(t, ok)
	assert.Equal(t, "Button title to dismiss the sheet", note.Text)
	assert.Equal(t, "developer", note.From)

	// Placeholder %@ must be preserved as an inline code, not flattened text.
	helloFr := byName["Hello, %@!/fr"]
	require.NotNil(t, helloFr)
	assert.Equal(t, "Bonjour, %@ !", model.RenderRunsWithData(helloFr.TargetRuns("fr")))
	// The %@ specifier should be a placeholder run in the source.
	hasPh := false
	for _, run := range helloFr.SourceRuns() {
		if run.Ph != nil {
			hasPh = true
			assert.Equal(t, "%@", run.Ph.Data)
		}
	}
	assert.True(t, hasPh, "expected a placeholder run for %%@")
	assert.Equal(t, "manual", helloFr.Properties["xcstrings.extractionState"])
}

// TestPluralModeling verifies plural-category leaves become distinct blocks.
func TestPluralModeling(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "plural.xcstrings"))
	blocks := testutil.FilterBlocks(parts)

	byName := make(map[string]*model.Block)
	for _, b := range blocks {
		byName[b.Name] = b
	}

	// English source variations (one/other) + Russian (few/many/one/other).
	enOther := byName["%lld items selected/en/other"]
	require.NotNil(t, enOther)
	assert.Equal(t, "%lld items selected", model.RenderRunsWithData(enOther.SourceRuns()))

	ruMany := byName["%lld items selected/ru/many"]
	require.NotNil(t, ruMany)
	assert.Equal(t, "Выбрано %lld элементов", model.RenderRunsWithData(ruMany.TargetRuns("ru")))
	assert.Equal(t, "plural", ruMany.Properties["xcstrings.kind"])
	assert.Equal(t, "many", ruMany.Properties["xcstrings.category"])
}

// TestDeviceModeling verifies device-class variations.
func TestDeviceModeling(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "device.xcstrings"))
	blocks := testutil.FilterBlocks(parts)

	byName := make(map[string]*model.Block)
	for _, b := range blocks {
		byName[b.Name] = b
	}

	frMac := byName["Tap to continue/fr/mac"]
	require.NotNil(t, frMac)
	assert.Equal(t, "Cliquez pour continuer", frMac.TargetText("fr"))
	assert.Equal(t, "device", frMac.Properties["xcstrings.kind"])
	assert.Equal(t, "mac", frMac.Properties["xcstrings.category"])
}

// TestSubstitutionModeling verifies substitution plural leaves.
func TestSubstitutionModeling(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "substitutions.xcstrings"))
	blocks := testutil.FilterBlocks(parts)

	byName := make(map[string]*model.Block)
	for _, b := range blocks {
		byName[b.Name] = b
	}

	require.Len(t, blocks, 2) // one + other under photo_count
	one := byName["%1$@ has %2$lld photos in %3$@/en/photo_count/one"]
	require.NotNil(t, one)
	assert.Equal(t, "subPlural", one.Properties["xcstrings.kind"])
	assert.Equal(t, "photo_count", one.Properties["xcstrings.sub"])
	assert.Equal(t, "one", one.Properties["xcstrings.category"])
	assert.Equal(t, "%1$@ has %arg photo in %3$@", model.RenderRunsWithData(one.SourceRuns()))
}

// TestUntranslatedModeling verifies empty localizations and new/needs_review
// states round-trip and are modeled.
func TestUntranslatedModeling(t *testing.T) {
	parts, original := readParts(t, filepath.Join("testdata", "untranslated.xcstrings"))
	blocks := testutil.FilterBlocks(parts)

	byName := make(map[string]*model.Block)
	for _, b := range blocks {
		byName[b.Name] = b
	}

	// Done/es (state new, empty value) and Welcome/es (needs_review).
	doneEs := byName["Done/es"]
	require.NotNil(t, doneEs)
	assert.Equal(t, "new", doneEs.Properties["state"])
	assert.Empty(t, doneEs.TargetText("es"))

	welcome := byName["Welcome to %@/es"]
	require.NotNil(t, welcome)
	assert.Equal(t, "needs_review", welcome.Properties["state"])
	assert.Equal(t, "Bienvenido a %@", model.RenderRunsWithData(welcome.TargetRuns("es")))

	// "About" has empty localizations object → no blocks, but must round-trip.
	_, ok := byName["About/es"]
	assert.False(t, ok)

	out := writeParts(t, parts, "")
	assert.Equal(t, string(original), string(out))
}

// TestTranslationUpdate verifies that changing a target value rewrites only
// that value and leaves all other bytes intact.
func TestTranslationUpdate(t *testing.T) {
	parts, original := readParts(t, filepath.Join("testdata", "simple.xcstrings"))

	// Mutate the German translation of "Cancel".
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if b.Name == "Cancel/de" {
			b.SetTargetText("de", "Abbruch")
		}
	}

	out := writeParts(t, parts, "de")
	assert.NotEqual(t, string(original), string(out))
	assert.Contains(t, string(out), "Abbruch")
	assert.NotContains(t, string(out), "Abbrechen")
	// French value untouched.
	assert.Contains(t, string(out), "Annuler")
	// The output must still parse.
	r := xcstrings.NewReader()
	require.NoError(t, r.Open(t.Context(), &model.RawDocument{
		URI: "out", Encoding: "UTF-8", Reader: ioNopCloser(out),
	}))
	defer r.Close()
	_ = testutil.CollectParts(t, r.Read(t.Context()))
}

// TestStateUpdate verifies that updating both value and state rewrites both.
func TestStateUpdate(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "untranslated.xcstrings"))
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if b.Name == "Done/es" {
			b.SetTargetText("es", "Listo")
			b.Properties["state"] = "translated"
		}
	}
	out := writeParts(t, parts, "es")
	assert.Contains(t, string(out), "Listo")
	assert.Contains(t, string(out), `"state" : "translated"`)
}

// TestArgTokenSingleRun ensures %arg is treated as a single opaque placeholder
// rather than split into "%a" + "rg".
func TestArgTokenSingleRun(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "substitutions.xcstrings"))
	blocks := testutil.FilterBlocks(parts)
	var one *model.Block
	for _, b := range blocks {
		if b.Properties["xcstrings.category"] == "one" {
			one = b
		}
	}
	require.NotNil(t, one)
	var phData []string
	for _, run := range one.SourceRuns() {
		if run.Ph != nil {
			phData = append(phData, run.Ph.Data)
		}
	}
	assert.Contains(t, phData, "%arg")
	assert.Contains(t, phData, "%1$@")
	assert.Contains(t, phData, "%3$@")
}

// TestWriteFromScratch verifies that the writer can emit a valid catalog when
// no original document was read (synthetic pipeline). The output must be
// re-readable.
func TestWriteFromScratch(t *testing.T) {
	layer := &model.Layer{
		ID:     "doc1",
		Format: "xcstrings",
		Properties: map[string]string{
			"xcstrings.sourceLanguage": "en",
			"xcstrings.version":        "1.0",
		},
	}
	mkBlock := func(id, key, lang, value, state string, src model.LocaleID) *model.Block {
		b := &model.Block{
			ID:           id,
			Translatable: true,
			SourceLocale: src,
			Source:       []model.Run{{Text: &model.TextRun{Text: value}}},
			Targets:      map[model.VariantKey]*model.Target{},
			Properties:   map[string]string{},
			Annotations:  map[string]model.Annotation{},
		}
		// Mirror the property scheme the reader uses (see path.go).
		b.Properties["xcstrings.key"] = key
		b.Properties["xcstrings.lang"] = lang
		b.Properties["xcstrings.kind"] = "stringUnit"
		b.Properties["state"] = state
		if model.LocaleID(lang) != src {
			b.SetTargetText(model.LocaleID(lang), value)
		}
		return b
	}

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: layer},
		{Type: model.PartBlock, Resource: mkBlock("tu1", "Hello", "fr", "Bonjour", "translated", "en")},
		{Type: model.PartLayerEnd, Resource: layer},
	}

	out := writeParts(t, parts, "fr")
	// Re-read.
	r := xcstrings.NewReader()
	require.NoError(t, r.Open(t.Context(), &model.RawDocument{
		URI: "out", Encoding: "UTF-8", Reader: ioNopCloser(out),
	}))
	defer r.Close()
	blocks := testutil.CollectBlocks(t, r.Read(t.Context()))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Bonjour", model.RenderRunsWithData(blocks[0].TargetRuns("fr")))
	assert.Contains(t, string(out), `"version" : "1.0"`)
	assert.Contains(t, string(out), `"sourceLanguage" : "en"`)
}
