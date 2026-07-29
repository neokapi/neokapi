package pluginhost

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithoutProviderKeys(t *testing.T) {
	env := []string{
		"ANTHROPIC_API_KEY=secret-anthropic",
		"OPENAI_API_KEY=secret-openai",
		"GEMINI_API_KEY=secret-gemini",
		"GOOGLE_API_KEY=secret-google",
		"AZURE_OPENAI_API_KEY=secret-azure",
		// Everything a local-compute plugin actually runs on.
		"PATH=/usr/bin",
		"HOME=/synthetic-home",
		"TMPDIR=/tmp",
		"XDG_CACHE_HOME=/synthetic-cache",
		"KAPI_PLUGIN_DIR=/plugins/x",
		"KAPI_VISION_MODELS_DIR=/models",
	}

	got := WithoutProviderKeys(env)

	for _, name := range []string{
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY",
		"GOOGLE_API_KEY", "AZURE_OPENAI_API_KEY",
	} {
		assert.NotContains(t, got, name+"=secret-"+name, name+" must not survive")
	}
	for _, kv := range []string{
		"PATH=/usr/bin", "HOME=/synthetic-home", "TMPDIR=/tmp",
		"XDG_CACHE_HOME=/synthetic-cache",
		"KAPI_PLUGIN_DIR=/plugins/x", "KAPI_VISION_MODELS_DIR=/models",
	} {
		assert.Contains(t, got, kv, "a plugin's own environment must survive")
	}
	assert.Len(t, got, 6)
}

// TestWithoutProviderKeys_MatchesWholeNames: the denylist is exact. A variable
// that merely starts with a provider key's name is a different variable, and
// removing it would break a plugin for no reason.
func TestWithoutProviderKeys_MatchesWholeNames(t *testing.T) {
	got := WithoutProviderKeys([]string{
		"ANTHROPIC_API_KEY=drop",
		"ANTHROPIC_API_KEY_BACKUP=keep",
		"MY_ANTHROPIC_API_KEY=keep",
		// A value that happens to name a key is still just a value.
		"NOTES=set ANTHROPIC_API_KEY= before running",
	})
	assert.ElementsMatch(t, []string{
		"ANTHROPIC_API_KEY_BACKUP=keep",
		"MY_ANTHROPIC_API_KEY=keep",
		"NOTES=set ANTHROPIC_API_KEY= before running",
	}, got)
}

// TestWithoutProviderKeys_KeepsMalformedEntries: an entry with no "=" is not an
// assignment. Passing it through unchanged is the honest thing; guessing is not.
func TestWithoutProviderKeys_KeepsMalformedEntries(t *testing.T) {
	got := WithoutProviderKeys([]string{"NOT_AN_ASSIGNMENT", "PATH=/usr/bin"})
	assert.ElementsMatch(t, []string{"NOT_AN_ASSIGNMENT", "PATH=/usr/bin"}, got)
}

// TestWithoutProviderKeys_EmptyValueStillRemoved: an exported-but-empty key is
// still the variable, and leaving it would let a plugin distinguish "unset"
// from "set to empty" — a small signal, and pointless to leak.
func TestWithoutProviderKeys_EmptyValueStillRemoved(t *testing.T) {
	assert.Empty(t, WithoutProviderKeys([]string{"OPENAI_API_KEY="}))
}

// makeEnvDumpPlugin writes a shell-script plugin that dumps its environment to
// the given path, so a test can assert on what a real subprocess received
// rather than on what the helper returns.
func makeEnvDumpPlugin(t *testing.T, dumpPath string) *Plugin {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script plugin stub is not portable to Windows")
	}

	dir := t.TempDir()
	binPath := filepath.Join(dir, "dumper")
	require.NoError(t, os.WriteFile(binPath,
		[]byte("#!/bin/sh\nenv > "+dumpPath+"\n"), 0o755))

	m := &manifest.Manifest{
		ManifestVersion: manifest.CurrentVersion,
		Plugin:          "dumper",
		Version:         "0.0.1",
		Binary:          "dumper",
		Capabilities: manifest.Capabilities{
			Commands: []manifest.Command{{Name: "dump"}},
		},
	}
	require.NoError(t, m.Validate())

	return &Plugin{
		Dir:        dir,
		BinaryPath: binPath,
		Manifest:   m,
		Source:     Source{Order: 1, Label: "test", Path: dir},
	}
}

// TestExecPluginDropsProviderKeys is the end-to-end form: a real subprocess,
// spawned the way kapi spawns plugins, must not be able to read the user's
// model credentials out of its own environment.
func TestExecPluginDropsProviderKeys(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "secret-anthropic")
	t.Setenv("OPENAI_API_KEY", "secret-openai")
	t.Setenv("KAPI_VISION_MODELS_DIR", "/models")

	dump := filepath.Join(t.TempDir(), "env.txt")
	p := makeEnvDumpPlugin(t, dump)

	_, err := execPlugin(context.Background(), p, []string{"dump"}, true)
	require.NoError(t, err)

	got, err := os.ReadFile(dump)
	require.NoError(t, err)
	env := string(got)

	assert.NotContains(t, env, "secret-anthropic")
	assert.NotContains(t, env, "secret-openai")
	// The host's own plugin context still arrives.
	assert.Contains(t, env, "KAPI_PLUGIN_NAME=dumper")
	assert.Contains(t, env, "KAPI_VISION_MODELS_DIR=/models")
}
