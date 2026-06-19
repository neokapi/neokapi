package video

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/asr"
	"github.com/neokapi/neokapi/core/av"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeASR struct{}

func (fakeASR) Transcribe(context.Context, string, asr.Options) (*asr.Result, error) {
	return &asr.Result{Language: "en", Segments: []asr.Segment{
		{Text: "spoken words", StartMS: 0, EndMS: 1000, Confidence: 0.8},
	}}, nil
}
func (fakeASR) Close() error { return nil }

// TestVideoReaderDemuxAudio demuxes a real ffmpeg-generated clip and transcribes
// its audio track via a fake ASR engine — proving the video → demux → ASR →
// timing-anchored Blocks path without needing whisper. Gated on ffmpeg.
func TestVideoReaderDemuxAudio(t *testing.T) {
	if !av.FFmpegAvailable() {
		t.Skip("ffmpeg/ffprobe not on PATH")
	}
	asr.ResetForTest()
	t.Cleanup(asr.ResetForTest)
	asr.RegisterEngine("fake", func() (asr.Engine, error) { return fakeASR{}, nil })

	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	mk := exec.CommandContext(context.Background(), "ffmpeg", "-nostdin", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-shortest", "-pix_fmt", "yuv420p", clip)
	require.NoError(t, mk.Run())

	f, err := os.Open(clip)
	require.NoError(t, err)
	r := NewReader()
	require.NoError(t, r.Open(context.Background(), &model.RawDocument{URI: clip, Reader: f}))

	var blocks []*model.Block
	var sawAudioLayer bool
	for res := range r.Read(context.Background()) {
		require.NoError(t, res.Error)
		switch res.Part.Type {
		case model.PartLayerStart:
			if l, ok := res.Part.Resource.(*model.Layer); ok && l.ID == "audio" {
				sawAudioLayer = true
			}
		case model.PartBlock:
			blocks = append(blocks, res.Part.Resource.(*model.Block))
		}
	}
	assert.True(t, sawAudioLayer, "expected an audio child layer")
	require.Len(t, blocks, 1)
	assert.Equal(t, "spoken words", blocks[0].SourceText())
	tm, ok := blocks[0].Timing()
	require.True(t, ok)
	assert.Equal(t, int64(1000), tm.EndMS)
	o, ok := blocks[0].SourceOrigin()
	require.True(t, ok)
	assert.Equal(t, model.OriginASR, o.Kind)
}
