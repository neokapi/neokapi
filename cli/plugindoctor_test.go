package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFakePluginBinary writes an executable shell script acting as a plugin
// binary that responds to `version` and `doctor`. Returns the script path.
func writeFakePluginBinary(t *testing.T, dir, name, versionOut, doctorOut string, doctorExit int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  version) echo \"" + versionOut + "\" ;;\n" +
		"  doctor) echo \"" + doctorOut + "\"; exit " + strconv.Itoa(doctorExit) + " ;;\n" +
		"  *) echo \"unknown $1\" >&2; exit 2 ;;\n" +
		"esac\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func fakePlugin(name, version, binaryPath string, selfCheck bool) *pluginhost.Plugin {
	return &pluginhost.Plugin{
		Dir:        filepath.Dir(binaryPath),
		BinaryPath: binaryPath,
		Manifest: &manifest.Manifest{
			ManifestVersion: "1",
			Plugin:          name,
			Version:         version,
			Binary:          filepath.Base(binaryPath),
			Capabilities:    manifest.Capabilities{SelfCheck: selfCheck},
		},
	}
}

func TestDiagnosePlugin_HealthyWithSelfCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake plugin binary is a shell script")
	}
	dir := t.TempDir()
	bin := writeFakePluginBinary(t, dir, "kapi-av", "1.0.0", "ffmpeg bundled", 0)
	res := diagnosePlugin(context.Background(), fakePlugin("av", "1.0.0", bin, true))

	assert.True(t, res.healthy)
	assert.Equal(t, "healthy", res.summary)
	assert.Contains(t, res.output, "ffmpeg bundled")
}

func TestDiagnosePlugin_SelfCheckFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake plugin binary is a shell script")
	}
	dir := t.TempDir()
	bin := writeFakePluginBinary(t, dir, "kapi-av", "1.0.0", "ffmpeg missing", 1)
	res := diagnosePlugin(context.Background(), fakePlugin("av", "1.0.0", bin, true))

	assert.False(t, res.healthy)
	assert.Equal(t, "self-check failed", res.summary)
	assert.Contains(t, res.output, "ffmpeg missing")
}

func TestDiagnosePlugin_VersionMismatchIsWarningNotFatal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake plugin binary is a shell script")
	}
	dir := t.TempDir()
	// Binary reports 9.9.9 but the manifest claims 1.0.0. A version mismatch is
	// surfaced as a warning — the passing self-check keeps the plugin healthy.
	bin := writeFakePluginBinary(t, dir, "kapi-av", "9.9.9", "ok", 0)
	res := diagnosePlugin(context.Background(), fakePlugin("av", "1.0.0", bin, true))

	assert.True(t, res.healthy)
	assert.Equal(t, "healthy", res.summary)
	found := false
	for _, c := range res.checks {
		if strings.Contains(c, "⚠") && strings.Contains(c, "version") {
			found = true
		}
	}
	assert.True(t, found, "expected a version warning check, got %v", res.checks)
}

func TestDiagnosePlugin_NoSelfCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake plugin binary is a shell script")
	}
	dir := t.TempDir()
	bin := writeFakePluginBinary(t, dir, "kapi-demo", "2.0.0", "unused", 0)
	res := diagnosePlugin(context.Background(), fakePlugin("demo", "2.0.0", bin, false))

	assert.True(t, res.healthy)
	assert.Equal(t, "ok (no self-check)", res.summary)
	assert.Empty(t, res.output, "no self-check should not run the doctor probe")
}

func TestDiagnosePlugin_BinaryMissing(t *testing.T) {
	res := diagnosePlugin(context.Background(), fakePlugin("gone", "1.0.0", "/no/such/kapi-gone", true))
	assert.False(t, res.healthy)
	assert.Contains(t, res.summary, "binary missing")
}

// TestPluginDoctorCmd_NoPlugins drives the command end-to-end with an isolated,
// empty plugins dir: no plugins installed → friendly message, no error.
func TestPluginDoctorCmd_NoPlugins(t *testing.T) {
	withIsolatedXDG(t)
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")

	app := &App{}
	var stdout, stderr bytes.Buffer
	cmd := app.NewPluginCmd()
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"doctor"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, stdout.String(), "No plugins installed")
}

// TestPluginDoctorCmd_UnknownPlugin asks for a plugin that is not installed.
func TestPluginDoctorCmd_UnknownPlugin(t *testing.T) {
	withIsolatedXDG(t)
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")

	app := &App{}
	var stdout, stderr bytes.Buffer
	cmd := app.NewPluginCmd()
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"doctor", "nope"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not installed")
}
