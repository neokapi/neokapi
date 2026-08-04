package vtt_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/vtt"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// A block's Name is the context half of its identity (core/model/structural.go,
// core/reconcile). These tests pin what a WebVTT cue is named by, and what an
// edit elsewhere in the file is allowed to do to it.
//
// The reader has two walks — one with a skeleton store, one without — and they
// name blocks independently, so every property is asserted on both.

// vttNamesNoSkeleton reads via readContentSimple (no skeleton store).
func vttNamesNoSkeleton(t *testing.T, input string) []string {
	t.Helper()
	ctx := t.Context()
	reader := vtt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	t.Cleanup(func() { reader.Close() })
	return blockNames(testutil.FilterBlocks(testutil.CollectParts(t, reader.Read(ctx))))
}

// vttNamesSkeleton reads via readContentSkeleton (skeleton store attached).
func vttNamesSkeleton(t *testing.T, input string) []string {
	t.Helper()
	ctx := t.Context()
	reader := vtt.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	reader.SetSkeletonStore(store)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	t.Cleanup(func() { reader.Close() })
	return blockNames(testutil.FilterBlocks(testutil.CollectParts(t, reader.Read(ctx))))
}

func blockNames(blocks []*model.Block) []string {
	names := make([]string, len(blocks))
	for i, b := range blocks {
		names[i] = b.Name
	}
	return names
}

// eachVTTWalk runs fn against both reader walks.
func eachVTTWalk(t *testing.T, fn func(t *testing.T, names func(*testing.T, string) []string)) {
	t.Helper()
	t.Run("no skeleton", func(t *testing.T) { fn(t, vttNamesNoSkeleton) })
	t.Run("skeleton", func(t *testing.T) { fn(t, vttNamesSkeleton) })
}

// An explicit cue identifier is the one piece of identity WebVTT lets an author
// write down, so where there is one it is the name.
func TestCueIdentifierIsTheName(t *testing.T) {
	t.Parallel()
	eachVTTWalk(t, func(t *testing.T, names func(*testing.T, string) []string) {
		got := names(t, "WEBVTT\n\nintro\n00:00:01.000 --> 00:00:04.000\nAlpha\n\n"+
			"outro\n00:00:05.000 --> 00:00:08.000\nBravo\n")
		assert.Equal(t, []string{"intro", "outro"}, got)
	})
}

// Without an identifier the cue is named by its start timestamp.
func TestUnidentifiedCueIsNamedByStartTime(t *testing.T) {
	t.Parallel()
	eachVTTWalk(t, func(t *testing.T, names func(*testing.T, string) []string) {
		got := names(t, "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nAlpha\n\n"+
			"00:01:05.500 --> 00:01:08.000\nBravo\n")
		assert.Equal(t, []string{"00:00:01.000", "00:01:05.500"}, got)
	})
}

func TestVTTDeletingAnEarlierCueKeepsLaterNames(t *testing.T) {
	t.Parallel()
	eachVTTWalk(t, func(t *testing.T, names func(*testing.T, string) []string) {
		before := names(t, "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nAlpha\n\n"+
			"00:00:05.000 --> 00:00:08.000\nBravo\n\n"+
			"00:00:09.000 --> 00:00:12.000\nCharlie\n")
		after := names(t, "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nAlpha\n\n"+
			"00:00:09.000 --> 00:00:12.000\nCharlie\n")

		require.Len(t, before, 3)
		require.Len(t, after, 2)
		assert.Equal(t, before[0], after[0])
		assert.Equal(t, before[2], after[1], "the surviving later cue keeps its name")
	})
}

func TestVTTIdenticalTextGetsDistinctNames(t *testing.T) {
	t.Parallel()
	eachVTTWalk(t, func(t *testing.T, names func(*testing.T, string) []string) {
		got := names(t, "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nYes\n\n"+
			"00:00:05.000 --> 00:00:08.000\nYes\n")
		require.Len(t, got, 2)
		assert.NotEqual(t, got[0], got[1])
	})
}

// Two cues sharing one identifier is malformed WebVTT, but a reader meets it
// anyway; the second is disambiguated rather than colliding with the first.
func TestVTTDuplicateIdentifiersAreDisambiguated(t *testing.T) {
	t.Parallel()
	eachVTTWalk(t, func(t *testing.T, names func(*testing.T, string) []string) {
		got := names(t, "WEBVTT\n\nintro\n00:00:01.000 --> 00:00:04.000\nAlpha\n\n"+
			"intro\n00:00:05.000 --> 00:00:08.000\nBravo\n")
		require.Len(t, got, 2)
		assert.Equal(t, "intro", got[0])
		assert.NotEqual(t, got[0], got[1])
	})
}

func TestVTTNamesAreStableAcrossReads(t *testing.T) {
	t.Parallel()
	eachVTTWalk(t, func(t *testing.T, names func(*testing.T, string) []string) {
		const input = "WEBVTT\n\nintro\n00:00:01.000 --> 00:00:04.000\nAlpha\n\n" +
			"00:00:05.000 --> 00:00:08.000\nBravo\n"
		first := names(t, input)
		second := names(t, input)
		assert.Equal(t, first, second)
	})
}

// The judgement call, pinned. A retimed cue with no identifier IS renamed; the
// same cue with an identifier is not. This is the argument for writing cue
// identifiers, and the reason the identifier is preferred over the timestamp.
func TestVTTShiftingTheTimingRenamesOnlyUnidentifiedCues(t *testing.T) {
	t.Parallel()
	eachVTTWalk(t, func(t *testing.T, names func(*testing.T, string) []string) {
		bare := names(t, "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nAlpha\n")
		bareShifted := names(t, "WEBVTT\n\n00:00:01.200 --> 00:00:04.200\nAlpha\n")
		assert.NotEqual(t, bare, bareShifted, "an unidentified cue is named by its start time")

		named := names(t, "WEBVTT\n\nintro\n00:00:01.000 --> 00:00:04.000\nAlpha\n")
		namedShifted := names(t, "WEBVTT\n\nintro\n00:00:01.200 --> 00:00:04.200\nAlpha\n")
		assert.Equal(t, named, namedShifted, "an identified cue rides through a retime")
	})
}

// Cue settings are presentation, not identity: re-aligning a cue must not
// rename it. Nor must extending how long it stays on screen.
func TestVTTCueSettingsAndEndTimeDoNotAffectTheName(t *testing.T) {
	t.Parallel()
	eachVTTWalk(t, func(t *testing.T, names func(*testing.T, string) []string) {
		base := names(t, "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nAlpha\n")
		assert.Equal(t, base, names(t, "WEBVTT\n\n00:00:01.000 --> 00:00:04.000 align:start line:0\nAlpha\n"))
		assert.Equal(t, base, names(t, "WEBVTT\n\n00:00:01.000 --> 00:00:09.000\nAlpha\n"))
	})
}

// The hours field is optional in WebVTT; the same instant spelled two ways is
// one name.
func TestVTTTimestampSpellingIsCanonicalised(t *testing.T) {
	t.Parallel()
	eachVTTWalk(t, func(t *testing.T, names func(*testing.T, string) []string) {
		assert.Equal(t,
			names(t, "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nAlpha\n"),
			names(t, "WEBVTT\n\n00:01.000 --> 00:04.000\nAlpha\n"))
	})
}

// The point of all of the above, asserted where it lands: two reads through
// reconcile, and the cue nobody touched comes back Unchanged — both signals
// matching. This needs the positional `index` property gone as well as the
// timestamp on the name, because Block.Properties is hashed into the context
// signal alongside the name.
func TestVTTUntouchedCueReconcilesAsUnchanged(t *testing.T) {
	t.Parallel()
	const scope = "movie.vtt"

	read := func(input string) []*model.Block {
		ctx := t.Context()
		reader := vtt.NewReader()
		require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
		t.Cleanup(func() { reader.Close() })
		return testutil.FilterBlocks(testutil.CollectParts(t, reader.Read(ctx)))
	}

	v1 := read("WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nAlpha\n\n" +
		"00:00:05.000 --> 00:00:08.000\nBravo\n\n" +
		"00:00:09.000 --> 00:00:12.000\nCharlie\n")
	v2 := read("WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nAlpha\n\n" +
		"00:00:09.000 --> 00:00:12.000\nCharlie\n")

	prior := make([]reconcile.Unit, len(v1))
	for i, b := range v1 {
		u := reconcile.Identify(scope, b)
		u.Key = "k" + strconv.Itoa(i)
		prior[i] = u
	}

	results := reconcile.Blocks(scope, v2, prior)
	require.Len(t, results, 2)
	assert.Equal(t, reconcile.Unchanged, results[1].Kind, "Charlie, after an earlier cue was deleted")
	assert.Equal(t, "k2", results[1].Key, "Charlie keeps the identity it had in v1")
}

// A cue's name must not carry a document-wide counter, so a STYLE block ahead of
// it — which is not a cue at all — cannot shift it.
func TestVTTStyleBlockDoesNotShiftCueNames(t *testing.T) {
	t.Parallel()
	const withStyle = "WEBVTT\n\nSTYLE\n::cue { color: yellow }\n\n00:00:01.000 --> 00:00:04.000\nAlpha\n"
	const withoutStyle = "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nAlpha\n"

	ctx := t.Context()
	read := func(input string) []*model.Block {
		// Extraction of non-translatable content is on by default, so the STYLE
		// body arrives as its own block ahead of the cue.
		reader := vtt.NewReader()
		require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
		t.Cleanup(func() { reader.Close() })
		return testutil.FilterBlocks(testutil.CollectParts(t, reader.Read(ctx)))
	}

	styled := read(withStyle)
	plain := read(withoutStyle)
	require.Len(t, plain, 1)
	require.Len(t, styled, 2, "STYLE body plus the cue")
	assert.Equal(t, plain[0].Name, styled[1].Name)
}
