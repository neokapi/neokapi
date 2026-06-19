package asr

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEngine struct{ res *Result }

func (f *fakeEngine) Transcribe(context.Context, string, Options) (*Result, error) {
	return f.res, nil
}
func (f *fakeEngine) Close() error { return nil }

func TestRegistry(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	assert.False(t, Available(""))
	_, err := Open("")
	require.ErrorIs(t, err, ErrNoEngine)

	RegisterEngine("whisper", func() (Engine, error) {
		return &fakeEngine{res: &Result{Language: "en"}}, nil
	})
	assert.True(t, Available("")) // first registered becomes default
	assert.True(t, Available("whisper"))
	assert.False(t, Available("nope"))

	eng, err := Open("")
	require.NoError(t, err)
	res, err := eng.Transcribe(context.Background(), "/tmp/a.wav", Options{})
	require.NoError(t, err)
	assert.Equal(t, "en", res.Language)

	_, err = Open("nope")
	require.ErrorIs(t, err, ErrNoEngine)
}

func TestBlocksFromASR(t *testing.T) {
	res := &Result{
		Language: "en",
		Segments: []Segment{
			{Text: "hello there", StartMS: 0, EndMS: 1200, Confidence: 0.94},
			{Text: "", StartMS: 1200, EndMS: 1300, Confidence: 0.1}, // skipped
			{Text: "general kenobi", StartMS: 1300, EndMS: 2600, Confidence: 0.38},
		},
	}
	c := 0
	blocks := BlocksFromASR(res, &c)
	require.Len(t, blocks, 2)

	tm, ok := blocks[0].Timing()
	require.True(t, ok)
	assert.Equal(t, int64(0), tm.StartMS)
	assert.Equal(t, int64(1200), tm.EndMS)
	o, ok := blocks[0].SourceOrigin()
	require.True(t, ok)
	assert.Equal(t, model.OriginASR, o.Kind)
	assert.InDelta(t, 0.94, o.Confidence, 1e-9)

	// Inverse round-trips text, timing, and confidence.
	rt := ResultFromBlocks(blocks)
	require.Len(t, rt.Segments, 2)
	assert.Equal(t, "general kenobi", rt.Segments[1].Text)
	assert.Equal(t, int64(1300), rt.Segments[1].StartMS)
	assert.InDelta(t, 0.38, rt.Segments[1].Confidence, 1e-9)
}
