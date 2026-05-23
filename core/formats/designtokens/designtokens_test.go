package designtokens_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/formats/designtokens"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

// readParts reads a fixture through the design-tokens reader and returns the
// emitted parts plus the original bytes.
func readParts(t *testing.T, path string) ([]*model.Part, []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	r := designtokens.NewReader()
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

// writeParts writes the parts back out through the design-tokens writer for the
// given target locale (empty = source round-trip) and returns the bytes.
func writeParts(t *testing.T, parts []*model.Part, locale model.LocaleID) []byte {
	t.Helper()
	w := designtokens.NewWriter()
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

// collectBlocks returns the translatable blocks emitted by the reader, keyed by
// block name.
func collectBlocks(parts []*model.Part) map[string]*model.Block {
	m := make(map[string]*model.Block)
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok {
				m[b.Name] = b
			}
		}
	}
	return m
}

func fixtures() []string {
	return []string{
		"tokens.tokens.json",
		"no_descriptions.tokens",
		"compact.tokens.json",
	}
}

// TestByteFaithfulRoundTrip verifies that reading then writing an unchanged
// design-tokens file reproduces the original bytes exactly, across nested
// groups with $type cascade, aliases, $extensions, $deprecated, a
// description-free file, and a compact single-line layout.
func TestByteFaithfulRoundTrip(t *testing.T) {
	for _, name := range fixtures() {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name)
			parts, original := readParts(t, path)
			out := writeParts(t, parts, "")
			assert.Equal(t, string(original), string(out),
				"round-trip should reproduce %s byte-for-byte", name)
		})
	}
}

// TestOnlyDescriptionsExtracted verifies that ONLY $description values become
// translatable blocks; every token $value, $type, $extensions content, and
// $deprecated message is non-translatable passthrough.
func TestOnlyDescriptionsExtracted(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "tokens.tokens.json"))
	byName := collectBlocks(parts)

	wantDescriptions := map[string]string{
		"/color/$description":                     "Brand and semantic colours used across the product.",
		"/color/primary/$description":             "Primary brand colour used for calls to action.",
		"/color/secondary/$description":           "Muted secondary colour for supporting elements.",
		"/color/background/elevated/$description": "Surface colour for cards raised above the page.",
		"/spacing/small/$description":             "Tight spacing for compact, dense layouts.",
		"/font/body/$description":                 "Default reading typeface for body copy.",
		"/button/background/$description":         "The button surface, aliasing the primary brand colour.",
		"/button/padding/$description":            "Internal button padding.",
	}

	// Exactly the $description blocks are extracted — no more, no less.
	assert.Len(t, byName, len(wantDescriptions), "only $description keys should be extracted")
	for name, text := range wantDescriptions {
		b := byName[name]
		require.NotNil(t, b, "missing $description block %s", name)
		assert.Equal(t, text, b.SourceText(), "extracted text for %s", name)
	}

	// Token values, types, and structural keys must NOT be extracted.
	for name := range byName {
		assert.Contains(t, name, "$description",
			"only $description should be a block; %s was extracted unexpectedly", name)
	}

	// Spot-check that values appear as non-translatable Data, not blocks. The
	// $value strings (#0d6efd, the alias {color.primary}, the $deprecated
	// message, the $extensions leaf) are never translatable blocks.
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		assert.NotEqual(t, "#0d6efd", b.SourceText(), "colour value must not be extracted")
		assert.NotEqual(t, "{color.primary}", b.SourceText(), "alias value must not be extracted")
		assert.NotEqual(t, "Use the responsive padding scale instead.", b.SourceText(),
			"$deprecated message must not be extracted")
		assert.NotEqual(t, "S:1234", b.SourceText(), "$extensions leaf must not be extracted")
	}
}

// TestNoDescriptionsYieldsNoBlocks verifies that a token file without any
// $description produces zero translatable blocks (the low-translatable-surface
// caveat in practice) while still round-tripping faithfully.
func TestNoDescriptionsYieldsNoBlocks(t *testing.T) {
	parts, original := readParts(t, filepath.Join("testdata", "no_descriptions.tokens"))
	assert.Empty(t, collectBlocks(parts), "a description-free token file has no translatable surface")

	out := writeParts(t, parts, "")
	assert.Equal(t, string(original), string(out), "still byte-faithful with no blocks")
}

// TestExtractDescriptionsDisabled verifies that turning ExtractDescriptions off
// extracts nothing at all, so the document reads as pure structure.
func TestExtractDescriptionsDisabled(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "tokens.tokens.json"))
	require.NoError(t, err)

	r := designtokens.NewReader()
	cfg := &designtokens.Config{}
	cfg.Reset()
	cfg.ExtractDescriptions = false
	require.NoError(t, r.SetConfig(cfg))

	doc := &model.RawDocument{
		URI:          "tokens.tokens.json",
		SourceLocale: "en-US",
		Encoding:     "UTF-8",
		Reader:       &nopCloser{bytes.NewReader(data)},
	}
	require.NoError(t, r.Open(t.Context(), doc))
	defer r.Close()
	parts := testutil.CollectParts(t, r.Read(t.Context()))
	assert.Empty(t, collectBlocks(parts), "extractDescriptions=false extracts nothing")
}

// TestTranslatedWrite verifies that translating $description values and writing
// for a target locale substitutes the translated documentation while preserving
// every token value and the surrounding structure verbatim.
func TestTranslatedWrite(t *testing.T) {
	parts, _ := readParts(t, filepath.Join("testdata", "tokens.tokens.json"))

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
		case "/color/primary/$description":
			b.SetTargetText(fr, "Couleur de marque principale pour les appels à l'action.")
		case "/spacing/small/$description":
			b.SetTargetText(fr, "Espacement serré pour les mises en page compactes.")
		}
	}

	out := writeParts(t, parts, fr)
	s := string(out)

	// Translated descriptions are written.
	assert.Contains(t, s, `"$description": "Couleur de marque principale pour les appels à l'action."`)
	assert.Contains(t, s, `"$description": "Espacement serré pour les mises en page compactes."`)

	// Token values and structure are preserved verbatim.
	assert.Contains(t, s, `"$value": "#0d6efd"`)
	assert.Contains(t, s, `"$value": "{color.primary}"`)
	assert.Contains(t, s, `"$type": "color"`)
	assert.Contains(t, s, `"$deprecated": "Use the responsive padding scale instead."`)
	assert.Contains(t, s, `"figmaStyleId": "S:1234"`)

	// Untranslated descriptions keep their source text.
	assert.Contains(t, s, `"$description": "Default reading typeface for body copy."`)
}

// TestFormatIdentity verifies the format reports its design-tokens identity,
// claims the .tokens extension but no MIME, and relabels the root layer.
func TestFormatIdentity(t *testing.T) {
	r := designtokens.NewReader()
	assert.Equal(t, "designtokens", r.Name())
	assert.Equal(t, "Design Tokens (DTCG)", r.DisplayName())

	sig := r.Signature()
	assert.Equal(t, []string{".tokens"}, sig.Extensions, "claims the unique .tokens extension")
	assert.Empty(t, sig.MIMETypes, "must not claim application/json")
	assert.NotNil(t, sig.Sniff, "provides a DTCG content sniff")

	w := designtokens.NewWriter()
	assert.Equal(t, "designtokens", w.Name())

	parts, _ := readParts(t, filepath.Join("testdata", "compact.tokens.json"))
	require.NotEmpty(t, parts)
	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.True(t, layer.IsRoot())
	assert.Equal(t, "designtokens", layer.Format, "root layer relabelled to designtokens")
}

// TestSniff verifies the content sniff recognises DTCG files (both extensions)
// and rejects plain JSON.
func TestSniff(t *testing.T) {
	cases := []struct {
		name string
		file string
		want bool
	}{
		{"dtcg full", "tokens.tokens.json", true},
		{"dtcg compact", "compact.tokens.json", true},
		{"dtcg no descriptions", "no_descriptions.tokens", true},
		{"plain json", "plain.json", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tc.file))
			require.NoError(t, err)
			assert.Equal(t, tc.want, designtokens.Sniff(data))
		})
	}

	// A token object missing $type (only $value) is not claimed: $type is the
	// disambiguating marker that keeps the sniff off arbitrary JSON with a
	// "$value" key.
	assert.False(t, designtokens.Sniff([]byte(`{"x":{"$value":"#fff"}}`)),
		"$value without $type is not enough to claim DTCG")
	assert.True(t, designtokens.Sniff([]byte(`{"x":{"$type":"color","$value":"#fff"}}`)))
}

// TestConfigApplyMap verifies the config parses its toggle and rejects unknown
// keys and bad types.
func TestConfigApplyMap(t *testing.T) {
	c := &designtokens.Config{}
	c.Reset()
	assert.True(t, c.ExtractDescriptions)

	require.NoError(t, c.ApplyMap(map[string]any{"extractDescriptions": false}))
	assert.False(t, c.ExtractDescriptions)

	assert.Error(t, c.ApplyMap(map[string]any{"extractDescriptions": "nope"}))
	assert.Error(t, c.ApplyMap(map[string]any{"unknownKey": true}))
}

// TestSchema verifies the format exposes a schema with the design-tokens
// identity, the .tokens extension, and no MIME claim.
func TestSchema(t *testing.T) {
	c := &designtokens.Config{}
	c.Reset()
	s := c.Schema()
	require.NotNil(t, s)
	assert.Equal(t, "designtokens", s.FormatMeta.ID)
	assert.Equal(t, []string{".tokens"}, s.FormatMeta.Extensions)
	assert.Empty(t, s.FormatMeta.MimeTypes)
	assert.Contains(t, s.Properties, "extractDescriptions")
}
