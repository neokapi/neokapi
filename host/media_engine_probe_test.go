package host

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/asr"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/vision"
)

// Without the asr and vision plugins, neither engine is available, and an
// audio file reads as media with no transcription rather than failing.
func TestMediaEnginesAreAbsentWithoutTheirPlugins(t *testing.T) {
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", "")
	t.Setenv("KAPI_ASR_PLUGIN", "")

	assert.False(t, asr.Available(""), "no asr plugin is installed")
	assert.False(t, vision.Available(""), "no vision plugin is installed")

	app := newToolboxApp(t)
	blocks := 0
	_, err := app.StreamBlocks(context.Background(), filepath.Join("..", "core", "formats", "audio", "testdata", "tone.wav"),
		func(int, *model.Block) error { blocks++; return nil })
	require.NoError(t, err, "audio reads as media when no speech recognizer is installed")
	assert.Zero(t, blocks, "nothing is transcribed")
}
