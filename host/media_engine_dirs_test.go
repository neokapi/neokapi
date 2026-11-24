package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
// $KAPI_PLUGINS_DIR; without one, the engine is absent. The plugins are
// discovered once per run, so each change below is followed by the refresh a
// long-lived process makes after an install.
func TestMediaEngineProbesSearchWhereKapiInstalls(t *testing.T) {
	t.Setenv("KAPI_ASR_PLUGIN", "")
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "")
	t.Setenv("KAPI_PLUGINS_DIR", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	RefreshMediaEngines()
	t.Cleanup(RefreshMediaEngines)

	target, err := pluginhost.InstallTarget()
	require.NoError(t, err)
	assert.False(t, asr.Available(""), "nothing installed yet")
	assert.False(t, vision.Available(""))

	fakeMediaPlugin(t, target, "asr")
	fakeMediaPlugin(t, target, "vision")
	RefreshMediaEngines()
	assert.True(t, asr.Available(""), "found where kapi installs")
	assert.True(t, vision.Available(""))

	scratch := t.TempDir()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", scratch)
	RefreshMediaEngines()
	assert.False(t, asr.Available(""))
	fakeMediaPlugin(t, scratch, "asr")
	RefreshMediaEngines()
	assert.True(t, asr.Available(""), "found in $KAPI_PLUGINS_DIR")
}

// A sync asks whether an engine is available once per media file; the
// plugins behind the answer are discovered once per run, and again only after
// RefreshMediaEngines.
func TestMediaEnginesDiscoverOncePerRun(t *testing.T) {
	RefreshMediaEngines()
	t.Cleanup(RefreshMediaEngines)
	calls := 0
	real := discoverMediaPlugins
	discoverMediaPlugins = func() ([]*pluginhost.Plugin, []pluginhost.Refusal) {
		calls++
		return nil, nil
	}
	t.Cleanup(func() { discoverMediaPlugins = real })
	t.Setenv("KAPI_ASR_PLUGIN", "")

	audio := project.ResolvedFile{Format: "audio"}
	for range 50 {
		missingMediaEngines(audio)
		asr.Available("")
		vision.Available("")
	}
	assert.Equal(t, 1, calls, "fifty files, one discovery")

	RefreshMediaEngines()
	missingMediaEngines(audio)
	assert.Equal(t, 2, calls, "a refresh discovers again")
}

// A plugin installed and refused is named with its reason and fix, never with
// an instruction to install it.
func TestUnreadMediaNamesARefusedEngine(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("KAPI_ASR_PLUGIN", "")
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", scratch)
	fakeMediaPlugin(t, scratch, "vision")
	require.NoError(t, os.Chmod(filepath.Join(scratch, "vision"), 0o777))
	RefreshMediaEngines()
	t.Cleanup(RefreshMediaEngines)

	assert.False(t, vision.Available(""))
	var out strings.Builder
	writeUnreadMedia(&out, []UnreadMedia{{File: "media/logo.png", Engines: []string{"vision"}}, {File: "media/talk.wav", Engines: []string{"asr"}}})
	text := out.String()
	assert.Contains(t, text, `The "vision" plugin is installed but not loaded`)
	assert.Contains(t, text, "chmod o-w")
	assert.Contains(t, text, "kapi plugins install asr", "a plugin that is not there is still to be installed")
	assert.NotContains(t, text, "install asr vision")
	assert.NotContains(t, text, "install vision")
	assert.Contains(t, text, "No target is written for them")
}

// A file whose reader is missing its engine is stamped with it, and the
// stamp drifts when the engine is installed. An image read with OCR off
// needs no engine.
func TestMissingMediaEnginesFollowThePlugins(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("KAPI_ASR_PLUGIN", "")
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", scratch)
	RefreshMediaEngines()
	t.Cleanup(RefreshMediaEngines)

	audio := project.ResolvedFile{Format: "audio"}
	image := project.ResolvedFile{Format: "image"}
	noOCR := project.ResolvedFile{Format: "image", Item: &project.ContentItem{Format: &project.FormatSpec{Name: "image", Config: map[string]any{"ocr": false}}}}
	assert.Equal(t, []string{"asr"}, missingMediaEngines(audio))
	assert.Equal(t, []string{"vision"}, missingMediaEngines(image))
	assert.Empty(t, missingMediaEngines(noOCR), "OCR is off, so no engine is needed")
	assert.Empty(t, missingMediaEngines(project.ResolvedFile{Format: "json"}))

	fakeMediaPlugin(t, scratch, "asr")
	RefreshMediaEngines()
	assert.Empty(t, missingMediaEngines(audio))
}

// setMediaPluginVersion rewrites the installed stand-in plugin's manifest
// version, as an upgrade would.
func setMediaPluginVersion(t *testing.T, dir, name, version string) {
	t.Helper()
	path := filepath.Join(dir, name, "manifest.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	m["version"] = version
	out, err := json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// The parse cache keys a media file by its engines' identities and nothing
// else by them: an engine installed after a read without it, or upgraded,
// changes the key of the formats that read through it, and of no other
// format. A text file's key never names an engine.
func TestFormatProducer_KeysMediaByTheirEnginesAlone(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("KAPI_ASR_PLUGIN", "")
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", scratch)
	RefreshMediaEngines()
	t.Cleanup(RefreshMediaEngines)
	a := newToolboxApp(t)

	key := func(format string) string {
		t.Helper()
		RefreshMediaEngines()
		id, ok := a.formatProducer(format)
		require.True(t, ok, format)
		return id
	}

	text0, audio0, image0, video0 := key("json"), key("audio"), key("image"), key("video")
	assert.Equal(t, "asr=absent", audio0)
	assert.Equal(t, "vision=absent", image0)

	// Installed after a read without it: the formats it reads miss.
	fakeMediaPlugin(t, scratch, "asr")
	fakeMediaPlugin(t, scratch, "vision")
	audio1, image1, video1 := key("audio"), key("image"), key("video")
	assert.NotEqual(t, audio0, audio1)
	assert.NotEqual(t, image0, image1)
	assert.NotEqual(t, video0, video1)
	assert.Equal(t, text0, key("json"), "a text file's key names no engine")

	// One engine upgraded: only its formats miss.
	setMediaPluginVersion(t, scratch, "vision", "9.9.9")
	assert.NotEqual(t, image1, key("image"), "vision upgraded: an image misses")
	assert.NotEqual(t, video1, key("video"), "and a video, which reads through vision too")
	assert.Equal(t, audio1, key("audio"), "audio reads through asr alone and still hits")
	assert.Equal(t, text0, key("json"), "a text file still hits")
	assert.Contains(t, key("image"), "vision=vision@9.9.9#")
}
