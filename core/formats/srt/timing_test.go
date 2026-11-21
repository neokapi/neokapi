package srt_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/formats/srt"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The timing annotation records both endpoints of an SRT cue in milliseconds.
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

func TestWriteSRTWithForeignTiming(t *testing.T) {
	block := model.NewBlock("cue", "The draft is ready.")
	block.Properties["timecode"] = "00:00:05.250 --> 00:00:08.000"
	block.SetTiming(&model.TimingAnnotation{StartMS: 5250, EndMS: 8000})
	var out bytes.Buffer
	writer := srt.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&out))
	require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel([]*model.Part{
		{Type: model.PartBlock, Resource: block},
	})))
	require.NoError(t, writer.Close())
	assert.Equal(t, "1\n00:00:05,250 --> 00:00:08,000\nThe draft is ready.\n", out.String())
}
