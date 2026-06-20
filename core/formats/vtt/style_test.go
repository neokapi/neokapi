package vtt_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/vtt"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// styleVTT is a WebVTT file with an embedded STYLE block (CSS) before the cue.
const styleVTT = "WEBVTT\n\n" +
	"STYLE\n" +
	"::cue {\n" +
	"  color: yellow;\n" +
	"  background-color: black;\n" +
	"}\n\n" +
	"00:00:01.000 --> 00:00:04.000\n" +
	"Hello world\n"

// readVTTBlocksOff reads a VTT snippet with extraction of non-translatable
// content disabled (the Okapi-faithful / legacy path).
func readVTTBlocksOff(t *testing.T, snippet string) ([]*model.Part, []*model.Block) {
	t.Helper()
	ctx := t.Context()
	reader := vtt.NewReader()
	reader.Config().(*vtt.Config).SetExtractNonTranslatableContent(false)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish)))
	t.Cleanup(func() { reader.Close() })
	parts := testutil.CollectParts(t, reader.Read(ctx))
	return parts, testutil.FilterBlocks(parts)
}

// hasDataNamed reports whether parts contain a Data resource with the given name.
func hasVTTDataNamed(parts []*model.Part, name string) bool {
	for _, p := range parts {
		if p.Type == model.PartData {
			if d, ok := p.Resource.(*model.Data); ok && d.Name == name {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// #928 — STYLE block embedded CSS surfaced as non-translatable content
// ---------------------------------------------------------------------------

// TestStyle_DefaultOn_SurfacesCSSContentBlock verifies that, with extraction on
// (the default), a STYLE block's embedded CSS is surfaced as a non-translatable
// RoleCode content block — not mis-parsed as a translatable cue.
func TestStyle_DefaultOn_SurfacesCSSContentBlock(t *testing.T) {
	parts := readVTT(t, styleVTT)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2, "STYLE CSS block + one cue block")

	style := blocks[0]
	assert.False(t, style.Translatable, "CSS content must be non-translatable")
	assert.Equal(t, model.RoleCode, style.SemanticRole(), "CSS should carry RoleCode")
	assert.True(t, style.PreserveWhitespace, "CSS whitespace is significant")
	assert.Equal(t,
		"::cue {\n  color: yellow;\n  background-color: black;\n}",
		style.SourceText(), "CSS body should be the verbatim block text")
	// A single verbatim run, not inline-parsed.
	require.Len(t, style.SourceRuns(), 1)
	require.NotNil(t, style.SourceRuns()[0].Text)

	// The real cue is unchanged: a translatable subtitle.
	cue := blocks[1]
	assert.True(t, cue.Translatable)
	assert.Equal(t, "Hello world", cue.SourceText())
	assert.Equal(t, "00:00:01.000 --> 00:00:04.000", cue.Properties["timecode"])

	// STYLE must NOT have leaked out as a cue identifier Data part.
	assert.False(t, hasVTTDataNamed(parts, "cue-id.1"),
		"STYLE must not be mis-parsed into a cue identifier")
	assert.True(t, hasVTTDataNamed(parts, "vtt-header"))
}

// TestStyle_FlagOff_UnchangedLegacyBehavior pins the legacy behavior when
// extraction is disabled: the STYLE block is left to the cue parser exactly as
// before (the keyword becomes a cue identifier, the first CSS line a timecode,
// the rest translatable cue text). This is the path parity forces.
func TestStyle_FlagOff_UnchangedLegacyBehavior(t *testing.T) {
	parts, blocks := readVTTBlocksOff(t, styleVTT)
	require.Len(t, blocks, 2)

	// The CSS is mis-parsed as a translatable cue (legacy behavior).
	legacy := blocks[0]
	assert.True(t, legacy.Translatable, "legacy path keeps the cue translatable")
	assert.Empty(t, legacy.SemanticRole(), "no RoleCode in the legacy path")
	assert.False(t, legacy.PreserveWhitespace)
	assert.Equal(t, "STYLE", legacy.Properties["cue-id"])
	assert.Equal(t, "::cue {", legacy.Properties["timecode"])
	assert.Equal(t, "  color: yellow;\n  background-color: black;\n}", legacy.SourceText())

	// STYLE leaks out as a cue identifier Data part (legacy behavior).
	assert.True(t, hasVTTDataNamed(parts, "cue-id.1"),
		"legacy path emits STYLE as a cue identifier Data part")

	// The real cue follows as the second cue.
	assert.Equal(t, "Hello world", blocks[1].SourceText())
}

// TestStyle_RoundTrip_ByteExact_FlagOn verifies the STYLE block round-trips
// byte-exact with the default (extraction on) reader: the keyword and
// delimiters ride the skeleton while the CSS body rides as a content-block ref.
func TestStyle_RoundTrip_ByteExact_FlagOn(t *testing.T) {
	output := snippetRoundtripWithSkeleton(t, styleVTT)
	assert.Equal(t, styleVTT, output, "STYLE block roundtrip should be byte-exact")
}

// TestStyle_RoundTrip_ByteExact_FlagOff verifies the legacy (extraction off)
// path also round-trips byte-exact, so disabling the flag never corrupts output.
func TestStyle_RoundTrip_ByteExact_FlagOff(t *testing.T) {
	ctx := t.Context()

	reader := vtt.NewReader()
	reader.Config().(*vtt.Config).SetExtractNonTranslatableContent(false)
	writer := vtt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(styleVTT, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	assert.Equal(t, styleVTT, buf.String(),
		"legacy STYLE handling should still roundtrip byte-exact")
}

// TestStyle_RoundTrip_CRLF verifies byte-exact roundtrip of a STYLE block with
// CRLF line endings (extraction on).
func TestStyle_RoundTrip_CRLF(t *testing.T) {
	input := "WEBVTT\r\n\r\n" +
		"STYLE\r\n" +
		"::cue {\r\n" +
		"  color: papayawhip;\r\n" +
		"}\r\n\r\n" +
		"00:00:01.000 --> 00:00:04.000\r\n" +
		"Hello world\r\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "CRLF STYLE block roundtrip should be byte-exact")
}

// TestStyle_ApplyMap_TogglesFlag verifies the extractNonTranslatableContent key
// flows through ApplyMap alongside the existing JSON-backed keys.
func TestStyle_ApplyMap_TogglesFlag(t *testing.T) {
	cfg := &vtt.Config{}
	assert.True(t, cfg.ExtractNonTranslatableContent(), "default is on")

	require.NoError(t, cfg.ApplyMap(map[string]any{
		"extractNonTranslatableContent": false,
		"maxCharsPerLine":               42,
	}))
	assert.False(t, cfg.ExtractNonTranslatableContent())
	assert.Equal(t, 42, cfg.MaxCharsPerLine)

	require.NoError(t, cfg.ApplyMap(map[string]any{"extractNonTranslatableContent": true}))
	assert.True(t, cfg.ExtractNonTranslatableContent())
}
