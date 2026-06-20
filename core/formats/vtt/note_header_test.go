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

// vttDataNamed returns the first Data resource with the given name, or nil.
func vttDataNamed(parts []*model.Part, name string) *model.Data {
	for _, p := range parts {
		if p.Type == model.PartData {
			if d, ok := p.Resource.(*model.Data); ok && d.Name == name {
				return d
			}
		}
	}
	return nil
}

// roundtripSkeletonOff reads with extraction disabled and writes back through the
// skeleton store, returning the reconstructed output (used to pin byte-exact
// round-trip on the parity / legacy path).
func roundtripSkeletonOff(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := vtt.NewReader()
	reader.Config().(*vtt.Config).SetExtractNonTranslatableContent(false)
	writer := vtt.NewWriter()

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
	return buf.String()
}

// ---------------------------------------------------------------------------
// #928 finding A — WEBVTT header suffix surfaced as non-translatable content
// ---------------------------------------------------------------------------

const headerSuffixVTT = "WEBVTT - Episode 1 subtitles\n\n" +
	"00:00:01.000 --> 00:00:04.000\n" +
	"Hello world\n"

// TestHeader_DefaultOn_SurfacesSuffixCaptionBlock verifies that, with extraction
// on (the default), the freeform text after the bare "WEBVTT" signature is
// surfaced as a non-translatable RoleCaption content block — and the cue is
// untouched.
func TestHeader_DefaultOn_SurfacesSuffixCaptionBlock(t *testing.T) {
	parts := readVTT(t, headerSuffixVTT)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2, "header-suffix block + one cue block")

	suffix := blocks[0]
	assert.False(t, suffix.Translatable, "header suffix must be non-translatable")
	assert.Equal(t, model.RoleCaption, suffix.SemanticRole())
	assert.Equal(t, "- Episode 1 subtitles", suffix.SourceText())
	// Single verbatim run, not inline-parsed.
	require.Len(t, suffix.SourceRuns(), 1)
	require.NotNil(t, suffix.SourceRuns()[0].Text)

	// The cue is unchanged: a translatable subtitle.
	cue := blocks[1]
	assert.True(t, cue.Translatable)
	assert.Equal(t, "Hello world", cue.SourceText())

	// The header Data part is still emitted with the full signature line.
	hdr := vttDataNamed(parts, "vtt-header")
	require.NotNil(t, hdr)
	assert.Equal(t, "WEBVTT - Episode 1 subtitles", hdr.Properties["content"])
}

// TestHeader_KindMetadata_SurfacesSuffix covers a metadata-style header suffix
// ("WEBVTT Kind: captions"): the suffix after the separator becomes the block.
func TestHeader_KindMetadata_SurfacesSuffix(t *testing.T) {
	blocks := readVTTBlocks(t, "WEBVTT Kind: captions\n\n00:00:01.000 --> 00:00:04.000\nHi\n")
	require.Len(t, blocks, 2)
	assert.False(t, blocks[0].Translatable)
	assert.Equal(t, "Kind: captions", blocks[0].SourceText())
}

// TestHeader_Bare_NoSuffixBlock pins that a bare "WEBVTT" header produces no
// suffix block (regression guard for the common case).
func TestHeader_Bare_NoSuffixBlock(t *testing.T) {
	blocks := readVTTBlocks(t, "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nHi\n")
	require.Len(t, blocks, 1, "only the cue — no header-suffix block for a bare signature")
	assert.True(t, blocks[0].Translatable)
}

// TestHeader_FlagOff_NoSuffixBlock pins the legacy behavior when extraction is
// disabled: the whole header line stays opaque (only the Data part), no block.
func TestHeader_FlagOff_NoSuffixBlock(t *testing.T) {
	parts, blocks := readVTTBlocksOff(t, headerSuffixVTT)
	require.Len(t, blocks, 1, "legacy path: only the cue is a block")
	assert.Equal(t, "Hello world", blocks[0].SourceText())

	hdr := vttDataNamed(parts, "vtt-header")
	require.NotNil(t, hdr)
	assert.Equal(t, "WEBVTT - Episode 1 subtitles", hdr.Properties["content"])
}

// TestHeader_RoundTrip_ByteExact_FlagOn verifies the header suffix round-trips
// byte-exact with the default reader: the keyword + separator ride the skeleton
// while the suffix rides a content-block ref.
func TestHeader_RoundTrip_ByteExact_FlagOn(t *testing.T) {
	assert.Equal(t, headerSuffixVTT, snippetRoundtripWithSkeleton(t, headerSuffixVTT))
}

// TestHeader_RoundTrip_ByteExact_FlagOff verifies the legacy path round-trips
// byte-exact too (the whole header line stays in the skeleton).
func TestHeader_RoundTrip_ByteExact_FlagOff(t *testing.T) {
	assert.Equal(t, headerSuffixVTT, roundtripSkeletonOff(t, headerSuffixVTT))
}

// TestHeader_RoundTrip_CRLF verifies byte-exact round-trip of a header suffix
// with CRLF line endings.
func TestHeader_RoundTrip_CRLF(t *testing.T) {
	input := "WEBVTT - Episode 1\r\n\r\n00:00:01.000 --> 00:00:04.000\r\nHello\r\n"
	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input))
}

// ---------------------------------------------------------------------------
// #928 finding B — NOTE comment blocks carried as Data; adjacent cue fixed
// ---------------------------------------------------------------------------

const noteVTT = "WEBVTT\n\n" +
	"NOTE This is a comment\n\n" +
	"00:00:01.000 --> 00:00:04.000\n" +
	"Hello world\n"

// TestNote_DefaultOn_SingleLine_CarriesText_FixesCue verifies a single-line NOTE
// is carried on a non-translatable Data part and no longer corrupts the cue.
func TestNote_DefaultOn_SingleLine_CarriesText_FixesCue(t *testing.T) {
	parts := readVTT(t, noteVTT)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1, "the NOTE must not become a block; only the real cue")

	// The cue is parsed correctly — the NOTE no longer swallows the timecode.
	cue := blocks[0]
	assert.True(t, cue.Translatable)
	assert.Equal(t, "Hello world", cue.SourceText())
	assert.Equal(t, "00:00:01.000 --> 00:00:04.000", cue.Properties["timecode"])

	// The comment text is reachable on a dedicated Data part.
	note := vttDataNamed(parts, "vtt-note.1")
	require.NotNil(t, note, "NOTE comment should be carried on a vtt-note Data part")
	assert.Equal(t, "This is a comment", note.Properties["text"])

	// NOTE must NOT have leaked out as a cue identifier Data part.
	assert.Nil(t, vttDataNamed(parts, "cue-id.1"),
		"NOTE must not be mis-parsed into a cue identifier")
}

// TestNote_DefaultOn_MultiLine carries a multi-line NOTE's text with newlines.
func TestNote_DefaultOn_MultiLine(t *testing.T) {
	input := "WEBVTT\n\n" +
		"NOTE\n" +
		"This comment spans\n" +
		"two lines\n\n" +
		"00:00:01.000 --> 00:00:04.000\n" +
		"Hello\n"
	parts := readVTT(t, input)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())

	note := vttDataNamed(parts, "vtt-note.1")
	require.NotNil(t, note)
	assert.Equal(t, "This comment spans\ntwo lines", note.Properties["text"])
}

// TestNote_FlagOff_LegacyMisparse pins the legacy behavior parity forces: with
// extraction off the NOTE is left to the cue parser (mis-parsed) and no
// vtt-note Data part appears.
func TestNote_FlagOff_LegacyMisparse(t *testing.T) {
	parts, blocks := readVTTBlocksOff(t, noteVTT)
	require.Len(t, blocks, 1)

	// Legacy: the NOTE becomes a cue identifier and the real timecode line is
	// swallowed into the cue text.
	assert.Equal(t, "NOTE This is a comment", blocks[0].Properties["cue-id"])
	assert.Contains(t, blocks[0].SourceText(), "00:00:01.000 --> 00:00:04.000")

	assert.Nil(t, vttDataNamed(parts, "vtt-note.1"),
		"legacy path emits no vtt-note Data part")
	assert.NotNil(t, vttDataNamed(parts, "cue-id.1"),
		"legacy path emits the NOTE as a cue identifier Data part")
}

// TestNote_RoundTrip_ByteExact_FlagOn verifies a NOTE block round-trips
// byte-exact with the default reader (the whole comment stays in the skeleton).
func TestNote_RoundTrip_ByteExact_FlagOn(t *testing.T) {
	assert.Equal(t, noteVTT, snippetRoundtripWithSkeleton(t, noteVTT))
}

// TestNote_RoundTrip_ByteExact_FlagOff verifies the legacy NOTE handling also
// round-trips byte-exact, so disabling the flag never corrupts the output.
func TestNote_RoundTrip_ByteExact_FlagOff(t *testing.T) {
	assert.Equal(t, noteVTT, roundtripSkeletonOff(t, noteVTT))
}

// TestNote_MultiPosition_RoundTrip exercises NOTE blocks before the first cue,
// between cues, and at EOF — all carried as Data and all byte-exact on write.
func TestNote_MultiPosition_RoundTrip(t *testing.T) {
	input := "WEBVTT\n\n" +
		"NOTE intro comment\n\n" +
		"00:00:01.000 --> 00:00:04.000\n" +
		"First\n\n" +
		"NOTE\n" +
		"a mid-stream\n" +
		"comment\n\n" +
		"00:00:05.000 --> 00:00:08.000\n" +
		"Second\n\n" +
		"NOTE trailing comment\n"

	parts := readVTT(t, input)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2, "two real cues, NOTE blocks are Data not blocks")
	assert.Equal(t, "First", blocks[0].SourceText())
	assert.Equal(t, "Second", blocks[1].SourceText())

	require.NotNil(t, vttDataNamed(parts, "vtt-note.1"))
	require.NotNil(t, vttDataNamed(parts, "vtt-note.2"))
	require.NotNil(t, vttDataNamed(parts, "vtt-note.3"))
	assert.Equal(t, "intro comment", vttDataNamed(parts, "vtt-note.1").Properties["text"])
	assert.Equal(t, "a mid-stream\ncomment", vttDataNamed(parts, "vtt-note.2").Properties["text"])
	assert.Equal(t, "trailing comment", vttDataNamed(parts, "vtt-note.3").Properties["text"])

	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input), "byte-exact round-trip")
}

// TestNote_RoundTrip_CRLF verifies byte-exact round-trip of a NOTE block with
// CRLF line endings.
func TestNote_RoundTrip_CRLF(t *testing.T) {
	input := "WEBVTT\r\n\r\n" +
		"NOTE A comment\r\n\r\n" +
		"00:00:01.000 --> 00:00:04.000\r\n" +
		"Hello\r\n"
	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input))
}

// TestNote_HeaderSuffixAndNote_Combined exercises a header suffix and a NOTE in
// the same file — the suffix is a block, the NOTE is Data, both byte-exact.
func TestNote_HeaderSuffixAndNote_Combined(t *testing.T) {
	input := "WEBVTT - Show subtitles\n\n" +
		"NOTE translator: keep brand names\n\n" +
		"00:00:01.000 --> 00:00:04.000\n" +
		"Hello\n"

	parts := readVTT(t, input)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2, "header-suffix caption + one cue")
	assert.Equal(t, "- Show subtitles", blocks[0].SourceText())
	assert.False(t, blocks[0].Translatable)
	assert.Equal(t, "Hello", blocks[1].SourceText())
	assert.True(t, blocks[1].Translatable)

	note := vttDataNamed(parts, "vtt-note.1")
	require.NotNil(t, note)
	assert.Equal(t, "translator: keep brand names", note.Properties["text"])

	assert.Equal(t, input, snippetRoundtripWithSkeleton(t, input))
}
