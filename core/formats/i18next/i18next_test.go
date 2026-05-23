package i18next_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/i18next"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newResolver builds a real format registry with the json, html, and i18next
// formats registered so the `_html` HTML subfilter resolves against the actual
// html reader/writer (no mocks).
func newResolver(t *testing.T) *registry.FormatRegistry {
	t.Helper()
	reg := registry.NewFormatRegistry()
	reg.RegisterReader("json", func() format.DataFormatReader { return jsonfmt.NewReader() },
		format.FormatSignature{}, "JSON")
	reg.RegisterWriter("json", func() format.DataFormatWriter { return jsonfmt.NewWriter() })
	reg.RegisterReader("html", func() format.DataFormatReader { return html.NewReader() },
		format.FormatSignature{}, "HTML")
	reg.RegisterWriter("html", func() format.DataFormatWriter { return html.NewWriter() })
	reg.RegisterReader("i18next", func() format.DataFormatReader { return i18next.NewReader() },
		format.FormatSignature{}, "i18next JSON")
	reg.RegisterWriter("i18next", func() format.DataFormatWriter { return i18next.NewWriter() })
	return reg
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

func readParts(t *testing.T, path string, resolver format.SubfilterResolver) ([]*model.Part, []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	r := i18next.NewReader()
	if resolver != nil {
		r.SetSubfilterResolver(resolver)
	}
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: "en-US",
		Encoding:     "UTF-8",
		Reader:       &nopCloser{bytes.NewReader(data)},
	}
	require.NoError(t, r.Open(t.Context(), doc))
	defer r.Close()
	parts := testutil.CollectParts(t, r.Read(t.Context()))
	return parts, data
}

func writeParts(t *testing.T, parts []*model.Part, locale model.LocaleID, resolver format.SubfilterResolver) []byte {
	t.Helper()
	w := i18next.NewWriter()
	if resolver != nil {
		w.SetSubfilterResolver(resolver)
	}
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

// collectBlocks returns the top-level i18next blocks (those not inside a child
// subfilter layer), preserving stream order.
func collectBlocks(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	depth := 0
	for _, p := range parts {
		switch p.Type {
		case model.PartLayerStart:
			if l, ok := p.Resource.(*model.Layer); ok && !l.IsRoot() {
				depth++
			}
		case model.PartLayerEnd:
			if l, ok := p.Resource.(*model.Layer); ok && !l.IsRoot() {
				depth--
			}
		case model.PartBlock:
			if depth == 0 {
				if b, ok := p.Resource.(*model.Block); ok {
					blocks = append(blocks, b)
				}
			}
		}
	}
	return blocks
}

func blocksByName(blocks []*model.Block) map[string]*model.Block {
	m := make(map[string]*model.Block, len(blocks))
	for _, b := range blocks {
		m[b.Name] = b
	}
	return m
}

func fixtures() []string {
	return []string{
		"namespaces_en.json",
		"plurals_v4_en.json",
		"plurals_legacy_en.json",
		"context_en.json",
		"interpolation_en.json",
		"compact_en.json",
	}
}

// TestByteFaithfulRoundTrip verifies that reading then writing an unchanged
// i18next file reproduces the original bytes exactly, across nested namespaces,
// interpolation, $t() nesting, embedded HTML, plurals, context keys, and a
// compact single-line layout.
func TestByteFaithfulRoundTrip(t *testing.T) {
	resolver := newResolver(t)
	for _, name := range fixtures() {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name)
			parts, original := readParts(t, path, resolver)
			out := writeParts(t, parts, "", resolver)
			assert.Equal(t, string(original), string(out),
				"round-trip should reproduce %s byte-for-byte", name)
		})
	}
}

// TestFormatIdentity verifies the format reports its i18next identity and
// relabels the root layer's format, while claiming no extension/MIME so it does
// not steal generic .json detection.
func TestFormatIdentity(t *testing.T) {
	r := i18next.NewReader()
	assert.Equal(t, "i18next", r.Name())
	assert.Equal(t, "i18next JSON", r.DisplayName())

	sig := r.Signature()
	assert.Empty(t, sig.Extensions, "i18next must not claim any extension")
	assert.Empty(t, sig.MIMETypes, "i18next must not claim application/json")

	w := i18next.NewWriter()
	assert.Equal(t, "i18next", w.Name())

	parts, _ := readParts(t, filepath.Join("testdata", "compact_en.json"), newResolver(t))
	require.NotEmpty(t, parts)
	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.True(t, layer.IsRoot())
	assert.Equal(t, "i18next", layer.Format, "root layer relabelled to i18next")
}

// TestInterpolationProtected verifies that {{var}}, {{var, format}}, and $t()
// nesting are protected as inline placeholder runs and contribute no
// translatable text.
func TestInterpolationProtected(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "interpolation_en.json"), newResolver(t))
	byName := blocksByName(collectBlocks(parts))

	type want struct {
		placeholders []string
		plainText    string
	}
	cases := map[string]want{
		"/welcome": {
			placeholders: []string{"{{name}}", "{{count}}"},
			plainText:    "Welcome , you have  new messages",
		},
		"/formatted": {
			placeholders: []string{"{{value, currency}}", "{{date, datetime}}"},
			plainText:    "Total:  as of ",
		},
		"/nested": {
			placeholders: []string{"$t(welcome)", "$t(common:terms)"},
			plainText:    " Please review .",
		},
		"/mixed": {
			placeholders: []string{"{{user}}", "$t(home.footer)"},
			plainText:    "Hi  — see  for details",
		},
		"/noVars": {
			placeholders: nil,
			plainText:    "A plain sentence with no interpolation.",
		},
	}

	for name, w := range cases {
		t.Run(name, func(t *testing.T) {
			b := byName[name]
			require.NotNil(t, b, "missing block %s", name)
			require.Len(t, b.Source, 1)

			var phData []string
			for _, run := range b.Source[0].Runs {
				if run.Ph != nil {
					phData = append(phData, run.Ph.Data)
				}
			}
			assert.Equal(t, w.placeholders, phData, "protected inline codes")
			assert.Equal(t, w.plainText, b.Source[0].Text(), "translatable text excludes codes")
		})
	}
}

// TestPluralsV4Annotated verifies that v4 CLDR plural sibling keys are emitted
// as one block each, annotated with base key, CLDR category, and a shared
// plural-group id scoped by namespace.
func TestPluralsV4Annotated(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "plurals_v4_en.json"), newResolver(t))
	byName := blocksByName(collectBlocks(parts))

	// Top-level key_one / key_other.
	one := byName["/key_one"]
	other := byName["/key_other"]
	require.NotNil(t, one)
	require.NotNil(t, other)
	assert.Equal(t, "key", one.Properties["i18next.baseKey"])
	assert.Equal(t, "one", one.Properties["i18next.pluralCategory"])
	assert.Equal(t, "key", other.Properties["i18next.baseKey"])
	assert.Equal(t, "other", other.Properties["i18next.pluralCategory"])
	// Same group id for the sibling set (top-level → bare base key).
	assert.Equal(t, "key", one.Properties["i18next.pluralGroup"])
	assert.Equal(t, one.Properties["i18next.pluralGroup"], other.Properties["i18next.pluralGroup"])
	assert.NotContains(t, one.Properties, "i18next.pluralLegacy")

	// Nested cart.items_* — group id scoped by the parent namespace.
	cats := []string{"zero", "one", "two", "few", "many", "other"}
	var groupID string
	for _, cat := range cats {
		b := byName["/cart/items_"+cat]
		require.NotNil(t, b, "missing cart.items_%s", cat)
		assert.Equal(t, "items", b.Properties["i18next.baseKey"])
		assert.Equal(t, cat, b.Properties["i18next.pluralCategory"])
		if groupID == "" {
			groupID = b.Properties["i18next.pluralGroup"]
		}
		assert.Equal(t, "cart.items", b.Properties["i18next.pluralGroup"])
		assert.Equal(t, groupID, b.Properties["i18next.pluralGroup"])
	}
}

// TestPluralsLegacyAnnotated verifies the legacy key_plural and key_N forms are
// recognised and flagged as legacy.
func TestPluralsLegacyAnnotated(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "plurals_legacy_en.json"), newResolver(t))
	byName := blocksByName(collectBlocks(parts))

	// "key" (no suffix) is not a plural on its own — it is the singular base and
	// carries no plural annotation.
	base := byName["/key"]
	require.NotNil(t, base)
	assert.NotContains(t, base.Properties, "i18next.pluralCategory")

	// key_plural → other, flagged legacy.
	plural := byName["/key_plural"]
	require.NotNil(t, plural)
	assert.Equal(t, "key", plural.Properties["i18next.baseKey"])
	assert.Equal(t, "other", plural.Properties["i18next.pluralCategory"])
	assert.Equal(t, "true", plural.Properties["i18next.pluralLegacy"])
	assert.Equal(t, "plural", plural.Properties["i18next.pluralLegacyForm"])

	// Numeric indexed forms.
	zero := byName["/keyWithCount_0"]
	require.NotNil(t, zero)
	assert.Equal(t, "keyWithCount", zero.Properties["i18next.baseKey"])
	assert.Equal(t, "one", zero.Properties["i18next.pluralCategory"])
	assert.Equal(t, "0", zero.Properties["i18next.pluralLegacyForm"])

	two := byName["/keyWithCount_2"]
	require.NotNil(t, two)
	assert.Equal(t, "keyWithCount", two.Properties["i18next.baseKey"])
	assert.Equal(t, "2", two.Properties["i18next.pluralLegacyForm"])
}

// TestLegacyPluralFormsDisabled verifies that disabling LegacyPluralForms stops
// key_plural and key_N being treated as plurals (they then read as context).
func TestLegacyPluralFormsDisabled(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "plurals_legacy_en.json"))
	require.NoError(t, err)

	r := i18next.NewReader()
	cfg := &i18next.Config{}
	cfg.Reset()
	cfg.LegacyPluralForms = false
	require.NoError(t, r.SetConfig(cfg))

	doc := &model.RawDocument{
		URI:          "legacy.json",
		SourceLocale: "en-US",
		Encoding:     "UTF-8",
		Reader:       &nopCloser{bytes.NewReader(data)},
	}
	require.NoError(t, r.Open(t.Context(), doc))
	defer r.Close()
	byName := blocksByName(collectBlocks(testutil.CollectParts(t, r.Read(t.Context()))))

	plural := byName["/key_plural"]
	require.NotNil(t, plural)
	assert.NotContains(t, plural.Properties, "i18next.pluralCategory",
		"legacy plural disabled → not a plural")
	assert.Equal(t, "plural", plural.Properties["i18next.context"],
		"with legacy off, the _plural suffix reads as context")
}

// TestContextAnnotated verifies context sibling keys are annotated with base key
// and context, including combined context+plural keys.
func TestContextAnnotated(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "context_en.json"), newResolver(t))
	byName := blocksByName(collectBlocks(parts))

	// Plain base — no annotation.
	base := byName["/friend"]
	require.NotNil(t, base)
	assert.NotContains(t, base.Properties, "i18next.context")
	assert.NotContains(t, base.Properties, "i18next.baseKey")

	male := byName["/friend_male"]
	require.NotNil(t, male)
	assert.Equal(t, "friend", male.Properties["i18next.baseKey"])
	assert.Equal(t, "male", male.Properties["i18next.context"])
	assert.NotContains(t, male.Properties, "i18next.pluralCategory")

	female := byName["/friend_female"]
	require.NotNil(t, female)
	assert.Equal(t, "female", female.Properties["i18next.context"])

	// Combined context + plural: invite_male_one.
	combo := byName["/invite_male_one"]
	require.NotNil(t, combo)
	assert.Equal(t, "invite", combo.Properties["i18next.baseKey"])
	assert.Equal(t, "male", combo.Properties["i18next.context"])
	assert.Equal(t, "one", combo.Properties["i18next.pluralCategory"])
	assert.Equal(t, "invite", combo.Properties["i18next.pluralGroup"])

	comboOther := byName["/invite_female_other"]
	require.NotNil(t, comboOther)
	assert.Equal(t, "female", comboOther.Properties["i18next.context"])
	assert.Equal(t, "other", comboOther.Properties["i18next.pluralCategory"])
}

// TestNestedNamespaceNames verifies nested objects produce full key-path block
// names. With the default config (HTML subfilter off), every value — including
// the _html one — is a top-level i18next block, keeping round-trip byte-faithful.
func TestNestedNamespaceNames(t *testing.T) {
	resolver := newResolver(t)
	parts, _ := readParts(t, filepath.Join("testdata", "namespaces_en.json"), resolver)
	byName := blocksByName(collectBlocks(parts))

	assert.Contains(t, byName, "/common/appName")
	assert.Contains(t, byName, "/common/actions/save")
	assert.Contains(t, byName, "/common/actions/cancel")
	assert.Contains(t, byName, "/home/title")
	assert.Contains(t, byName, "/home/greeting")
	assert.Contains(t, byName, "/home/body_html", "default: _html value is a plain block")

	// {{appName}} protected in the title.
	title := byName["/home/title"]
	require.NotNil(t, title)
	var phData []string
	for _, run := range title.Source[0].Runs {
		if run.Ph != nil {
			phData = append(phData, run.Ph.Data)
		}
	}
	assert.Equal(t, []string{"{{appName}}"}, phData)

	// No embedded HTML child layer when the subfilter is off.
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			if l, ok := p.Resource.(*model.Layer); ok {
				assert.False(t, l.IsEmbedded(), "no embedded layers by default")
			}
		}
	}
}

// TestHTMLSubfilterOptIn verifies that enabling the HTML subfilter routes the
// _html value to an embedded HTML child layer (so its tags are protected), at
// the documented cost of byte-faithful round-trip for that value.
func TestHTMLSubfilterOptIn(t *testing.T) {
	resolver := newResolver(t)
	data, err := os.ReadFile(filepath.Join("testdata", "namespaces_en.json"))
	require.NoError(t, err)

	r := i18next.NewReader()
	r.SetSubfilterResolver(resolver)
	cfg := &i18next.Config{}
	cfg.Reset()
	cfg.SubfilterHTMLValues = true
	require.NoError(t, r.SetConfig(cfg))

	doc := &model.RawDocument{
		URI:          "namespaces.json",
		SourceLocale: "en-US",
		Encoding:     "UTF-8",
		Reader:       &nopCloser{bytes.NewReader(data)},
	}
	require.NoError(t, r.Open(t.Context(), doc))
	defer r.Close()
	parts := testutil.CollectParts(t, r.Read(t.Context()))

	var htmlChild bool
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			if l, ok := p.Resource.(*model.Layer); ok && l.IsEmbedded() && l.Format == "html" {
				htmlChild = true
			}
		}
	}
	assert.True(t, htmlChild, "body_html value should be subfiltered as HTML when enabled")

	// body_html is therefore not a top-level i18next block.
	byName := blocksByName(collectBlocks(parts))
	assert.NotContains(t, byName, "/home/body_html")
}

// TestTranslatedWrite verifies that applying target translations and writing for
// a target locale substitutes the translated values while preserving the
// surrounding structure and protected interpolation.
func TestTranslatedWrite(t *testing.T) {
	resolver := newResolver(t)
	parts, _ := readParts(t, filepath.Join("testdata", "plurals_v4_en.json"), resolver)

	const fr = model.LocaleID("fr-FR")
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok {
			continue
		}
		switch b.Name {
		case "/key_one":
			b.SetTargetText(fr, "{{count}} article")
		case "/key_other":
			b.SetTargetText(fr, "{{count}} articles")
		}
	}

	out := writeParts(t, parts, fr, resolver)
	s := string(out)
	assert.Contains(t, s, `"key_one": "{{count}} article"`)
	assert.Contains(t, s, `"key_other": "{{count}} articles"`)
	// Untranslated nested values are preserved verbatim.
	assert.Contains(t, s, `"items_zero": "Your cart is empty"`)
}

// TestConfigApplyMap verifies the config parses its toggles and rejects unknown
// keys and bad types.
func TestConfigApplyMap(t *testing.T) {
	c := &i18next.Config{}
	c.Reset()
	assert.True(t, c.ProtectInterpolation)
	assert.False(t, c.SubfilterHTMLValues, "HTML subfilter is opt-in for byte-faithfulness")
	assert.True(t, c.LegacyPluralForms)

	require.NoError(t, c.ApplyMap(map[string]any{
		"protectInterpolation": false,
		"subfilterHtmlValues":  true,
		"legacyPluralForms":    false,
	}))
	assert.False(t, c.ProtectInterpolation)
	assert.True(t, c.SubfilterHTMLValues)
	assert.False(t, c.LegacyPluralForms)

	require.Error(t, c.ApplyMap(map[string]any{"protectInterpolation": "nope"}))
	require.Error(t, c.ApplyMap(map[string]any{"unknownKey": true}))
}

// TestSchema verifies the format exposes a schema with the i18next identity and
// no extension/MIME claim.
func TestSchema(t *testing.T) {
	c := &i18next.Config{}
	c.Reset()
	s := c.Schema()
	require.NotNil(t, s)
	assert.Equal(t, "i18next", s.FormatMeta.ID)
	assert.Empty(t, s.FormatMeta.Extensions)
	assert.Empty(t, s.FormatMeta.MimeTypes)
	assert.Contains(t, s.Properties, "protectInterpolation")
	assert.Contains(t, s.Properties, "subfilterHtmlValues")
	assert.Contains(t, s.Properties, "legacyPluralForms")
}

// TestMalformedJSON verifies that malformed JSON input surfaces a clean error
// through the read channel (delegated from the inner JSON reader) rather than
// panicking. An unterminated string is invalid even under the inner reader's
// lenient JSON5 mode.
func TestMalformedJSON(t *testing.T) {
	r := i18next.NewReader()
	malformed := []byte(`{"key": "unterminated`)
	doc := &model.RawDocument{
		URI:          "malformed.json",
		SourceLocale: "en-US",
		Encoding:     "UTF-8",
		Reader:       &nopCloser{bytes.NewReader(malformed)},
	}
	require.NoError(t, r.Open(t.Context(), doc))
	defer r.Close()

	var foundError bool
	require.NotPanics(t, func() {
		for pr := range r.Read(t.Context()) {
			if pr.Error != nil {
				foundError = true
			}
		}
	})
	assert.True(t, foundError, "expected a clean error for malformed JSON, no panic")
}

// TestPlainJSONBaseline documents the delegation boundary: feeding an ordinary
// (non-i18next) JSON object reads sanely as plain key/value blocks, with no
// plural/context grouping applied because there are no plural or context
// sibling keys to recognise. This is exactly the generic JSON behavior the
// i18next layer delegates to; the i18next-specific annotations are simply
// absent.
func TestPlainJSONBaseline(t *testing.T) {
	input := []byte(`{"title": "Welcome", "subtitle": "A plain resource bundle", "nested": {"greeting": "Hello"}}`)
	r := i18next.NewReader()
	doc := &model.RawDocument{
		URI:          "plain.json",
		SourceLocale: "en-US",
		Encoding:     "UTF-8",
		Reader:       &nopCloser{bytes.NewReader(input)},
	}
	require.NoError(t, r.Open(t.Context(), doc))
	defer r.Close()

	parts := testutil.CollectParts(t, r.Read(t.Context()))
	byName := blocksByName(collectBlocks(parts))

	// Every string value is extracted as a plain translatable block, including
	// the nested one (full key-path names from the inner JSON reader).
	require.Len(t, byName, 3, "all three string values read as plain blocks")
	assert.Equal(t, "Welcome", byName["/title"].SourceText())
	assert.Equal(t, "A plain resource bundle", byName["/subtitle"].SourceText())
	assert.Equal(t, "Hello", byName["/nested/greeting"].SourceText())

	// No i18next plural/context grouping is applied — none of the keys are
	// plural or context siblings, so those annotations are simply absent.
	for name, b := range byName {
		assert.NotContains(t, b.Properties, "i18next.pluralCategory",
			"plain JSON key %s must not gain a plural category", name)
		assert.NotContains(t, b.Properties, "i18next.pluralGroup",
			"plain JSON key %s must not gain a plural group", name)
		assert.NotContains(t, b.Properties, "i18next.context",
			"plain JSON key %s must not gain a context", name)
		assert.NotContains(t, b.Properties, "i18next.baseKey",
			"plain JSON key %s must not gain a base key", name)
	}
}
