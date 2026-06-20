package srt_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/srt"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadSRTSetsTiming verifies the SRT reader populates the canonical
// model.TimingAnnotation (start/end in ms) from the cue timecode — not just the
// raw Properties["timecode"] string — so subtitle cues carry the format-agnostic
// temporal anchor the content model and frontends consume.
func TestReadSRTSetsTiming(t *testing.T) {
	ctx := t.Context()
	reader := srt.NewReader()
	input := "1\n00:00:01,000 --> 00:00:04,500\nHello world\n\n2\n00:01:05,250 --> 00:01:08,000\nSecond subtitle\n"
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)

	tm0, ok := blocks[0].Timing()
	require.True(t, ok, "first cue should carry a TimingAnnotation")
	assert.Equal(t, int64(1000), tm0.StartMS)
	assert.Equal(t, int64(4500), tm0.EndMS)

	tm1, ok := blocks[1].Timing()
	require.True(t, ok, "second cue should carry a TimingAnnotation")
	assert.Equal(t, int64(65250), tm1.StartMS) // 1m05.250s
	assert.Equal(t, int64(68000), tm1.EndMS)
}
