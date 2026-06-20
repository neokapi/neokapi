package ttml_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ttml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// headMetaInput is a TTML document whose <head><metadata> carries ttm:title,
// ttm:copyright and ttm:agent (all non-translatable contextual content per #928)
// and two body captions.
const headMetaInput = `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata">
  <head>
    <metadata>
      <ttm:title>My Subtitles</ttm:title>
      <ttm:copyright>Copyright 2024 Example Corp.</ttm:copyright>
      <ttm:agent type="person">Jane Director</ttm:agent>
    </metadata>
  </head>
  <body><div>
    <p begin="00:00:01.000" end="00:00:04.000">Hello world</p>
    <p begin="00:00:05.000" end="00:00:08.000">Second subtitle</p>
  </div></body>
</tt>`

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

func translatableBlocks(parts []*model.Part) []*model.Block {
	var out []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
				out = append(out, b)
			}
		}
	}
	return out
}

// #928: by default (ExtractNonTranslatableContent on) ttm:title, ttm:copyright
// and ttm:agent surface as non-translatable content blocks — visible to
// ingestion, skipped by MT — while the translatable caption payload is
// untouched. title carries RoleTitle; copyright/agent carry RoleCode.
func TestHeadMetadata_DefaultOn(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(headMetaInput, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	content := nonTranslatableBlocks(parts)
	require.Len(t, content, 3, "title, copyright and agent surface as non-translatable blocks")

	titleB := content[0]
	assert.False(t, titleB.Translatable)
	assert.Equal(t, "title", titleB.Type)
	assert.Equal(t, "ttm:title", titleB.Name)
	assert.Equal(t, model.RoleTitle, titleB.SemanticRole())
	assert.Equal(t, model.LayerMetadata, titleB.LayoutLayer())
	assert.True(t, titleB.PreserveWhitespace)
	assert.Equal(t, "My Subtitles", titleB.SourceText())

	copyrightB := content[1]
	assert.False(t, copyrightB.Translatable)
	assert.Equal(t, "copyright", copyrightB.Type)
	assert.Equal(t, "ttm:copyright", copyrightB.Name)
	assert.Equal(t, model.RoleCode, copyrightB.SemanticRole())
	assert.Equal(t, model.LayerMetadata, copyrightB.LayoutLayer())
	assert.True(t, copyrightB.PreserveWhitespace)
	assert.Equal(t, "Copyright 2024 Example Corp.", copyrightB.SourceText())

	agentB := content[2]
	assert.False(t, agentB.Translatable)
	assert.Equal(t, "agent", agentB.Type)
	assert.Equal(t, model.RoleCode, agentB.SemanticRole())
	assert.True(t, agentB.PreserveWhitespace)
	assert.Equal(t, "Jane Director", agentB.SourceText())

	// Translatable payload is unchanged: exactly the two body captions.
	trans := translatableBlocks(parts)
	require.Len(t, trans, 2)
	assert.Equal(t, "Hello world", trans[0].SourceText())
	assert.Equal(t, "Second subtitle", trans[1].SourceText())
}

// #928: ttm:desc (description prose) surfaces as a non-translatable RoleCaption
// content block, ordered with the other head metadata, and the document still
// round-trips byte-exact through the skeleton store.
func TestHeadMetadata_TitleAndDesc(t *testing.T) {
	ctx := t.Context()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata">
  <head>
    <metadata>
      <ttm:title>Episode One</ttm:title>
      <ttm:desc>A short description of the programme.</ttm:desc>
    </metadata>
  </head>
  <body><div>
    <p begin="00:00:01.000" end="00:00:04.000">Hello world</p>
  </div></body>
</tt>`

	reader := ttml.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	content := nonTranslatableBlocks(parts)
	require.Len(t, content, 2, "title and desc surface as non-translatable blocks")

	assert.Equal(t, "title", content[0].Type)
	assert.Equal(t, model.RoleTitle, content[0].SemanticRole())
	assert.Equal(t, "Episode One", content[0].SourceText())

	descB := content[1]
	assert.False(t, descB.Translatable)
	assert.Equal(t, "desc", descB.Type)
	assert.Equal(t, "ttm:desc", descB.Name)
	assert.Equal(t, model.RoleCaption, descB.SemanticRole())
	assert.Equal(t, model.LayerMetadata, descB.LayoutLayer())
	assert.True(t, descB.PreserveWhitespace)
	assert.Equal(t, "A short description of the programme.", descB.SourceText())

	// title/desc never enter the MT payload.
	assert.Len(t, translatableBlocks(parts), 1)

	// Byte-exact skeleton roundtrip with both metadata blocks riding refs.
	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input))
}

// #928: with the flag off the head metadata stays buried (no content blocks) and
// only the translatable captions are emitted — byte-identical to the prior
// behavior.
func TestHeadMetadata_FlagOff(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(headMetaInput, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	assert.Empty(t, nonTranslatableBlocks(parts), "no head metadata blocks when flag is off")

	trans := translatableBlocks(parts)
	require.Len(t, trans, 2)
	assert.Equal(t, "Hello world", trans[0].SourceText())
	assert.Equal(t, "Second subtitle", trans[1].SourceText())
}

// SetExtractNonTranslatableContent(false) is the type-asserted opt-out the parity
// runner uses; it must suppress the head metadata blocks just like the ApplyMap key.
func TestHeadMetadata_SetterOff(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()
	cfg, ok := reader.Config().(*ttml.Config)
	require.True(t, ok)
	cfg.SetExtractNonTranslatableContent(false)
	assert.False(t, cfg.ExtractNonTranslatableContent())

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(headMetaInput, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	assert.Empty(t, nonTranslatableBlocks(parts))
	assert.Len(t, translatableBlocks(parts), 2)
}

// #928: with the flag ON (default) the document round-trips byte-exact through the
// skeleton store — the carved-out copyright/agent content rides its own refs.
func TestHeadMetadata_RoundTripByteExact_Skeleton(t *testing.T) {
	output := snippetRoundtripWithSkeleton(t, headMetaInput)
	assert.Equal(t, headMetaInput, output, "head-metadata TTML roundtrip should be byte-exact (flag on)")
}

// #928: round-trip byte-exact via the non-skeleton (whole-document Data) writer
// path as well, with the flag on by default.
func TestHeadMetadata_RoundTripByteExact_Simple(t *testing.T) {
	ctx := t.Context()
	reader := ttml.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(headMetaInput, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := ttml.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(model.LocaleEnglish)
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	assert.Equal(t, headMetaInput, buf.String(), "non-skeleton roundtrip should be byte-exact")
}

// roundtripWithSkeletonNoExtract runs a skeleton roundtrip with the
// non-translatable-content surfacing turned OFF.
func roundtripWithSkeletonNoExtract(t *testing.T, input string) (string, []*model.Part) {
	t.Helper()
	ctx := t.Context()

	reader := ttml.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{"extractNonTranslatableContent": false}))
	writer := ttml.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	return buf.String(), parts
}

// #928: with the flag OFF the skeleton path is byte-identical to before — the
// head metadata stays one verbatim skeleton chunk and no content blocks appear.
func TestHeadMetadata_FlagOff_SkeletonByteExact(t *testing.T) {
	output, parts := roundtripWithSkeletonNoExtract(t, headMetaInput)
	assert.Equal(t, headMetaInput, output, "flag-off skeleton roundtrip should be byte-exact")
	assert.Empty(t, nonTranslatableBlocks(parts), "no head metadata refs/blocks when flag is off")
}

// A structured ttm:agent (with a child ttm:name) rides its verbatim inner content
// — child markup included — so the round-trip stays byte-exact even though the
// surfaced block is not pure text.
func TestHeadMetadata_StructuredAgent(t *testing.T) {
	ctx := t.Context()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tt xml:lang="en" xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata">
  <head>
    <metadata>
      <ttm:agent type="person" xml:id="a1"><ttm:name type="full">Jane Director</ttm:name></ttm:agent>
    </metadata>
  </head>
  <body><div>
    <p begin="00:00:01.000" end="00:00:04.000">Hello world</p>
  </div></body>
</tt>`

	reader := ttml.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	content := nonTranslatableBlocks(parts)
	require.Len(t, content, 1)
	assert.Equal(t, "agent", content[0].Type)
	assert.Equal(t, `<ttm:name type="full">Jane Director</ttm:name>`, content[0].SourceText(),
		"structured agent rides verbatim inner content")

	// Byte-exact skeleton roundtrip.
	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input))
}

// Translating the captions while the head metadata is surfaced still produces a
// correct, byte-faithful document: only the <p> text changes; the carved
// copyright/agent refs write back their (untranslated) source verbatim.
func TestHeadMetadata_SkeletonWithTranslation(t *testing.T) {
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := ttml.NewReader()
	writer := ttml.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(headMetaInput, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Hello world":
				b.SetTargetText(locale, "Bonjour le monde")
			case "Second subtitle":
				b.SetTargetText(locale, "Deuxieme sous-titre")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	out := buf.String()
	assert.Contains(t, out, "Bonjour le monde")
	assert.Contains(t, out, "Deuxieme sous-titre")
	// Non-translatable head metadata is written back verbatim, untranslated.
	assert.Contains(t, out, "<ttm:copyright>Copyright 2024 Example Corp.</ttm:copyright>")
	assert.Contains(t, out, `<ttm:agent type="person">Jane Director</ttm:agent>`)
}
