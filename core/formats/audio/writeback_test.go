package audio

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/asr"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAudioWritesBackWithAndWithoutAnEngine reads a file and writes it back,
// once with an ASR engine registered and once with none. The writer copies the
// Media part, so the reader must emit it on both paths; with only the
// transcription the writer has nothing to write.
func TestAudioWritesBackWithAndWithoutAnEngine(t *testing.T) {
	for _, withEngine := range []bool{true, false} {
		asr.ResetForTest()
		t.Cleanup(asr.ResetForTest)
		if withEngine {
			asr.RegisterEngine("fake", func() (asr.Engine, error) {
				return &fakeEngine{res: &asr.Result{Language: "en", Segments: []asr.Segment{
					{Text: "hello there", StartMS: 0, EndMS: 1200, Confidence: 0.9},
				}}}, nil
			})
		}
		path := writeTemp(t, "a.wav")
		parts := readAll(t, path)
		ch := make(chan *model.Part, len(parts))
		for _, p := range parts {
			ch <- p
		}
		close(ch)
		var out bytes.Buffer
		w := NewWriter()
		require.NoError(t, w.SetOutputWriter(&out))
		require.NoError(t, w.Write(context.Background(), ch), "engine registered: %v", withEngine)
		src, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, src, out.Bytes(), "engine registered: %v", withEngine)
	}
}
