package whisper

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/plugins/asr/asrproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A trimmed real whisper-cli -oj payload (the shape captured from whisper.cpp).
const fixtureJSON = `{
  "result": { "language": "en" },
  "transcription": [
    { "offsets": { "from": 0, "to": 1200 }, "text": " The quick brown fox" },
    { "offsets": { "from": 1200, "to": 2600 }, "text": "jumps over the lazy dog." },
    { "offsets": { "from": 2600, "to": 2700 }, "text": "  " }
  ]
}`

func TestParseWhisperJSON(t *testing.T) {
	lang, segs, err := parseWhisperJSON([]byte(fixtureJSON))
	require.NoError(t, err)
	assert.Equal(t, "en", lang)
	require.Len(t, segs, 2) // the whitespace-only segment is dropped
	assert.Equal(t, "The quick brown fox", segs[0].Text)
	assert.Equal(t, int64(0), segs[0].StartMS)
	assert.Equal(t, int64(1200), segs[0].EndMS)
	assert.Equal(t, "jumps over the lazy dog.", segs[1].Text)
	assert.Equal(t, int64(2600), segs[1].EndMS)
}

func TestParseWhisperJSON_Malformed(t *testing.T) {
	_, _, err := parseWhisperJSON([]byte("not json"))
	require.Error(t, err)
}

// TestEngineE2E runs the real whisper-cli over a `say`-synthesized clip with
// known text. Opt-in (KAPI_ASR_E2E=1) and gated on the toolchain: it needs
// whisper-cli + ffmpeg + say on PATH and a model at KAPI_ASR_TEST_MODEL.
func TestEngineE2E(t *testing.T) {
	if os.Getenv("KAPI_ASR_E2E") != "1" {
		t.Skip("set KAPI_ASR_E2E=1 to run the real whisper-cli e2e")
	}
	model := os.Getenv("KAPI_ASR_TEST_MODEL")
	if model == "" {
		t.Skip("set KAPI_ASR_TEST_MODEL to a ggml model path")
	}
	for _, bin := range []string{"whisper-cli", "ffmpeg", "say"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
	dir := t.TempDir()
	aiff := filepath.Join(dir, "v.aiff")
	wav := filepath.Join(dir, "v.wav")
	ctx := context.Background()
	must(t, exec.CommandContext(ctx, "say", "-o", aiff, "The quick brown fox jumps over the lazy dog.").Run())
	must(t, exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-y", "-i", aiff, "-ac", "1", "-ar", "16000", wav).Run())

	eng := &Engine{Model: model}
	lang, segs, err := eng.Transcribe(context.Background(), wav, "")
	require.NoError(t, err)
	assert.Equal(t, "en", lang)
	require.NotEmpty(t, segs)
	joined := strings.ToLower(strings.Join(segTexts(segs), " "))
	assert.Contains(t, joined, "quick brown fox")
	assert.Contains(t, joined, "lazy dog")
}

func segTexts(segs []asrproto.Segment) []string {
	out := make([]string, len(segs))
	for i, s := range segs {
		out[i] = s.Text
	}
	return out
}

func must(t *testing.T, err error) { t.Helper(); require.NoError(t, err) }
