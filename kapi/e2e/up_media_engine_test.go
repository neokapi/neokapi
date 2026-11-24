//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A recording read before its speech recognizer is installed is reported as
// read without it, in the run summary and in kapi status, not as up to date.
// Once the recognizer is installed, the next run reads the recording again
// and drafts its speech.
func TestUpReadsARecordingAgainOnceItsRecognizerIsInstalled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in recognizer is a shell script")
	}
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "media"), 0o755))
	wav, err := os.ReadFile(filepath.Join(findRoot(), "core", "formats", "audio", "testdata", "tone.wav"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "media", "intro.wav"), wav, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(`version: v1
name: media
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - name: media
    path: media/*.wav
    target: media/{lang}/{path}.wav
`), 0o644))
	dataRoot := t.TempDir()
	kapi := func(recognizer string, args ...string) string {
		t.Helper()
		cmd := exec.Command(kapiBin, args...)
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), isoEnv...),
			"KAPI_DATA_DIR="+dataRoot, "KAPI_AI_PROVIDER=demo", "ANTHROPIC_API_KEY=", "OPENAI_API_KEY=",
			"KAPI_ASR_PLUGIN="+recognizer)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "kapi %v:\n%s", args, out)
		return string(out)
	}

	out := kapi("", "up", "-p", "kapi.yaml")
	assert.Contains(t, out, "1 file(s) read without the engine their content needs (asr): media/intro.wav", out)
	assert.Contains(t, out, "Not up to date: files wait for an engine to read them.", out)
	assert.NotContains(t, out, "Up to date.", out)
	status := kapi("", "status", "-p", "kapi.yaml")
	assert.Contains(t, status, "read without the engine their content needs (asr): media/intro.wav", status)

	recognizer := filepath.Join(testdata, "fake-asr", "kapi-asr")
	out = kapi(recognizer, "up", "-p", "kapi.yaml")
	assert.Contains(t, out, "  produced            1", "the recording is read again and its speech drafted:\n%s", out)
	assert.NotContains(t, out, "read without the engine", out)
	status = kapi(recognizer, "status", "-p", "kapi.yaml")
	assert.NotContains(t, status, "read without the engine", status)
}
