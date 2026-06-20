package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaDataURL(t *testing.T) {
	app := NewApp()

	dir := t.TempDir()
	png := filepath.Join(dir, "shot.png")
	want := []byte("\x89PNG\r\n\x1a\n fake png payload")
	require.NoError(t, os.WriteFile(png, want, 0o644))

	url, err := app.MediaDataURL(png)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(url, "data:image/png;base64,"), "got %q", url)

	mp4 := filepath.Join(dir, "clip.mp4")
	require.NoError(t, os.WriteFile(mp4, []byte("ftyp fake mp4"), 0o644))
	url, err = app.MediaDataURL(mp4)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(url, "data:video/mp4;base64,"), "got %q", url)
}

func TestMediaDataURL_Errors(t *testing.T) {
	app := NewApp()
	_, err := app.MediaDataURL("")
	require.Error(t, err)
	_, err = app.MediaDataURL(filepath.Join(t.TempDir(), "missing.png"))
	require.Error(t, err)
}

func TestMediaMimeType(t *testing.T) {
	cases := map[string]string{
		"a.png":  "image/png",
		"a.JPG":  "image/jpeg",
		"a.mp3":  "audio/mpeg",
		"a.wav":  "audio/wav",
		"a.mp4":  "video/mp4",
		"a.webm": "video/webm",
		"a.bin":  "application/octet-stream",
	}
	for path, want := range cases {
		assert.Equal(t, want, mediaMimeType(path), "mime for %s", path)
	}
}

// ensureMediaEngine must be a no-op (no network install) when the format has no
// engine provider; the real install branch hits the registry and is covered by
// the pluginhost install tests.
func TestEnsureMediaEngine_Noop(t *testing.T) {
	app := NewApp()
	require.NotPanics(t, func() { app.ensureMediaEngine("json") })         // no engine provider
	require.NotPanics(t, func() { app.ensureMediaEngine("not-a-format") }) // unknown
}

// The engine provider map must point each media format at its engine plugin.
func TestEnginePluginProviders(t *testing.T) {
	assert.Equal(t, "vision", enginePluginProviders["image"])
	assert.Equal(t, "asr", enginePluginProviders["audio"])
	assert.Equal(t, "av", enginePluginProviders["video"])
}
