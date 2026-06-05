package arb_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	arb "github.com/neokapi/neokapi/core/formats/arb"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readParts reads a fixture file and returns its parts plus the original bytes.
func readParts(t *testing.T, path string) ([]*model.Part, []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	r := arb.NewReader()
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: "",
		Encoding:     "UTF-8",
		Reader:       nopReadCloser(data),
	}
	require.NoError(t, r.Open(t.Context(), doc))
	defer r.Close()
	parts := testutil.CollectParts(t, r.Read(t.Context()))
	return parts, data
}

func nopReadCloser(data []byte) *nopCloser {
	return &nopCloser{Reader: bytes.NewReader(data)}
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

// writeParts feeds parts through the writer and returns the produced bytes.
func writeParts(t *testing.T, parts []*model.Part, locale model.LocaleID) []byte {
	t.Helper()
	w := arb.NewWriter()
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
		"simple_en.arb",
		"icu_en.arb",
		"translated_fr.arb",
		"compact_en.arb",
	}
}

func blocksByName(blocks []*model.Block) map[string]*model.Block {
	m := make(map[string]*model.Block, len(blocks))
	for _, b := range blocks {
		m[b.Name] = b
	}
	return m
}

// TestByteFaithfulRoundTrip verifies that reading then writing an unchanged
// ARB file reproduces the original bytes exactly, across pretty-printed and
// compact layouts, ICU constructs, escapes, and @/@@ metadata.
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

// TestReaderWriterImplementInterface ensures the reader/writer satisfy the
// format interfaces.
func TestReaderWriterImplementInterface(t *testing.T) {
	var _ format.DataFormatReader = arb.NewReader()
	var _ format.DataFormatWriter = arb.NewWriter()
}

// TestConfigInterfaces ensures the config satisfies the schema/kind providers.
func TestConfigInterfaces(t *testing.T) {
	r := arb.NewReader()
	cfg := r.Config()
	require.NotNil(t, cfg)

	sp, ok := cfg.(format.SchemaProvider)
	require.True(t, ok, "config must provide a schema")
	sc := sp.Schema()
	require.NotNil(t, sc)
	assert.Equal(t, "arb", sc.FormatMeta.ID)
	assert.Equal(t, []string{".arb"}, sc.FormatMeta.Extensions)

	ckp, ok := cfg.(format.ConfigKindProvider)
	require.True(t, ok, "config must provide a config kind")
	assert.NotEmpty(t, string(ckp.ConfigKind()))
}

// TestSimpleModeling checks plain messages, @-descriptions become notes, and
// placeholders are protected.
func TestSimpleModeling(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "simple_en.arb"))
	blocks := testutil.FilterBlocks(parts)

	// appTitle, githubRepo, aboutDialogDescription, cancel.
	require.Len(t, blocks, 4)
	byName := blocksByName(blocks)

	appTitle := byName["appTitle"]
	require.NotNil(t, appTitle)
	assert.Equal(t, "Flutter Gallery", appTitle.SourceText())
	note, ok := appTitle.Annotations["note"].(*model.NoteAnnotation)
	require.True(t, ok)
	assert.Equal(t, "The application title shown in the app bar.", note.Text)
	assert.Equal(t, "developer", note.From)

	// githubRepo carries a {repoName} placeholder that must be protected.
	githubRepo := byName["githubRepo"]
	require.NotNil(t, githubRepo)
	assert.Equal(t, "{repoName} GitHub repository", model.RenderRunsWithData(githubRepo.SourceRuns()))
	var phData []string
	for _, run := range githubRepo.SourceRuns() {
		if run.Ph != nil {
			phData = append(phData, run.Ph.Data)
		}
	}
	assert.Equal(t, []string{"{repoName}"}, phData)

	// cancel has no @-metadata, so no note.
	cancel := byName["cancel"]
	require.NotNil(t, cancel)
	_, hasNote := cancel.Annotations["note"]
	assert.False(t, hasNote)
}

// TestPlainTextIsTranslatable verifies the literal text around an ICU
// construct stays in a TextRun (translatable), with the construct protected.
func TestPlainTextIsTranslatable(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "icu_en.arb"))
	blocks := testutil.FilterBlocks(parts)
	byName := blocksByName(blocks)

	greeting := byName["greeting"]
	require.NotNil(t, greeting)
	runs := greeting.SourceRuns()
	// Expect: "Hello, " (text), {name} (ph), "!" (text).
	require.Len(t, runs, 3)
	require.NotNil(t, runs[0].Text)
	assert.Equal(t, "Hello, ", runs[0].Text.Text)
	require.NotNil(t, runs[1].Ph)
	assert.Equal(t, "{name}", runs[1].Ph.Data)
	require.NotNil(t, runs[2].Text)
	assert.Equal(t, "!", runs[2].Text.Text)
}

// TestPluralProtected verifies a full plural construct is one opaque
// placeholder, never split into translatable branches.
func TestPluralProtected(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "icu_en.arb"))
	byName := blocksByName(testutil.FilterBlocks(parts))

	itemCount := byName["itemCount"]
	require.NotNil(t, itemCount)
	runs := itemCount.SourceRuns()
	// The whole value is a single plural construct → one placeholder run.
	require.Len(t, runs, 1)
	require.NotNil(t, runs[0].Ph)
	assert.Equal(t, "{count, plural, =0{No items} =1{1 item} other{{count} items}}", runs[0].Ph.Data)
	// Rendering reproduces the value exactly (including the nested {count}).
	assert.Equal(t, "{count, plural, =0{No items} =1{1 item} other{{count} items}}",
		model.RenderRunsWithData(runs))
}

// TestSelectProtected verifies a select/gender construct is protected while the
// trailing literal text remains translatable text.
func TestSelectProtected(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "icu_en.arb"))
	byName := blocksByName(testutil.FilterBlocks(parts))

	pronoun := byName["pronoun"]
	require.NotNil(t, pronoun)
	runs := pronoun.SourceRuns()
	// "{gender, select, …}" (ph) + " liked your post." (text).
	require.Len(t, runs, 2)
	require.NotNil(t, runs[0].Ph)
	assert.Equal(t, "{gender, select, male{He} female{She} other{They}}", runs[0].Ph.Data)
	require.NotNil(t, runs[1].Text)
	assert.Equal(t, " liked your post.", runs[1].Text.Text)
}

// TestGlobalsAndAttributesNotExtracted verifies @@-globals and @-attributes
// produce no blocks but are preserved on round-trip.
func TestGlobalsAndAttributesNotExtracted(t *testing.T) {
	parts, original := readParts(t, filepath.Join("testdata", "icu_en.arb"))
	blocks := testutil.FilterBlocks(parts)
	byName := blocksByName(blocks)

	// itemCount, pronoun, greeting, lastUpdated — no @@locale/@@author block.
	require.Len(t, blocks, 4)
	for _, k := range []string{"@@locale", "@@author", "@itemCount", "@greeting"} {
		_, ok := byName[k]
		assert.Falsef(t, ok, "metadata key %q must not become a block", k)
	}

	// The layer carries the locale parsed from @@locale.
	var layer *model.Layer
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layer = p.Resource.(*model.Layer)
		}
	}
	require.NotNil(t, layer)
	assert.Equal(t, model.LocaleID("en"), layer.Locale)
	assert.Equal(t, "en", layer.Properties["arb.locale"])

	// Unchanged round-trip preserves the @@-globals and @-attributes verbatim.
	out := writeParts(t, parts, "")
	assert.Equal(t, string(original), string(out))
}

// TestTranslationUpdate verifies that changing a target value rewrites only
// that message string and leaves all other bytes intact.
func TestTranslationUpdate(t *testing.T) {
	parts, original := readParts(t, filepath.Join("testdata", "simple_en.arb"))

	// The file's locale is "en" (from filename/fallback), so blocks are
	// source-locale "en". Set a target value for "fr" and write as "fr".
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if b.Name == "cancel" {
			b.SetTargetText("fr", "Annuler")
		}
	}

	out := writeParts(t, parts, "fr")
	assert.NotEqual(t, string(original), string(out))
	assert.Contains(t, string(out), `"cancel": "Annuler"`)
	// Other messages are untouched.
	assert.Contains(t, string(out), `"appTitle": "Flutter Gallery"`)
	// @-metadata preserved.
	assert.Contains(t, string(out), `"description": "The application title shown in the app bar."`)

	// Output must still parse.
	r := arb.NewReader()
	require.NoError(t, r.Open(t.Context(), &model.RawDocument{
		URI: "out", Encoding: "UTF-8", Reader: nopReadCloser(out),
	}))
	defer r.Close()
	_ = testutil.CollectParts(t, r.Read(t.Context()))
}

// TestPlaceholderUpdatePreservesICU verifies that editing the translatable text
// around a placeholder rewrites the value while keeping the ICU placeholder
// bytes intact.
func TestPlaceholderUpdatePreservesICU(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "icu_en.arb"))

	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if b.Name == "greeting" {
			// Build French target runs: "Bonjour, " + {name} (preserved) + " !"
			src := b.SourceRuns()
			var target []model.Run
			target = append(target, model.Run{Text: &model.TextRun{Text: "Bonjour, "}})
			for _, run := range src {
				if run.Ph != nil {
					target = append(target, run)
				}
			}
			target = append(target, model.Run{Text: &model.TextRun{Text: " !"}})
			b.SetTargetRuns("fr", target)
		}
	}

	out := writeParts(t, parts, "fr")
	assert.Contains(t, string(out), `"greeting": "Bonjour, {name} !"`)
	// The plural/select messages were not given fr targets → fall back to
	// source, unchanged.
	assert.Contains(t, string(out), `"pronoun": "{gender, select, male{He} female{She} other{They}} liked your post."`)
}

// TestTranslatedTargetRoundTrip verifies a translation file (target-locale
// values) round-trips byte-for-byte, including escapes and an empty message.
func TestTranslatedTargetRoundTrip(t *testing.T) {
	parts, original := readParts(t, filepath.Join("testdata", "translated_fr.arb"))
	blocks := testutil.FilterBlocks(parts)
	byName := blocksByName(blocks)

	// The locale is fr; the value IS the source content for this monolingual file.
	quote := byName["quote"]
	require.NotNil(t, quote)
	assert.Equal(t, `Elle a dit "bonjour" à tous.`, quote.SourceText())

	path := byName["path"]
	require.NotNil(t, path)
	assert.Equal(t, `Chemin : C:\Users\test`, path.SourceText())

	empty := byName["emptyMessage"]
	require.NotNil(t, empty)
	assert.Empty(t, empty.SourceText())

	out := writeParts(t, parts, "")
	assert.Equal(t, string(original), string(out))
}

// TestWriteFromScratch verifies the writer emits a valid ARB document when no
// original was read (synthetic pipeline), and that it re-reads.
func TestWriteFromScratch(t *testing.T) {
	layer := &model.Layer{
		ID:         "doc1",
		Format:     "arb",
		Properties: map[string]string{"arb.locale": "en"},
	}
	mkBlock := func(id, key, value, desc string) *model.Block {
		b := &model.Block{
			ID:           id,
			Name:         key,
			Translatable: true,
			SourceLocale: "en",
			Source:       []model.Run{{Text: &model.TextRun{Text: value}}},
			Targets:      map[model.VariantKey]*model.Target{},
			Properties:   map[string]string{"arb.key": key},
			Annotations:  map[string]model.Annotation{},
		}
		if desc != "" {
			b.Annotations["note"] = &model.NoteAnnotation{Text: desc, From: "developer", Annotates: "general"}
		}
		return b
	}

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: layer},
		{Type: model.PartBlock, Resource: mkBlock("tu1", "hello", "Hello", "A greeting")},
		{Type: model.PartBlock, Resource: mkBlock("tu2", "bye", "Goodbye", "")},
		{Type: model.PartLayerEnd, Resource: layer},
	}

	out := writeParts(t, parts, "en")
	assert.Contains(t, string(out), `"@@locale": "en"`)
	assert.Contains(t, string(out), `"hello": "Hello"`)
	assert.Contains(t, string(out), `"@hello"`)
	assert.Contains(t, string(out), `"description": "A greeting"`)
	assert.Contains(t, string(out), `"bye": "Goodbye"`)

	// Re-read the synthetic output.
	r := arb.NewReader()
	require.NoError(t, r.Open(t.Context(), &model.RawDocument{
		URI: "out", Encoding: "UTF-8", Reader: nopReadCloser(out),
	}))
	defer r.Close()
	blocks := testutil.CollectBlocks(t, r.Read(t.Context()))
	byName := blocksByName(blocks)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello", byName["hello"].SourceText())
	note, ok := byName["hello"].Annotations["note"].(*model.NoteAnnotation)
	require.True(t, ok)
	assert.Equal(t, "A greeting", note.Text)
}

// TestConfigApplyMap verifies the config's ApplyMap parsing and validation.
func TestConfigApplyMap(t *testing.T) {
	cfg := &arb.Config{}
	cfg.Reset()
	assert.True(t, cfg.DescriptionNotes, "default should be on")

	require.NoError(t, cfg.ApplyMap(map[string]any{"descriptionNotes": false}))
	assert.False(t, cfg.DescriptionNotes)

	// Wrong type and unknown key are rejected.
	require.Error(t, cfg.ApplyMap(map[string]any{"descriptionNotes": "nope"}))
	require.Error(t, cfg.ApplyMap(map[string]any{"unknown": true}))

	require.NoError(t, cfg.Validate())
	assert.Equal(t, "arb", cfg.FormatName())
}
