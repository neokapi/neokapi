package srt_test

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats/srt"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// A block's Name is the context half of its identity (core/model/structural.go,
// core/reconcile). These tests pin what an SRT cue is named by, and — more to
// the point — what an edit elsewhere in the file is allowed to do to it.

func srtNames(t *testing.T, input string) []string {
	t.Helper()
	ctx := t.Context()
	reader := srt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	t.Cleanup(func() { reader.Close() })
	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	names := make([]string, len(blocks))
	for i, b := range blocks {
		names[i] = b.Name
	}
	return names
}

// The name is the cue's start timestamp, not its in-file sequence number.
func TestCueNameIsTheStartTimestamp(t *testing.T) {
	t.Parallel()
	names := srtNames(t, "1\n00:00:01,000 --> 00:00:04,000\nAlpha\n\n"+
		"2\n00:01:05,500 --> 00:01:08,000\nBravo\n")
	assert.Equal(t, []string{"00:00:01,000", "00:01:05,500"}, names)
}

// Deleting an earlier cue renumbers every cue after it — that is what SRT tools
// do — but it must not rename them.
func TestDeletingAnEarlierCueKeepsLaterNames(t *testing.T) {
	t.Parallel()
	before := srtNames(t,
		"1\n00:00:01,000 --> 00:00:04,000\nAlpha\n\n"+
			"2\n00:00:05,000 --> 00:00:08,000\nBravo\n\n"+
			"3\n00:00:09,000 --> 00:00:12,000\nCharlie\n")
	// Bravo removed and the file renumbered, exactly as a subtitle editor writes it.
	after := srtNames(t,
		"1\n00:00:01,000 --> 00:00:04,000\nAlpha\n\n"+
			"2\n00:00:09,000 --> 00:00:12,000\nCharlie\n")

	require.Len(t, before, 3)
	require.Len(t, after, 2)
	assert.Equal(t, before[0], after[0], "the first cue keeps its name")
	assert.Equal(t, before[2], after[1], "the surviving later cue keeps its name despite renumbering")
}

// Two cues with the same words are still two cues.
func TestIdenticalTextGetsDistinctNames(t *testing.T) {
	t.Parallel()
	names := srtNames(t, "1\n00:00:01,000 --> 00:00:04,000\nYes\n\n"+
		"2\n00:00:05,000 --> 00:00:08,000\nYes\n")
	require.Len(t, names, 2)
	assert.NotEqual(t, names[0], names[1])
}

func TestNamesAreStableAcrossReads(t *testing.T) {
	t.Parallel()
	const input = "1\n00:00:01,000 --> 00:00:04,000\nAlpha\n\n" +
		"2\n00:00:05,000 --> 00:00:08,000\nBravo\n"
	assert.Equal(t, srtNames(t, input), srtNames(t, input))
}

// The judgement call, pinned: a retime DOES rename. Timestamps buy stability
// against insertion and deletion and pay for it here. The block still keeps its
// identity through reconcile — content matches, context differs, which grades as
// "moved" — so what is lost is only the free ride, not the history.
func TestShiftingTheTimingRenamesTheCue(t *testing.T) {
	t.Parallel()
	before := srtNames(t, "1\n00:00:01,000 --> 00:00:04,000\nAlpha\n")
	after := srtNames(t, "1\n00:00:01,200 --> 00:00:04,200\nAlpha\n")
	require.Len(t, before, 1)
	require.Len(t, after, 1)
	assert.NotEqual(t, before[0], after[0])
}

// Only the START time names the cue, so extending how long it stays on screen —
// a routine readability fix — leaves the name alone.
func TestExtendingTheEndTimeKeepsTheName(t *testing.T) {
	t.Parallel()
	before := srtNames(t, "1\n00:00:01,000 --> 00:00:04,000\nAlpha\n")
	after := srtNames(t, "1\n00:00:01,000 --> 00:00:06,500\nAlpha\n")
	assert.Equal(t, before, after)
}

// Timestamps are canonicalised, so a cosmetic reserialization of the same
// instant is not an edit.
func TestTimestampSpellingIsCanonicalised(t *testing.T) {
	t.Parallel()
	assert.Equal(t,
		srtNames(t, "1\n00:00:01,000 --> 00:00:04,000\nAlpha\n"),
		srtNames(t, "1\n00:00:01,0 --> 00:00:04,000\nAlpha\n"))
}

// The entry number belongs to the output, not to the block. Writing a cue that
// never came from an SRT file — the cross-format case — must still produce a
// numbered entry; it used to produce a blank line where the number goes.
func TestWriterNumbersEntriesItself(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	blocks := []*model.Block{
		model.NewBlock("a", "Alpha"),
		model.NewBlock("b", "Bravo"),
	}
	blocks[0].Properties["timecode"] = "00:00:01,000 --> 00:00:04,000"
	blocks[1].Properties["timecode"] = "00:00:05,000 --> 00:00:08,000"

	parts := make([]*model.Part, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: b})
	}

	var buf bytes.Buffer
	writer := srt.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	assert.Equal(t,
		"1\n00:00:01,000 --> 00:00:04,000\nAlpha\n\n2\n00:00:05,000 --> 00:00:08,000\nBravo\n",
		buf.String())
}

// The point of all of the above, asserted where it actually lands: run two reads
// through reconcile and the cue that nobody touched must come back Unchanged —
// both signals matching — not merely re-matched on content after its context
// moved. This is the property that a positional name cannot have, and it needs
// the sequence number off the block as well as the timestamp on the name.
func TestUntouchedCueReconcilesAsUnchanged(t *testing.T) {
	t.Parallel()
	const scope = "movie.srt"

	read := func(input string) []*model.Block {
		ctx := t.Context()
		reader := srt.NewReader()
		require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
		t.Cleanup(func() { reader.Close() })
		return testutil.CollectBlocks(t, reader.Read(ctx))
	}

	v1 := read("1\n00:00:01,000 --> 00:00:04,000\nAlpha\n\n" +
		"2\n00:00:05,000 --> 00:00:08,000\nBravo\n\n" +
		"3\n00:00:09,000 --> 00:00:12,000\nCharlie\n")
	// Bravo deleted; the file renumbers, Charlie is otherwise untouched.
	v2 := read("1\n00:00:01,000 --> 00:00:04,000\nAlpha\n\n" +
		"2\n00:00:09,000 --> 00:00:12,000\nCharlie\n")

	prior := make([]reconcile.Unit, len(v1))
	for i, b := range v1 {
		u := reconcile.Identify(scope, b)
		u.Key = "k" + strconv.Itoa(i)
		prior[i] = u
	}

	results := reconcile.Blocks(scope, v2, prior)
	require.Len(t, results, 2)
	assert.Equal(t, reconcile.Unchanged, results[0].Kind, "Alpha")
	assert.Equal(t, "k0", results[0].Key)
	assert.Equal(t, reconcile.Unchanged, results[1].Kind, "Charlie, after an earlier cue was deleted")
	assert.Equal(t, "k2", results[1].Key, "Charlie keeps the identity it had in v1")
}

// Malformed timing has no timestamp to name the cue by. The fallback is the raw
// line, and where even that is absent the builder's own ordinal — a positional
// name, but reached only when the format gave us nothing.
func TestUnparseableTimingFallsBackToTheRawLine(t *testing.T) {
	t.Parallel()
	names := srtNames(t, "1\nnot a timecode\nAlpha\n")
	require.Len(t, names, 1)
	assert.Equal(t, "not a timecode", names[0])
}
