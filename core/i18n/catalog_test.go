package i18n

import (
	"bytes"
	"context"
	"testing"

	"github.com/leonelquinteros/gotext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats/mo"
	"github.com/neokapi/neokapi/core/model"
)

// buildMoCatalog creates an in-memory MO catalog from a set of
// (scope, source, target) triples, using the actual MO writer (so tests
// exercise the real encode + decode path end-to-end).
func buildMoCatalog(t *testing.T, locale string, entries ...[3]string) *gotext.Mo {
	t.Helper()

	w := mo.NewWriter()
	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	w.SetLocale(model.LocaleID(locale))

	parts := make(chan *model.Part, len(entries))
	for i, e := range entries {
		b := model.NewBlock(("tu" + itoa(i+1)), e[1])
		b.Name = e[0]
		b.SetTargetText(model.LocaleID(locale), e[2])
		parts <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(parts)
	require.NoError(t, w.Write(context.Background(), parts))

	m := gotext.NewMo()
	m.Parse(buf.Bytes())
	return m
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

func TestTranslator_HitReturnsTarget(t *testing.T) {
	cat := buildMoCatalog(t, "fr-FR",
		[3]string{"tools.ai-translate.displayName", "AI Translate", "Traduction IA"},
	)
	tr := NewTranslator("fr-FR", cat)
	assert.Equal(t, "Traduction IA",
		tr.T("tools.ai-translate.displayName", "AI Translate"))
}

func TestTranslator_MissReturnsSource(t *testing.T) {
	cat := buildMoCatalog(t, "fr-FR",
		[3]string{"tools.ai-translate.displayName", "AI Translate", "Traduction IA"},
	)
	tr := NewTranslator("fr-FR", cat)
	// Unknown scope → miss → source
	assert.Equal(t, "Some Description",
		tr.T("tools.unknown.description", "Some Description"))
	// Unknown source text in a known scope → miss → source
	assert.Equal(t, "Different source",
		tr.T("tools.ai-translate.displayName", "Different source"))
}

func TestTranslator_ScopeIsolation_SameSourceDifferentScopes(t *testing.T) {
	cat := buildMoCatalog(t, "fr-FR",
		[3]string{"tools.ai-translate.description", "Description", "Description de l'outil"},
		[3]string{"formats.okf_html.description", "Description", "Description du filtre"},
	)
	tr := NewTranslator("fr-FR", cat)
	assert.Equal(t, "Description de l'outil",
		tr.T("tools.ai-translate.description", "Description"))
	assert.Equal(t, "Description du filtre",
		tr.T("formats.okf_html.description", "Description"))
}

func TestTranslator_EmptyLocaleReturnsNoop(t *testing.T) {
	tr := NewTranslator("", nil)
	_, isNoop := tr.(NoopTranslator)
	assert.True(t, isNoop)
	assert.Equal(t, "Hello", tr.T("any", "Hello"))
	assert.Equal(t, model.LocaleID("en"), tr.Locale())
}

func TestTranslator_EnglishLocaleReturnsNoop(t *testing.T) {
	// "en" is the source language — no lookup is needed, so we skip
	// even-looking-at-catalogs and return a NoopTranslator.
	cat := buildMoCatalog(t, "en",
		[3]string{"x", "Hello", "Hello"},
	)
	tr := NewTranslator("en", cat)
	_, isNoop := tr.(NoopTranslator)
	assert.True(t, isNoop)
}

func TestTranslator_MultipleCatalogs_FirstHitWins(t *testing.T) {
	primary := buildMoCatalog(t, "fr-FR",
		[3]string{"x", "Hello", "Bonjour (primary)"},
	)
	secondary := buildMoCatalog(t, "fr-FR",
		[3]string{"x", "Hello", "Bonjour (secondary)"},
		[3]string{"y", "World", "Monde"},
	)
	tr := NewTranslator("fr-FR", primary, secondary)
	assert.Equal(t, "Bonjour (primary)", tr.T("x", "Hello"),
		"primary catalog must win when both have an entry")
	assert.Equal(t, "Monde", tr.T("y", "World"),
		"secondary catalog fills in what primary doesn't have")
}

func TestTranslator_NilCatalogsAreIgnored(t *testing.T) {
	cat := buildMoCatalog(t, "fr-FR",
		[3]string{"x", "Hello", "Bonjour"},
	)
	tr := NewTranslator("fr-FR", nil, cat, nil)
	assert.Equal(t, "Bonjour", tr.T("x", "Hello"))
}

func TestTranslator_EmptySourceReturnsEmpty(t *testing.T) {
	cat := buildMoCatalog(t, "fr-FR",
		[3]string{"x", "Hello", "Bonjour"},
	)
	tr := NewTranslator("fr-FR", cat)
	assert.Empty(t, tr.T("x", ""))
}
