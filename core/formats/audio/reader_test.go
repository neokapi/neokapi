package audio

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/asr"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEngine struct{ res *asr.Result }

func (f *fakeEngine) Transcribe(context.Context, string, asr.Options) (*asr.Result, error) {
	return f.res, nil
}
func (f *fakeEngine) Close() error { return nil }

func readAll(t *testing.T, path string) []*model.Part {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	r := NewReader()
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{URI: path, Reader: f}))
	var parts []*model.Part
	for res := range r.Read(context.Background()) {
		require.NoError(t, res.Error)
		parts = append(parts, res.Part)
	}
	return parts
}

func writeTemp(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte("RIFFxxxx"), 0o600))
	return p
}

func TestAudioReaderTranscribes(t *testing.T) {
	asr.ResetForTest()
	t.Cleanup(asr.ResetForTest)
	asr.RegisterEngine("fake", func() (asr.Engine, error) {
		return &fakeEngine{res: &asr.Result{Language: "en", Segments: []asr.Segment{
			{Text: "hello there", StartMS: 0, EndMS: 1200, Confidence: 0.9},
			{Text: "general kenobi", StartMS: 1200, EndMS: 2600, Confidence: 0.4},
		}}}, nil
	})

	parts := readAll(t, writeTemp(t, "a.wav"))
	var blocks []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			blocks = append(blocks, p.Resource.(*model.Block))
		}
	}
	require.Len(t, blocks, 2)
	tm, ok := blocks[0].Timing()
	require.True(t, ok)
	assert.Equal(t, int64(1200), tm.EndMS)
	o, ok := blocks[1].SourceOrigin()
	require.True(t, ok)
	assert.Equal(t, model.OriginASR, o.Kind)
	assert.InDelta(t, 0.4, o.Confidence, 1e-9)
}

func TestAudioReaderNoEngineEmitsMedia(t *testing.T) {
	asr.ResetForTest() // no engine registered
	t.Cleanup(asr.ResetForTest)

	parts := readAll(t, writeTemp(t, "a.wav"))
	var media int
	for _, p := range parts {
		if p.Type == model.PartMedia {
			media++
		}
	}
	assert.Equal(t, 1, media, "with no ASR engine the audio is a Media asset")
}
