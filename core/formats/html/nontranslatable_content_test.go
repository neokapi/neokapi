package html_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nonTranslatableBlocks returns only the non-translatable blocks among parts.
func nonTranslatableBlocks(parts []*model.Part) []*model.Block {
	var out []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok && !b.Translatable {
				out = append(out, b)
			}
		}
	}
	return out
}

// hasDataNamed reports whether parts include a Data part with the given name.
func hasDataNamed(parts []*model.Part, name string) bool {
	for _, p := range parts {
		if p.Type == model.PartData {
			if d, ok := p.Resource.(*model.Data); ok && d.Name == name {
				return true
			}
		}
	}
	return false
}

// By default (ExtractNonTranslatableContent on) a <noscript> fallback subtree
// surfaces as a non-translatable RoleCode content block — visible to ingestion,
// skipped by MT — instead of opaque Data, while the translatable payload is
// untouched.
func TestRead_NoscriptAsContent(t *testing.T) {
	ctx := t.Context()
	input := `<html><body><noscript><p>Please enable JavaScript</p></noscript><p>Welcome</p></body></html>`

	reader := htmlfmt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()
	parts := testutil.CollectParts(t, reader.Read(ctx))

	content := nonTranslatableBlocks(parts)
	require.Len(t, content, 1, "noscript surfaces as one non-translatable content block")
	nb := content[0]
	assert.False(t, nb.Translatable)
	assert.Equal(t, model.RoleCode, nb.SemanticRole())
	assert.True(t, nb.PreserveWhitespace)
	assert.Equal(t, "noscript", nb.Type)
	assert.Equal(t, "<p>Please enable JavaScript</p>", nb.SourceText())
	assert.False(t, hasDataNamed(parts, "noscript"), "noscript should not also emit opaque Data")

	// Translatable payload is unchanged: only the <p>Welcome</p> stays translatable.
	var translatable []*model.Block
	for _, b := range testutil.FilterBlocks(parts) {
		if b.Translatable {
			translatable = append(translatable, b)
		}
	}
	require.Len(t, translatable, 1)
	assert.Equal(t, "Welcome", translatable[0].SourceText())
}

// A JSON data island (<script type="application/ld+json">) surfaces its body as
// a non-translatable RoleCode content block; a plain <script type="application/json">
// does the same.
func TestRead_JSONDataIslandAsContent(t *testing.T) {
	ctx := t.Context()
	for _, typ := range []string{"application/ld+json", "application/json"} {
		t.Run(typ, func(t *testing.T) {
			body := `{"@context":"https://schema.org","@type":"Organization","name":"Acme"}`
			input := `<html><head><script type="` + typ + `">` + body + `</script></head><body><p>Hi</p></body></html>`

			reader := htmlfmt.NewReader()
			require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
			defer reader.Close()
			parts := testutil.CollectParts(t, reader.Read(ctx))

			content := nonTranslatableBlocks(parts)
			require.Len(t, content, 1, "JSON data island surfaces as one content block")
			nb := content[0]
			assert.False(t, nb.Translatable)
			assert.Equal(t, model.RoleCode, nb.SemanticRole())
			assert.True(t, nb.PreserveWhitespace)
			assert.Equal(t, "script", nb.Type)
			assert.Equal(t, body, nb.SourceText())
			assert.Equal(t, typ, nb.Properties["type"])
		})
	}
}

// Generic executable <script> and <style> stay opaque Data even with the flag
// on — only renderable contextual content (noscript, JSON islands) is surfaced.
func TestRead_GenericScriptStyleStayOpaque(t *testing.T) {
	ctx := t.Context()
	input := `<html><head><style>body{color:red}</style></head><body><script>var x=1;</script>` +
		`<script type="text/javascript">var y=2;</script><p>Text</p></body></html>`

	reader := htmlfmt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()
	parts := testutil.CollectParts(t, reader.Read(ctx))

	assert.Empty(t, nonTranslatableBlocks(parts), "generic script/style must not surface content blocks")
	assert.True(t, hasDataNamed(parts, "style"), "style stays opaque Data")
	assert.True(t, hasDataNamed(parts, "script"), "generic script stays opaque Data")
}

// With surfacing disabled (the Okapi-faithful / parity config) a <noscript>
// subtree and a JSON data island stay opaque Data, exactly as before — no
// content blocks are emitted.
func TestRead_NonTranslatableContentDisabled(t *testing.T) {
	ctx := t.Context()
	input := `<html><head><script type="application/ld+json">{"a":"b"}</script></head>` +
		`<body><noscript><p>No JS</p></noscript><p>Welcome</p></body></html>`

	reader := htmlfmt.NewReader()
	reader.Config().(*htmlfmt.Config).SetExtractNonTranslatableContent(false)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()
	parts := testutil.CollectParts(t, reader.Read(ctx))

	assert.Empty(t, nonTranslatableBlocks(parts), "no content blocks when flag is off")
	assert.True(t, hasDataNamed(parts, "noscript"), "noscript stays opaque Data when flag is off")
	assert.True(t, hasDataNamed(parts, "script"), "JSON island stays opaque Data when flag is off")
}

// Skeleton round-trip stays byte-exact with the flag ON: the surfaced
// noscript/JSON body rides a skeleton ref and is re-emitted verbatim (no
// quote-escaping / whitespace-collapse), delimiters stay skeleton.
func TestSkeletonRoundtrip_NonTranslatableContent(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{
			name:  "noscript",
			input: `<html><body><noscript><img src="x.png" alt="No JS"></noscript><p>Text</p></body></html>`,
		},
		{
			name:  "json_ld_island",
			input: `<html><body><script type="application/ld+json">{"@context":"https://schema.org","@type":"Organization","name":"Acme & Co"}</script><p>Hi</p></body></html>`,
		},
		{
			name:  "json_island",
			input: `<html><body><script type="application/json">{"k":"v with \"quotes\""}</script><p>Hi</p></body></html>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input),
				"skeleton round-trip must stay byte-exact with non-translatable content surfaced")
		})
	}
}

// The skeleton reader surfaces the non-translatable content block (default on)
// and keeps the translatable payload intact.
func TestSkeletonRead_NoscriptAsContent(t *testing.T) {
	ctx := t.Context()
	input := `<html><body><noscript><p>No JS here</p></noscript><p>Welcome</p></body></html>`

	reader := htmlfmt.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()
	parts := testutil.CollectParts(t, reader.Read(ctx))

	content := nonTranslatableBlocks(parts)
	require.Len(t, content, 1)
	assert.Equal(t, model.RoleCode, content[0].SemanticRole())
	assert.True(t, content[0].PreserveWhitespace)
	assert.Equal(t, "<p>No JS here</p>", content[0].SourceText())
}

// With the flag off, the skeleton path keeps the old opaque behavior: a
// <noscript> subtree is Data and the document still round-trips byte-exact.
func TestSkeletonRoundtrip_NonTranslatableContentDisabled(t *testing.T) {
	ctx := t.Context()
	input := `<html><body><noscript><p>No JS</p></noscript><p>Text</p></body></html>`

	reader := htmlfmt.NewReader()
	reader.Config().(*htmlfmt.Config).SetExtractNonTranslatableContent(false)
	writer := htmlfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	assert.Empty(t, nonTranslatableBlocks(parts), "no content blocks when flag is off")
	assert.True(t, hasDataNamed(parts, "noscript"), "noscript stays opaque Data when flag is off")

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	assert.Equal(t, input, buf.String(), "flag-off skeleton round-trip stays byte-exact")
}
