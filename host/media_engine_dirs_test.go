package host

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/asr"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/vision"
	"github.com/neokapi/neokapi/host/pluginhost"
)

// fakeMediaPlugin installs a stand-in for the named media plugin into dir:
// its real manifest beside a binary that does nothing.
func fakeMediaPlugin(t *testing.T, dir, name string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in plugin binary is a shell script")
	}
	manifest, err := os.ReadFile(filepath.Join("..", "plugins", name, "manifest.json"))
	require.NoError(t, err)
	pdir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(pdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pdir, "manifest.json"), manifest, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pdir, "kapi-"+name), []byte("#!/bin/sh\nexit 0\n"), 0o755))
}

// The probes find a plugin in the directory kapi installs into, the one the
// CLI and Kapi Desktop share (pluginhost.InstallTarget), and in a
// $KAPI_PLUGINS_DIR; without one, the engine is absent.
func TestMediaEngineProbesSearchWhereKapiInstalls(t *testing.T) {
	t.Setenv("KAPI_ASR_PLUGIN", "")
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "")
	t.Setenv("KAPI_PLUGINS_DIR", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Cleanup(RefreshMediaEngines)

	target, err := pluginhost.InstallTarget()
	require.NoError(t, err)
	assert.False(t, asr.Available(""), "nothing installed yet")
	assert.False(t, vision.Available(""))

	fakeMediaPlugin(t, target, "asr")
	fakeMediaPlugin(t, target, "vision")
	assert.True(t, asr.Available(""), "found where kapi installs")
	assert.True(t, vision.Available(""))

	scratch := t.TempDir()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", scratch)
	assert.False(t, asr.Available(""))
	fakeMediaPlugin(t, scratch, "asr")
	assert.True(t, asr.Available(""), "found in $KAPI_PLUGINS_DIR")
}

// A file whose reader is missing its engine is stamped with it, and the
// stamp drifts when the engine is installed. An image read with OCR off
// needs no engine.
func TestMissingMediaEnginesFollowThePlugins(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("KAPI_ASR_PLUGIN", "")
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", scratch)
	t.Cleanup(RefreshMediaEngines)

	audio := project.ResolvedFile{Format: "audio"}
	image := project.ResolvedFile{Format: "image"}
	noOCR := project.ResolvedFile{Format: "image", Item: &project.ContentItem{Format: &project.FormatSpec{Name: "image", Config: map[string]any{"ocr": false}}}}
	assert.Equal(t, []string{"asr"}, missingMediaEngines(audio))
	assert.Equal(t, []string{"vision"}, missingMediaEngines(image))
	assert.Empty(t, missingMediaEngines(noOCR), "OCR is off, so no engine is needed")
	assert.Empty(t, missingMediaEngines(project.ResolvedFile{Format: "json"}))

	before := mediaEngineStamp()
	fakeMediaPlugin(t, scratch, "asr")
	assert.Empty(t, missingMediaEngines(audio))
	RefreshMediaEngines()
	assert.NotEqual(t, before, mediaEngineStamp(), "the parse cache keys on the installed engines")
}
