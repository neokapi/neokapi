package video_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/video"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVideoDegradesToMediaWithoutFFmpeg verifies that when no av/ffmpeg engine is
// resolvable, the video reader does NOT error — it degrades to emitting the video
// as a single opaque Media asset (still localizable via replace-asset), mirroring
// the audio reader's no-engine path.
func TestVideoDegradesToMediaWithoutFFmpeg(t *testing.T) {
	// Force ffmpeg/ffprobe to be unresolvable: empty PATH and no bundled av dir.
	t.Setenv("PATH", "")
	t.Setenv("KAPI_AV_DIR", "")

	ctx := t.Context()
	reader := video.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString("\x00\x00\x00 fake video bytes", model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx)) // fatals on any PartResult.Error

	var media, blocks int
	for _, p := range parts {
		switch p.Type {
		case model.PartMedia:
			media++
		case model.PartBlock:
			blocks++
		}
	}
	assert.Equal(t, 1, media, "video should degrade to exactly one Media part")
	assert.Equal(t, 0, blocks, "no blocks without a demux engine")
}
