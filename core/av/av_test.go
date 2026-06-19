package av

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeTestVideo renders a 2-second 160×120 test clip with a 440 Hz tone using
// ffmpeg's lavfi sources, so the demux path has a real video + audio track.
func makeTestVideo(t *testing.T, dir string) string {
	t.Helper()
	out := filepath.Join(dir, "clip.mp4")
	cmd := exec.CommandContext(context.Background(), "ffmpeg", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-shortest", "-pix_fmt", "yuv420p", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg make test video: %v: %s", err, lastLine(b))
	}
	return out
}

func TestDemux(t *testing.T) {
	if !FFmpegAvailable() {
		t.Skip("ffmpeg/ffprobe not on PATH")
	}
	dir := t.TempDir()
	video := makeTestVideo(t, dir)

	hasAudio, durMS, err := Probe(context.Background(), video)
	require.NoError(t, err)
	assert.True(t, hasAudio)
	assert.InDelta(t, 2000, durMS, 500)

	work := t.TempDir()
	res, err := Demux(context.Background(), video, work, Options{FPS: 2})
	require.NoError(t, err)
	assert.True(t, res.HasAudio)
	assert.NotEmpty(t, res.AudioPath)
	assert.FileExists(t, res.AudioPath)
	// testsrc animates, so frames are distinct — sampling 2 fps over 2s yields
	// ~4 frames, none deduped away.
	assert.GreaterOrEqual(t, len(res.Frames), 2)
	for _, f := range res.Frames {
		assert.FileExists(t, f.Path)
	}
	// Timestamps are monotonically increasing.
	for i := 1; i < len(res.Frames); i++ {
		assert.Greater(t, res.Frames[i].TimeMS, res.Frames[i-1].TimeMS)
	}
}
