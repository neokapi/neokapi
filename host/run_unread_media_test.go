package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kapi run writes no target for a media file whose engine is not loaded: an
// image without the vision plugin is named and left out, and the rest of the
// batch runs. A run of that image alone runs nothing and fails, saying why.
func TestRun_WritesNoTargetForMediaWithoutItsEngine(t *testing.T) {
	t.Setenv("KAPI_ASR_PLUGIN", "")
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")
	RefreshMediaEngines()
	t.Cleanup(RefreshMediaEngines)

	dir := t.TempDir()
	png, err := os.ReadFile(filepath.Join("..", "core", "formats", "image", "testdata", "swatch.png"))
	require.NoError(t, err)
	image := filepath.Join(dir, "swatch.png")
	text := filepath.Join(dir, "greeting.json")
	require.NoError(t, os.WriteFile(image, png, 0o644))
	require.NoError(t, os.WriteFile(text, []byte(`{"greeting":"Hello world"}`), 0o644))
	out := filepath.Join(dir, "out")

	run := func(inputs ...string) (string, error) {
		a := newToolboxApp(t)
		cmd := NewEnvCommand(context.Background(), "run")
		a.AddFlowRunFlags(cmd)
		for _, in := range inputs {
			require.NoError(t, cmd.Flags().Set("input", in))
		}
		require.NoError(t, cmd.Flags().Set("output", filepath.Join(out, "{name}_qps.{ext}")))
		var stderr bytes.Buffer
		cmd.SetErr(&stderr)
		cmd.SetOut(&bytes.Buffer{})
		err := a.RunFlow(context.Background(), cmd, "pseudo-translate")
		return stderr.String(), err
	}

	stderr, err := run(image, text)
	require.NoError(t, err)
	assert.Contains(t, stderr, "swatch.png is not run, and no target is written for it")
	assert.Contains(t, stderr, `the "vision" plugin is not installed`)
	assert.FileExists(t, filepath.Join(out, "greeting_qps.json"), "the rest of the batch runs")
	assert.NoFileExists(t, filepath.Join(out, "swatch_qps.png"))

	_, err = run(image)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing was run")
	assert.NoFileExists(t, filepath.Join(out, "swatch_qps.png"))
}
