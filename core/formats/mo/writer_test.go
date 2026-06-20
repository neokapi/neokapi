package mo

import (
	"bytes"
	"context"
	"testing"

	"github.com/leonelquinteros/gotext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// block is a small helper that constructs a Block with a single source run,
// a single target run for the given locale, and optional Properties.
func block(id, name, src, targetLocale, tgt string, props map[string]string) *model.Block {
	b := model.NewBlock(id, src)
	b.Name = name
	b.Properties = props
	b.SetTargetText(model.LocaleID(targetLocale), tgt)
	return b
}

func feed(parts chan<- *model.Part, blocks ...*model.Block) {
	for _, b := range blocks {
		parts <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(parts)
}

// Run the writer and return the MO bytes. Shared helper to keep tests short.
func write(t *testing.T, locale string, blocks ...*model.Block) []byte {
	t.Helper()
	w := NewWriter()
	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	w.SetLocale(model.LocaleID(locale))

	parts := make(chan *model.Part, len(blocks))
	go feed(parts, blocks...)
	require.NoError(t, w.Write(context.Background(), parts))
	return buf.Bytes()
}

// parse runs the gotext loader over the produced bytes so we test against a
// real third-party MO reader, not our own encoder's mirror.
func parse(t *testing.T, data []byte) *gotext.Mo {
	t.Helper()
	mo := gotext.NewMo()
	mo.Parse(data)
	return mo
}

func TestWriter_RoundTrip_WithContext(t *testing.T) {
	data := write(t, "fr-FR",
		block("tu1", "/tools/translate/displayName",
			"AI Translate", "fr-FR", "Traduction IA", nil),
		block("tu2", "/tools/translate/description",
			"Translate content with an LLM provider", "fr-FR",
			"Traduire le contenu avec un LLM", nil),
		block("tu3", "/formats/okf_html/displayName",
			"HTML Filter", "fr-FR", "Filtre HTML", nil),
	)

	mo := parse(t, data)
	// Same source text in different contexts resolves independently via msgctxt.
	assert.Equal(t, "Traduction IA",
		mo.GetC("AI Translate", "/tools/translate/displayName"))
	assert.Equal(t, "Traduire le contenu avec un LLM",
		mo.GetC("Translate content with an LLM provider",
			"/tools/translate/description"))
	assert.Equal(t, "Filtre HTML",
		mo.GetC("HTML Filter", "/formats/okf_html/displayName"))
}

func TestWriter_ContextPrecedence_PropertiesOverName(t *testing.T) {
	// Explicit Properties["context"] wins over Block.Name.
	b := block("tu1", "/tools/translate/displayName",
		"AI Translate", "fr-FR", "Traduction IA",
		map[string]string{"context": "explicit/ctx"})

	data := write(t, "fr-FR", b)
	mo := parse(t, data)

	assert.Equal(t, "Traduction IA", mo.GetC("AI Translate", "explicit/ctx"))
	// The Name-derived context must NOT be populated.
	assert.Equal(t, "AI Translate",
		mo.GetC("AI Translate", "/tools/translate/displayName"),
		"should fall through to source text when context key has no entry")
}

func TestWriter_SkipsUntranslated(t *testing.T) {
	// Block with no target for the requested locale is dropped.
	b := model.NewBlock("tu1", "Only source")
	b.Name = "/something"

	data := write(t, "fr-FR", b)
	mo := parse(t, data)

	// Lookup returns the source unchanged (miss semantics).
	assert.Equal(t, "Only source", mo.GetC("Only source", "/something"))
}

func TestWriter_ScopeIsolation(t *testing.T) {
	// Homonym source text "Description" in two different scopes stays separate.
	data := write(t, "fr-FR",
		block("tu1", "/tools/translate/description",
			"Description", "fr-FR", "Description de l'outil", nil),
		block("tu2", "/formats/okf_html/description",
			"Description", "fr-FR", "Description du filtre", nil),
	)

	mo := parse(t, data)
	assert.Equal(t, "Description de l'outil",
		mo.GetC("Description", "/tools/translate/description"))
	assert.Equal(t, "Description du filtre",
		mo.GetC("Description", "/formats/okf_html/description"))
}

func TestWriter_EmptyLocale_EmitsHeaderOnly(t *testing.T) {
	// An empty locale drops every entry (no translations selectable) but the
	// MO file must still be well-formed — i.e. contain the required empty-msgid
	// metadata header entry.
	data := write(t, "",
		block("tu1", "/x", "Hello", "fr-FR", "Bonjour", nil),
	)
	mo := parse(t, data)
	assert.Equal(t, "Hello", mo.Get("Hello"), "untranslated lookup returns source")
	require.GreaterOrEqual(t, len(data), 28, "MO file must be at least the 28-byte header")
}

func TestWriter_DeterministicOutput(t *testing.T) {
	// Feeding the same blocks twice must produce byte-identical output.
	blocks := []*model.Block{
		block("tu1", "/a", "Alpha", "fr-FR", "Alpha-FR", nil),
		block("tu2", "/b", "Beta", "fr-FR", "Beta-FR", nil),
		block("tu3", "/c", "Gamma", "fr-FR", "Gamma-FR", nil),
	}
	first := write(t, "fr-FR", blocks...)
	second := write(t, "fr-FR", blocks...)
	assert.Equal(t, first, second)
}
